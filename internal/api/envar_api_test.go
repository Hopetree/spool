package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestEnvarsApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	operator := ensureOperator(t, e)

	w := e.do(t, http.MethodGet, "/api/v1/envars", nil, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}

	w = e.do(t, http.MethodGet, "/api/v1/envars", nil, operator)
	if w.Code != http.StatusForbidden {
		t.Fatalf("operator list status = %d", w.Code)
	}

	body := map[string]any{
		"name": "APP_URL", "value": "http://demo", "description": "demo", "sensitive": false,
	}
	w = e.do(t, http.MethodPost, "/api/v1/envars", body, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)
	if created["name"] != "APP_URL" || created["sensitive"] != false {
		t.Fatalf("created = %+v", created)
	}

	w = e.do(t, http.MethodPost, "/api/v1/envars", body, admin)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("duplicate status = %d", w.Code)
	}

	w = e.do(t, http.MethodPut, "/api/v1/envars/"+id, map[string]any{
		"name": "APP_URL", "value": "", "description": "updated", "sensitive": true,
	}, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}
	var updated map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["sensitive"] != true || updated["description"] != "updated" {
		t.Fatalf("updated = %+v", updated)
	}

	w = e.do(t, http.MethodGet, "/api/v1/envars", nil, admin)
	var list struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("items = %+v", list.Items)
	}
	for key := range list.Items[0] {
		if key == "value" || key == "value_enc" {
			t.Fatalf("response leaked value field %q", key)
		}
	}

	if w := e.do(t, http.MethodDelete, "/api/v1/envars/"+id, nil, admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w.Code)
	}
}
