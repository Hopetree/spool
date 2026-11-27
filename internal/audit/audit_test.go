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
