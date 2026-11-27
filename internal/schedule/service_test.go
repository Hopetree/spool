package schedule

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/flow"
	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/tool"
)

type fixture struct {
	svc    *Service
	tools  *tool.Service
	flows  *flow.Service
	store  *store.Store
	hostID string
	toolID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cipher, _ := credential.NewCipher(make([]byte, 32))
	creds := credential.NewService(st.DB, cipher)
	tools := tool.NewService(st.DB)
	flows := flow.NewService(st.DB, tools)
	svc := NewService(st.DB, tools, flows, creds, nil, nil, time.UTC)

	cred, err := creds.Create(context.Background(), credential.Input{
		Name: "c", AuthType: "password", Username: "root", Secret: "pw",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	hostID := store.NewULID()
	_, err = st.DB.Exec(`INSERT INTO hosts (id, name, address, port, credential_id, enabled, created_at, updated_at)
		VALUES (?, 'h1', '127.0.0.1', 22, ?, 1, 1, 1)`, hostID, cred.ID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := tools.Create(context.Background(), tool.Input{
		Name: "t1", Type: "shell", Content: "echo hi", TimeoutSec: 30,
		Params: []tool.Param{
			{Name: "ENV", Type: "string", Default: "prod"},
			{Name: "TOKEN", Type: "string", Sensitive: true},
		},
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, tools: tools, flows: flows, store: st, hostID: hostID, toolID: detail.ID}
}

func baseInput(f *fixture) Input {
	return Input{
		Name: "nightly", TargetKind: "tool", TargetRefID: f.toolID,
		Params: map[string]string{"TOKEN": "s3cr3t"}, HostIDs: []string{f.hostID},
		CronExpr: "0 0 1 1 *", ConcurrencyPolicy: "skip", NotifyEvents: "failure", Enabled: true,
	}
}

func TestCreateGetUpdateAndSensitive(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	created, err := f.svc.Create(ctx, baseInput(f), "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.TargetName != "t1" || created.CronExpr != "0 0 1 1 *" {
		t.Fatalf("created = %+v", created)
	}
	if _, ok := created.Params["TOKEN"]; ok {
		t.Fatalf("sensitive param leaked: %v", created.Params)
	}
	if created.Params["ENV"] != "prod" {
		t.Fatalf("params = %v", created.Params)
	}
	if created.NextRunAt == nil {
		t.Fatal("next_run_at should be set")
	}
	secret, err := f.svc.decryptSensitive(created)
	if err != nil || secret["TOKEN"] != "s3cr3t" {
		t.Fatalf("secret = %v err=%v", secret, err)
	}

	in := baseInput(f)
	in.Name = "nightly-v2"
	in.Params = map[string]string{"TOKEN": ""} // keep existing secret
	updated, err := f.svc.Update(ctx, created.ID, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "nightly-v2" {
		t.Fatalf("name = %q", updated.Name)
	}
	secret, _ = f.svc.decryptSensitive(updated)
	if secret["TOKEN"] != "s3cr3t" {
		t.Fatalf("secret lost on update: %v", secret)
	}

	times, err := f.svc.Preview(in.CronExpr, 3)
	if err != nil || len(times) != 3 {
		t.Fatalf("preview = %v err=%v", times, err)
	}
	list, total, err := f.svc.List(ctx, ListFilter{Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list: %v total=%d", err, total)
	}
	if err := f.svc.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted = %v", err)
	}
}

func TestValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	disabled, _ := f.tools.Create(ctx, tool.Input{Name: "off", Type: "shell", Content: "true"}, "admin")
	_ = f.tools.SetEnabled(ctx, disabled.ID, false)

	cases := map[string]func(*Input){
		"empty name":     func(in *Input) { in.Name = "" },
		"bad kind":       func(in *Input) { in.TargetKind = "x" },
		"missing target": func(in *Input) { in.TargetRefID = "nope" },
		"disabled tool":  func(in *Input) { in.TargetRefID = disabled.ID },
		"bad cron":       func(in *Input) { in.CronExpr = "not a cron" },
		"no hosts":       func(in *Input) { in.HostIDs = nil },
		"missing host":   func(in *Input) { in.HostIDs = []string{"nope"} },
		"bad policy":     func(in *Input) { in.ConcurrencyPolicy = "parallel" },
		"bad events":     func(in *Input) { in.NotifyEvents = "sometimes" },
		"missing channel": func(in *Input) {
			in.NotifyChannelIDs = []string{"nope"}
			in.NotifyEvents = "always"
		},
	}
	for name, mutate := range cases {
		in := baseInput(f)
		in.Name = "case-" + name
		mutate(&in)
		if _, err := f.svc.Create(ctx, in, "admin"); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestFlowScheduleHostsFollowNodeConfiguration(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	createFlow := func(name string, nodeHosts []string) string {
		t.Helper()
		d, err := f.flows.Create(ctx, flow.Input{
			Name: name, MaxDurationSec: 600,
			Nodes: []flow.NodeInput{
				{ID: "start", Type: "start", Name: "开始"},
				{ID: "n1", Type: "tool", Name: "run", ToolID: f.toolID, HostIDs: nodeHosts},
			},
			Edges: []flow.EdgeInput{{From: "start", To: "n1", Condition: flow.CondAlways}},
		}, "admin")
		if err != nil {
			t.Fatal(err)
		}
		return d.ID
	}

	configured := createFlow("configured", []string{f.hostID})
	in := baseInput(f)
	in.Name = "flow-configured"
	in.TargetKind = "flow"
	in.TargetRefID = configured
	in.Params = nil
	in.HostIDs = nil
	if _, err := f.svc.Create(ctx, in, "admin"); err != nil {
		t.Fatalf("flow with all nodes configured should allow empty default hosts: %v", err)
	}

	unconfigured := createFlow("unconfigured", nil)
	in = baseInput(f)
	in.Name = "flow-unconfigured"
	in.TargetKind = "flow"
	in.TargetRefID = unconfigured
	in.Params = nil
	in.HostIDs = nil
	if _, err := f.svc.Create(ctx, in, "admin"); err == nil {
		t.Fatal("flow with unconfigured node should require default hosts")
	}

	in.HostIDs = []string{f.hostID}
	if _, err := f.svc.Create(ctx, in, "admin"); err != nil {
		t.Fatalf("flow with default hosts should pass: %v", err)
	}

	in = baseInput(f)
	in.Name = "tool-no-host"
	in.HostIDs = nil
	if _, err := f.svc.Create(ctx, in, "admin"); err == nil {
		t.Fatal("tool schedule without hosts should fail")
	}
}

func TestFlowScheduleNodeHostSelection(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	d, err := f.flows.Create(ctx, flow.Input{
		Name: "host-input-flow", MaxDurationSec: 600,
		Nodes: []flow.NodeInput{
			{ID: "start", Type: "start", Name: "开始"},
			{ID: "a", Type: "tool", Name: "A", ToolID: f.toolID, HostSource: flow.HostSourceInput},
			{ID: "b", Type: "tool", Name: "B", ToolID: f.toolID, HostSource: flow.HostSourceNode, HostRefNodeID: "a"},
		},
		Edges: []flow.EdgeInput{
			{From: "start", To: "a", Condition: flow.CondAlways},
			{From: "a", To: "b", Condition: flow.CondAlways},
		},
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	in := baseInput(f)
	in.Name = "host-input-schedule"
	in.TargetKind = "flow"
	in.TargetRefID = d.ID
	in.HostIDs = nil
	in.NodeHostIDs = map[string][]string{"a": {f.hostID}}
	created, err := f.svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if len(created.NodeHostIDs["a"]) != 1 || created.NodeHostIDs["a"][0] != f.hostID {
		t.Fatalf("node hosts = %+v", created.NodeHostIDs)
	}
	in.NodeHostIDs = nil
	if _, err := f.svc.Create(ctx, in, "admin"); err == nil {
		t.Fatal("expected missing node host selection to fail")
	}
}

func TestFlowScheduleIgnoresRemovedParameters(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	flowInput := func(mapping map[string]flow.Mapping) flow.Input {
		return flow.Input{
			Name: "flow-params", MaxDurationSec: 600,
			Nodes: []flow.NodeInput{
				{ID: "start", Type: "start", Name: "开始"},
				{ID: "n1", Type: "tool", Name: "run", ToolID: f.toolID, HostIDs: []string{f.hostID}, Mapping: mapping},
			},
			Edges: []flow.EdgeInput{{From: "start", To: "n1", Condition: flow.CondAlways}},
		}
	}
	d, err := f.flows.Create(ctx, flowInput(map[string]flow.Mapping{
		"ENV": {Source: "input", InputName: "ENV", InputLabel: "环境"},
	}), "admin")
	if err != nil {
		t.Fatal(err)
	}
	in := baseInput(f)
	in.Name = "flow-param-schedule"
	in.TargetKind = "flow"
	in.TargetRefID = d.ID
	in.Params = map[string]string{"ENV": "prod"}
	created, err := f.svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatal(err)
	}

	// Remove the only input from the flow.
	if _, err := f.flows.Update(ctx, d.ID, flowInput(nil)); err != nil {
		t.Fatal(err)
	}
	in.Params = map[string]string{"ENV": "stale"}
	updated, err := f.svc.Update(ctx, created.ID, in)
	if err != nil {
		t.Fatalf("update schedule after parameter removal: %v", err)
	}
	if len(updated.Params) != 0 {
		t.Fatalf("stale params kept: %v", updated.Params)
	}
}

func TestSkipPolicyRecordsSkippedExecution(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	created, err := f.svc.Create(ctx, baseInput(f), "admin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	_, err = f.store.DB.Exec(`INSERT INTO executions (id, kind, source, ref_id, schedule_id, name_snapshot, plan_json, host_total, status, created_at)
		VALUES ('running-1', 'tool', 'schedule', ?, ?, 't1', '{}', 1, 'running', ?)`, f.toolID, created.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.fire(created.ID)
	var skipped int
	if err := f.store.DB.Get(&skipped, `SELECT count(*) FROM executions WHERE schedule_id = ? AND status = 'skipped'`, created.ID); err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Fatalf("skipped executions = %d", skipped)
	}
}
