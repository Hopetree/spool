package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Hopetree/spool/internal/sshx"
	"github.com/Hopetree/spool/internal/store"
)

func ensureOperator(t *testing.T, e *testEnv) *http.Cookie {
	t.Helper()
	hash, err := e.auth.HashPassword("oper1234")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	_, err = e.store.DB.Exec(`INSERT INTO users (id, username, display_name, password_hash, role, enabled, must_change_password, created_at, updated_at)
		VALUES (?, 'operator1', 'Operator', ?, 'operator', 1, 0, ?, ?)`, store.NewULID(), hash, now, now)
	if err != nil {
		t.Fatal(err)
	}
	return loginAs(t, e, "operator1", "oper1234")
}

func createTestHost(t *testing.T, e *testEnv, cookie *http.Cookie, credentialID string) map[string]any {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/v1/hosts", map[string]any{
		"name": "web-1", "address": "10.0.0.1", "port": 22,
		"credential_id": credentialID, "enabled": true, "tags": []string{"prod", "web"},
		"note": "primary",
	}, cookie)
	if w.Code != http.StatusCreated {
		t.Fatalf("create host: status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestHostCRUDApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	credID := cred["id"].(string)

	hostBody := createTestHost(t, e, admin, credID)
	hostID := hostBody["id"].(string)
	if hostBody["credential_name"] != "prod-root" {
		t.Fatalf("admin should see credential name: %v", hostBody["credential_name"])
	}

	operator := ensureOperator(t, e)
	w := e.do(t, http.MethodGet, "/api/v1/hosts?tag=prod", nil, operator)
	if w.Code != http.StatusOK {
		t.Fatalf("operator list status = %d", w.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page = %+v", page)
	}
	if _, ok := page.Items[0]["credential_id"]; ok {
		t.Fatalf("operator should not see credential_id: %v", page.Items[0])
	}
	if _, ok := page.Items[0]["credential_name"]; ok {
		t.Fatalf("operator should not see credential_name")
	}

	w = e.do(t, http.MethodPut, "/api/v1/hosts/"+hostID, map[string]any{
		"name": "web-1-renamed", "address": "10.0.0.2", "port": 2222,
		"credential_id": credID, "enabled": true, "tags": []string{"prod"},
	}, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}
	var updated map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["address"] != "10.0.0.2" || updated["port"] != float64(2222) {
		t.Fatalf("updated = %+v", updated)
	}

	if w := e.do(t, http.MethodPost, "/api/v1/hosts", map[string]any{
		"name": "x", "address": "1.1.1.1", "credential_id": credID,
	}, operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator create status = %d", w.Code)
	}

	if w := e.do(t, http.MethodDelete, "/api/v1/hosts/"+hostID, nil, admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w.Code)
	}
	if e.auditCount(t, "host.create") != 1 || e.auditCount(t, "host.update") != 1 || e.auditCount(t, "host.delete") != 1 {
		t.Fatal("host audit entries missing")
	}
}

func TestHostTestApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	hostBody := createTestHost(t, e, admin, cred["id"].(string))
	hostID := hostBody["id"].(string)

	operator := ensureOperator(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/hosts/"+hostID+"/test", nil, operator)
	if w.Code != http.StatusOK {
		t.Fatalf("test status = %d, body = %s", w.Code, w.Body.String())
	}
	var res map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["ok"] != true || res["fingerprint_recorded"] != true || res["fingerprint"] != "SHA256:new" {
		t.Fatalf("result = %+v", res)
	}
	var fp string
	if err := e.store.DB.Get(&fp, `SELECT fingerprint FROM hosts WHERE id = ?`, hostID); err != nil {
		t.Fatal(err)
	}
	if fp != "SHA256:new" {
		t.Fatalf("stored fingerprint = %q", fp)
	}
	if e.auditCount(t, "host.test") != 1 {
		t.Fatal("host.test audit missing")
	}
}

func TestHostTestHostKeyMismatch(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	hostBody := createTestHost(t, e, admin, cred["id"].(string))
	hostID := hostBody["id"].(string)

	e.prober.err = &sshx.HostKeyError{Expected: "SHA256:old", Actual: "SHA256:bad", KeyType: "ssh-ed25519", Changed: true}
	w := e.do(t, http.MethodPost, "/api/v1/hosts/"+hostID+"/test", nil, admin)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "host_key_mismatch" || body.Details["actual"] != "SHA256:bad" {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestSetFingerprintApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	hostBody := createTestHost(t, e, admin, cred["id"].(string))
	hostID := hostBody["id"].(string)

	w := e.do(t, http.MethodPut, "/api/v1/hosts/"+hostID+"/fingerprint", map[string]any{
		"fingerprint": "SHA256:trusted", "key_type": "ssh-ed25519",
	}, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var fp string
	_ = e.store.DB.Get(&fp, `SELECT fingerprint FROM hosts WHERE id = ?`, hostID)
	if fp != "SHA256:trusted" {
		t.Fatalf("fingerprint = %q", fp)
	}
	operator := ensureOperator(t, e)
	if w := e.do(t, http.MethodPut, "/api/v1/hosts/"+hostID+"/fingerprint", map[string]any{
		"fingerprint": "SHA256:x", "key_type": "ssh-ed25519",
	}, operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator fingerprint status = %d", w.Code)
	}
	if e.auditCount(t, "host.fingerprint_update") != 1 {
		t.Fatal("fingerprint audit missing")
	}
}

func TestGroupApi(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/host-groups", map[string]any{
		"name": "prod", "description": "production",
	}, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create group status = %d, body = %s", w.Code, w.Body.String())
	}
	var g map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	id := g["id"].(string)

	w = e.do(t, http.MethodGet, "/api/v1/host-groups", nil, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("list groups status = %d", w.Code)
	}
	if e.do(t, http.MethodPut, "/api/v1/host-groups/"+id, map[string]any{
		"name": "prod-renamed", "description": "x",
	}, admin).Code != http.StatusOK {
		t.Fatal("update group failed")
	}
	if e.do(t, http.MethodDelete, "/api/v1/host-groups/"+id, nil, admin).Code != http.StatusNoContent {
		t.Fatal("delete group failed")
	}
	operator := ensureOperator(t, e)
	if w := e.do(t, http.MethodPost, "/api/v1/host-groups", map[string]any{"name": "x"}, operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator group create status = %d", w.Code)
	}
}
