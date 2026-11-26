package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/auth"
	"github.com/Hopetree/spool/internal/config"
	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/exec"
	"github.com/Hopetree/spool/internal/flow"
	"github.com/Hopetree/spool/internal/host"
	"github.com/Hopetree/spool/internal/notify"
	"github.com/Hopetree/spool/internal/schedule"
	"github.com/Hopetree/spool/internal/sshx"
	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/tool"
)

type stubProber struct {
	res  sshx.ProbeResult
	err  error
	last sshx.Target
}

func (s *stubProber) Probe(ctx context.Context, t sshx.Target) (sshx.ProbeResult, error) {
	s.last = t
	return s.res, s.err
}

type testEnv struct {
	router   *gin.Engine
	store    *store.Store
	auth     *auth.Service
	prober   *stubProber
	runner   *stubRunner
	engine   *exec.Engine
	password string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	authSvc := auth.NewService(st.DB, time.Hour)
	pw, _, err := authSvc.BootstrapAdmin(context.Background())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	cipher, err := credential.NewCipher(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	credSvc := credential.NewService(st.DB, cipher)
	prober := &stubProber{res: sshx.ProbeResult{
		User: "root", OS: "Linux", Fingerprint: "SHA256:new", KeyType: "ssh-ed25519", SFTP: true,
	}}
	toolSvc := tool.NewService(st.DB)
	flowSvc := flow.NewService(st.DB, toolSvc)
	runner := &stubRunner{out: "hello\n"}
	engine := exec.New(st.DB, credSvc, &stubFactory{r: runner}, exec.Config{
		LogsDir:               filepath.Join(cfg.DataDir, "logs"),
		MaxConcurrentSteps:    2,
		MaxConcurrentHostRuns: 2,
	})
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("engine start: %v", err)
	}
	t.Cleanup(engine.Stop)
	notifySvc := notify.NewService(st.DB, credSvc, "")
	scheduleSvc := schedule.NewService(st.DB, toolSvc, flowSvc, credSvc, engine, notifySvc, audit.NewService(st.DB), time.UTC)
	if err := scheduleSvc.Start(context.Background()); err != nil {
		t.Fatalf("schedule start: %v", err)
	}
	t.Cleanup(scheduleSvc.Stop)
	env := &testEnv{
		router: NewRouter(Deps{
			Cfg:         &cfg,
			Store:       st,
			Auth:        authSvc,
			Audit:       audit.NewService(st.DB),
			Credentials: credSvc,
			Hosts:       host.NewService(st.DB, credSvc, prober, "tofu"),
			Tools:       toolSvc,
			Flows:       flowSvc,
			Engine:      engine,
			Schedules:   scheduleSvc,
			Notify:      notifySvc,
			Version:     "test",
		}),
		store:    st,
		auth:     authSvc,
		prober:   prober,
		runner:   runner,
		engine:   engine,
		password: pw,
	}
	return env
}

func (e *testEnv) do(t *testing.T, method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func (e *testEnv) login(t *testing.T, password string) (*httptest.ResponseRecorder, *http.Cookie) {
	t.Helper()
	w := e.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "admin", "password": password})
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName {
			return w, c
		}
	}
	return w, nil
}

func (e *testEnv) auditCount(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := e.store.DB.Get(&n, `SELECT count(*) FROM audit_logs WHERE action = ?`, action); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLoginSetsCookieAndAudits(t *testing.T) {
	e := newTestEnv(t)
	w, cookie := e.login(t, e.password)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if cookie == nil || !cookie.HttpOnly {
		t.Fatalf("cookie = %+v", cookie)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["username"] != "admin" {
		t.Fatalf("body = %v", body)
	}
	if e.auditCount(t, "login") != 1 {
		t.Fatal("login audit missing")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	e := newTestEnv(t)
	w, _ := e.login(t, "wrong-pass1")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
	var body apiError
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "invalid_credentials" {
		t.Fatalf("code = %q", body.Code)
	}
	if e.auditCount(t, "login_failed") != 1 {
		t.Fatal("login_failed audit missing")
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newTestEnv(t)
	for i := 0; i < 5; i++ {
		if w, _ := e.login(t, "wrong-pass1"); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", i, w.Code)
		}
	}
	w, _ := e.login(t, e.password)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	w := e.do(t, http.MethodGet, "/api/v1/auth/me", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestMeReturnsUser(t *testing.T) {
	e := newTestEnv(t)
	_, cookie := e.login(t, e.password)
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	w := e.do(t, http.MethodGet, "/api/v1/auth/me", nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["username"] != "admin" || body["must_change_password"] != true {
		t.Fatalf("body = %v", body)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	e := newTestEnv(t)
	_, cookie := e.login(t, e.password)
	if w := e.do(t, http.MethodPost, "/api/v1/auth/logout", nil, cookie); w.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", w.Code)
	}
	if w := e.do(t, http.MethodGet, "/api/v1/auth/me", nil, cookie); w.Code != http.StatusUnauthorized {
		t.Fatalf("me status = %d", w.Code)
	}
}

func TestForcePasswordChangeMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ctxUserKey, &auth.User{ID: "u1", MustChangePassword: true})
		c.Next()
	})
	r.Use(ForcePasswordChange())
	r.GET("/blocked", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/api/v1/auth/me", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/blocked", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("blocked status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("allowlisted status = %d", w.Code)
	}
}

func TestOriginCheckRejectsCrossOrigin(t *testing.T) {
	e := newTestEnv(t)
	b, _ := json.Marshal(map[string]string{"username": "admin", "password": e.password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestChangePassword(t *testing.T) {
	e := newTestEnv(t)
	_, cookie := e.login(t, e.password)
	w := e.do(t, http.MethodPut, "/api/v1/auth/password", map[string]string{
		"old_password": "wrong-old-1",
		"new_password": "newpass123",
	}, cookie)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("wrong old status = %d", w.Code)
	}
	w = e.do(t, http.MethodPut, "/api/v1/auth/password", map[string]string{
		"old_password": e.password,
		"new_password": "newpass123",
	}, cookie)
	if w.Code != http.StatusNoContent {
		t.Fatalf("change status = %d, body = %s", w.Code, w.Body.String())
	}
	if w, _ := e.login(t, "newpass123"); w.Code != http.StatusOK {
		t.Fatalf("new password login status = %d", w.Code)
	}
	if w, _ := e.login(t, e.password); w.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d", w.Code)
	}
}

func TestVersionIsPublic(t *testing.T) {
	e := newTestEnv(t)
	w := e.do(t, http.MethodGet, "/api/v1/version", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["version"] != "test" {
		t.Fatalf("body = %v", body)
	}
}
