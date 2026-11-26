# Spool M2 工具与执行实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付工具（不可变版本、参数声明）与多主机脚本执行：SSH/SFTP 下发、环境变量注入、实时输出（SSE）、历史回看与下载、取消/超时/重试。

**Architecture:** 新增 `internal/tool`（定义与版本）、`internal/exec`（执行计划、状态机、并发池、SSH 执行器、JSONL 事件日志、SSE hub）；扩展 `internal/sshx` 的流式执行与上传能力。工具执行 = 单阶段单步骤计划；执行引擎按"阶段串行、阶段内步骤并行"实现，为 M3 流程复用。

**Tech Stack:** Go 1.26、sqlx/sqlite、x/crypto/ssh、pkg/sftp、Gin、React 18、Ant Design 5、CodeMirror 6（M2 前端先使用 TextArea，编辑器组件在 M3 前替换）。

**Spec:** `docs/superpowers/specs/2026-11-26-spool-design.md` §6、§8、§9、§10、§16、§17、§18、§19

## Global Constraints

- 沿用 M0/M1：ULID、UTC 毫秒、Cookie 会话、角色（admin/operator/viewer）、审计、中文 UI。
- 工具版本不可变；每次保存生成新版本；执行开始时解析最新版本并快照到 step_run。
- 参数以环境变量注入（不拼命令行）；未声明参数拒绝；敏感参数值加密存储、界面脱敏。
- 输出上限单步 20MB（含重试）；日志为 `data/logs/<execution_id>.jsonl`，事件含 seq/ts/type/host_run/step_run/stream/data。
- 并发：全局步骤 ≤20、主机执行 ≤50、同一主机跨执行串行；同一 host_run 内阶段并行不受主机锁限制。
- 执行取消：TERM 进程组 → 10s 后 KILL；始终清理远端临时目录。
- 每个任务结束 `go test ./...` 通过并单独提交；前端 `npm run build` 通过。

## 文件结构

```
internal/store/migrations/0003_tools_executions.sql
internal/tool/model.go            参数/工具/版本类型与校验
internal/tool/service.go          CRUD、版本、回滚、复制、diff 数据
internal/tool/tool_test.go
internal/exec/plan.go             Plan/Stage/Step 类型与工具计划构建
internal/exec/state.go            状态派生
internal/exec/events.go           JSONL 事件写入/读取
internal/exec/hub.go              SSE 订阅中心
internal/exec/remote.go           远端执行协议（env/脚本/包装/清理）
internal/exec/engine.go           执行状态机、并发、取消、超时、重试
internal/exec/transport.go        Runner 接口与 SSH 实现（含 fake）
internal/exec/*_test.go
internal/api/tool_handlers.go
internal/api/execution_handlers.go
internal/api/exec_stream.go       SSE
internal/api/m2_test.go
web/src/api/tools.ts / executions.ts
web/src/pages/Tools.tsx / ToolEditor.tsx / Executions.tsx / ExecutionDetail.tsx
web/src/components/LogViewer.tsx
```

---

### Task 1: 迁移 0003

新增表：`tools`、`tool_versions`、`executions`、`host_runs`、`step_runs`（字段见设计 §6；`tool_versions.content_hash`、`params_json`、`prepare_command`、`default_timeout_sec`、`sudo`；`executions.plan_json`、`sensitive_params_enc`、状态与统计；`step_runs` 含 `attempt`、`log_bytes`、`truncated`、`dropped_bytes`）。索引：`tool_versions(tool_id, version_no unique)`、`executions(created_at desc)`、`host_runs(execution_id)`、`step_runs(host_run_id)`。

### Task 2: 工具服务

**Interfaces:**
- `type Param struct { Name, Label, Type, Default string; Required, Sensitive bool; Options []string; Description string }`；`ValidateParams([]Param) error`；`ValidateValues(params []Param, values map[string]string) (map[string]string, error)`（trim、默认值、int/bool/select/必填、拒绝未知）。
- `type Tool struct { ID, Name, Description string; Enabled bool; CurrentVersion *Version; CreatedAt, UpdatedAt int64 }`
- `type Version struct { ID, ToolID string; VersionNo int; Type, Interpreter, Content, PrepareCommand, ContentHash, ChangeNote string; Params []Param; TimeoutSec int; Sudo bool; CreatedBy string; CreatedAt int64 }`
- `List(ctx, filter)`, `Get`, `Create(ctx, Input, actorID)`, `Update(ctx, id, Input)`（新版本）、`SetEnabled`, `Delete`（有执行历史则 `ErrHasHistory`）、`Versions`, `Version(ctx, toolID, no)`, `Rollback(ctx, id, no, note, actorID)`, `Copy(ctx, id, actorID)`。
- 校验：type shell/python；interpreter 单 token 且非空；content 非空；timeout 1..86400；参数名唯一且 env 合法。
- 敏感参数值在执行提交时加密存储（AES-GCM），不落审计。

**Tests:** 创建/更新产生版本 1→2；回滚复制旧内容为新版本；参数校验矩阵；未知参数拒绝；删除保护。

### Task 3: 执行计划与状态

- `Plan{Kind, Name, Stages []Stage}`；`Stage{Index, Name, AlwaysRun, Steps []Step}`；`Step{Index, Name, ToolID, ToolVersionID, ToolVersionNo, ContentHash, Type, Interpreter, Content, PrepareCommand, Sudo, TimeoutSec, RetryCount, RetryDelaySec, Params map[string]string}`。
- `BuildToolPlan(tool, version, params) Plan`。
- 状态派生纯函数：`DeriveExecutionStatus(hostRuns)`, `DeriveHostStatus(steps)`，测试覆盖 success/partial/failed/cancelled/skipped。

### Task 4: SSH 流式执行与远端协议

- 扩展 `sshx.Client`：`Upload(ctx, path, data, mode)`, `RunStream(ctx, cmd, onOut, onErr) (exitCode, error)`（ctx 取消 → 关闭 session 并返回 `context.Canceled`）、`ReadFile(path)`。
- `internal/exec/remote.go`：
  - `ScriptName(type)`, `EnvFile(params)`（单引号转义）、`Wrapper()`（`cd dir && exec /bin/sh -c 'set -a; . ./.spool-env; set +a; prepare && exec interpreter ./script'`，setsid 写 `.spool.pid`，外层 timeout）。
  - 取消：读 `.spool.pid` 后 `kill -TERM -PGID`，10s 后 KILL；sudo 时加 `sudo -n`。
  - 清理：`rm -rf <dir>`。
- 测试：env 转义、wrapper 生成、假 SSH 端到端（上传/执行/stdout/stderr/退出码/取消/清理）。

### Task 5: 执行引擎

**Interfaces:**
- `type Engine struct { ... }`；`New(db, logsDir, cfg, runnerFactory)`；`Start()`（恢复 interrupted、启动 worker）、`Submit(ctx, req SubmitRequest) (*Execution, error)`、`Cancel(ctx, execID)`, `CancelHostRun(ctx, hostRunID)`, `Hub()`。
- `SubmitRequest{Kind, RefID, Name, Plan, Params, SensitiveParams map[string]string, HostIDs []string, Source, ScheduleID, ActorID}`。
- 提交事务：创建 execution/host_runs/step_runs（全 queued）、写 plan_json、敏感参数加密；返回 ID 后异步执行。
- 每 host_run：获取主机槽位与跨执行主机锁 → 连接（复用）→ 逐阶段执行（阶段内步骤并行）→ 终态。步骤：取全局槽位、preflight 解释器（能力缓存缺失则跳过）、上传脚本/env、执行、流式写事件、超时/重试/取消、清理。
- 每步结果写 step_runs；终态派生并写 execution 统计；写审计由 API 负责。
- 日志写入与 hub 广播；执行结束 hub 发 end。
- 启动恢复：queued/running → interrupted。

**Tests:** 用内存/临时 SQLite + fake Runner（记录调用、可控输出/退出码/阻塞）覆盖：状态派生、并发上限、取消、超时、重试、输出截断、清理调用、重启恢复。

### Task 6: 工具与执行 API

- 工具：`GET/POST /tools`、`GET/PUT/DELETE /tools/:id`、`POST /tools/:id/enabled`、`GET /tools/:id/versions`、`GET /tools/:id/versions/:no`、`POST /tools/:id/rollback`、`POST /tools/:id/copy`；写操作 admin，读所有角色。
- 执行：`POST /executions`（kind=tool，ref_id、params、host_ids）；`GET /executions`（kind/status/ref_id/host_id/created_by/from/to/page）；`GET /executions/:id`（含 host_runs/steps）；`POST /executions/:id/cancel`；`POST /executions/:id/rerun`；`POST /executions/:id/host-runs/:hostRunId/cancel`；`GET /executions/:id/events?after_seq=&limit=`；`GET /executions/:id/stream`（SSE）；`GET /executions/:id/logs/download?host_run=&step_run=`。
- 参数：非敏感明文、敏感密文；响应敏感值 `******`；审计不记录值。
- 权限：operator+ 可执行/取消；viewer 只读。

### Task 7: 前端

- `/tools`：列表（名称/类型/版本/启用/更新时间）、启用开关、复制、删除、执行入口。
- `/tools/new`、`/tools/:id`：表单（名称/描述/类型/解释器/content TextArea/参数表格/准备命令/超时/sudo），保存生成新版本；版本历史抽屉、选择两版本 diff（jsdiff）、回滚；执行弹窗：主机多选（复用分组/标签筛选）、参数动态表单、敏感输入。
- `/executions`：筛选列表。
- `/executions/:id`：总览、主机列表/步骤时间线、实时日志（EventSource）、暂停跟随、按主机/步骤过滤、下载、取消、重跑。
- 菜单新增"工具""执行历史"。

### Task 8: 验收与合并

- 用假 SSH 主机（测试内）跑通；本机无 sshd 时用 API 验证错误路径。
- `make test`、`make build`；README 更新 M2 状态；合并 main。

## 自检

覆盖设计 §8（工具/版本/参数）、§9（执行协议/输出/重试/超时）、§10（状态机/并发/SSE/保留）、§18（API）、§19（页面）。流程（§11）与调度（§12）不在本计划，M3/M4 实现。
