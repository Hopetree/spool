package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestFlowParameterSourcesApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	toolA := createNamedTool(t, e, admin, "param-tool", []map[string]any{
		{"name": "ENV", "type": "string", "default": "prod"},
		{"name": "TOKEN", "type": "string", "sensitive": true},
	})
	toolID := toolA["id"].(string)
	body := map[string]any{
		"name": "param-sources", "category": "运维/参数", "params": nil,
		"max_duration_sec": 600,
		"nodes": []map[string]any{
			{"id": "start", "type": "start", "name": "开始", "x": 0, "y": 0},
			{
				"id": "step1", "type": "tool", "name": "准备", "tool_id": toolID, "x": 240, "y": 0,
				"mapping": map[string]any{
					"ENV":   map[string]string{"source": "input", "input_name": "ENV", "input_label": "环境"},
					"TOKEN": map[string]string{"source": "input", "input_name": "SECRET", "input_label": "访问令牌"},
				},
			},
			{
				"id": "step2", "type": "tool", "name": "执行", "tool_id": toolID, "x": 480, "y": 0,
				"mapping": map[string]any{
					"ENV":   map[string]string{"source": "node", "ref_node_id": "step1", "ref_param": "ENV"},
					"TOKEN": map[string]string{"source": "node", "ref_node_id": "step1", "ref_param": "TOKEN"},
				},
			},
		},
		"edges": []map[string]string{
			{"from": "start", "to": "step1", "condition": "always"},
			{"from": "step1", "to": "step2", "condition": "always"},
		},
	}
	w := e.do(t, http.MethodPost, "/api/v1/flows", body, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create flow: status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	params, _ := created["params"].([]any)
	if len(params) != 2 {
		t.Fatalf("params = %+v", created["params"])
	}
	labels := map[string]string{}
	defaults := map[string]string{}
	for _, raw := range params {
		p := raw.(map[string]any)
		labels[p["name"].(string)] = p["label"].(string)
		defaults[p["name"].(string)] = p["default"].(string)
	}
	if labels["ENV"] != "环境" || labels["SECRET"] != "访问令牌" {
		t.Fatalf("labels = %+v", labels)
	}
	if defaults["ENV"] != "" {
		t.Fatalf("execution input inherited tool default: %+v", defaults)
	}
	nodes, _ := created["nodes"].([]any)
	for _, raw := range nodes {
		node := raw.(map[string]any)
		if node["id"] != "step2" {
			continue
		}
		mapping := node["mapping"].(map[string]any)
		env := mapping["ENV"].(map[string]any)
		if env["source"] != "node" || env["ref_node_id"] != "step1" || env["ref_param"] != "ENV" {
			t.Fatalf("step2 mapping = %+v", mapping)
		}
	}
}
