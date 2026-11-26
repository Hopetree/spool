package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hopetree/spool/internal/exec"
	"github.com/Hopetree/spool/internal/store"
)

type stubRunner struct {
	mu      sync.Mutex
	out     string
	code    int
	err     error
	block   bool
	uploads int
	cmds    []string
}

func (r *stubRunner) Upload(ctx context.Context, path string, data []byte, mode fs.FileMode) error {
	r.mu.Lock()
	r.uploads++
	r.mu.Unlock()
	return nil
}

func (r *stubRunner) RunStream(ctx context.Context, cmd string, onOut, onErr func([]byte)) (int, error) {
	r.mu.Lock()
	r.cmds = append(r.cmds, cmd)
	out, code, runErr, block := r.out, r.code, r.err, r.block
	r.mu.Unlock()
	if !strings.Contains(cmd, "run.sh") {
		return 0, nil
	}
	if block {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	if out != "" && onOut != nil {
		onOut([]byte(out))
	}
	if runErr != nil {
		return -1, runErr
	}
	return code, nil
}

func (r *stubRunner) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return []byte("1"), nil
}

func (r *stubRunner) Close() error { return nil }

type stubFactory struct {
	r *stubRunner
}

func (f *stubFactory) Connect(ctx context.Context, h exec.HostRef) (exec.Runner, error) {
	return f.r, nil
}

func createTestTool(t *testing.T, e *testEnv, cookie *http.Cookie, params []map[string]any) map[string]any {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/v1/tools", map[string]any{
		"name": "demo", "description": "demo tool", "type": "shell",
		"interpreter": "/bin/bash", "content": "echo hello", "params": params, "timeout_sec": 30,
	}, cookie)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tool: status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func ensureUser(t *testing.T, e *testEnv, username, role string) *http.Cookie {
	t.Helper()
	hash, err := e.auth.HashPassword("oper1234")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	_, err = e.store.DB.Exec(`INSERT INTO users (id, username, display_name, password_hash, role, enabled, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, 0, ?, ?)`, store.NewULID(), username, username, hash, role, now, now)
	if err != nil {
		t.Fatal(err)
	}
	return loginAs(t, e, username, "oper1234")
}

func TestToolCRUDAndVersions(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	created := createTestTool(t, e, admin, []map[string]any{
		{"name": "ENV", "type": "select", "required": true, "options": []string{"prod", "dev"}},
	})
	id := created["id"].(string)
	if created["version_no"] != float64(1) || created["param_count"] != float64(1) {
		t.Fatalf("created = %+v", created)
	}

	w := e.do(t, http.MethodPut, "/api/v1/tools/"+id, map[string]any{
		"name": "demo", "description": "v2", "type": "shell", "interpreter": "/bin/bash",
		"content": "echo v2", "params": []map[string]any{{"name": "ENV", "type": "string"}},
		"timeout_sec": 30, "change_note": "second",
	}, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}
	var updated map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["version_no"] != float64(2) {
		t.Fatalf("version = %v", updated["version_no"])
	}

	w = e.do(t, http.MethodGet, "/api/v1/tools/"+id+"/versions", nil, admin)
	var versions struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &versions)
	if len(versions.Items) != 2 || versions.Items[0]["version_no"] != float64(2) {
		t.Fatalf("versions = %+v", versions.Items)
	}

	w = e.do(t, http.MethodPost, "/api/v1/tools/"+id+"/rollback", map[string]any{"version_no": 1}, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("rollback status = %d, body = %s", w.Code, w.Body.String())
	}
	var rolled map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rolled)
	if rolled["version_no"] != float64(3) {
		t.Fatalf("rolled version = %v", rolled["version_no"])
	}

	if w := e.do(t, http.MethodPost, "/api/v1/tools/"+id+"/copy", nil, admin); w.Code != http.StatusCreated {
		t.Fatalf("copy status = %d, body = %s", w.Code, w.Body.String())
	}

	operator := ensureOperator(t, e)
	if w := e.do(t, http.MethodGet, "/api/v1/tools", nil, operator); w.Code != http.StatusOK {
		t.Fatalf("operator list status = %d", w.Code)
	}
	if w := e.do(t, http.MethodPost, "/api/v1/tools", map[string]any{
		"name": "x", "type": "shell", "content": "true",
	}, operator); w.Code != http.StatusForbidden {
		t.Fatalf("operator create status = %d", w.Code)
	}
	if e.auditCount(t, "tool.create") != 1 || e.auditCount(t, "tool.update") != 1 || e.auditCount(t, "tool.rollback") != 1 || e.auditCount(t, "tool.copy") != 1 {
		t.Fatal("tool audit entries missing")
	}
}

func waitExecution(t *testing.T, e *testEnv, cookie *http.Cookie, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w := e.do(t, http.MethodGet, "/api/v1/executions/"+id, nil, cookie)
		if w.Code != http.StatusOK {
			t.Fatalf("get execution status = %d, body = %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		switch body["status"] {
		case "success", "partial", "failed", "cancelled", "interrupted":
			return body
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("execution did not finish")
	return nil
}

func TestExecuteToolAndHistory(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	hostID := host["id"].(string)

	toolBody := createTestTool(t, e, admin, []map[string]any{
		{"name": "ENV", "type": "string", "default": "prod"},
		{"name": "TOKEN", "type": "string", "sensitive": true},
	})
	toolID := toolBody["id"].(string)

	operator := ensureOperator(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "tool", "ref_id": toolID, "params": map[string]string{"TOKEN": "s3cr3t"},
		"host_ids": []string{hostID},
	}, operator)
	if w.Code != http.StatusCreated {
		t.Fatalf("execute status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	execID := created["id"].(string)

	detail := waitExecution(t, e, operator, execID)
	if detail["status"] != "success" {
		t.Fatalf("detail = %+v", detail)
	}
	params := detail["params"].(map[string]any)
	if params["ENV"] != "prod" {
		t.Fatalf("params = %v", params)
	}
	if _, ok := params["TOKEN"]; ok {
		t.Fatalf("sensitive param leaked: %v", params)
	}
	hostRuns := detail["host_runs"].([]any)
	steps := detail["step_runs"].([]any)
	if len(hostRuns) != 1 || len(steps) != 1 {
		t.Fatalf("host_runs=%d steps=%d", len(hostRuns), len(steps))
	}
	step := steps[0].(map[string]any)
	if step["status"] != "success" {
		t.Fatalf("step = %+v", step)
	}

	w = e.do(t, http.MethodGet, "/api/v1/executions/"+execID+"/events", nil, operator)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hello") {
		t.Fatalf("events status=%d body=%s", w.Code, w.Body.String())
	}

	w = e.do(t, http.MethodGet, "/api/v1/executions/"+execID+"/logs/download", nil, operator)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hello") {
		t.Fatalf("download status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("missing attachment header")
	}

	if e.auditCount(t, "execution.start") != 1 {
		t.Fatal("execution.start audit missing")
	}
}

func TestExecuteMissingRequiredParam(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	toolBody := createTestTool(t, e, admin, []map[string]any{
		{"name": "ENV", "type": "string", "required": true},
	})
	operator := ensureOperator(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "tool", "ref_id": toolBody["id"], "params": map[string]string{},
		"host_ids": []string{host["id"].(string)},
	}, operator)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestViewerCannotExecute(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	toolBody := createTestTool(t, e, admin, nil)
	viewer := ensureUser(t, e, "viewer1", "viewer")
	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "tool", "ref_id": toolBody["id"], "host_ids": []string{host["id"].(string)},
	}, viewer)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
	if w := e.do(t, http.MethodGet, "/api/v1/executions", nil, viewer); w.Code != http.StatusOK {
		t.Fatalf("viewer list status = %d", w.Code)
	}
}

func TestExecuteDisabledTool(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	toolBody := createTestTool(t, e, admin, nil)
	toolID := toolBody["id"].(string)
	if w := e.do(t, http.MethodPost, "/api/v1/tools/"+toolID+"/enabled", map[string]any{"enabled": false}, admin); w.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d", w.Code)
	}
	operator := ensureOperator(t, e)
	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "tool", "ref_id": toolID, "host_ids": []string{host["id"].(string)},
	}, operator)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestExecutionSSEReplay(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	toolBody := createTestTool(t, e, admin, nil)
	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "tool", "ref_id": toolBody["id"], "host_ids": []string{host["id"].(string)},
	}, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("execute status = %d", w.Code)
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	execID := created["id"].(string)
	waitExecution(t, e, admin, execID)

	w = e.do(t, http.MethodGet, "/api/v1/executions/"+execID+"/stream", nil, admin)
	if w.Code != http.StatusOK {
		t.Fatalf("stream status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: log") || !strings.Contains(body, "event: end") {
		t.Fatalf("stream body = %s", body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestExecutionCancelAPI(t *testing.T) {
	e := newTestEnv(t)
	admin := adminCookie(t, e)
	cred := createTestCredential(t, e, admin)
	host := createTestHost(t, e, admin, cred["id"].(string))
	toolBody := createTestTool(t, e, admin, nil)
	e.runner.mu.Lock()
	e.runner.block = true
	e.runner.mu.Unlock()

	w := e.do(t, http.MethodPost, "/api/v1/executions", map[string]any{
		"kind": "tool", "ref_id": toolBody["id"], "host_ids": []string{host["id"].(string)},
	}, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("execute status = %d", w.Code)
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	execID := created["id"].(string)
	time.Sleep(100 * time.Millisecond)
	if w := e.do(t, http.MethodPost, "/api/v1/executions/"+execID+"/cancel", nil, admin); w.Code != http.StatusNoContent {
		t.Fatalf("cancel status = %d, body = %s", w.Code, w.Body.String())
	}
	detail := waitExecution(t, e, admin, execID)
	if detail["status"] != "cancelled" {
		t.Fatalf("status = %v", detail["status"])
	}
}
