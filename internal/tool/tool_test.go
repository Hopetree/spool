package tool

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Hopetree/spool/internal/store"
)

func newTestToolService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	return NewService(st.DB), st
}

func baseToolInput() Input {
	return Input{
		Name:        "deploy",
		Description: "deploy app",
		Type:        "shell",
		Interpreter: "/bin/bash",
		Content:     "echo hello",
		Params: []Param{
			{Name: "ENV", Label: "环境", Type: "select", Required: true, Options: []string{"prod", "staging"}},
			{Name: "COUNT", Type: "int", Default: "1"},
		},
		TimeoutSec: 60,
	}
}

func TestCreateAndVersioning(t *testing.T) {
	svc, _ := newTestToolService(t)
	ctx := context.Background()
	in := baseToolInput()
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.VersionNo != 1 || created.ParamCount != 2 || created.Enabled != true {
		t.Fatalf("created = %+v", created)
	}
	if created.Version.ContentHash != HashContent("echo hello") {
		t.Fatal("content hash mismatch")
	}

	in.Content = "echo v2"
	in.ChangeNote = "second"
	updated, err := svc.Update(ctx, created.ID, in, "admin")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.VersionNo != 2 || updated.Version.Content != "echo v2" {
		t.Fatalf("updated = %+v", updated)
	}
	versions, err := svc.Versions(ctx, created.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions = %v err=%v", versions, err)
	}
	if versions[0].VersionNo != 2 || versions[1].Content != "echo hello" {
		t.Fatalf("versions order/content wrong: %+v", versions)
	}
}

func TestRollbackCreatesNewVersion(t *testing.T) {
	svc, _ := newTestToolService(t)
	ctx := context.Background()
	in := baseToolInput()
	created, _ := svc.Create(ctx, in, "admin")
	in.Content = "echo v2"
	_, err := svc.Update(ctx, created.ID, in, "admin")
	if err != nil {
		t.Fatal(err)
	}
	rolled, err := svc.Rollback(ctx, created.ID, 1, "", "admin")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rolled.VersionNo != 3 || rolled.Version.Content != "echo hello" {
		t.Fatalf("rolled = %+v", rolled)
	}
	if rolled.Version.ChangeNote != "回滚到 v1" {
		t.Fatalf("note = %q", rolled.Version.ChangeNote)
	}
}

func TestCopy(t *testing.T) {
	svc, _ := newTestToolService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, baseToolInput(), "admin")
	copied, err := svc.Copy(ctx, created.ID, "admin")
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if copied.Name != "deploy-copy" || copied.VersionNo != 1 || copied.Version.Content != "echo hello" {
		t.Fatalf("copied = %+v", copied)
	}
}

func TestValidateValues(t *testing.T) {
	params := []Param{
		{Name: "ENV", Type: "select", Required: true, Options: []string{"prod", "staging"}},
		{Name: "COUNT", Type: "int", Default: "1"},
		{Name: "DRY", Type: "bool", Default: "false"},
		{Name: "TOKEN", Type: "string", Sensitive: true},
	}
	got, err := ValidateValues(params, map[string]string{"ENV": "prod"})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got["COUNT"] != "1" || got["DRY"] != "false" || got["ENV"] != "prod" {
		t.Fatalf("values = %v", got)
	}
	if _, err := ValidateValues(params, map[string]string{}); err == nil {
		t.Fatal("required param should fail")
	}
	if _, err := ValidateValues(params, map[string]string{"ENV": "dev"}); err == nil {
		t.Fatal("invalid select should fail")
	}
	if _, err := ValidateValues(params, map[string]string{"ENV": "prod", "COUNT": "x"}); err == nil {
		t.Fatal("invalid int should fail")
	}
	if _, err := ValidateValues(params, map[string]string{"ENV": "prod", "EXTRA": "x"}); err == nil {
		t.Fatal("unknown param should fail")
	}
}

func TestValidateParams(t *testing.T) {
	cases := [][]Param{
		{{Name: "1BAD", Type: "string"}},
		{{Name: "SPOOL_X", Type: "string"}},
		{{Name: "A", Type: "string"}, {Name: "A", Type: "string"}},
		{{Name: "A", Type: "select"}},
		{{Name: "A", Type: "string"}, {Name: "B", Type: "int", Default: "x"}},
		{{Name: "A", Type: "bool", Default: "yes"}},
	}
	for i, params := range cases {
		if err := ValidateParams(params); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestDeleteProtection(t *testing.T) {
	svc, st := newTestToolService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, baseToolInput(), "admin")
	now := int64(1700000000000)
	_, err := st.DB.Exec(`INSERT INTO executions (id, kind, status, created_at) VALUES ('e1', 'tool', 'success', ?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.DB.Exec(`INSERT INTO host_runs (id, execution_id, status) VALUES ('hr1', 'e1', 'success')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.DB.Exec(`INSERT INTO step_runs (id, host_run_id, tool_id, tool_version_id, status) VALUES ('sr1', 'hr1', ?, ?, 'success')`,
		created.ID, created.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, ErrHasHistory) {
		t.Fatalf("err = %v, want ErrHasHistory", err)
	}
	other, _ := svc.Create(ctx, Input{Name: "other", Type: "shell", Content: "true"}, "admin")
	if err := svc.Delete(ctx, other.ID); err != nil {
		t.Fatalf("delete unused: %v", err)
	}
}

func TestSetEnabledAndList(t *testing.T) {
	svc, _ := newTestToolService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, baseToolInput(), "admin")
	if err := svc.SetEnabled(ctx, created.ID, false); err != nil {
		t.Fatal(err)
	}
	enabled := true
	items, total, err := svc.List(ctx, ListFilter{Enabled: &enabled, Page: 1, PageSize: 10})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("list enabled: %v total=%d", err, total)
	}
	items, total, _ = svc.List(ctx, ListFilter{Keyword: "deploy", Page: 1, PageSize: 10})
	if total != 1 || len(items) != 1 {
		t.Fatalf("keyword list: total=%d", total)
	}
}

func TestListByType(t *testing.T) {
	svc, _ := newTestToolService(t)
	ctx := context.Background()
	shellTool, err := svc.Create(ctx, baseToolInput(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	pyInput := baseToolInput()
	pyInput.Name = "collect"
	pyInput.Type = "python"
	pyInput.Interpreter = "python3"
	pyInput.Content = "print('ok')"
	pyTool, err := svc.Create(ctx, pyInput, "admin")
	if err != nil {
		t.Fatal(err)
	}

	items, total, err := svc.List(ctx, ListFilter{Type: "python", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != pyTool.ID {
		t.Fatalf("python list: total=%d items=%+v err=%v", total, items, err)
	}
	items, total, err = svc.List(ctx, ListFilter{Type: "shell", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != shellTool.ID {
		t.Fatalf("shell list: total=%d items=%+v err=%v", total, items, err)
	}
}

func TestDuplicateName(t *testing.T) {
	svc, _ := newTestToolService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, baseToolInput(), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, baseToolInput(), "admin"); err == nil {
		t.Fatal("duplicate name should fail")
	}
}
