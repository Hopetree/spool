package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func flowBody(name, toolA, toolB string) map[string]any {
	return map[string]any{
		"name": name, "description": "flow",
		"params": []map[string]any{
			{"name": "ENV", "type": "string", "default": "prod"},
			{"name": "SECRET", "type": "string", "sensitive": true},
		},
		"failure_policy": "abort", "max_duration_sec": 600,
		"stages": []map[string]any{
			{"name": "执行", "steps": []map[string]any{{
				"name": "step1", "tool_id": toolA, "timeout_sec": 10,
				"mapping": map[string]any{
					"ENV":   map[string]string{"source": "flow", "value": "ENV"},
					"TOKEN": map[string]string{"source": "flow", "value": "SECRET"},
				},
			}}},
			{"name": "清理", "always_run": true, "steps": []map[string]any{
				{"name": "cleanup", "tool_id": toolB},
			}},
		},
	}
}

func createFlowTools(t *testing.T, e *testEnv, admin *http.Cookie) (string, string) {
	t.Helper()
	a := createNamedTool(t, e, admin, "flow-tool", []map[string]any{
		{"name": "ENV", "type": "string", "default": "prod"},
		{"name": "TOKEN", "type": "string", "sensitive": true},
	})
	b := createNamedTool(t, e, admin, "cleanup-tool", nil)
	return a["id"].(string), b["id"].(string)
}

func createTestFlow(t *testing.T, e *testEnv, admin *http.Cookie, name string) map[string]any {
	t.Helper()
	toolA, toolB := createFlowTools(t, e, admin)
	w := e.do(t, http.MethodPost, "/api/v1/flows", flowBody(name, toolA, toolB), admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create flow: status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestFlowCRUDApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	toolA, toolB := createFlowTools(t, e, admin)
	w := e.do(t, http.MethodPost, "/api/v1/flows", flowBody("deploy", toolA, toolB), admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create flow: status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)
	if created["stage_count"] != float64(2) || created["step_count"] != float64(2) {
		t.Fatalf("created = %+v", created)
	}

	w = e.do(t, http.MethodGet, "/api/v1/flows", nil, admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "deploy") {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}

	w = e.do(t, http.MethodPut, "/api/v1/flows/"+id, flowBody("deploy-v2", toolA, toolB), admin)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	if w := e.do(t, http.MethodPost, "/api/v1/flows/"+id+"/copy", nil, admin); w.Code != http.StatusCreated {
		t.Fatalf("copy status = %d, body = %s", w.Code, w.Body.String())
	}

	operator := ensureOperator(t, e)
	if w := e.do(t, http.MethodPost, "/api/v1/flows", flowBody("x", "a", "b"), operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator create status = %d", w.Code)
	}
	if e.auditCount(t, "flow.create") != 1 || e.auditCount(t, "flow.update") != 1 || e.auditCount(t, "flow.copy") != 1 {
		t.Fatal("flow audit entries missing")
	}
}

func TestExecuteFlowAndRerun(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	flow := createTestFlow(t, e, admin, "deploy")
	operator := ensureOperator(t, e)

	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "flow", "ref_id": flow["id"], "params": map[string]string{"SECRET": "s3cr3t"},
		"host_ids": []string{host["id"].(string)},
	}, operator)
	if w.Code != http.StatusCreated {
		t.Fatalf("execute flow status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	execID := created["id"].(string)

	detail := waitExecution(t, e, operator, execID)
	if detail["status"] != "success" || detail["kind"] != "flow" {
		t.Fatalf("detail = %+v", detail)
	}
	steps := detail["step_runs"].([]any)
	if len(steps) != 2 {
		t.Fatalf("steps = %d", len(steps))
	}
	for _, raw := range steps {
		if raw.(map[string]any)["status"] != "success" {
			t.Fatalf("step = %+v", raw)
		}
	}
	params := detail["params"].(map[string]any)
	if params["ENV"] != "prod" {
		t.Fatalf("params = %v", params)
	}
	if _, ok := params["SECRET"]; ok {
		t.Fatalf("sensitive flow param leaked: %v", params)
	}

	w = e.do(t, http.MethodGet, "/api/v1/executions/"+execID+"/events", nil, operator)
	if w.Code != http.StatusOK || strings.Count(w.Body.String(), "hello") < 2 {
		t.Fatalf("events status=%d body=%s", w.Code, w.Body.String())
	}

	w = e.do(t, http.MethodPost, "/api/v1/executions/"+execID+"/rerun", nil, operator)
	if w.Code != http.StatusCreated {
		t.Fatalf("rerun status = %d, body = %s", w.Code, w.Body.String())
	}
	var rerun map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rerun)
	if again := waitExecution(t, e, operator, rerun["id"].(string)); again["status"] != "success" {
		t.Fatalf("rerun detail = %+v", again)
	}
	if e.auditCount(t, "execution.start") != 1 || e.auditCount(t, "execution.rerun") != 1 {
		t.Fatal("execution audit entries missing")
	}
}

func TestFlowValidationApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/flows", flowBody("bad", "missing", "missing"), admin)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestFlowDisabledApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	flow := createTestFlow(t, e, admin, "deploy")
	id := flow["id"].(string)
	if w := e.do(t, http.MethodPost, "/api/v1/flows/"+id+"/enabled", map[string]any{"enabled": false}, admin); w.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d", w.Code)
	}
	operator := ensureOperator(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "flow", "ref_id": id, "params": map[string]string{"SECRET": "x"},
		"host_ids": []string{host["id"].(string)},
	}, operator)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
