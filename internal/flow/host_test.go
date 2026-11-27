package flow

import (
	"context"
	"testing"

	"github.com/Hopetree/spool/internal/tool"
)

func hostFlowInput(toolID string, nodes []NodeInput) Input {
	return Input{
		Name: "host-flow", MaxDurationSec: 600,
		Nodes: append([]NodeInput{{ID: "start", Type: NodeStart, Name: "开始"}}, nodes...),
		Edges: []EdgeInput{
			{From: "start", To: nodes[0].ID, Condition: CondAlways},
			{From: nodes[0].ID, To: nodes[1].ID, Condition: CondAlways},
			{From: nodes[1].ID, To: nodes[2].ID, Condition: CondAlways},
			{From: nodes[2].ID, To: nodes[3].ID, Condition: CondAlways},
			{From: nodes[3].ID, To: nodes[4].ID, Condition: CondAlways},
		},
	}
}

func TestHostSourcesResolveAndCompile(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	td := createTool(t, tools, "host-tool", []tool.Param{{Name: "X", Type: "string", Default: "x"}})
	nodes := []NodeInput{
		{ID: "a", Type: NodeTool, Name: "A", ToolID: td.ID, HostSource: HostSourceFixed, HostIDs: []string{"h1", "h2"}},
		{ID: "b", Type: NodeTool, Name: "B", ToolID: td.ID, HostSource: HostSourceInput},
		{ID: "c", Type: NodeTool, Name: "C", ToolID: td.ID, HostSource: HostSourceNode, HostRefNodeID: "a"},
		{ID: "d", Type: NodeTool, Name: "D", ToolID: td.ID, HostSource: HostSourceNode, HostRefNodeID: "b"},
		{ID: "e", Type: NodeTool, Name: "E", ToolID: td.ID, HostSource: HostSourceDefault},
	}
	created, err := svc.Create(ctx, hostFlowInput(td.ID, nodes), "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	byID := map[string]Node{}
	for _, node := range created.Nodes {
		byID[node.ID] = node
	}
	if byID["a"].HostSource != HostSourceFixed || len(byID["a"].HostIDs) != 2 {
		t.Fatalf("a = %+v", byID["a"])
	}
	if byID["b"].HostSource != HostSourceInput || byID["b"].HostRefNodeID != "" {
		t.Fatalf("b = %+v", byID["b"])
	}
	if byID["c"].HostRefNodeID != "a" || byID["d"].HostRefNodeID != "b" {
		t.Fatalf("refs = %+v / %+v", byID["c"], byID["d"])
	}

	res, err := svc.Compile(ctx, created.ID, nil, map[string][]string{"b": {"h3"}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	hosts := map[string][]string{}
	for _, node := range res.Plan.Graph.Nodes {
		hosts[node.ID] = node.HostIDs
	}
	if len(hosts["a"]) != 2 || hosts["a"][0] != "h1" {
		t.Fatalf("a hosts = %v", hosts["a"])
	}
	if len(hosts["b"]) != 1 || hosts["b"][0] != "h3" {
		t.Fatalf("b hosts = %v", hosts["b"])
	}
	if len(hosts["c"]) != 2 || hosts["c"][1] != "h2" {
		t.Fatalf("c hosts = %v", hosts["c"])
	}
	if len(hosts["d"]) != 1 || hosts["d"][0] != "h3" {
		t.Fatalf("d hosts = %v", hosts["d"])
	}
	if len(hosts["e"]) != 0 {
		t.Fatalf("e hosts = %v", hosts["e"])
	}

	if _, err := svc.Compile(ctx, created.ID, nil, nil); err == nil {
		t.Fatal("expected missing host input to fail")
	}
}

func TestHostReferenceValidation(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	td := createTool(t, tools, "host-tool", nil)
	cases := []struct {
		name  string
		nodes []NodeInput
	}{
		{
			name: "downstream reference",
			nodes: []NodeInput{
				{ID: "a", Type: NodeTool, Name: "A", ToolID: td.ID, HostSource: HostSourceNode, HostRefNodeID: "b"},
				{ID: "b", Type: NodeTool, Name: "B", ToolID: td.ID, HostSource: HostSourceFixed, HostIDs: []string{"h1"}},
				{ID: "c", Type: NodeTool, Name: "C", ToolID: td.ID},
				{ID: "d", Type: NodeTool, Name: "D", ToolID: td.ID},
				{ID: "e", Type: NodeTool, Name: "E", ToolID: td.ID},
			},
		},
		{
			name: "missing target",
			nodes: []NodeInput{
				{ID: "a", Type: NodeTool, Name: "A", ToolID: td.ID, HostSource: HostSourceNode, HostRefNodeID: "nope"},
				{ID: "b", Type: NodeTool, Name: "B", ToolID: td.ID},
				{ID: "c", Type: NodeTool, Name: "C", ToolID: td.ID},
				{ID: "d", Type: NodeTool, Name: "D", ToolID: td.ID},
				{ID: "e", Type: NodeTool, Name: "E", ToolID: td.ID},
			},
		},
		{
			name: "fixed without hosts",
			nodes: []NodeInput{
				{ID: "a", Type: NodeTool, Name: "A", ToolID: td.ID, HostSource: HostSourceFixed},
				{ID: "b", Type: NodeTool, Name: "B", ToolID: td.ID},
				{ID: "c", Type: NodeTool, Name: "C", ToolID: td.ID},
				{ID: "d", Type: NodeTool, Name: "D", ToolID: td.ID},
				{ID: "e", Type: NodeTool, Name: "E", ToolID: td.ID},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := hostFlowInput(td.ID, tc.nodes)
			in.Name = "case-" + tc.name
			if _, err := svc.Create(context.Background(), in, "admin"); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
