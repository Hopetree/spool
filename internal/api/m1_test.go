package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Hopetree/spool/internal/store"
)

func loginAs(t *testing.T, e *testEnv, username, password string) *http.Cookie {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": username, "password": password,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login %s: status = %d, body = %s", username, w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func adminCookie(t *testing.T, e *testEnv) *http.Cookie {
	t.Helper()
	cookie := loginAs(t, e, "admin", e.password)
	w := e.do(t, http.MethodPut, "/api/v1/auth/password", map[string]string{
		"old_password": e.password, "new_password": "adminpass123",
	}, cookie)
	if w.Code != http.StatusNoContent {
		t.Fatalf("change admin password: status = %d, body = %s", w.Code, w.Body.String())
	}
	e.password = "adminpass123"
	return cookie
}

func createTestCredential(t *testing.T, e *testEnv, cookie *http.Cookie) map[string]any {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/v1/credentials", map[string]any{
		"name": "prod-root", "auth_type": "password", "username": "root",
		"secret": "topsecret123", "description": "prod hosts",
	}, cookie)
	if w.Code != http.StatusCreated {
		t.Fatalf("create credential: status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "topsecret123") || strings.Contains(w.Body.String(), "secret_enc") {
		t.Fatalf("response leaks secret: %s", w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestCredentialCRUDAsAdmin(t *testing.T) {
	e := newTestEnv(t)
	cookie := adminCookie(t, e)

	created := createTestCredential(t, e, cookie)
	id, _ := created["id"].(string)
	if id == "" || created["has_passphrase"] != false {
		t.Fatalf("unexpected created = %v", created)
	}

	w := e.do(t, http.MethodGet, "/api/v1/credentials", nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "topsecret123") {
		t.Fatal("list leaks secret")
	}

	w = e.do(t, http.MethodPut, "/api/v1/credentials/"+id, map[string]any{
		"name": "prod-root-renamed", "auth_type": "password", "username": "root", "description": "updated",
	}, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}
	var updated map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["name"] != "prod-root-renamed" {
		t.Fatalf("updated = %v", updated)
	}

	if w := e.do(t, http.MethodDelete, "/api/v1/credentials/"+id, nil, cookie); w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", w.Code)
	}
	if e.auditCount(t, "credential.create") != 1 || e.auditCount(t, "credential.update") != 1 || e.auditCount(t, "credential.delete") != 1 {
		t.Fatal("credential audit entries missing")
	}
}

func TestCredentialRequiresAdmin(t *testing.T) {
	e := newTestEnv(t)
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
	cookie := loginAs(t, e, "operator1", "oper1234")
	if w := e.do(t, http.MethodGet, "/api/v1/credentials", nil, cookie); w.Code != http.StatusForbidden {
		t.Fatalf("operator list status = %d, want 403", w.Code)
	}
	if w := e.do(t, http.MethodPost, "/api/v1/credentials", map[string]any{
		"name": "x", "auth_type": "password", "username": "root", "secret": "y",
	}, cookie); w.Code != http.StatusForbidden {
		t.Fatalf("operator create status = %d, want 403", w.Code)
	}
}

func TestCredentialDeleteInUse(t *testing.T) {
	e := newTestEnv(t)
	cookie := adminCookie(t, e)
	created := createTestCredential(t, e, cookie)
	id := created["id"].(string)
	now := time.Now().UnixMilli()
	_, err := e.store.DB.Exec(`INSERT INTO hosts (id, name, address, port, credential_id, created_at, updated_at)
		VALUES (?, 'web-1', '10.0.0.1', 22, ?, ?, ?)`, store.NewULID(), id, now, now)
	if err != nil {
		t.Fatal(err)
	}
	w := e.do(t, http.MethodDelete, "/api/v1/credentials/"+id, nil, cookie)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "credential_in_use" {
		t.Fatalf("code = %v", body["code"])
	}
}

func TestCredentialValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := adminCookie(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/credentials", map[string]any{
		"name": "bad", "auth_type": "password", "username": "root",
	}, cookie)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
