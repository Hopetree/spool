# Spool M0 基础骨架实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付可运行的 Spool 骨架：单二进制启动、SQLite 迁移、本地账号登录/会话/改密、审计基础、React 前端外壳，以及构建与 CI。

**Architecture:** 模块化单体。Gin 提供 `/api/v1`，SQLite（modernc，无 CGO）经 sqlx + 内嵌 goose 迁移；认证用服务端会话表 + HttpOnly Cookie；前端 Vite/React 构建产物通过 `go:embed` 打入二进制并由 SPA fallback 提供。

**Tech Stack:** Go 1.22、Gin、go-playground/validator、sqlx、goose、modernc.org/sqlite、oklog/ulid、bcrypt、log/slog；React 18、TypeScript、Vite、Ant Design 5、TanStack Query、Zustand、React Router。

**Spec:** `docs/superpowers/specs/2026-11-26-spool-design.md`（本计划覆盖第 1、3、4、5、6、16、17、18、19、20、22 节中属于 M0 的部分；主机/凭据/工具/执行在后续里程碑计划中实现）

## Global Constraints

- Go module：`github.com/Hopetree/spool`；Go 1.22。
- 禁止 CGO：SQLite 使用 `modernc.org/sqlite`（驱动名 `sqlite`），构建 `CGO_ENABLED=0`。
- 主键统一 ULID（TEXT，26 字符）；时间统一 UTC 毫秒（INTEGER）。
- 认证只使用服务端会话表 + HttpOnly Cookie；不使用 JWT。
- 所有修改类请求校验 `Origin`；Cookie `SameSite=Lax`，`Secure` 由配置控制。
- 界面文案中文；代码标识符与注释英文。
- 密码 bcrypt cost 12；密码长度 ≥8 且同时包含字母与数字。
- 服务端日志不得输出密码、Cookie、会话 token。
- 每个任务结束必须 `go test ./...` 通过并单独提交；前端任务需 `npm run build` 通过。
- 不引入本计划之外的依赖；不实现 M1+ 的功能（主机、凭据、工具、执行）。

## 里程碑上下文

M0 只交付"能登录的空系统"。M1（主机/凭据）、M2（工具/执行）、M3（流程）、M4（定时任务/通知）、M5（临时命令/文件分发/用户与审计页面/打包）各自单独出计划。

## 文件结构

```
cmd/spool/main.go                      启动、装配、优雅停机
internal/config/config.go              配置加载/校验/默认值
internal/config/config_test.go
internal/lockfile/lockfile.go          单实例文件锁（flock）
internal/lockfile/lockfile_test.go
internal/store/store.go                SQLite 打开、连接池、迁移
internal/store/store_test.go
internal/store/migrations/0001_init.sql
internal/auth/auth.go                  密码、用户、会话、登录限流
internal/auth/auth_test.go
internal/audit/audit.go                审计写入
internal/audit/audit_test.go
internal/api/router.go                 路由与中间件装配
internal/api/response.go               统一错误/成功响应
internal/api/middleware.go             认证、角色、Origin、安全头、请求日志
internal/api/auth_handlers.go          登录/登出/me/改密
internal/api/api_test.go               httptest 集成测试
internal/web/embed.go                  前端产物嵌入与 SPA fallback
internal/web/dist/.gitkeep
web/                                   React 应用（Vite）
web/package.json / vite.config.ts / tsconfig.json / index.html
web/src/main.tsx / App.tsx / api/client.ts / stores/auth.ts
web/src/pages/Login.tsx / Home.tsx / Password.tsx
web/src/components/AppLayout.tsx / ProtectedRoute.tsx
web/src/api/client.test.ts             parseApiError 单测
Makefile
config.example.yaml
deploy/spool.service
.github/workflows/ci.yml
README.md
```

---

### Task 1: Go 模块与配置

**Files:**
- Create: `go.mod`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `type Config struct { Server ServerConfig; DataDir string; Timezone string; Logging LoggingConfig; Security SecurityConfig }`
  - `func Default() Config`
  - `func Load(path string) (*Config, error)` — 文件不存在时返回默认值；存在则 YAML 覆盖默认值；随后应用环境变量覆盖并校验。
  - `func (c *Config) Validate() error`
  - `func (c *Config) Location() (*time.Location, error)`

- [ ] **Step 1: 初始化模块**

```bash
cd /Users/leizhu/Documents/Develop/zcode/spool
go mod init github.com/Hopetree/spool
```

- [ ] **Step 2: 写失败测试**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenFileMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != ":8080" {
		t.Fatalf("listen = %q, want :8080", cfg.Server.Listen)
	}
	if cfg.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q", cfg.Timezone)
	}
	if cfg.Security.SessionTTLHours != 12 {
		t.Fatalf("session ttl = %d", cfg.Security.SessionTTLHours)
	}
}

func TestLoadYAMLOverridesDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("server:\n  listen: \":9090\"\ndata_dir: /tmp/spool-test\ntimezone: UTC\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != ":9090" || cfg.DataDir != "/tmp/spool-test" || cfg.Timezone != "UTC" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("default log level lost: %q", cfg.Logging.Level)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("SPOOL_LISTEN", ":7070")
	t.Setenv("SPOOL_LOG_LEVEL", "debug")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != ":7070" || cfg.Logging.Level != "debug" {
		t.Fatalf("env override not applied: %+v", cfg)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cases := []Config{
		func() Config { c := Default(); c.Timezone = "Mars/Base"; return c }(),
		func() Config { c := Default(); c.Security.HostKeyPolicy = "whatever"; return c }(),
		func() Config { c := Default(); c.Logging.Level = "verbose"; return c }(),
		func() Config { c := Default(); c.Security.SessionTTLHours = 0; return c }(),
		func() Config { c := Default(); c.DataDir = ""; return c }(),
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/config/ -v`
Expected: FAIL（包内无实现，编译失败或 undefined）

- [ ] **Step 4: 实现配置**

`internal/config/config.go` 要点：

```go
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Listen       string `yaml:"listen"`
	BaseURL      string `yaml:"base_url"`
	CookieSecure bool   `yaml:"cookie_secure"`
	TLS          struct {
		Cert string `yaml:"cert"`
		Key  string `yaml:"key"`
	} `yaml:"tls"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

type SecurityConfig struct {
	SessionTTLHours     int    `yaml:"session_ttl_hours"`
	LoginMaxFailures    int    `yaml:"login_max_failures"`
	LoginLockoutMinutes int    `yaml:"login_lockout_minutes"`
	HostKeyPolicy       string `yaml:"host_key_policy"`
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DataDir  string         `yaml:"data_dir"`
	Timezone string         `yaml:"timezone"`
	Logging  LoggingConfig  `yaml:"logging"`
	Security SecurityConfig `yaml:"security"`
}

func Default() Config {
	c := Config{DataDir: "./data", Timezone: "Asia/Shanghai"}
	c.Server.Listen = ":8080"
	c.Logging.Level = "info"
	c.Security.SessionTTLHours = 12
	c.Security.LoginMaxFailures = 5
	c.Security.LoginLockoutMinutes = 5
	c.Security.HostKeyPolicy = "tofu"
	return c
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, &cfg); err != nil {
				return nil, fmt.Errorf("parse config: %w", err)
			}
		case !os.IsNotExist(err):
			return nil, fmt.Errorf("read config: %w", err)
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("SPOOL_LISTEN"); v != "" { c.Server.Listen = v }
	if v := os.Getenv("SPOOL_BASE_URL"); v != "" { c.Server.BaseURL = v }
	if v := os.Getenv("SPOOL_DATA_DIR"); v != "" { c.DataDir = v }
	if v := os.Getenv("SPOOL_LOG_LEVEL"); v != "" { c.Logging.Level = v }
	if v := os.Getenv("SPOOL_TIMEZONE"); v != "" { c.Timezone = v }
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Server.Listen) == "" { return fmt.Errorf("server.listen is required") }
	if strings.TrimSpace(c.DataDir) == "" { return fmt.Errorf("data_dir is required") }
	if _, err := c.Location(); err != nil { return err }
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be debug|info|warn|error")
	}
	switch c.Security.HostKeyPolicy {
	case "tofu", "strict":
	default:
		return fmt.Errorf("security.host_key_policy must be tofu|strict")
	}
	if c.Security.SessionTTLHours <= 0 { return fmt.Errorf("security.session_ttl_hours must be > 0") }
	if c.Security.LoginMaxFailures <= 0 { return fmt.Errorf("security.login_max_failures must be > 0") }
	if c.Security.LoginLockoutMinutes <= 0 { return fmt.Errorf("security.login_lockout_minutes must be > 0") }
	return nil
}

func (c *Config) Location() (*time.Location, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil { return nil, fmt.Errorf("invalid timezone %q: %w", c.Timezone, err) }
	return loc, nil
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/config/ -v`
Expected: PASS（5 个测试）

- [ ] **Step 6: 提交**

```bash
git add go.mod internal/config
git commit -m "feat(config): add yaml config with defaults and env overrides"
```

---

### Task 2: SQLite Store 与内嵌迁移

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/migrations/0001_init.sql`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Produces:
  - `type Store struct { DB *sqlx.DB }`
  - `func Open(path string) (*Store, error)`
  - `func (s *Store) Migrate() error`
  - `func (s *Store) Close() error`
  - `func NewULID() string`（供全项目复用；基于 `oklog/ulid/v2` 的 `ulid.Make().String()`）

- [ ] **Step 1: 写失败测试**

```go
package store

import (
	"path/filepath"
	"testing"
)

func TestMigrateCreatesCoreTables(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil { t.Fatalf("Open: %v", err) }
	defer s.Close()
	if err := s.Migrate(); err != nil { t.Fatalf("Migrate: %v", err) }
	for _, table := range []string{"users", "sessions", "audit_logs", "settings", "goose_db_version"} {
		var n int
		if err := s.DB.Get(&n, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table); err != nil { t.Fatal(err) }
		if n != 1 { t.Fatalf("table %s missing", table) }
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil { t.Fatal(err) }
	defer s.Close()
	if err := s.Migrate(); err != nil { t.Fatal(err) }
	if err := s.Migrate(); err != nil { t.Fatalf("second Migrate: %v", err) }
}

func TestSessionCascadeOnUserDelete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil { t.Fatal(err) }
	defer s.Close()
	if err := s.Migrate(); err != nil { t.Fatal(err) }

	uid := NewULID()
	now := int64(1700000000000)
	_, err = s.DB.Exec(`INSERT INTO users (id, username, display_name, password_hash, role, enabled, must_change_password, created_at, updated_at)
		VALUES (?, 'u1', 'U1', 'x', 'admin', 1, 0, ?, ?)`, uid, now, now)
	if err != nil { t.Fatal(err) }
	sid := NewULID()
	_, err = s.DB.Exec(`INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, 'h', ?, ?)`, sid, uid, now+1000, now)
	if err != nil { t.Fatal(err) }
	if _, err := s.DB.Exec(`DELETE FROM users WHERE id=?`, uid); err != nil { t.Fatal(err) }
	var n int
	if err := s.DB.Get(&n, `SELECT count(*) FROM sessions WHERE id=?`, sid); err != nil { t.Fatal(err) }
	if n != 0 { t.Fatal("session not cascaded") }
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/store/ -v`
Expected: FAIL（undefined: Open）

- [ ] **Step 3: 写迁移 SQL**

`internal/store/migrations/0001_init.sql`：

```sql
-- +goose Up
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('admin','operator','viewer')),
  enabled INTEGER NOT NULL DEFAULT 1,
  must_change_password INTEGER NOT NULL DEFAULT 0,
  last_login_at INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE audit_logs (
  id TEXT PRIMARY KEY,
  ts INTEGER NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  username_snapshot TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL DEFAULT '',
  resource_id TEXT NOT NULL DEFAULT '',
  resource_name TEXT NOT NULL DEFAULT '',
  summary_json TEXT NOT NULL DEFAULT '{}',
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_audit_ts ON audit_logs(ts DESC);
CREATE INDEX idx_audit_action ON audit_logs(action);

CREATE TABLE settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE settings;
DROP TABLE audit_logs;
DROP TABLE sessions;
DROP TABLE users;
```

- [ ] **Step 4: 实现 Store**

`internal/store/store.go`：

```go
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct{ DB *sqlx.DB }

func NewULID() string { return ulid.Make().String() }

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil { return nil, fmt.Errorf("create db dir: %w", err) }
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sqlx.Open("sqlite", dsn)
	if err != nil { return nil, fmt.Errorf("open sqlite: %w", err) }
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	if err := db.Ping(); err != nil { db.Close(); return nil, fmt.Errorf("ping sqlite: %w", err) }
	return &Store{DB: db}, nil
}

func (s *Store) Migrate() error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil { return fmt.Errorf("goose dialect: %w", err) }
	if err := goose.Up(s.DB.DB, "migrations"); err != nil { return fmt.Errorf("migrate: %w", err) }
	return nil
}

func (s *Store) Close() error { return s.DB.Close() }
```

依赖：`go get github.com/jmoiron/sqlx github.com/pressly/goose/v3 modernc.org/sqlite github.com/oklog/ulid/v2`

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/store/ -v`
Expected: PASS（3 个测试）

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum internal/store
git commit -m "feat(store): sqlite store with embedded goose migrations"
```

---

### Task 3: 认证服务（密码、会话、引导管理员）

**Files:**
- Create: `internal/auth/auth.go`
- Test: `internal/auth/auth_test.go`

**Interfaces:**
- Produces:
  - `type User struct { ID, Username, DisplayName, PasswordHash, Role string; Enabled, MustChangePassword bool; LastLoginAt *int64; CreatedAt, UpdatedAt int64 }`
  - `type Service struct{ ... }`
  - `func NewService(db *sqlx.DB, ttl time.Duration) *Service`
  - `func (s *Service) BootstrapAdmin(ctx context.Context) (initialPassword string, created bool, err error)`
  - `func (s *Service) Authenticate(ctx context.Context, username, password string) (*User, error)`
  - `func (s *Service) CreateSession(ctx context.Context, userID, ip, userAgent string) (token string, err error)`
  - `func (s *Service) UserBySessionToken(ctx context.Context, token string) (*User, error)`
  - `func (s *Service) DeleteSession(ctx context.Context, token string) error`
  - `func (s *Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error`
  - `func ValidatePassword(pw string) error`
  - 哨兵错误：`ErrInvalidCredentials`、`ErrUserDisabled`、`ErrSessionInvalid`、`ErrWeakPassword`
  - 测试钩子：`func (s *Service) SetNowForTest(func() time.Time)`（仅测试使用，注释标明）

- [ ] **Step 1: 写失败测试**

覆盖：

```go
func TestHashAndVerifyPassword(t *testing.T)          // 同一密码两次 hash 不同；Verify 正确/错误密码
func TestValidatePassword(t *testing.T)               // 短密码、纯字母、纯数字、合法密码
func TestBootstrapAdminCreatesOnce(t *testing.T)      // 首次返回 created=true 且密码可登录；第二次 created=false
func TestAuthenticateRejectsDisabledUser(t *testing.T)
func TestSessionLifecycle(t *testing.T)               // 创建→按 token 取用户→删除后失效
func TestSessionExpiry(t *testing.T)                  // SetNowForTest 前进到过期后返回 ErrSessionInvalid
func TestChangePassword(t *testing.T)                 // 旧密码错误失败；成功后旧密码不能登录、新密码可登录
```

测试辅助：`newTestStore(t)` 打开临时 SQLite 并迁移。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/auth/ -v`
Expected: FAIL（undefined: NewService）

- [ ] **Step 3: 实现**

要点：

- `HashPassword` 用 `bcrypt.GenerateFromPassword([]byte(pw), 12)`。
- `ValidatePassword`：长度 ≥8，`strings.ContainsAny` 检查字母与数字。
- `BootstrapAdmin`：`SELECT count(*) FROM users`；为 0 时生成 16 位随机密码（crypto/rand，字符集 `abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789`），插入 `role=admin, must_change_password=1`，返回明文密码。
- 会话 token：`crypto/rand` 32 字节，`base64.RawURLEncoding`；数据库只存 `sha256(token)` 的 hex。
- `CreateSession` 插入 `expires_at = now + ttl`。
- `UserBySessionToken`：join users，条件 `s.expires_at > ?` 且 `u.enabled=1`；过期/不存在统一 `ErrSessionInvalid`。
- `Authenticate`：查用户；不存在或 bcrypt 不匹配 → `ErrInvalidCredentials`；禁用 → `ErrUserDisabled`；成功后更新 `last_login_at`。
- `ChangePassword`：校验旧密码 → `ValidatePassword` 新密码 → 更新 hash 与 `must_change_password=0`。
- 所有时间通过 `s.now()` 获取，便于测试。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/auth/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/auth
git commit -m "feat(auth): password, session and bootstrap admin"
```

---

### Task 4: 审计服务

**Files:**
- Create: `internal/audit/audit.go`
- Test: `internal/audit/audit_test.go`

**Interfaces:**
- Produces:
  - `type Entry struct { ActorID, ActorName, Action, ResourceType, ResourceID, ResourceName string; Summary map[string]any; IP, UserAgent string }`
  - `type Service struct{ ... }`
  - `func NewService(db *sqlx.DB) *Service`
  - `func (s *Service) Write(ctx context.Context, e Entry) error`
  - `func (s *Service) List(ctx context.Context, f Filter) ([]Record, int64, error)`
  - `type Filter struct { ActorID string; Action string; FromTS, ToTS int64; Page, PageSize int }`

- [ ] **Step 1: 写失败测试**

```go
func TestWriteAndList(t *testing.T) {
	// 写 2 条，按 action 过滤拿到 1 条；摘要 JSON 可还原
}
func TestWriteRedactsNilSummary(t *testing.T) {
	// Summary 为 nil 时写入 "{}"
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/audit/ -v` → FAIL

- [ ] **Step 3: 实现**

- `Write`：ULID 主键，`ts=time.Now().UnixMilli()`，`json.Marshal(Summary)`（nil → `{}`），插入 `audit_logs`。
- `List`：按 ts DESC 分页，返回总数；`Action` 精确匹配，`ActorID` 精确匹配，时间范围可选。
- 不提供 Update/Delete 方法。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/audit/ -v` → PASS

- [ ] **Step 5: 提交**

```bash
git add internal/audit
git commit -m "feat(audit): append-only audit log service"
```

---

### Task 5: HTTP 基础与认证接口

**Files:**
- Create: `internal/api/router.go`
- Create: `internal/api/response.go`
- Create: `internal/api/middleware.go`
- Create: `internal/api/auth_handlers.go`
- Test: `internal/api/api_test.go`

**Interfaces:**
- Consumes: `store.Store`、`auth.Service`、`audit.Service`、`config.Config`
- Produces:
  - `type Deps struct { Cfg *config.Config; Store *store.Store; Auth *auth.Service; Audit *audit.Service; Version string; StartedAt time.Time }`
  - `func NewRouter(d Deps) *gin.Engine`
  - Cookie 名常量 `SessionCookieName = "spool_session"`
  - 路由：`POST /api/v1/auth/login`、`POST /api/v1/auth/logout`、`GET /api/v1/auth/me`、`PUT /api/v1/auth/password`、`GET /api/v1/version`
  - 上下文键：`ctxUserKey`（`*auth.User`）
  - `func RequireAuth(a *auth.Service) gin.HandlerFunc`
  - `func RequireRole(role string) gin.HandlerFunc`
  - `func ForcePasswordChange() gin.HandlerFunc`
  - `func OriginCheck() gin.HandlerFunc`
  - `func SecurityHeaders() gin.HandlerFunc`
  - 统一错误：`{"code":"...","message":"...","details":...}`

- [ ] **Step 1: 写失败测试（httptest）**

```go
func TestLoginSetsCookieAndAudits(t *testing.T)     // 200；Set-Cookie 含 spool_session；DB 有会话；审计有 login 成功
func TestLoginWrongPassword(t *testing.T)           // 401 code=invalid_credentials；审计有 login_failed
func TestLoginRateLimit(t *testing.T)               // 连续 5 次失败后，第 6 次即使密码正确也返回 429
func TestMeRequiresAuth(t *testing.T)               // 无 Cookie → 401
func TestMeReturnsUser(t *testing.T)                // 登录后带 Cookie → 200 且 username 正确
func TestLogoutInvalidatesSession(t *testing.T)     // 登出后再 me → 401
func TestForcePasswordChangeMiddleware(t *testing.T)// 直接构造 gin 路由：must_change 用户访问非白名单 → 403
func TestOriginCheckRejectsCrossOrigin(t *testing.T)// POST 带外站 Origin → 403
func TestChangePassword(t *testing.T)               // 旧密码错 → 400；成功后新密码可登录
```

测试辅助：`newTestRouter(t)` 用临时 SQLite + 迁移 + `BootstrapAdmin`，返回 router 与初始密码。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/api/ -v` → FAIL

- [ ] **Step 3: 实现**

`response.go`：

```go
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
func fail(c *gin.Context, status int, code, msg string, details any) { c.AbortWithStatusJSON(status, apiError{code, msg, details}) }
func ok(c *gin.Context, v any) { c.JSON(http.StatusOK, v) }
```

`middleware.go`：

- `SecurityHeaders`：`X-Frame-Options: DENY`、`X-Content-Type-Options: nosniff`、`Referrer-Policy: same-origin`、CSP `default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'`。
- `OriginCheck`：仅对 POST/PUT/PATCH/DELETE；若 `Origin` 非空，解析后 host 必须等于 `c.Request.Host`；不匹配返回 403 `cross_origin_denied`。
- `RequireAuth`：读 Cookie → `UserBySessionToken` → 失败清 Cookie 并 401；成功 `c.Set(ctxUserKey, user)`。
- `RequireRole`：admin 恒通过；否则角色必须相等；不满足 403 `forbidden`。
- `ForcePasswordChange`：若 `MustChangePassword` 为真且请求不在白名单（`GET /api/v1/auth/me`、`PUT /api/v1/auth/password`、`POST /api/v1/auth/logout`），返回 403 `password_change_required`。
- 请求日志：`slog.Info("http", method, path, status, duration_ms)`，不记录 Cookie/请求体。

`auth_handlers.go`：

- 登录请求 `{username,password}`（validator: required）；调用内存限流器（key = clientIP+"|"+username，`LoginMaxFailures` 次失败锁 `LoginLockoutMinutes` 分钟，成功清零）；成功后 `CreateSession`、写审计 `login`、设置 Cookie（HttpOnly、Path=/、SameSite=Lax、Secure=cfg、MaxAge=ttl 秒）、返回用户 DTO。
- 失败：写审计 `login_failed`（ActorName=username，Summary `{"reason":"invalid_credentials"}`），401。
- me：返回当前用户 DTO（含 role、must_change_password）。
- 登出：删除会话、清 Cookie、审计 `logout`。
- 改密：`{old_password,new_password}`；`ChangePassword`；删除该用户其他会话（`DELETE FROM sessions WHERE user_id=? AND token_hash<>?`）；审计 `password_change`；204。
- `httputil.ClientIP` 不引入额外依赖：优先 `X-Forwarded-For` 首项，其次 `RemoteAddr` 的 host 部分。

`router.go`：

```go
func NewRouter(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), SecurityHeaders(), OriginCheck(), requestLog())
	v1 := r.Group("/api/v1")
	v1.GET("/version", func(c *gin.Context) { ok(c, gin.H{"version": d.Version, "started_at": d.StartedAt.UTC().Format(time.RFC3339)}) })
	v1.POST("/auth/login", loginHandler(d))
	authed := v1.Group("")
	authed.Use(RequireAuth(d.Auth))
	authed.GET("/auth/me", meHandler())
	authed.POST("/auth/logout", logoutHandler(d))
	authed.PUT("/auth/password", changePasswordHandler(d))
	// 受 ForcePasswordChange 保护的后续业务路由在 M1 挂载时使用该中间件
	r.NoRoute(gin.WrapH(d.WebHandler)) // M0 可为 nil，Task 7 注入
	return r
}
```

注意：`ForcePasswordChange` 在 M0 只对后续业务路由组生效（M1 起），本任务通过单元测试验证其行为。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/api/ -v` → PASS

- [ ] **Step 5: 提交**

```bash
git add internal/api
git commit -m "feat(api): auth endpoints, middleware and audit integration"
```

---

### Task 6: 单实例锁与 main 装配

**Files:**
- Create: `internal/lockfile/lockfile.go`
- Test: `internal/lockfile/lockfile_test.go`
- Create: `cmd/spool/main.go`

**Interfaces:**
- Produces:
  - `func Acquire(path string) (release func(), err error)`
  - `var version = "dev"`（ldflags 注入）
- 启动行为：
  1. `-config` 默认 `config.yaml`；加载失败退出。
  2. 创建 `data_dir`、`logs/`、`tmp/`（0755）。
  3. 打开 store 并迁移。
  4. `Acquire(data_dir/spool.lock)`；失败退出并提示已有实例。
  5. 认证服务 TTL = `session_ttl_hours`；`BootstrapAdmin`，若创建则打印：

```
========================================
 Initial admin account: admin
 Initial password: <password>
 Change it immediately after login.
========================================
```

  6. 装配 router；`http.Server` 监听配置地址；支持 TLS（cert/key 都非空）。
  7. `SIGINT/SIGTERM` 优雅停机，30s 超时；关闭 store；释放锁。

- [ ] **Step 1: 写失败测试**

```go
func TestAcquireExcludesSecondHolder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "spool.lock")
	rel1, err := Acquire(p)
	if err != nil { t.Fatal(err) }
	defer rel1()
	if _, err := Acquire(p); err == nil { t.Fatal("second acquire should fail") }
}

func TestReleaseAllowsReacquire(t *testing.T) {
	p := filepath.Join(t.TempDir(), "spool.lock")
	rel, err := Acquire(p)
	if err != nil { t.Fatal(err) }
	rel()
	rel2, err := Acquire(p)
	if err != nil { t.Fatalf("reacquire: %v", err) }
	rel2()
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/lockfile/ -v` → FAIL

- [ ] **Step 3: 实现锁**

```go
package lockfile

import (
	"fmt"
	"os"
	"syscall"
)

func Acquire(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil { return nil, fmt.Errorf("open lock file: %w", err) }
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another spool instance is running: %w", err)
	}
	if err := f.Truncate(0); err == nil { _, _ = f.WriteString(fmt.Sprint(os.Getpid())) }
	release := func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
	return release, nil
}
```

（仅支持 linux/darwin，符合部署目标。）

- [ ] **Step 4: 实现 main**

要点：

```go
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Hopetree/spool/internal/api"
	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/auth"
	"github.com/Hopetree/spool/internal/config"
	"github.com/Hopetree/spool/internal/lockfile"
	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/web"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	// setup slog by cfg.Logging.Level
	// mkdir data_dir, logs, tmp
	// st, err := store.Open(filepath.Join(cfg.DataDir, "spool.db"))
	// defer st.Close(); st.Migrate()
	// release, err := lockfile.Acquire(filepath.Join(cfg.DataDir, "spool.lock")); defer release()
	// authSvc := auth.NewService(st.DB, time.Duration(cfg.Security.SessionTTLHours)*time.Hour)
	// auditSvc := audit.NewService(st.DB)
	// pw, created, err := authSvc.BootstrapAdmin(context.Background())
	// if created { print banner }
	// router := api.NewRouter(api.Deps{...})
	// srv := &http.Server{Addr: cfg.Server.Listen, Handler: router}
	// goroutine ListenAndServe / ListenAndServeTLS
	// signal.NotifyContext; <-ctx.Done(); shutdown with 30s timeout
	return nil
}
```

- [ ] **Step 5: 构建并手工冒烟**

```bash
go build -o bin/spool ./cmd/spool
./bin/spool -config /tmp/spool-m0.yaml   # 空配置即可，data_dir 用 ./data 或环境变量
curl -s http://127.0.0.1:8080/api/v1/version
```

Expected：打印初始密码；version 返回 JSON；再次启动第二个实例报"another spool instance is running"。

- [ ] **Step 6: 提交**

```bash
git add internal/lockfile cmd/spool
git commit -m "feat(cmd): wire config, store, auth, audit and single-instance lock"
```

---

### Task 7: 前端外壳（登录、布局、改密）与嵌入

**Files:**
- Create: `web/package.json`、`web/tsconfig.json`、`web/tsconfig.node.json`、`web/vite.config.ts`、`web/index.html`
- Create: `web/src/main.tsx`、`web/src/App.tsx`
- Create: `web/src/api/client.ts`、`web/src/api/client.test.ts`
- Create: `web/src/stores/auth.ts`
- Create: `web/src/components/AppLayout.tsx`、`web/src/components/ProtectedRoute.tsx`
- Create: `web/src/pages/Login.tsx`、`web/src/pages/Home.tsx`、`web/src/pages/Password.tsx`
- Create: `internal/web/embed.go`、`internal/web/dist/.gitkeep`
- Test: `web/src/api/client.test.ts`

**Interfaces:**
- Produces:
  - `web.Handler() http.Handler` — 服务 `dist/` 静态文件；非 `/api` 的未知路径回退 `index.html`；`index.html` 不存在时返回 `frontend not built` 纯文本。
  - 前端 `request<T>(method, path, body?)`；错误抛 `ApiError { status, code, message, details }`。
  - `useAuthStore`：`{ user, status, fetchMe, login, logout, changePassword }`。
- 路由：`/login`、`/`（受保护布局）、`/password`（受保护）。

- [ ] **Step 1: 初始化前端**

```bash
cd web
npm init -y
npm install react@^18.3.1 react-dom@^18.3.1 react-router-dom@^6.26.0 antd@^5.20.0 @ant-design/icons@^5.4.0 @tanstack/react-query@^5.51.0 zustand@^4.5.4 dayjs@^1.11.12
npm install -D vite@^5.4.0 @vitejs/plugin-react@^4.3.1 typescript@^5.5.4 vitest@^2.0.5
```

`package.json` scripts：

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "test": "vitest run"
  }
}
```

`vite.config.ts`：

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: { port: 5173, proxy: { "/api": "http://127.0.0.1:8080" } },
  build: { outDir: "dist" },
});
```

- [ ] **Step 2: 写失败测试（client）**

```ts
import { describe, expect, it } from "vitest";
import { ApiError, parseApiError } from "./client";

describe("parseApiError", () => {
  it("parses structured error body", async () => {
    const res = new Response(JSON.stringify({ code: "invalid_credentials", message: "用户名或密码错误" }), {
      status: 401,
      headers: { "Content-Type": "application/json" },
    });
    const err = await parseApiError(res);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(401);
    expect(err.code).toBe("invalid_credentials");
    expect(err.message).toBe("用户名或密码错误");
  });

  it("falls back for non-json body", async () => {
    const res = new Response("oops", { status: 502 });
    const err = await parseApiError(res);
    expect(err.code).toBe("http_502");
    expect(err.message).toContain("502");
  });
});
```

Run: `npm test` → FAIL（模块不存在）

- [ ] **Step 3: 实现 API client 与认证 store**

`client.ts` 要点：

```ts
export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string, public details?: unknown) {
    super(message);
  }
}

export async function parseApiError(res: Response): Promise<ApiError> {
  try {
    const body = await res.json();
    return new ApiError(res.status, body.code ?? `http_${res.status}`, body.message ?? `请求失败 (${res.status})`, body.details);
  } catch {
    return new ApiError(res.status, `http_${res.status}`, `请求失败 (${res.status})`);
  }
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw await parseApiError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
```

`stores/auth.ts`：zustand store；`status: "loading" | "authed" | "anon"`；`fetchMe()` 调用 `GET /auth/me`，401 置 anon；`login(username,password)` 调 `POST /auth/login` 后写入 user；`logout()`；`changePassword(old,new)`。

- [ ] **Step 4: 实现页面与路由**

- `main.tsx`：`ConfigProvider locale={zhCN}`、`QueryClientProvider`、`BrowserRouter`、`App`；`dayjs/locale/zh-cn`。
- `App.tsx`：`/login` → Login；受保护 `ProtectedRoute` 内 `AppLayout`，子路由 `/` → Home、`/password` → Password。
- `ProtectedRoute`：`status==="loading"` 显示 `Spin`；`anon` → `<Navigate to="/login" replace />`；否则 `<Outlet />`。
- 启动时调用一次 `fetchMe()`（在 `ProtectedRoute` 的 `useEffect` 或 App 顶层）。
- `Login.tsx`：AntD Form（用户名/密码）、提交按钮 loading、错误 `Alert`；成功后若 `must_change_password` 跳 `/password`，否则跳 `/`。
- `AppLayout.tsx`：Sider（Logo "Spool"、菜单"首页"）、Header 右侧用户名下拉（修改密码/退出登录）、Content `<Outlet />`。
- `Home.tsx`：`<Result status="info" title="Spool 已就绪" subTitle="主机、工具、流程将在后续里程碑上线" />`。
- `Password.tsx`：旧密码/新密码/确认新密码；成功后提示并跳 `/`。

- [ ] **Step 5: 运行测试与构建**

```bash
cd web && npm test && npm run build
```

Expected：2 个测试通过；`web/dist/index.html` 与 `assets/` 生成。

- [ ] **Step 6: 实现嵌入**

`internal/web/embed.go`：

```go
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var assets embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(assets, "dist")
	if err != nil { panic(err) }
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" { p = "index.html" }
		if _, err := fs.Stat(sub, p); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		b, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("frontend not built\n"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
}
```

`internal/web/dist/.gitkeep` 必须存在，否则 `go:embed` 编译失败。

`api.Deps` 增加 `WebHandler http.Handler`，`router.go` 的 `NoRoute` 使用它；`main.go` 传入 `web.Handler()`。

- [ ] **Step 7: 提交**

```bash
git add web internal/web internal/api cmd/spool
git commit -m "feat(web): react shell with login, layout and embedded SPA"
```

（`web/node_modules`、`web/dist`、`internal/web/dist/*` 已被 .gitignore 忽略，仅提交源码与 `.gitkeep`。）

---

### Task 8: 构建、部署文件与端到端验收

**Files:**
- Create: `Makefile`
- Create: `config.example.yaml`
- Create: `deploy/spool.service`
- Create: `README.md`
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Makefile**

```makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build web-build go-build test dev-api dev-web clean

build: web-build go-build

web-build:
	cd web && npm ci && npm run build
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	cp -r web/dist/. internal/web/dist/

go-build:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/spool ./cmd/spool

test:
	go test ./...
	cd web && npm test

dev-api:
	go run ./cmd/spool -config config.yaml

dev-web:
	cd web && npm run dev

clean:
	rm -rf bin internal/web/dist web/dist
```

- [ ] **Step 2: 配置样例**

`config.example.yaml`：

```yaml
server:
  listen: ":8080"
  base_url: "http://spool.intra"
  cookie_secure: false
  tls:
    cert: ""
    key: ""
data_dir: /var/lib/spool
timezone: Asia/Shanghai
logging:
  level: info
security:
  session_ttl_hours: 12
  login_max_failures: 5
  login_lockout_minutes: 5
  host_key_policy: tofu
```

- [ ] **Step 3: systemd 单元**

```ini
[Unit]
Description=Spool intranet ops platform
After=network-online.target

[Service]
Type=simple
User=spool
Group=spool
ExecStart=/opt/spool/spool -config /etc/spool/config.yaml
Restart=on-failure
RestartSec=3
LimitNOFILE=65535
KillMode=mixed
TimeoutStopSec=40
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 4: CI**

```yaml
name: ci
on:
  push:
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: npm
          cache-dependency-path: web/package-lock.json
      - run: cd web && npm ci
      - run: cd web && npm run build
      - run: rm -rf internal/web/dist && mkdir -p internal/web/dist && cp -r web/dist/. internal/web/dist/
      - run: go vet ./...
      - run: go test ./...
      - run: CGO_ENABLED=0 go build -o /tmp/spool ./cmd/spool
```

- [ ] **Step 5: README**

包含：项目简介、技术栈、本地开发（`make dev-api` + `make dev-web`，初始密码在服务端 stdout）、构建（`make build`）、部署（二进制 + config + systemd）、数据目录与备份说明（`spool.db` + 后续 `master.key`）、安全提示（首次登录改密）。

- [ ] **Step 6: 全量验收**

```bash
make test
make build
SPOOL_DATA_DIR=/tmp/spool-m0 SPOOL_LISTEN=:18080 ./bin/spool -config config.example.yaml
```

手工验收：

1. stdout 出现初始管理员密码。
2. 浏览器打开 `http://127.0.0.1:18080/`，自动跳登录页。
3. 用 admin + 初始密码登录成功；提示修改密码。
4. 修改密码后重新登录成功。
5. 刷新页面保持登录；退出后回到登录页。
6. 错误密码 5 次后提示锁定。
7. 审计表出现 login/login_failed/password_change 记录。

- [ ] **Step 7: 提交**

```bash
git add Makefile config.example.yaml deploy README.md .github
git commit -m "chore(m0): build, deploy files, CI and acceptance docs"
```

---

## 自检记录

- 规格覆盖（M0 范围）：配置/部署（§20）、角色与认证（§3、§16）、审计基础（§17）、API 约定（§18）、前端外壳（§19）、测试与 CI（§22）。主机/凭据/工具/执行/流程/调度/附加模块不属于 M0，由后续里程碑计划覆盖。
- 无占位符：每个任务包含文件路径、接口签名、测试断言与实现要点；非平凡逻辑给出代码。
- 类型一致性：`config.Config`、`store.Store`、`auth.Service`、`audit.Service`、`api.Deps` 在任务间签名一致；Cookie 名与路由路径与设计文档一致。
