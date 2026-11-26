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
		Params: params, TimeoutSec: 30,
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func baseFlowInput(toolA, toolB string) Input {
	return Input{
		Name: "deploy", Description: "deploy flow",
		Params: []tool.Param{
			{Name: "ENV", Type: "string", Default: "prod"},
			{Name: "SECRET", Type: "string", Sensitive: true},
		},
		FailurePolicy: "abort", MaxDurationSec: 600,
		Stages: []StageInput{
			{Name: "准备", Steps: []StepInput{{
				Name: "deploy", ToolID: toolA, Enabled: true,
				Mapping: map[string]Mapping{
					"ENV":   {Source: "flow", Value: "ENV"},
					"TOKEN": {Source: "flow", Value: "SECRET"},
				},
				TimeoutSec: 60, RetryCount: 1, RetryDelaySec: 2,
			}}},
			{Name: "清理", AlwaysRun: true, Steps: []StepInput{{Name: "cleanup", ToolID: toolB, Enabled: true}}},
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
	if created.StageCount != 2 || created.StepCount != 2 || len(created.Stages) != 2 {
		t.Fatalf("created = %+v", created)
	}
	if !created.Stages[1].AlwaysRun || created.Stages[0].Steps[0].ToolName != "t1" {
		t.Fatalf("stages = %+v", created.Stages)
	}

	list, total, err := svc.List(ctx, ListFilter{Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(list) != 1 || list[0].StageCount != 2 {
		t.Fatalf("list: %v total=%d list=%+v", err, total, list)
	}

	in := baseFlowInput(a.ID, b.ID)
	in.Name = "deploy-v2"
	in.Stages[0].Steps[0].Mapping["ENV"] = Mapping{Source: "literal", Value: "staging"}
	updated, err := svc.Update(ctx, created.ID, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "deploy-v2" || updated.Stages[0].Steps[0].Mapping["ENV"].Value != "staging" {
		t.Fatalf("updated = %+v", updated)
	}

	copied, err := svc.Copy(ctx, created.ID, "admin")
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if copied.Name != "deploy-v2-copy" {
		t.Fatalf("copy name = %q", copied.Name)
	}

	if err := svc.SetEnabled(ctx, created.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, copied.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, copied.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted = %v", err)
	}
}

func TestValidationRejectsBadDefinitions(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "t1", []tool.Param{
		{Name: "ENV", Type: "string", Default: "prod"},
		{Name: "TOKEN", Type: "string", Sensitive: true},
		{Name: "REQ", Type: "string", Required: true},
	})
	b := createTool(t, tools, "t2", nil)
	disabled, _ := tools.Create(ctx, tool.Input{Name: "disabled", Type: "shell", Content: "true"}, "admin")
	if err := tools.SetEnabled(ctx, disabled.ID, false); err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(*Input){
		"empty name":        func(in *Input) { in.Name = "" },
		"no stages":         func(in *Input) { in.Stages = nil },
		"missing tool":      func(in *Input) { in.Stages[0].Steps[0].ToolID = "missing" },
		"disabled tool":     func(in *Input) { in.Stages[0].Steps[0].ToolID = disabled.ID },
		"unknown mapping":   func(in *Input) { in.Stages[0].Steps[0].Mapping["NOPE"] = Mapping{Source: "literal", Value: "x"} },
		"missing flow ref":  func(in *Input) { in.Stages[0].Steps[0].Mapping["ENV"] = Mapping{Source: "flow", Value: "MISSING"} },
		"sensitive literal": func(in *Input) { in.Stages[0].Steps[0].Mapping["TOKEN"] = Mapping{Source: "literal", Value: "x"} },
		"sensitive mismatch": func(in *Input) {
			in.Stages[0].Steps[0].Mapping["TOKEN"] = Mapping{Source: "flow", Value: "ENV"}
		},
		"required unmapped": func(in *Input) {
			delete(in.Stages[0].Steps[0].Mapping, "REQ")
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

func TestCompileSplitsSensitiveValues(t *testing.T) {
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
	res, err := svc.Compile(ctx, created.ID, map[string]string{"SECRET": "s3cr3t"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Plain["ENV"] != "prod" {
		t.Fatalf("plain = %v", res.Plain)
	}
	if _, ok := res.Plain["SECRET"]; ok {
		t.Fatalf("sensitive flow param leaked into plain: %v", res.Plain)
	}
	if res.Sensitive["SECRET"] != "s3cr3t" {
		t.Fatalf("sensitive = %v", res.Sensitive)
	}
	if len(res.Plan.Stages) != 2 || res.Plan.Stages[1].AlwaysRun != true {
		t.Fatalf("plan = %+v", res.Plan)
	}
	step := res.Plan.Stages[0].Steps[0]
	if step.Params["ENV"] != "prod" || step.TimeoutSec != 60 || step.RetryCount != 1 {
		t.Fatalf("step = %+v", step)
	}
	key := step.SensitiveParams["TOKEN"]
	if key == "" || res.Sensitive[key] != "s3cr3t" {
		t.Fatalf("sensitive mapping = %+v sensitive=%v", step.SensitiveParams, res.Sensitive)
	}
	if _, ok := step.Params["TOKEN"]; ok {
		t.Fatal("sensitive value must not be in plain step params")
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
	created, _ := svc.Create(ctx, baseFlowInput(a.ID, b.ID), "admin")
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

func TestToolDeleteBlockedByFlow(t *testing.T) {
	svc, tools, _ := newFlowFixture(t)
	ctx := context.Background()
	a := createTool(t, tools, "t1", []tool.Param{
		{Name: "ENV", Type: "string", Default: "prod"},
		{Name: "TOKEN", Type: "string", Sensitive: true},
	})
	b := createTool(t, tools, "t2", nil)
	if _, err := svc.Create(ctx, baseFlowInput(a.ID, b.ID), "admin"); err != nil {
		t.Fatal(err)
	}
	if err := tools.Delete(ctx, a.ID); !errors.Is(err, tool.ErrHasHistory) {
		t.Fatalf("err = %v, want ErrHasHistory", err)
	}
}
