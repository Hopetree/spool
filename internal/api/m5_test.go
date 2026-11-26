package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAdhocExecutionApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	operator := ensureOperator(t, e)

	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "adhoc", "command": "echo hello-adhoc", "timeout_sec": 10,
		"host_ids": []string{host["id"].(string)},
	}, operator)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var ex map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &ex)
	execID := ex["id"].(string)
	detail := waitExecution(t, e, operator, execID)
	if detail["status"] != "success" || detail["kind"] != "adhoc" || detail["command"] != "echo hello-adhoc" {
		t.Fatalf("detail = %+v", detail)
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

func TestUsersApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)

	w := e.do(t, http.MethodGet, "/api/v1/users", nil, admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"username":"admin"`) {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}

	w = e.do(t, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "alice", "display_name": "Alice", "role": "operator", "password": "alice1234",
	}, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var alice map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &alice)
	aliceID := alice["id"].(string)

	if w := e.do(t, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "alice", "display_name": "Alice", "role": "viewer", "password": "alice1234",
	}, admin); w.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d", w.Code)
	}

	w = e.do(t, http.MethodPut, "/api/v1/users/"+aliceID, map[string]any{
		"display_name": "Alice V", "role": "viewer", "enabled": true,
	}, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	if w := e.do(t, http.MethodPost, "/api/v1/users/"+aliceID+"/reset-password", map[string]any{
		"password": "newpass123",
	}, admin); w.Code != http.StatusNoContent {
		t.Fatalf("reset status = %d", w.Code)
	}
	if w := e.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "newpass123",
	}); w.Code != http.StatusOK {
		t.Fatalf("alice login after reset status = %d", w.Code)
	}

	// Self deletion and last-admin guards.
	me := e.do(t, http.MethodGet, "/api/v1/auth/me", nil, admin)
	var meBody map[string]any
	_ = json.Unmarshal(me.Body.Bytes(), &meBody)
	if w := e.do(t, http.MethodDelete, "/api/v1/users/"+meBody["id"].(string), nil, admin); w.Code != http.StatusConflict {
		t.Fatalf("self delete status = %d", w.Code)
	}
	if w := e.do(t, http.MethodPut, "/api/v1/users/"+meBody["id"].(string), map[string]any{
		"display_name": "Administrator", "role": "admin", "enabled": false,
	}, admin); w.Code != http.StatusConflict {
		t.Fatalf("last admin status = %d", w.Code)
	}

	operator := ensureOperator(t, e)
	if w := e.do(t, http.MethodGet, "/api/v1/users", nil, operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator list status = %d", w.Code)
	}
	if w := e.do(t, http.MethodDelete, "/api/v1/users/"+aliceID, nil, admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w.Code)
	}
	if e.auditCount(t, "user.create") != 1 || e.auditCount(t, "user.reset_password") != 1 || e.auditCount(t, "user.delete") != 1 {
		t.Fatal("user audit entries missing")
	}
}

func TestAuditApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	if w := e.do(t, http.MethodPost, "/api/v1/tools", map[string]any{
		"name": "audited", "type": "shell", "content": "true",
	}, admin); w.Code != http.StatusCreated {
		t.Fatalf("create tool status = %d", w.Code)
	}
	w := e.do(t, http.MethodGet, "/api/v1/audit?action=tool.create", nil, admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "audited") {
		t.Fatalf("audit status=%d body=%s", w.Code, w.Body.String())
	}
	operator := ensureOperator(t, e)
	if w := e.do(t, http.MethodGet, "/api/v1/audit", nil, operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator audit status = %d", w.Code)
	}
}
