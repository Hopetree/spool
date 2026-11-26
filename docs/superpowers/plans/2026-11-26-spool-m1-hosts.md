# Spool M1 主机与凭据实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 支持凭据加密管理、主机/分组/标签管理、SSH 连通性测试与环境探测、主机指纹确认，以及对应前端页面。

**Architecture:** 在 M0 模块化单体上新增 `internal/credential`（AES-256-GCM 与凭据服务）、`internal/sshx`（SSH 拨号、指纹校验、探测命令）、`internal/host`（主机/分组/标签与测试编排）；API 与前端沿用现有模式。主密钥在启动时从 `data_dir/master.key` 加载或生成。

**Tech Stack:** Go 1.26、`golang.org/x/crypto/ssh`、`github.com/pkg/sftp`、`gliderlabs/ssh`（仅测试）、React 18 + AntD 5。

**Spec:** `docs/superpowers/specs/2026-11-26-spool-design.md` §3、§6、§7、§16、§17、§18、§19

## Global Constraints

- 沿用 M0：ULID 主键、UTC 毫秒、Cookie 会话、Gin、sqlx、sqlite（modernc）、中文 UI。
- 凭据与主密钥：AES-256-GCM，AAD=记录 ID；`master.key` 为 base64 的 32 字节，权限 0600；接口永不返回明文。
- 主机指纹：`tofu` 首次自动信任并写审计；变化一律拒绝连接；`strict` 首次也拒绝；只有 admin 可显式更新指纹。
- operator 可查看主机与执行测试，不可增删改主机/凭据/分组；凭据接口仅 admin。
- 删除凭据前检查主机引用；删除分组时主机 `group_id` 置空。
- 每个任务结束 `go test ./...` 通过并单独提交；前端任务需 `npm run build`。

## 文件结构

```
internal/store/migrations/0002_hosts.sql
internal/credential/crypto.go       主密钥加载/生成、AES-GCM
internal/credential/crypto_test.go
internal/credential/service.go      凭据 CRUD、机密读取
internal/credential/service_test.go
internal/sshx/sshx.go               Dial、指纹策略、Run、SFTP
internal/sshx/probe.go              探测脚本与结果解析
internal/sshx/sshx_test.go          假 SSH 服务端测试
internal/host/model.go              类型与 DTO
internal/host/group_service.go      分组 CRUD
internal/host/service.go            主机 CRUD、标签、测试编排
internal/host/service_test.go       使用 fake Prober
internal/api/host_handlers.go
internal/api/credential_handlers.go
internal/api/group_handlers.go
internal/api/m1_test.go
web/src/api/hosts.ts, credentials.ts, groups.ts
web/src/pages/Hosts.tsx, HostEditModal.tsx, Groups.tsx, Credentials.tsx
web/src/components/TagSelect.tsx（若需要）
```

---

### Task 1: 迁移与主密钥加密

**Files:** `internal/store/migrations/0002_hosts.sql`、`internal/credential/crypto.go`、`crypto_test.go`

**Interfaces:**
- `func LoadOrCreateMasterKey(path string) ([]byte, error)`：文件存在则读取 base64 并校验 32 字节；不存在则生成、以 0600 写入（O_EXCL）。
- `func NewCipher(key []byte) (*Cipher, error)`
- `func (c *Cipher) Encrypt(plaintext []byte, aad string) ([]byte, error)`
- `func (c *Cipher) Decrypt(ciphertext []byte, aad string) ([]byte, error)`
- `func CheckMasterKey(ctx context.Context, db *sqlx.DB, c *Cipher) error`：若 `credentials` 有记录，取一条尝试解密，失败返回 `ErrMasterKeyMismatch`。

**Steps:** 写测试（往返、AAD 不同失败、篡改失败、密钥文件持久化与权限、CheckMasterKey 不匹配）→ 确认失败 → 写迁移 SQL（credentials/host_groups/hosts/host_tags，见设计 §6）→ 实现 → 通过 → 提交。

迁移 SQL 关键约束：

```sql
CREATE TABLE credentials (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  auth_type TEXT NOT NULL CHECK (auth_type IN ('password','private_key')),
  username TEXT NOT NULL,
  secret_enc BLOB NOT NULL,
  passphrase_enc BLOB,
  description TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE host_groups (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE hosts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  address TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 22,
  group_id TEXT REFERENCES host_groups(id) ON DELETE SET NULL,
  credential_id TEXT NOT NULL REFERENCES credentials(id) ON DELETE RESTRICT,
  enabled INTEGER NOT NULL DEFAULT 1,
  fingerprint TEXT NOT NULL DEFAULT '',
  fingerprint_type TEXT NOT NULL DEFAULT '',
  capability_json TEXT NOT NULL DEFAULT '{}',
  last_test_at INTEGER,
  last_test_ok INTEGER,
  note TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX idx_hosts_group ON hosts(group_id);
CREATE INDEX idx_hosts_credential ON hosts(credential_id);
CREATE TABLE host_tags (host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE, tag TEXT NOT NULL, PRIMARY KEY (host_id, tag));
CREATE INDEX idx_host_tags_tag ON host_tags(tag);
```

### Task 2: 凭据服务与 API

**Interfaces:**
- `type Credential struct { ID, Name, AuthType, Username, Description string; HasPassphrase bool; CreatedAt, UpdatedAt int64 }`
- `func NewService(db *sqlx.DB, cipher *Cipher) *Service`
- `List(ctx)`, `Get(ctx,id)`, `Create(ctx, in Input, actorID string)`, `Update(ctx, id, in Input)`, `Delete(ctx, id)`, `Open(ctx, id) (authType, username, secret, passphrase string, err error)`
- 校验：name/username/secret 必填；`auth_type` 枚举；password 不允许 passphrase；删除时若被主机引用返回 `ErrInUse`（包含引用数量）。
- API：`GET/POST /credentials`、`PUT/DELETE /credentials/:id`，全部 `RequireRole("admin")`；响应永不包含 secret/passphrase；`Update` 的 secret 留空表示不变。
- 审计：credential.create/update/delete，摘要只含名称与类型。

**Steps:** 服务测试（含 secret 复用、删除被引用、Open 解密）→ API 测试（operator 403、响应无 secret）→ 实现 → 通过 → 提交。

### Task 3: 分组与主机服务

**Interfaces:**
- `type Host`, `type Capability`, `type TestResult`, `type Prober interface { Probe(ctx context.Context, t sshx.Target) (sshx.ProbeResult, error) }`
- `func NewService(db *sqlx.DB, creds *credential.Service, prober Prober) *Service`
- `List(ctx, Filter) ([]Host, error)`（group_id/tag/enabled/keyword）、`Get`、`Create`、`Update`、`Delete`、`SetFingerprint`、`Test(ctx,id)`。
- 标签规则：trim、去重、单个 ≤32 字符、最多 20 个。
- `Test`：读取凭据 → 组装 `sshx.Target`（含已存指纹与策略）→ 调 Prober → 成功则保存 capability、`last_test_at/ok`，首次 TOFU 保存指纹；`sshx.HostKeyError` 原样返回供 API 输出指纹详情；失败写 `last_test_ok=false` 并按错误类型返回。
- API：`GET/POST /hosts`、`GET/PUT/DELETE /hosts/:id`（写操作 admin）、`POST /hosts/:id/test`（operator+）、`PUT /hosts/:id/fingerprint`（admin）；`GET/POST/PUT/DELETE /host-groups`（写 admin）。
- 响应中 `credential_id`/`credential_name` 仅 admin 可见。
- 审计：host.create/update/delete/test/fingerprint_update，group.create/update/delete。

**Steps:** fake Prober 测试 CRUD/过滤/标签/测试成功与指纹变化 → API 测试（角色、字段脱敏、删除引用）→ 实现 → 通过 → 提交。

### Task 4: SSH 拨号、指纹与探测

**Interfaces:**
- `type Target struct { Address string; Port int; Username, AuthType, Secret, Passphrase string; ExpectedFingerprint, ExpectedKeyType, Policy string }`
- `type HostKeyError struct { Expected, Actual, KeyType string; Changed bool }`（实现 error）
- `func Dial(ctx context.Context, t Target) (*Client, error)`；`(*Client).Run(ctx, cmd string) (stdout, stderr []byte, exitCode int, err error)`；`(*Client).SFTP() (*sftp.Client, error)`；`Close()`
- `type ProbeResult struct { User, OS, Kernel, Arch, Distro, BashPath, BashVersion, PythonPath, PythonVersion string; SFTP, Sudo, Setsid, Timeout bool; LatencyMS int64; Fingerprint, KeyType string }`
- `func NewProber() *Prober`；`(*Prober).Probe(ctx, t Target) (ProbeResult, error)`
- 策略：expected 为空 + tofu → 接受并在结果中返回指纹；expected 为空 + strict → HostKeyError{Changed:false}；不匹配 → HostKeyError{Changed:true}。
- 探测脚本：单次 `sh -c`，输出 `SPOOL_*=` 行（用户、OS、内核、架构、发行版、bash、python3、setsid、timeout、sudo -n）；SFTP 单独打开子系统验证。
- 超时：握手 10s；命令由 ctx 控制。

**Steps:** 用 gliderlabs/ssh 搭假主机（PasswordHandler + 固定 host key）写测试：密码登录成功、错误密码、TOFU 返回指纹、strict 空指纹失败、指纹不匹配报 HostKeyError、Run 的 stdout/stderr/退出码、Probe 解析、SFTP 可用 → 实现 → 通过 → 提交。

### Task 5: 主机测试 API 与审计集成

**Steps:** 将 `host.Service` 注入 `api.Deps`；实现 `POST /hosts/:id/test` 返回 `{ok, latency_ms, fingerprint, key_type, capability}`，指纹错误返回 409 `host_key_mismatch` + `details{expected,actual,key_type}`；`PUT /hosts/:id/fingerprint` 更新并审计；扩展 `api_test` 覆盖角色与错误映射（用 fake Prober）。提交。

### Task 6: 前端主机/分组/凭据页面

**内容：**
- `web/src/api/credentials.ts`、`groups.ts`、`hosts.ts`：类型与请求函数。
- `/hosts`：筛选（分组、标签、启用、关键字）、表格（名称/地址/分组/标签/状态/最近测试）、行操作（测试、编辑、删除）、批量选择预留、新增按钮（admin）。
- 主机编辑弹窗：名称、地址、端口、分组、凭据（admin）、标签（tags 模式）、备注、启用。
- 测试结果弹窗：成功显示能力矩阵；指纹不匹配显示期望/实际指纹与"信任并更新"按钮（admin）。
- `/hosts/groups`：分组 CRUD。
- `/credentials`：仅 admin 可见；列表、创建/编辑（secret 输入留空表示不改）、删除。
- 侧边栏菜单按角色显示：主机、凭据（admin）。
- 请求状态用 TanStack Query；错误用 AntD message。

**Steps:** 实现页面 → `npm test` → `npm run build` → 手工冒烟（登录、建凭据、建主机、测试失败提示、编辑、删除）→ 提交。

### Task 7: M1 验收与合并

- `make test`、`make build`；启动二进制，用本地可达的 SSH 主机（或本机 127.0.0.1，需 sshd）验证：建凭据 → 建主机 → 测试连接 → 指纹记录 → 修改密码场景不涉及。
- 更新 README 的里程碑状态与 M1 功能说明。
- 合并 `feat/m1-hosts` 到 main（本地），删除分支。

## 自检记录

- 覆盖设计 §7（主机/凭据/指纹/探测）、§3（角色）、§17（审计）、§18（API）、§19（页面）。
- 指纹错误通过 `HostKeyError` 在服务层与 API 层保持类型一致；凭据明文只经 `Open` 暴露给主机测试与后续执行引擎。
- `Prober` 接口用于解耦 SSH 实现，便于 API/服务测试；真实 SSH 由 Task 4 的假服务端覆盖。
