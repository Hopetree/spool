package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotifyChannelApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := e.do(t, http.MethodPost, "/api/v1/notify-channels", map[string]any{
		"name": "ops-webhook", "type": "webhook", "url": srv.URL, "enabled": true,
	}, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create channel status = %d, body = %s", w.Code, w.Body.String())
	}
	var ch map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &ch)
	id := ch["id"].(string)
	if ch["has_secret"] != false || ch["url"] != srv.URL {
		t.Fatalf("channel = %+v", ch)
	}

	if w := e.do(t, http.MethodGet, "/api/v1/notify-channels", nil, admin); w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	operator := ensureOperator(t, e)
	if w := e.do(t, http.MethodGet, "/api/v1/notify-channels", nil, operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator list status = %d", w.Code)
	}
	if w := e.do(t, http.MethodPost, "/api/v1/notify-channels/"+id+"/test", nil, admin); w.Code != http.StatusOK {
		t.Fatalf("test status = %d, body = %s", w.Code, w.Body.String())
	}
	if body == "" || !strings.Contains(body, "测试通知") {
		t.Fatalf("webhook body = %q", body)
	}
	if w := e.do(t, http.MethodDelete, "/api/v1/notify-channels/"+id, nil, admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w.Code)
	}
	if e.auditCount(t, "notify.create") != 1 || e.auditCount(t, "notify.test") != 1 || e.auditCount(t, "notify.delete") != 1 {
		t.Fatal("notify audit entries missing")
	}
}

func createTestSchedule(t *testing.T, e *testEnv, admin *http.Cookie, toolID, hostID string) map[string]any {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "nightly", "target_kind": "tool", "target_ref_id": toolID,
		"params": map[string]string{"TOKEN": "s3cr3t"}, "host_ids": []string{hostID},
		"cron_expr": "0 0 1 1 *", "concurrency_policy": "skip",
		"notify_events": "failure", "enabled": true,
	}, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create schedule status = %d, body = %s", w.Code, w.Body.String())
	}
	var sch map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sch)
	return sch
}

func TestScheduleApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	toolBody := createTestTool(t, e, admin, []map[string]any{
		{"name": "ENV", "type": "string", "default": "prod"},
		{"name": "TOKEN", "type": "string", "sensitive": true},
	})
	sch := createTestSchedule(t, e, admin, toolBody["id"].(string), host["id"].(string))
	id := sch["id"].(string)
	if sch["target_name"] != "demo" || sch["next_run_at"] == nil {
		t.Fatalf("schedule = %+v", sch)
	}
	params := sch["params"].(map[string]any)
	if _, ok := params["TOKEN"]; ok {
		t.Fatalf("sensitive param leaked: %v", params)
	}

	w := e.do(t, http.MethodGet, "/api/v1/schedules", nil, admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "nightly") {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}
	w = e.do(t, http.MethodGet, "/api/v1/schedules/"+id+"/next-runs?count=5", nil, admin)
	var next struct {
		Items []int64 `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &next)
	if w.Code != http.StatusOK || len(next.Items) != 5 {
		t.Fatalf("next runs status=%d items=%v", w.Code, next.Items)
	}
	w = e.do(t, http.MethodPost, "/api/v1/schedules/preview", map[string]any{"cron_expr": "*/5 * * * *", "count": 3}, admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "items") {
		t.Fatalf("preview status=%d body=%s", w.Code, w.Body.String())
	}

	w = e.do(t, http.MethodPut, "/api/v1/schedules/"+id, map[string]any{
		"name": "nightly-v2", "target_kind": "tool", "target_ref_id": toolBody["id"],
		"params": map[string]string{"TOKEN": ""}, "host_ids": []string{host["id"].(string)},
		"cron_expr": "0 2 * * *", "concurrency_policy": "queue",
		"notify_events": "always", "enabled": true,
	}, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	if w := e.do(t, http.MethodPost, "/api/v1/schedules/"+id+"/enabled", map[string]any{"enabled": false}, admin); w.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d", w.Code)
	}
	var enabled bool
	var nextRun *int64
	_ = e.store.DB.Get(&enabled, `SELECT enabled FROM schedules WHERE id = ?`, id)
	_ = e.store.DB.Get(&nextRun, `SELECT next_run_at FROM schedules WHERE id = ?`, id)
	if enabled || nextRun != nil {
		t.Fatalf("enabled=%v next=%v", enabled, nextRun)
	}

	operator := ensureOperator(t, e)
	if w := e.do(t, http.MethodPost, "/api/v1/schedules", map[string]any{"name": "x"}, operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator create status = %d", w.Code)
	}
	if w := e.do(t, http.MethodDelete, "/api/v1/schedules/"+id, nil, admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w.Code)
	}
	if e.auditCount(t, "schedule.create") != 1 || e.auditCount(t, "schedule.update") != 1 || e.auditCount(t, "schedule.delete") != 1 {
		t.Fatal("schedule audit entries missing")
	}
}

func TestScheduleManualRun(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	toolBody := createTestTool(t, e, admin, []map[string]any{{"name": "TOKEN", "type": "string", "sensitive": true}})
	sch := createTestSchedule(t, e, admin, toolBody["id"].(string), host["id"].(string))
	operator := ensureOperator(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/schedules/"+sch["id"].(string)+"/run", nil, operator)
	if w.Code != http.StatusCreated {
		t.Fatalf("run status = %d, body = %s", w.Code, w.Body.String())
	}
	var ex map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &ex)
	detail := waitExecution(t, e, operator, ex["id"].(string))
	if detail["status"] != "success" || detail["source"] != "manual" || detail["schedule_id"] != sch["id"] {
		t.Fatalf("detail = %+v", detail)
	}
	if e.auditCount(t, "schedule.trigger") != 1 {
		t.Fatal("schedule.trigger audit missing")
	}
}

func TestScheduleValidationApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	toolBody := createTestTool(t, e, admin, nil)
	w := e.do(t, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "bad", "target_kind": "tool", "target_ref_id": toolBody["id"],
		"cron_expr": "not a cron", "host_ids": []string{host["id"].(string)}, "enabled": true,
	}, admin)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
