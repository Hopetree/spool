package audit

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Hopetree/spool/internal/store"
)

func newTestAudit(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(st.DB), st
}

func TestWriteAndList(t *testing.T) {
	svc, _ := newTestAudit(t)
	ctx := context.Background()
	entries := []Entry{
		{ActorID: "u1", ActorName: "alice", Action: "host.create", ResourceType: "host", ResourceID: "h1", ResourceName: "web-1", Summary: map[string]any{"address": "10.0.0.1"}, IP: "127.0.0.1"},
		{ActorID: "u1", ActorName: "alice", Action: "tool.update", ResourceType: "tool", ResourceID: "t1", ResourceName: "deploy"},
	}
	for _, e := range entries {
		if err := svc.Write(ctx, e); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	recs, total, err := svc.List(ctx, Filter{Action: "host.create", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(recs) != 1 {
		t.Fatalf("total=%d len=%d", total, len(recs))
	}
	if recs[0].ResourceName != "web-1" || recs[0].UsernameSnapshot != "alice" {
		t.Fatalf("unexpected record: %+v", recs[0])
	}
	var summary map[string]string
	if err := json.Unmarshal([]byte(recs[0].SummaryJSON), &summary); err != nil {
		t.Fatalf("summary json: %v", err)
	}
	if summary["address"] != "10.0.0.1" {
		t.Fatalf("summary = %v", summary)
	}
}

func TestWriteNilSummary(t *testing.T) {
	svc, _ := newTestAudit(t)
	if err := svc.Write(context.Background(), Entry{Action: "login"}); err != nil {
		t.Fatal(err)
	}
	recs, _, err := svc.List(context.Background(), Filter{Action: "login", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].SummaryJSON != "{}" {
		t.Fatalf("summary = %q", recs[0].SummaryJSON)
	}
}

func TestListPagination(t *testing.T) {
	svc, _ := newTestAudit(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := svc.Write(ctx, Entry{Action: "host.create"}); err != nil {
			t.Fatal(err)
		}
	}
	recs, total, err := svc.List(ctx, Filter{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(recs) != 1 {
		t.Fatalf("total=%d len=%d", total, len(recs))
	}
}

func TestListFillsMissingHostResourceName(t *testing.T) {
	svc, st := newTestAudit(t)
	ctx := context.Background()
	if _, err := st.DB.Exec(`INSERT INTO credentials (id, name, auth_type, username, secret_enc, created_at, updated_at)
		VALUES ('c1', 'cred', 'password', 'root', ?, 0, 0)`, []byte{0}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO hosts (id, name, address, port, credential_id, created_at, updated_at)
		VALUES ('h1', 'web-1', '10.0.0.1', 22, 'c1', 0, 0)`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Write(ctx, Entry{Action: "host.test", ResourceType: "host", ResourceID: "h1"}); err != nil {
		t.Fatal(err)
	}
	recs, _, err := svc.List(ctx, Filter{Action: "host.test", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ResourceName != "web-1" {
		t.Fatalf("records = %+v", recs)
	}
}

func TestListEnrichesSummaryDisplay(t *testing.T) {
	svc, st := newTestAudit(t)
	ctx := context.Background()
	if _, err := st.DB.Exec(`INSERT INTO credentials (id, name, auth_type, username, secret_enc, created_at, updated_at)
		VALUES ('c1', 'cred', 'password', 'root', ?, 0, 0)`, []byte{0}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO hosts (id, name, address, port, credential_id, created_at, updated_at)
		VALUES ('h1', 'web-1', '10.0.0.1', 22, 'c1', 0, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO executions (id, kind, name_snapshot, status, created_at)
		VALUES ('e1', 'flow', '夜间巡检', 'success', 0)`); err != nil {
		t.Fatal(err)
	}
	entries := []Entry{
		{Action: "transfer.download", ResourceType: "transfer", Summary: map[string]any{
			"host_id": "h1", "host_run": "01M8B4XQV4BRXAS9J6PNHYR85H", "path": "/tmp/a.txt",
		}},
		{Action: "schedule.run", ResourceType: "schedule", Summary: map[string]any{
			"execution_id": "e1", "elapsed_ms": 12,
		}},
		{Action: "flow.copy", ResourceType: "flow", Summary: map[string]any{"source": "missing"}},
	}
	for _, e := range entries {
		if err := svc.Write(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	recs, _, err := svc.List(ctx, Filter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	byAction := map[string]map[string]any{}
	for _, rec := range recs {
		var summary map[string]any
		if err := json.Unmarshal([]byte(rec.SummaryDisplay), &summary); err != nil {
			t.Fatalf("summary display: %v", err)
		}
		byAction[rec.Action] = summary
	}
	download := byAction["transfer.download"]
	if download["host_id"] != "web-1" || download["path"] != "/tmp/a.txt" {
		t.Fatalf("download summary = %+v", download)
	}
	if _, ok := download["host_run"]; ok {
		t.Fatalf("host_run should be hidden: %+v", download)
	}
	if byAction["schedule.run"]["execution_id"] != "夜间巡检" {
		t.Fatalf("schedule summary = %+v", byAction["schedule.run"])
	}
	if _, ok := byAction["flow.copy"]["source"]; ok {
		t.Fatalf("unresolved source should be hidden: %+v", byAction["flow.copy"])
	}
}
