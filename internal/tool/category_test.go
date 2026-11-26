package tool

import (
	"context"
	"errors"
	"testing"
)

func TestCategoryValidationListAndCounts(t *testing.T) {
	svc, _ := newTestToolService(t)
	ctx := context.Background()

	in1 := baseToolInput()
	in1.Name = "deploy"
	in1.Category = " 运维 / 部署 / 应用 "
	created, err := svc.Create(ctx, in1, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Category != "运维/部署/应用" {
		t.Fatalf("category = %q", created.Category)
	}
	in2 := baseToolInput()
	in2.Name = "monitor"
	in2.Category = "运维/监控"
	if _, err := svc.Create(ctx, in2, "admin"); err != nil {
		t.Fatal(err)
	}
	in3 := baseToolInput()
	in3.Name = "misc"
	if _, err := svc.Create(ctx, in3, "admin"); err != nil {
		t.Fatal(err)
	}

	cats, err := svc.Categories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, c := range cats {
		counts[c.Path] = c.Count
	}
	if counts[""] != 1 || counts["运维/部署/应用"] != 1 || counts["运维/监控"] != 1 {
		t.Fatalf("counts = %v", counts)
	}

	items, total, err := svc.List(ctx, ListFilter{Category: "运维", Page: 1, PageSize: 10})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("prefix list: total=%d err=%v", total, err)
	}
	items, total, err = svc.List(ctx, ListFilter{Uncategorized: true, Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].Name != "misc" {
		t.Fatalf("uncategorized list: total=%d items=%v err=%v", total, items, err)
	}

	bad := []string{"a//b", "/a", "a/", "a/b/c/d/e/f", ".", "..", "a/../b"}
	for _, category := range bad {
		in := baseToolInput()
		in.Name = "bad-" + category
		in.Category = category
		if _, err := svc.Create(ctx, in, "admin"); err == nil {
			t.Fatalf("category %q should be rejected", category)
		}
	}
}

func TestDeleteVersionGuards(t *testing.T) {
	svc, st := newTestToolService(t)
	ctx := context.Background()
	in := baseToolInput()
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatal(err)
	}
	in.Content = "echo v2"
	if _, err := svc.Update(ctx, created.ID, in, "admin"); err != nil {
		t.Fatal(err)
	}
	in.Content = "echo v3"
	updated, err := svc.Update(ctx, created.ID, in, "admin")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteVersion(ctx, created.ID, 3); !errors.Is(err, ErrCurrentVersion) {
		t.Fatalf("delete current err = %v", err)
	}
	if err := svc.DeleteVersion(ctx, created.ID, 1); err != nil {
		t.Fatalf("delete v1: %v", err)
	}
	versions, _ := svc.Versions(ctx, created.ID)
	if len(versions) != 2 {
		t.Fatalf("versions = %d", len(versions))
	}

	v2, _ := svc.Version(ctx, created.ID, 2)
	now := int64(1700000000000)
	_, err = st.DB.Exec(`INSERT INTO executions (id, kind, status, created_at) VALUES ('e1', 'tool', 'success', ?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO host_runs (id, execution_id, status) VALUES ('hr1', 'e1', 'success')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO step_runs (id, host_run_id, tool_id, tool_version_id, status) VALUES ('sr1', 'hr1', ?, ?, 'success')`,
		updated.ID, v2.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteVersion(ctx, created.ID, 2); !errors.Is(err, ErrVersionHasHistory) {
		t.Fatalf("delete referenced err = %v", err)
	}
}

func TestReferences(t *testing.T) {
	svc, st := newTestToolService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, baseToolInput(), "admin")
	now := int64(1700000000000)
	_, err := st.DB.Exec(`INSERT INTO flows (id, name, params_json, created_at, updated_at) VALUES ('f1', 'deploy-flow', '[]', ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.DB.Exec(`INSERT INTO flow_nodes (id, flow_id, node_key, type, name, tool_id, x, y)
		VALUES ('node1', 'f1', 'n1', 'tool', '部署应用', ?, 0, 0)`, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.DB.Exec(`INSERT INTO schedules (id, name, target_kind, target_ref_id, cron_expr, created_at, updated_at)
		VALUES ('s1', 'nightly', 'tool', ?, '0 2 * * *', ?, ?)`, created.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}

	refs, err := svc.References(ctx, created.ID)
	if err != nil {
		t.Fatalf("references: %v", err)
	}
	if len(refs.Flows) != 1 || refs.Flows[0].FlowName != "deploy-flow" ||
		refs.Flows[0].StageName != "流程图" || refs.Flows[0].StepName != "部署应用" {
		t.Fatalf("flows = %+v", refs.Flows)
	}
	if len(refs.Schedules) != 1 || refs.Schedules[0].Name != "nightly" {
		t.Fatalf("schedules = %+v", refs.Schedules)
	}
}
