package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func createCategorizedTool(t *testing.T, e *testEnv, cookie *http.Cookie, name, category string, params []map[string]any) map[string]any {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/v1/tools", map[string]any{
		"name": name, "category": category, "type": "shell", "interpreter": "/bin/bash",
		"content": "echo " + name, "params": params, "timeout_sec": 30,
		"host_param": map[string]any{"label": "执行主机", "default_host_ids": []string{"h1"}},
	}, cookie)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tool %s: status = %d, body = %s", name, w.Code, w.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body
}

func TestToolCategoryApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	createCategorizedTool(t, e, admin, "deploy", "运维/部署", nil)
	createCategorizedTool(t, e, admin, "monitor", "运维/监控", nil)
	createCategorizedTool(t, e, admin, "misc", "", nil)

	w := e.do(t, http.MethodGet, "/api/v1/tool-categories", nil, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("categories status = %d, body = %s", w.Code, w.Body.String())
	}
	var cats struct {
		Items []struct {
			Path  string `json:"path"`
			Count int    `json:"count"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &cats)
	counts := map[string]int{}
	for _, c := range cats.Items {
		counts[c.Path] = c.Count
	}
	if counts["运维/部署"] != 1 || counts["运维/监控"] != 1 || counts[""] != 1 {
		t.Fatalf("categories = %+v", cats.Items)
	}

	w = e.do(t, http.MethodGet, "/api/v1/tools?category=%E8%BF%90%E7%BB%B4", nil, admin)
	var page struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if w.Code != http.StatusOK || page.Total != 2 {
		t.Fatalf("prefix filter total=%d status=%d body=%s", page.Total, w.Code, w.Body.String())
	}
	w = e.do(t, http.MethodGet, "/api/v1/tools?uncategorized=true", nil, admin)
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if page.Total != 1 {
		t.Fatalf("uncategorized total=%d", page.Total)
	}
}

func TestDeleteToolVersionApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	tool := createCategorizedTool(t, e, admin, "versioned", "", nil)
	id := tool["id"].(string)
	for i := 0; i < 2; i++ {
		w := e.do(t, http.MethodPut, "/api/v1/tools/"+id, map[string]any{
			"name": "versioned", "type": "shell", "interpreter": "/bin/bash",
			"content": "echo v" + string(rune('2'+i)), "params": []any{}, "timeout_sec": 30,
		}, admin)
		if w.Code != http.StatusOK {
			t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
		}
	}
	if w := e.do(t, http.MethodDelete, "/api/v1/tools/"+id+"/versions/1", nil, admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete v1 status = %d, body = %s", w.Code, w.Body.String())
	}
	w := e.do(t, http.MethodDelete, "/api/v1/tools/"+id+"/versions/3", nil, admin)
	if w.Code != http.StatusConflict {
		t.Fatalf("delete current status = %d, body = %s", w.Code, w.Body.String())
	}
	if e.auditCount(t, "tool.version_delete") != 1 {
		t.Fatal("version delete audit missing")
	}
}

func TestToolReferencesApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	toolA := createCategorizedTool(t, e, admin, "flow-tool", "运维/部署", []map[string]any{
		{"name": "ENV", "type": "string", "default": "prod"},
		{"name": "TOKEN", "type": "string", "sensitive": true},
	})
	toolB := createCategorizedTool(t, e, admin, "cleanup-tool", "", nil)
	toolAID := toolA["id"].(string)
	toolBID := toolB["id"].(string)

	w := e.do(t, http.MethodPost, "/api/v1/flows", flowBody("ref-flow", toolAID, toolBID), admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create flow status = %d, body = %s", w.Code, w.Body.String())
	}
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	if w := e.do(t, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "ref-schedule", "target_kind": "tool", "target_ref_id": toolAID,
		"params": map[string]string{"TOKEN": "x"}, "host_ids": []string{host["id"].(string)},
		"cron_expr": "0 2 * * *", "enabled": true,
	}, admin); w.Code != http.StatusCreated {
		t.Fatalf("create schedule status = %d, body = %s", w.Code, w.Body.String())
	}

	w = e.do(t, http.MethodGet, "/api/v1/tools/"+toolAID+"/references", nil, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("references status = %d", w.Code)
	}
	var refs struct {
		Flows []struct {
			FlowName string `json:"flow_name"`
			StepName string `json:"step_name"`
		} `json:"flows"`
		Schedules []struct {
			Name string `json:"name"`
		} `json:"schedules"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &refs)
	if len(refs.Flows) != 1 || refs.Flows[0].FlowName != "ref-flow" ||
		len(refs.Schedules) != 1 || refs.Schedules[0].Name != "ref-schedule" {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestExecutionToolFilter(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	tool := createCategorizedTool(t, e, admin, "filtered", "", nil)
	toolID := tool["id"].(string)
	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "tool", "ref_id": toolID, "host_ids": []string{host["id"].(string)},
	}, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("execute status = %d", w.Code)
	}
	var ex map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &ex)
	waitExecution(t, e, admin, ex["id"].(string))

	w = e.do(t, http.MethodGet, "/api/v1/executions?tool_id="+toolID, nil, admin)
	var page struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if page.Total != 1 {
		t.Fatalf("tool filter total = %d", page.Total)
	}
}
