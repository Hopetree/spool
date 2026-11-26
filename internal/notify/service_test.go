package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/store"
)

func newNotifyFixture(t *testing.T, baseURL string) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cipher, err := credential.NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	creds := credential.NewService(st.DB, cipher)
	return NewService(st.DB, creds, baseURL), st
}

func TestChannelCRUDAndSecret(t *testing.T) {
	svc, _ := newNotifyFixture(t, "")
	ctx := context.Background()
	ch, err := svc.Create(ctx, Input{
		Name: "ops", Type: "dingtalk", URL: "https://example.com/hook", Secret: "sign-token", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.HasSecret || ch.URL != "https://example.com/hook" {
		t.Fatalf("channel = %+v", ch)
	}
	raw, _ := json.Marshal(ch)
	if strings.Contains(string(raw), "sign-token") {
		t.Fatal("secret leaked in JSON")
	}
	updated, err := svc.Update(ctx, ch.ID, Input{Name: "ops2", Type: "dingtalk", URL: ch.URL, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.HasSecret || updated.Enabled {
		t.Fatalf("updated = %+v", updated)
	}
	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v err=%v", list, err)
	}
	if err := svc.Delete(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, ch.ID); err != ErrNotFound {
		t.Fatalf("get deleted = %v", err)
	}
}

func TestWebhookSend(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	svc, _ := newNotifyFixture(t, "")
	ch, err := svc.Create(context.Background(), Input{Name: "wh", Type: "webhook", URL: srv.URL, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	reqSnip, respSnip, err := svc.Send(context.Background(), *ch, "hello ops")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(body, "hello ops") || !strings.Contains(respSnip, "ok") {
		t.Fatalf("body=%s resp=%s", body, respSnip)
	}
	if !strings.Contains(reqSnip, "POST") {
		t.Fatalf("req snippet = %s", reqSnip)
	}
}

func TestDingTalkSignAndFeishuPayload(t *testing.T) {
	var dingQuery string
	ding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dingQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ding.Close()
	svc, _ := newNotifyFixture(t, "")
	ch, _ := svc.Create(context.Background(), Input{Name: "dd", Type: "dingtalk", URL: ding.URL, Secret: "s3", Enabled: true})
	if _, _, err := svc.Send(context.Background(), *ch, "hi"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dingQuery, "timestamp=") || !strings.Contains(dingQuery, "sign=") {
		t.Fatalf("query = %s", dingQuery)
	}

	var feishuBody map[string]any
	feishu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&feishuBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer feishu.Close()
	fch, _ := svc.Create(context.Background(), Input{Name: "fs", Type: "feishu", URL: feishu.URL, Secret: "s3", Enabled: true})
	if _, _, err := svc.Send(context.Background(), *fch, "hi"); err != nil {
		t.Fatal(err)
	}
	if feishuBody["msg_type"] != "text" || feishuBody["sign"] == nil || feishuBody["timestamp"] == nil {
		t.Fatalf("feishu body = %v", feishuBody)
	}
}

func TestNotifyExecutionOnlyOnFailure(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	svc, st := newNotifyFixture(t, "http://spool.test")
	ch, _ := svc.Create(context.Background(), Input{Name: "wh", Type: "webhook", URL: srv.URL, Enabled: true})
	now := time.Now().UnixMilli()
	_, err := st.DB.Exec(`INSERT INTO schedules (id, name, target_kind, target_ref_id, cron_expr, notify_channel_ids_json, notify_events, created_at, updated_at)
		VALUES ('s1', 'job', 'tool', 't1', '* * * * *', ?, 'failure', ?, ?)`, `["`+ch.ID+`"]`, now, now)
	if err != nil {
		t.Fatal(err)
	}

	svc.NotifyExecution(context.Background(), ExecutionNotice{ID: "e1", Name: "job", Status: "success", ScheduleID: "s1"})
	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatal("success should not notify")
	}

	svc.NotifyExecution(context.Background(), ExecutionNotice{
		ID: "e2", Name: "job", Status: "failed", ScheduleID: "s1", HostTotal: 1, HostFailed: 1, ErrorSummary: "1 台失败",
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&hits) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("failure notification was not sent")
	}
	var deliveries int
	_ = st.DB.Get(&deliveries, `SELECT count(*) FROM notify_deliveries WHERE execution_id = 'e2' AND status = 'success'`)
	if deliveries != 1 {
		t.Fatalf("deliveries = %d", deliveries)
	}
}

func TestTestChannel(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	svc, _ := newNotifyFixture(t, "")
	ch, _ := svc.Create(context.Background(), Input{Name: "wh", Type: "webhook", URL: srv.URL, Enabled: true})
	if _, err := svc.Test(context.Background(), ch.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "测试通知") {
		t.Fatalf("body = %s", body)
	}
}
