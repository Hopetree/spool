package flow

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/tool"
)

func newFlowFixture(t *testing.T) (*Service, *tool.Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	tools := tool.NewService(st.DB)
	return NewService(st.DB, tools), tools, st
}

func createTool(t *testing.T, tools *tool.Service, name string, params []tool.Param) *tool.Detail {
	t.Helper()
	d, err := tools.Create(context.Background(), tool.Input{
		Name: name, Type: "shell", Interpreter: "/bin/bash", Content: "echo " + name,
		HostParam: tool.HostParam{DefaultHostIDs: []string{"h1"}},
		Params:    params, TimeoutSec: 30,
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func baseFlowInput(toolA, toolB string) Input {
	return Input{
		Name: "deploy", Description: "deploy flow", MaxDurationSec: 600,
		Params: []tool.Param{
			{Name: "ENV", Type: "string", Default: "prod"},
			{Name: "SECRET", Type: "string", Sensitive: true},
		},
		Nodes: []NodeInput{
			{ID: "start", Type: "start", Name: "开始"},
			{
				ID: "n1", Type: "tool", Name: "deploy", ToolID: toolA,
				Mapping: map[string]Mapping{
					"SPOOL_HOST": {Source: "literal", Value: `["h1","h2"]`},
					"ENV":        {Source: "flow", Value: "ENV"},
					"TOKEN":      {Source: "flow", Value: "SECRET"},
				},
				TimeoutSec: 60, RetryCount: 1, RetryDelaySec: 2, X: 260, Y: 40,
			},
			{ID: "n2", Type: "tool", Name: "cleanup", ToolID: toolB, Mapping: map[string]Mapping{
				"SPOOL_HOST": {Source: "literal", Value: `["h2"]`},
			}, X: 520, Y: 40},
		},
		Edges: []EdgeInput{
			{From: "start", To: "n1", Condition: CondAlways},
			{From: "n1", To: "n2", Condition: CondAlways},
		},
	}
}

func TestCreateGetUpdateCopyDelete(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "t1", []tool.Param{
		{Name: "ENV", Type: "string", Default: "prod"},
		{Name: "TOKEN", Type: "string", Sensitive: true},
	})
	b := createTool(t, tools, "t2", nil)

	created, err := svc.Create(ctx, baseFlowInput(a.ID, b.ID), "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.NodeCount != 2 || created.EdgeCount != 2 || len(created.Nodes) != 3 || len(created.Edges) != 2 {
		t.Fatalf("created = %+v", created)
	}
	if created.Nodes[0].Type != NodeStart || created.Nodes[0].ID != "start" {
		t.Fatalf("start node = %+v", created.Nodes[0])
	}
	list, total, err := svc.List(ctx, ListFilter{Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(list) != 1 || list[0].NodeCount != 2 {
		t.Fatalf("list: %v total=%d", err, total)
	}

	in := baseFlowInput(a.ID, b.ID)
	in.Name = "deploy-v2"
	in.Edges[1].Condition = CondFailure
	in.Nodes = append(in.Nodes, NodeInput{ID: "n3", Type: "tool", Name: "notify", ToolID: b.ID, X: 780, Y: 40})
	in.Edges = append(in.Edges, EdgeInput{From: "n1", To: "n3", Condition: CondSuccess})
	updated, err := svc.Update(ctx, created.ID, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "deploy-v2" || len(updated.Nodes) != 4 || len(updated.Edges) != 3 {
		t.Fatalf("updated = %+v", updated)
	}

	copied, err := svc.Copy(ctx, created.ID, "admin")
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if copied.Name != "deploy-v2-copy" || len(copied.Nodes) != 4 {
		t.Fatalf("copied = %+v", copied)
	}
	if err := svc.Delete(ctx, copied.ID); err != nil {
		t.Fatal(err)
	}
}

func TestValidation(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "t1", []tool.Param{
		{Name: "ENV", Type: "string", Default: "prod"},
		{Name: "TOKEN", Type: "string", Sensitive: true},
		{Name: "REQ", Type: "string", Required: true},
	})
	b := createTool(t, tools, "t2", nil)
	disabled, _ := tools.Create(ctx, tool.Input{Name: "disabled", Type: "shell", Content: "true"}, "admin")
	_ = tools.SetEnabled(ctx, disabled.ID, false)

	cases := map[string]func(*Input){
		"empty name":        func(in *Input) { in.Name = "" },
		"no nodes":          func(in *Input) { in.Nodes = nil; in.Edges = nil },
		"missing start":     func(in *Input) { in.Nodes = in.Nodes[1:]; in.Edges = in.Edges[1:] },
		"two starts":        func(in *Input) { in.Nodes = append(in.Nodes, NodeInput{ID: "start2", Type: NodeStart}) },
		"missing tool":      func(in *Input) { in.Nodes[1].ToolID = "nope" },
		"disabled tool":     func(in *Input) { in.Nodes[1].ToolID = disabled.ID },
		"unknown mapping":   func(in *Input) { in.Nodes[1].Mapping["NOPE"] = Mapping{Source: "literal", Value: "x"} },
		"missing flow ref":  func(in *Input) { in.Nodes[1].Mapping["ENV"] = Mapping{Source: "flow", Value: "MISSING"} },
		"sensitive literal": func(in *Input) { in.Nodes[1].Mapping["TOKEN"] = Mapping{Source: "literal", Value: "x"} },
		"required unmapped": func(in *Input) { delete(in.Nodes[1].Mapping, "REQ") },
		"bad condition":     func(in *Input) { in.Edges[0].Condition = "sometimes" },
		"output no value":   func(in *Input) { in.Edges[0].Condition = CondOutputContains },
		"duplicate edge":    func(in *Input) { in.Edges = append(in.Edges, in.Edges[0]) },
		"self edge":         func(in *Input) { in.Edges = append(in.Edges, EdgeInput{From: "n1", To: "n1", Condition: CondAlways}) },
		"unreachable node": func(in *Input) {
			in.Nodes = append(in.Nodes, NodeInput{ID: "lonely", Type: NodeTool, Name: "lonely", ToolID: b.ID})
		},
		"start no outgoing": func(in *Input) { in.Edges = in.Edges[1:] },
		"cycle": func(in *Input) {
			in.Edges = append(in.Edges, EdgeInput{From: "n2", To: "n1", Condition: CondAlways})
		},
	}
	for name, mutate := range cases {
		in := baseFlowInput(a.ID, b.ID)
		in.Name = "case-" + name
		mutate(&in)
		if _, err := svc.Create(ctx, in, "admin"); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestCompileBuildsGraph(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "t1", []tool.Param{
		{Name: "ENV", Type: "string", Default: "prod"},
		{Name: "TOKEN", Type: "string", Sensitive: true},
	})
	b := createTool(t, tools, "t2", nil)
	created, err := svc.Create(ctx, baseFlowInput(a.ID, b.ID), "admin")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Compile(ctx, created.ID, map[string]string{"ENV": "prod", "SECRET": "s3cr3t"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Plan.Graph == nil || len(res.Plan.Graph.Nodes) != 3 || len(res.Plan.Graph.Edges) != 2 {
		t.Fatalf("plan = %+v", res.Plan)
	}
	if res.Plain["ENV"] != "prod" {
		t.Fatalf("plain = %v", res.Plain)
	}
	if _, ok := res.Plain["SECRET"]; ok {
		t.Fatal("sensitive flow param leaked into plain")
	}
	var n1 *struct{ Params, Sensitive map[string]string }
	var params, sens map[string]string
	for _, node := range res.Plan.Graph.Nodes {
		if node.ID == "n1" {
			params, sens = node.Params, node.SensitiveParams
			if len(node.HostIDs) != 2 || node.HostIDs[0] != "h1" || node.HostIDs[1] != "h2" {
				t.Fatalf("compiled n1 hosts = %v", node.HostIDs)
			}
		}
	}
	_ = n1
	if params["ENV"] != "prod" || params["TOKEN"] != "" {
		t.Fatalf("node params = %v", params)
	}
	key := sens["TOKEN"]
	if key == "" || res.Sensitive[key] != "s3cr3t" {
		t.Fatalf("sensitive = %v key=%q", sens, key)
	}
}

func TestCompileDisabledFlowAndTool(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "t1", []tool.Param{
		{Name: "ENV", Type: "string", Default: "prod"},
		{Name: "TOKEN", Type: "string", Sensitive: true},
	})
	b := createTool(t, tools, "t2", nil)
	created, err := svc.Create(ctx, baseFlowInput(a.ID, b.ID), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetEnabled(ctx, created.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Compile(ctx, created.ID, map[string]string{"SECRET": "x"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if err := svc.SetEnabled(ctx, created.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := tools.SetEnabled(ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Compile(ctx, created.ID, map[string]string{"SECRET": "x"}); err == nil {
		t.Fatal("expected disabled tool error")
	}
}
