package flow

import (
	"context"
	"testing"

	"github.com/Hopetree/spool/internal/tool"
)

func createHostTool(t *testing.T, tools *tool.Service, name string, defaults []string, params []tool.Param) *tool.Detail {
	t.Helper()
	d, err := tools.Create(context.Background(), tool.Input{
		Name: name, Type: "shell", Content: "echo " + name,
		HostParam: tool.HostParam{Label: "执行主机", DefaultHostIDs: defaults},
		Params:    params, TimeoutSec: 30,
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestHostParameterSources(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	td := createHostTool(t, tools, "host-tool", []string{"h1"}, nil)
	in := Input{
		Name: "host-params", MaxDurationSec: 600,
		Nodes: []NodeInput{
			{ID: "start", Type: NodeStart, Name: "开始"},
			{ID: "a", Type: NodeTool, Name: "A", ToolID: td.ID},
			{ID: "b", Type: NodeTool, Name: "B", ToolID: td.ID, Mapping: map[string]Mapping{
				"SPOOL_HOST": {Source: "literal", Value: `["h2","h3"]`},
			}},
			{ID: "c", Type: NodeTool, Name: "C", ToolID: td.ID, Mapping: map[string]Mapping{
				"SPOOL_HOST": {Source: "input", InputName: "flow_hosts", InputLabel: "执行主机"},
			}},
			{ID: "d", Type: NodeTool, Name: "D", ToolID: td.ID, Mapping: map[string]Mapping{
				"SPOOL_HOST": {Source: "node", RefNodeID: "b", RefParam: "SPOOL_HOST"},
			}},
		},
		Edges: []EdgeInput{
			{From: "start", To: "a", Condition: CondAlways},
			{From: "a", To: "b", Condition: CondAlways},
			{From: "b", To: "c", Condition: CondAlways},
			{From: "c", To: "d", Condition: CondAlways},
		},
	}
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Params) != 1 || created.Params[0].Name != "flow_hosts" || created.Params[0].Type != "host" || !created.Params[0].Required {
		t.Fatalf("params = %+v", created.Params)
	}
	res, err := svc.Compile(ctx, created.ID, map[string]string{"flow_hosts": `["h4"]`})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	hosts := map[string][]string{}
	for _, node := range res.Plan.Graph.Nodes {
		hosts[node.ID] = node.HostIDs
	}
	if len(hosts["a"]) != 1 || hosts["a"][0] != "h1" {
		t.Fatalf("a hosts = %v", hosts["a"])
	}
	if len(hosts["b"]) != 2 || hosts["b"][0] != "h2" || hosts["b"][1] != "h3" {
		t.Fatalf("b hosts = %v", hosts["b"])
	}
	if len(hosts["c"]) != 1 || hosts["c"][0] != "h4" {
		t.Fatalf("c hosts = %v", hosts["c"])
	}
	if len(hosts["d"]) != 2 || hosts["d"][0] != "h2" {
		t.Fatalf("d hosts = %v", hosts["d"])
	}
	if _, err := svc.Compile(ctx, created.ID, nil); err == nil {
		t.Fatal("expected missing host input to fail")
	}
}

func TestHostParameterValidation(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	noDefault := createHostTool(t, tools, "no-default", nil, nil)
	withDefault := createHostTool(t, tools, "with-default", []string{"h1"}, nil)
	cases := []struct {
		name  string
		nodes []NodeInput
	}{
		{
			name: "missing required host",
			nodes: []NodeInput{
				{ID: "a", Type: NodeTool, Name: "A", ToolID: noDefault.ID},
			},
		},
		{
			name: "invalid literal host list",
			nodes: []NodeInput{
				{ID: "a", Type: NodeTool, Name: "A", ToolID: withDefault.ID, Mapping: map[string]Mapping{
					"SPOOL_HOST": {Source: "literal", Value: "not-json"},
				}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := Input{
				Name: "case-" + tc.name, MaxDurationSec: 600,
				Nodes: append([]NodeInput{{ID: "start", Type: NodeStart, Name: "开始"}}, tc.nodes...),
				Edges: []EdgeInput{{From: "start", To: tc.nodes[0].ID, Condition: CondAlways}},
			}
			if _, err := svc.Create(ctx, in, "admin"); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestHostParameterUpstreamReference(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	td := createHostTool(t, tools, "host-ref", []string{"h1"}, nil)
	in := Input{
		Name: "host-ref", MaxDurationSec: 600,
		Nodes: []NodeInput{
			{ID: "start", Type: NodeStart, Name: "开始"},
			{ID: "a", Type: NodeTool, Name: "A", ToolID: td.ID, Mapping: map[string]Mapping{
				"SPOOL_HOST": {Source: "node", RefNodeID: "b", RefParam: "SPOOL_HOST"},
			}},
			{ID: "b", Type: NodeTool, Name: "B", ToolID: td.ID},
		},
		Edges: []EdgeInput{
			{From: "start", To: "a", Condition: CondAlways},
			{From: "a", To: "b", Condition: CondAlways},
		},
	}
	if _, err := svc.Create(ctx, in, "admin"); err == nil {
		t.Fatal("expected downstream host reference to fail")
	}
}
