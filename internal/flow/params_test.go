package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/Hopetree/spool/internal/tool"
)

func paramFlowInput(a, b string, mapA, mapB map[string]Mapping) Input {
	return Input{
		Name: "参数流程", MaxDurationSec: 600,
		Nodes: []NodeInput{
			{ID: "start", Type: NodeStart, Name: "开始"},
			{ID: "a", Type: NodeTool, Name: "A", ToolID: a, Mapping: mapA, X: 240, Y: 40},
			{ID: "b", Type: NodeTool, Name: "B", ToolID: b, Mapping: mapB, X: 500, Y: 40},
		},
		Edges: []EdgeInput{
			{From: "start", To: "a", Condition: CondAlways},
			{From: "a", To: "b", Condition: CondAlways},
		},
	}
}

func TestParameterSourcesAndReferences(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "source", []tool.Param{
		{Name: "pkg_path", Type: "string", Required: true},
		{Name: "work_dir", Type: "string", Default: "/tmp"},
		{Name: "mode", Type: "select", Options: []string{"fast", "safe"}},
	})
	b := createTool(t, tools, "target", []tool.Param{
		{Name: "pkg_path", Type: "string", Required: true},
		{Name: "work_dir", Type: "string", Default: "/tmp"},
		{Name: "mode", Type: "select", Options: []string{"fast", "safe", "debug"}},
	})
	in := paramFlowInput(a.ID, b.ID,
		map[string]Mapping{
			"pkg_path": {Source: "input", InputName: "pkg_path", InputLabel: "巡检包路径"},
			"work_dir": {Source: "literal", Value: "/data"},
			"mode":     {Source: "input", InputName: "mode", InputLabel: "执行模式"},
		},
		map[string]Mapping{
			"pkg_path": {Source: "node", RefNodeID: "a", RefParam: "pkg_path"},
			"work_dir": {Source: "node", RefNodeID: "a", RefParam: "work_dir"},
			"mode":     {Source: "node", RefNodeID: "a", RefParam: "mode"},
		},
	)
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Params) != 2 {
		t.Fatalf("params = %+v", created.Params)
	}
	byName := map[string]tool.Param{}
	for _, p := range created.Params {
		byName[p.Name] = p
	}
	if p := byName["pkg_path"]; !p.Required || p.Label != "巡检包路径" || p.Type != "string" {
		t.Fatalf("pkg_path input = %+v", p)
	}
	if p := byName["mode"]; p.Required || p.Label != "执行模式" || len(p.Options) != 2 {
		t.Fatalf("mode input = %+v", p)
	}
	for _, node := range created.Nodes {
		if node.ID == "b" && node.Mapping["pkg_path"].Source != "node" {
			t.Fatalf("b mapping = %+v", node.Mapping)
		}
	}

	res, err := svc.Compile(ctx, created.ID, map[string]string{
		"pkg_path": "/opt/pkg.tar.gz", "mode": "safe", "STALE": "ignored",
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Plain["pkg_path"] != "/opt/pkg.tar.gz" || res.Plain["mode"] != "safe" {
		t.Fatalf("plain = %+v", res.Plain)
	}
	for _, node := range res.Plan.Graph.Nodes {
		if node.Type != NodeTool {
			continue
		}
		if node.Params["pkg_path"] != "/opt/pkg.tar.gz" || node.Params["work_dir"] != "/data" || node.Params["mode"] != "safe" {
			t.Fatalf("node %s params = %+v", node.Name, node.Params)
		}
	}
}

func TestParameterValidation(t *testing.T) {
	cases := []struct {
		name   string
		aParam []tool.Param
		bParam []tool.Param
		mapA   map[string]Mapping
		mapB   map[string]Mapping
	}{
		{
			name: "type mismatch", aParam: []tool.Param{{Name: "count", Type: "int"}},
			bParam: []tool.Param{{Name: "count", Type: "string"}},
			mapA:   map[string]Mapping{"count": {Source: "literal", Value: "1"}},
			mapB:   map[string]Mapping{"count": {Source: "node", RefNodeID: "a", RefParam: "count"}},
		},
		{
			name: "downstream reference", aParam: []tool.Param{{Name: "x", Type: "string"}},
			bParam: []tool.Param{{Name: "x", Type: "string"}},
			mapA:   map[string]Mapping{"x": {Source: "node", RefNodeID: "b", RefParam: "x"}},
			mapB:   map[string]Mapping{"x": {Source: "literal", Value: "v"}},
		},
		{
			name: "sensitive literal", aParam: []tool.Param{{Name: "token", Type: "string", Sensitive: true}},
			mapA: map[string]Mapping{"token": {Source: "literal", Value: "secret"}},
		},
		{
			name: "required unmapped", aParam: []tool.Param{{Name: "req", Type: "string", Required: true}},
		},
		{
			name: "sensitive reference to literal", aParam: []tool.Param{{Name: "plain", Type: "string"}},
			bParam: []tool.Param{{Name: "secret", Type: "string", Sensitive: true}},
			mapA:   map[string]Mapping{"plain": {Source: "literal", Value: "v"}},
			mapB:   map[string]Mapping{"secret": {Source: "node", RefNodeID: "a", RefParam: "plain"}},
		},
		{
			name: "sensitive reference to plain default", aParam: []tool.Param{{Name: "plain", Type: "string", Default: "v"}},
			bParam: []tool.Param{{Name: "secret", Type: "string", Sensitive: true}},
			mapB:   map[string]Mapping{"secret": {Source: "node", RefNodeID: "a", RefParam: "plain"}},
		},
		{
			name: "select options not covered", aParam: []tool.Param{{Name: "mode", Type: "select", Options: []string{"fast", "safe"}}},
			bParam: []tool.Param{{Name: "mode", Type: "select", Options: []string{"fast"}}},
			mapA:   map[string]Mapping{"mode": {Source: "literal", Value: "fast"}},
			mapB:   map[string]Mapping{"mode": {Source: "node", RefNodeID: "a", RefParam: "mode"}},
		},
		{
			name: "duplicate input type", aParam: []tool.Param{{Name: "x", Type: "string"}},
			bParam: []tool.Param{{Name: "y", Type: "int"}},
			mapA:   map[string]Mapping{"x": {Source: "input", InputName: "shared"}},
			mapB:   map[string]Mapping{"y": {Source: "input", InputName: "shared"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, tools, _ := newFlowFixture(t)
			a := createTool(t, tools, "a", tc.aParam)
			b := createTool(t, tools, "b", tc.bParam)
			in := paramFlowInput(a.ID, b.ID, tc.mapA, tc.mapB)
			in.Name = "case-" + tc.name
			if _, err := svc.Create(context.Background(), in, "admin"); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSiblingReferenceRejected(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	a := createTool(t, tools, "a", []tool.Param{{Name: "x", Type: "string"}})
	b := createTool(t, tools, "b", []tool.Param{{Name: "x", Type: "string"}})
	in := Input{
		Name: "siblings", MaxDurationSec: 600,
		Nodes: []NodeInput{
			{ID: "start", Type: NodeStart, Name: "开始"},
			{ID: "a", Type: NodeTool, Name: "A", ToolID: a.ID, Mapping: map[string]Mapping{"x": {Source: "literal", Value: "v"}}},
			{ID: "b", Type: NodeTool, Name: "B", ToolID: b.ID, Mapping: map[string]Mapping{"x": {Source: "node", RefNodeID: "a", RefParam: "x"}}},
		},
		Edges: []EdgeInput{
			{From: "start", To: "a", Condition: CondAlways},
			{From: "start", To: "b", Condition: CondAlways},
		},
	}
	if _, err := svc.Create(context.Background(), in, "admin"); err == nil {
		t.Fatal("expected sibling reference to fail")
	}
}

func TestSensitiveAndRequiredPropagation(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "a", []tool.Param{{Name: "seed", Type: "string"}})
	b := createTool(t, tools, "b", []tool.Param{{Name: "seed", Type: "string", Required: true, Sensitive: true}})
	in := paramFlowInput(a.ID, b.ID,
		map[string]Mapping{"seed": {Source: "input", InputName: "shared_seed", InputLabel: "共享种子"}},
		map[string]Mapping{"seed": {Source: "node", RefNodeID: "a", RefParam: "seed"}},
	)
	in.Name = "sensitive-propagation"
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Params) != 1 {
		t.Fatalf("params = %+v", created.Params)
	}
	p := created.Params[0]
	if !p.Sensitive || !p.Required || p.Label != "共享种子" {
		t.Fatalf("derived input = %+v", p)
	}
	if _, err := svc.Compile(ctx, created.ID, nil); err == nil {
		t.Fatal("expected missing required input to fail")
	}
	res, err := svc.Compile(ctx, created.ID, map[string]string{"shared_seed": "s3cr3t"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Sensitive["shared_seed"] != "s3cr3t" {
		t.Fatalf("sensitive = %+v", res.Sensitive)
	}
	for _, node := range res.Plan.Graph.Nodes {
		if node.Type != NodeTool {
			continue
		}
		if _, ok := node.Params["seed"]; ok {
			t.Fatalf("node %s leaked sensitive seed into plain params", node.Name)
		}
		key := node.SensitiveParams["seed"]
		if key == "" || res.Sensitive[key] != "s3cr3t" {
			t.Fatalf("node %s sensitive params = %+v", node.Name, node.SensitiveParams)
		}
	}
}

func TestDuplicateInputNamesAndLabelsRejected(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "a", []tool.Param{{Name: "x", Type: "string"}})
	b := createTool(t, tools, "b", []tool.Param{{Name: "y", Type: "string"}})
	in := paramFlowInput(a.ID, b.ID,
		map[string]Mapping{"x": {Source: "input", InputName: "shared", InputLabel: "共享值"}},
		map[string]Mapping{"y": {Source: "input", InputName: "shared", InputLabel: "另一个标题"}},
	)
	in.Name = "duplicate-input-name"
	if _, err := svc.Create(ctx, in, "admin"); err == nil {
		t.Fatal("expected duplicate input name to fail")
	} else if !strings.Contains(err.Error(), "输入名") || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("unexpected error: %v", err)
	}

	in = paramFlowInput(a.ID, b.ID,
		map[string]Mapping{"x": {Source: "input", InputName: "first", InputLabel: "同名标题"}},
		map[string]Mapping{"y": {Source: "input", InputName: "second", InputLabel: "同名标题"}},
	)
	in.Name = "duplicate-input-label"
	if _, err := svc.Create(ctx, in, "admin"); err == nil {
		t.Fatal("expected duplicate input label to fail")
	} else if !strings.Contains(err.Error(), "显示标题") || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLegacyFlowMappingConversionAndFiltering(t *testing.T) {
	svc, tools, st := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "legacy", []tool.Param{{Name: "ENV", Type: "string", Default: "prod"}})
	in := Input{
		Name: "legacy", MaxDurationSec: 600,
		Params: []tool.Param{
			{Name: "ENV", Type: "string", Default: "prod", Label: "环境"},
			{Name: "UNUSED", Type: "string"},
		},
		Nodes: []NodeInput{
			{ID: "start", Type: NodeStart, Name: "开始"},
			{ID: "a", Type: NodeTool, Name: "A", ToolID: a.ID, Mapping: map[string]Mapping{
				"ENV": {Source: "flow", Value: "ENV"},
			}},
		},
		Edges: []EdgeInput{{From: "start", To: "a", Condition: CondAlways}},
	}
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if len(created.Params) != 1 || created.Params[0].Name != "ENV" || created.Params[0].Default != "" {
		t.Fatalf("params = %+v", created.Params)
	}
	// Old flows may still carry stored defaults; responses must not expose them.
	if _, err := st.DB.Exec(`UPDATE flows SET params_json = ? WHERE id = ?`,
		`[{"name":"ENV","label":"环境","type":"string","required":false,"default":"prod","options":[]}]`, created.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Params) != 1 || loaded.Params[0].Default != "" {
		t.Fatalf("stored default exposed: %+v", loaded.Params)
	}
	m := created.Nodes[1].Mapping["ENV"]
	if m.Source != "input" || m.InputName != "ENV" || m.InputLabel != "环境" {
		t.Fatalf("mapping = %+v", m)
	}
	res, err := svc.Compile(ctx, created.ID, map[string]string{"ENV": "prod", "UNUSED": "ignored"})
	if err != nil {
		t.Fatalf("compile legacy: %v", err)
	}
	if res.Plan.Graph.Nodes[1].Params["ENV"] != "prod" {
		t.Fatalf("node params = %+v", res.Plan.Graph.Nodes[1].Params)
	}

	// Updating without the old mapping drops the unused input.
	in2 := Input{
		Name: "legacy", MaxDurationSec: 600,
		Nodes: []NodeInput{
			{ID: "start", Type: NodeStart, Name: "开始"},
			{ID: "a", Type: NodeTool, Name: "A", ToolID: a.ID},
		},
		Edges: []EdgeInput{{From: "start", To: "a", Condition: CondAlways}},
	}
	updated, err := svc.Update(ctx, created.ID, in2)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(updated.Params) != 0 {
		t.Fatalf("params after cleanup = %+v", updated.Params)
	}
	if _, err := svc.Compile(ctx, created.ID, map[string]string{"ENV": "stale", "UNUSED": "stale"}); err != nil {
		t.Fatalf("compile after cleanup: %v", err)
	}
}

func TestLegacySensitiveFlowParameterKeepsSensitivity(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "legacy-sensitive", []tool.Param{{Name: "TOKEN", Type: "string"}})
	in := Input{
		Name: "legacy-sensitive", MaxDurationSec: 600,
		Params: []tool.Param{{Name: "SECRET", Type: "string", Sensitive: true, Label: "令牌"}},
		Nodes: []NodeInput{
			{ID: "start", Type: NodeStart, Name: "开始"},
			{ID: "a", Type: NodeTool, Name: "A", ToolID: a.ID, Mapping: map[string]Mapping{
				"TOKEN": {Source: "flow", Value: "SECRET"},
			}},
		},
		Edges: []EdgeInput{{From: "start", To: "a", Condition: CondAlways}},
	}
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Params) != 1 || !created.Params[0].Sensitive || created.Params[0].Label != "令牌" {
		t.Fatalf("params = %+v", created.Params)
	}
	res, err := svc.Compile(ctx, created.ID, map[string]string{"SECRET": "s3cr3t"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Sensitive["SECRET"] != "s3cr3t" {
		t.Fatalf("sensitive = %+v", res.Sensitive)
	}
	node := res.Plan.Graph.Nodes[1]
	key := node.SensitiveParams["TOKEN"]
	if key == "" || res.Sensitive[key] != "s3cr3t" {
		t.Fatalf("node sensitive params = %+v", node.SensitiveParams)
	}
}

func TestCompileToleratesLegacySharedInputNames(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	_ = svc
	a := createTool(t, tools, "a", []tool.Param{{Name: "x", Type: "string"}})
	b := createTool(t, tools, "b", []tool.Param{{Name: "y", Type: "string"}})
	nodes := []NodeInput{
		{ID: "start", Type: NodeStart, Name: "开始"},
		{ID: "a", Type: NodeTool, Name: "A", ToolID: a.ID, Mapping: map[string]Mapping{
			"x": {Source: "input", InputName: "shared", InputLabel: "共享值"},
		}},
		{ID: "b", Type: NodeTool, Name: "B", ToolID: b.ID, Mapping: map[string]Mapping{
			"y": {Source: "input", InputName: "shared", InputLabel: "共享值"},
		}},
	}
	edges := []EdgeInput{
		{From: "start", To: "a", Condition: CondAlways},
		{From: "a", To: "b", Condition: CondAlways},
	}
	resolver, err := newParamResolver(nodes, edges, map[string]*tool.Detail{"a": a, "b": b})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deriveInputs(resolver, true); err == nil {
		t.Fatal("expected save-time uniqueness check to fail")
	}
	inputs, err := deriveInputs(resolver, false)
	if err != nil {
		t.Fatalf("compile-time derivation: %v", err)
	}
	if len(inputs) != 1 || inputs[0].Name != "shared" {
		t.Fatalf("inputs = %+v", inputs)
	}
}

func TestExecutionInputDoesNotInheritToolDefault(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "a", []tool.Param{{Name: "service", Type: "string", Default: "spool"}})
	in := Input{
		Name: "blank-input", MaxDurationSec: 600,
		Nodes: []NodeInput{
			{ID: "start", Type: NodeStart, Name: "开始"},
			{ID: "a", Type: NodeTool, Name: "A", ToolID: a.ID, Mapping: map[string]Mapping{
				"service": {Source: "input", InputName: "service", InputLabel: "服务名"},
			}},
		},
		Edges: []EdgeInput{{From: "start", To: "a", Condition: CondAlways}},
	}
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Params) != 1 || created.Params[0].Default != "" {
		t.Fatalf("params = %+v", created.Params)
	}
	res, err := svc.Compile(ctx, created.ID, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := res.Plan.Graph.Nodes[1].Params["service"]; got != "" {
		t.Fatalf("execution input fell back to tool default: %q", got)
	}
}
