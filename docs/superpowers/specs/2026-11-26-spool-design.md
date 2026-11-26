# Spool 内网运维系统设计文档

- 日期：2026-11-26
- 代码仓库：https://github.com/Hopetree/spool
- Go module：`github.com/Hopetree/spool`
- 状态：已确认（用户授权按推荐方案直接实施）

## 1. 背景与目标

内网 Linux 服务器运维需要一个无 agent 的轻量平台：集中管理主机与登录凭据，把日常操作沉淀为带版本的脚本工具，支持多主机批量执行与实时输出，用流程把工具串并联起来，并用定时任务驱动。v1 聚焦"管机器、跑脚本、编流程、定时间"，不做监控。

成功标准：

- 运维人员不必登录每台机器手动执行脚本，在浏览器完成一次多主机操作并看到实时输出。
- 所有脚本变更可追溯、可回滚；所有执行有完整历史与输出。
- 定时任务无人值守运行，失败可感知。
- 单二进制部署，备份只需数据目录。

## 2. 范围

### 2.1 v1 交付

| 模块 | 内容 |
|---|---|
| 主机管理 | 主机 CRUD、单分组 + 多标签、凭据复用、连通性测试、环境探测、主机指纹（TOFU） |
| 工具管理 | Shell/Python 单文件脚本、不可变版本、参数声明、版本 diff、回滚（生成新版本） |
| 工具执行 | 多主机批量、实时输出、历史回看与下载、取消、超时、步骤级重试 |
| 流程 | 多主机扇出、阶段内步骤并行、阶段间串行、参数映射、失败策略、`always_run` 阶段 |
| 定时任务 | 5 段 cron + 图形快捷方式、固定主机集合、并发策略（跳过/排队）、手动立即执行、执行历史 |
| 附加 | 临时命令执行、文件上传/下载、执行结果通知（Webhook/飞书/钉钉/企业微信） |
| 用户与审计 | 本地账号、admin/operator/viewer 三级角色、服务端会话、操作审计 |

### 2.2 明确不做（v1 非目标）

主机监控与指标采集、告警规则、图表大盘、Windows/WinRM、网络设备、Web 终端、容器/K8s 管理、审批流、按主机细粒度授权、多实例 HA、流程条件分支、步骤间传值、任务补跑/节假日日历、多文件脚本包、邮件通知（需 SMTP 配置）、动态主机组定时任务（新主机自动纳入）、Web 终端会话录屏。

"执行历史"属于可追溯性，不是监控：不采集主机指标、不做阈值告警、首页不放趋势图。

## 3. 角色与权限

| 能力 | admin | operator | viewer |
|---|:--:|:--:|:--:|
| 查看主机/工具/流程/定时任务/执行历史 | ✓ | ✓ | ✓ |
| 执行工具、流程、临时命令、文件分发 | ✓ | ✓ | ✗ |
| 取消执行 / 取消传输 | ✓ | ✓ | ✗ |
| 主机、分组、凭据、工具、流程、定时任务增删改 | ✓ | ✗ | ✗ |
| 用户管理、通知渠道、系统设置 | ✓ | ✗ | ✗ |
| 查看审计日志 | ✓ | ✗ | ✗ |

规则：

- operator 可执行但不可修改定义，防止执行人顺手改脚本绕过评审。
- 凭据只写不读：任何角色（含 admin）都拿不到明文，编辑只能整体覆盖；输入留空表示不修改。
- 后端强校验角色，前端仅按角色隐藏入口。
- 凭据管理页面与接口仅 admin 可用；operator 执行时通过主机间接使用凭据，不接触凭据元数据与密文。
- 系统至少保留一个启用状态的 admin，禁止停用/降级最后一个 admin。

## 4. 总体架构

模块化单体（方案 A）：一个 Go 二进制包含 HTTP API、内嵌 SPA、进程内 cron 调度、worker 池、SSE 日志分发、SQLite 与本地日志文件。

```
浏览器 SPA ──REST /api/v1（HttpOnly 会话 Cookie）──> api ──> service ──> store(SQLite)
   │                                                    │
   └──SSE /executions/{id}/stream──> exec.hub <── 执行器写 JSONL 日志 + 发布事件
                                        │
                             worker（全局步骤并发 ≤20，主机执行并发 ≤50）
                                        │
                             SSH/SFTP ──> 目标 Linux 主机
```

设计约束：

- `exec` 不依赖 `flow`。`flow` 把定义编译成执行计划（JSON），`exec` 只消费计划。
- 执行队列按"可拆进程"的形状设计：任务先落库、worker 从库中取、状态机显式。将来拆 API/Worker 只换消费者实现。
- 单实例运行，用 `spool.lock` 文件锁防止双进程重复调度。
- 实时输出用 SSE（单向下行、自动重连、普通 HTTP），不用 WebSocket。

### 4.1 模块边界

```
cmd/spool                 启动、装配、优雅停机
internal/config           配置加载与校验
internal/store            SQLite 连接、内嵌迁移、各领域查询
internal/auth             密码、会话、角色守卫、登录限流
internal/credential       凭据加密存储（AES-256-GCM）
internal/host             主机、分组、标签、连通性测试、环境探测、指纹
internal/tool             工具定义、不可变版本、参数校验
internal/exec             执行状态机、SSH 执行器、并发池、JSONL 日志、SSE hub
internal/flow             流程定义、校验、编译为执行计划
internal/schedule         cron 调度、并发策略、下次运行计算
internal/transfer         SFTP 分发与下载
internal/notify           通知渠道、签名、投递与重试
internal/api              HTTP 路由、DTO、中间件、审计写入
internal/web              go:embed 前端产物、SPA fallback
```

## 5. 技术选型（锁定）

| 层 | 选择 | 理由 |
|---|---|---|
| 语言 | Go 1.22 | 单二进制、并发模型适合执行器 |
| HTTP | Gin + go-playground/validator | 路由/绑定/校验成熟，开发快 |
| SQL | sqlx + goose（迁移内嵌，启动执行） | 显式 SQL，可测试，无 ORM 魔法 |
| SQLite | modernc.org/sqlite（纯 Go，无 CGO） | 跨平台单二进制；WAL；单机规模足够 |
| SSH/SFTP | golang.org/x/crypto/ssh + github.com/pkg/sftp | 标准实现 |
| 调度 | robfig/cron/v3（5 段表达式，指定时区） | 稳定、可测试 |
| 日志 | log/slog | 标准库，结构化 |
| ID | ULID（单调） | 有序、可做日志目录/事件序 |
| 配置 | YAML（gopkg.in/yaml.v3）+ 环境变量覆盖 | 部署友好 |
| 前端 | React 18 + TypeScript + Vite | 用户选定 |
| UI | Ant Design 5 + @ant-design/icons | 用户选定 |
| 前端状态 | TanStack Query（服务端状态）+ Zustand（本地 UI 状态）+ React Router | 请求缓存与失效清晰 |
| 脚本编辑 | CodeMirror 6（@uiw/react-codemirror，shell/python 高亮） | 比 Monaco 轻 |
| 日志渲染 | @tanstack/react-virtual | 大日志虚拟滚动 |
| diff | jsdiff | 版本对比 |
| 构建 | `make dev` / `make build`；前端产物复制到 internal/web/dist 后 go:embed | 单文件交付 |

## 6. 领域模型（表清单）

所有主键为 ULID（26 字符 TEXT）。所有时间为 UTC 毫秒（INTEGER）。JSON 字段为 TEXT。

| 表 | 关键字段 | 说明 |
|---|---|---|
| users | id, username, display_name, password_hash, role, enabled, must_change_password, last_login_at, created_at, updated_at | 本地账号 |
| sessions | id, user_id, token_hash, ip, user_agent, created_at, expires_at | 服务端会话，可撤销 |
| audit_logs | id, ts, user_id, username_snapshot, action, resource_type, resource_id, resource_name, summary_json, ip, user_agent | 只增不改 |
| credentials | id, name, auth_type, username, secret_enc, passphrase_enc, description, created_by, created_at, updated_at | secret 密文，AAD=记录 ID |
| host_groups | id, name, description, created_at | 平铺分组 |
| hosts | id, name, address, port, group_id, credential_id, enabled, fingerprint, fingerprint_type, capability_json, last_test_at, last_test_ok, note, created_at, updated_at | 主机名唯一 |
| host_tags | host_id, tag | 联合主键，标签筛选 |
| tools | id, name, description, type, current_version_id, enabled, created_by, created_at, updated_at | current 指向最新版本 |
| tool_versions | id, tool_id, version_no, type, interpreter, content, params_json, prepare_command, default_timeout_sec, sudo, content_hash, change_note, created_by, created_at | 不可变 |
| flows | id, name, description, params_json, failure_policy, max_duration_sec, enabled, created_by, created_at, updated_at | failure_policy: abort/continue |
| flow_stages | id, flow_id, idx, name, always_run | idx 从 1 递增 |
| flow_steps | id, stage_id, idx, name, tool_id, param_mapping_json, timeout_sec, retry_count, retry_delay_sec, enabled | 步骤引用工具（跟随最新版本） |
| schedules | id, name, target_kind, target_ref_id, params_json, sensitive_params_enc, host_ids_json, cron_expr, timezone, concurrency_policy, notify_channel_ids_json, notify_events, enabled, last_run_at, next_run_at, created_by, created_at, updated_at | target_kind: tool/flow |
| executions | id, kind, source, ref_id, schedule_id, name_snapshot, params_json, sensitive_params_enc, plan_json, host_total, host_success, host_failed, host_cancelled, status, failure_reason, error_summary, cancel_requested, created_by, created_at, started_at, finished_at | kind: tool/flow/adhoc；source: manual/schedule |
| host_runs | id, execution_id, host_id, host_name_snapshot, status, failure_reason, error_message, started_at, finished_at | 每台主机一条 |
| step_runs | id, host_run_id, stage_idx, step_idx, name_snapshot, tool_id, tool_version_id, tool_version_no, content_hash, interpreter, sudo, status, failure_reason, exit_code, error_message, attempt, started_at, finished_at, log_events, log_bytes, truncated, dropped_bytes | 一次执行的全部步骤在提交时创建 |
| transfers | id, direction, file_name, size_bytes, target_total, status, created_by, created_at, finished_at | direction: upload/download |
| transfer_targets | id, transfer_id, host_id, host_name_snapshot, remote_path, status, bytes_done, error, started_at, finished_at | |
| notify_channels | id, name, type, config_json, secret_enc, enabled, created_at, updated_at | type: webhook/feishu/dingtalk/wecom |
| notify_deliveries | id, execution_id, channel_id, status, attempts, request_snippet, response_snippet, error, created_at | 投递审计 |
| settings | key, value | base_url、默认通知渠道等运行时设置 |

索引：`executions(created_at desc)`、`executions(status)`、`host_runs(execution_id)`、`step_runs(host_run_id)`、`schedules(enabled)`、`audit_logs(ts desc)`、`host_tags(tag)`、`sessions(token_hash)`。

## 7. 主机与凭据

### 7.1 主机

- 字段见 6；`name` 全局唯一，`address` 为 IP 或域名，默认端口 22。
- 单分组 + 多标签；分组平铺不做树；标签自由输入，用于筛选与批量选择。
- 停用主机不可被选为执行目标。提交执行（含定时触发）时过滤停用主机，不为其创建 host_run；若存在被过滤的主机，在 `execution.error_summary` 记录"跳过 N 台已停用主机"；若全部目标主机均已停用，拒绝提交并提示。
- 删除主机：被定时任务引用时禁止删除并列出引用；否则允许删除。历史执行保存 `host_name_snapshot`，删除不影响历史可读性。
- 主机列表支持按分组、标签、启用状态、关键字筛选。

### 7.2 连通性测试与环境探测

`POST /hosts/:id/test`：使用凭据解密后建立 SSH 连接，记录指纹（策略见 7.3），执行一次探测脚本，返回并缓存：

- 登录用户、`uname -srm`、发行版（`/etc/os-release`）
- `bash` 路径与版本、`python3` 路径与版本
- SFTP 子系统是否可用
- `sudo -n true` 是否成功
- `setsid`、`timeout` 命令是否存在

结果写入 `hosts.capability_json`，同时更新 `last_test_at`/`last_test_ok`。主机详情页展示探测结果，并可重新探测。

执行预检：若能力缓存存在且工具所需解释器被标记为缺失，直接失败（`interpreter_missing`），不上传脚本；缓存不存在则跳过预检直接执行。

### 7.3 主机指纹（TOFU）

- 默认策略 `tofu`：首次成功连接自动记录指纹并写审计；之后指纹变化**拒绝连接**，主机列表标红，需管理员在详情页对比新旧指纹后手动更新。
- `strict` 策略：首次连接也不自动信任，必须先"测试并确认"。
- 指纹保存在 `hosts.fingerprint` / `fingerprint_type`，更新指纹需 admin 且写审计。

### 7.4 凭据

- 类型：`password`（密码）或 `private_key`（私钥正文 + 可选 passphrase）。
- 账号在凭据上，主机只引用凭据；改密码只改一处，可复用于一批主机。
- 加密：AES-256-GCM，每条记录随机 nonce，AAD 使用记录 ID；主密钥来自 `master.key`（首次启动生成，权限 0600）。
- 接口只写不读：列表/详情仅返回名称、类型、用户名、描述；编辑时 secret 留空表示不变。
- 启动校验：若库内存在凭据但主密钥无法解密（新增一条探测记录），直接报错退出，不静默降级。
- 凭据被主机引用时禁止删除。

## 8. 工具与版本

### 8.1 工具字段

| 字段 | 说明 |
|---|---|
| name / description | 名称全局唯一 |
| type | `shell` / `python` |
| interpreter | 默认 `/bin/bash` 或 `python3`，可改为 `/bin/sh`、`/usr/bin/python3.11`、`.venv/bin/python3` |
| content | 单文件脚本正文 |
| params | 参数声明数组 |
| prepare_command | 可选；与脚本同目录、同权限上下文、同日志；失败则中止 |
| default_timeout_sec | 默认 300 |
| sudo | `true` 时整个执行单元以 `sudo -n` 运行；不免密直接失败（`sudo_not_allowed`），不交互 |
| enabled | 停用后不可被选用 |

### 8.2 参数声明

| 属性 | 说明 |
|---|---|
| name | 环境变量名，`[A-Za-z_][A-Za-z0-9_]*`，工具内唯一 |
| label / description | 界面展示 |
| type | `string` / `int` / `bool` / `select` |
| required / default | 必填与默认值 |
| options | `select` 候选值 |
| sensitive | 敏感：界面脱敏、不写审计原文、日志不记录值 |

校验规则：提交执行时校验必填、类型、枚举；未声明参数直接拒绝（防拼错静默生效）；`int` 必须是十进制整数，`bool` 接受 `true`/`false`。参数值通过环境变量注入，不进入命令行。

### 8.3 版本模型

- 每次保存生成新版本，`version_no` 单调递增，内容与配置不可变。
- 回滚 = 以旧版本内容创建新版本，历史线性可审计。
- 任意两版本可 diff（脚本正文 + 解释器/超时/sudo/参数定义），只读展示。
- 执行开始时解析"最新版本"，把版本 ID/版本号/内容哈希写入 step_run；后续修改不影响本次追溯。
- 流程/定时任务引用的是工具 ID，执行时取最新版本（用户选定策略）。
- 被流程或定时任务引用的工具只能停用不能删除；历史保存工具名与版本快照。
- 支持复制工具。

### 8.4 Python 依赖

v1 只支持单文件，不做多文件包/requirements 上传。通过 `prepare_command` 覆盖：

```
python3 -m venv .venv && . .venv/bin/activate && python3 -m pip install --quiet requests
```

解释器可设为 `.venv/bin/python3`（先 `cd` 到工作目录再 exec，相对路径可用）。

## 9. 脚本执行协议

每个 step 在目标主机上的固定流程：

1. 获取全局步骤槽位（≤ `max_concurrent_steps`，默认 20）。
2. host_run 级：获取主机执行槽位（≤ `max_concurrent_host_runs`，默认 50）和该主机的跨执行独占锁（同一主机同一时刻只跑一个 host_run；同一 host_run 内流程阶段的并行步骤允许并发）。
3. 复用或建立 SSH 连接；keepalive 每 15s；连接断开按 `ssh_error` 处理。连接超时 10s。
4. 解释器预检（见 7.2）。
5. 创建远端目录 `<remote_base>/spool-<step_run_id>/`（默认 `remote_base=/tmp`）。路径仅含 ULID，无注入面。
6. SFTP 上传脚本 `script.sh` / `script.py`，权限 0700。
7. 生成 `.spool-env`（0600）：每行 `export NAME='value'`，值内单引号转义为 `'\''`；内容不写日志。
8. 注入只读上下文变量：`SPOOL_EXECUTION_ID`、`SPOOL_HOST_RUN_ID`、`SPOOL_STEP_RUN_ID`、`SPOOL_HOST_NAME`、`SPOOL_TOOL_NAME`、`SPOOL_TOOL_VERSION`。
9. 用 `/bin/sh` 统一包装（不使用 SSH SetEnv，sshd 默认白名单会拒绝）：

```
exec /bin/sh -c 'cd <dir> && set -a; . ./.spool-env; set +a; <prepare_command> && exec <interpreter> ./script'
```

- Python 解释器默认追加 `-u`（输出即时），Shell 不包 `stdbuf`。
- `sudo=true` 时包装为 `exec sudo -n /bin/sh -c '...'`，准备命令与脚本同处于 root 上下文。
- 远端存在 `setsid` 时：将执行放入新进程组并写 `.spool.pid`，取消时按进程组 `kill -TERM -PGID`，10s 后 `kill -KILL -PGID`；`sudo=true` 时 kill 同样经 `sudo -n`。
- 远端存在 `timeout` 时：套一层硬超时兜底；客户端同时计时，任一触发走同一取消路径。两者都缺失时关闭通道尽力而为，并在日志写一条警告。
10. stdout/stderr 分别捕获，按到达顺序写入 JSONL 事件日志，同时推送 SSE。
11. 记录退出码或信号；`failure_reason` 区分 `exit_code`/`timeout`/`ssh_error`/`upload_error`/`interpreter_missing`/`sudo_not_allowed`/`cancelled`/`flow_timeout`/`internal_error`。
12. 无论成功、失败、取消、超时，清理远端目录；`sudo=true` 时清理也经 `sudo -n rm -rf`；清理失败只记日志，不改变步骤结果。

### 9.1 输出与截断

- 单步（含重试的所有尝试）原始输出累计上限 20MB（配置）。超限后继续读取但不再落盘，避免阻塞远端；记录丢弃字节数，`truncated=true`。
- 事件日志为 JSONL，每个事件：`{"seq":n,"ts":ms,"type":"log|state","host_run":"...","step_run":"...","stream":"stdout|stderr","data":"..."}`，状态事件记录实体与状态变更。
- 执行结束后日志保留在 `data/logs/<execution_id>.jsonl`；下载接口可按主机/步骤过滤并转成纯文本。
- 日志内容包括命令说明、退出码、耗时；**不包含**凭据、`.spool-env` 内容、敏感参数值。

### 9.2 重试

- 步骤级 `retry_count`（0–3，默认 0）+ `retry_delay_sec`（默认 5）。默认不重试，因为重跑非幂等脚本有风险，由工具/流程显式开启。
- 仅在 `exit_code`/`timeout`/`ssh_error`/`upload_error` 时重试；`cancelled` 不重试。
- 每次尝试重新上传脚本与 env 文件，`attempt` 记录在 step_run，日志中标记尝试分段。

### 9.3 超时与取消

- 步骤超时：工具 `default_timeout_sec` 可被流程步骤覆盖；超时状态记为 `failed` + `failure_reason=timeout`。
- 流程可选 `max_duration_sec`（默认 3600，按 host_run 计时）；超时停止该 host 后续阶段，正在执行的步骤取消，host_run 记为 `failed` + `flow_timeout`。
- 取消执行：停止派发新阶段/新步骤；运行中步骤 TERM→KILL；未开始步骤 `skipped`，未开始 host_run `cancelled`；execution 终态 `cancelled`。
- 取消单台主机（host_run）允许，效果同上但只影响该主机。

## 10. 执行引擎

### 10.1 实体与状态机

```
execution:  queued → running → success | partial | failed | cancelled | interrupted | skipped
host_run:   queued → running → success | failed | cancelled | interrupted
step_run:   queued → running → success | failed | cancelled | skipped
```

状态派生规则：

- 用户取消 → execution `cancelled`。
- 全部 host_run 成功 → `success`；全部失败 → `failed`；部分成功部分失败 → `partial`。
- 服务重启 → 重启时把 `queued`/`running` 的 execution/host_run/step_run 置 `interrupted`，`error_message="service restarted"`，不自动重跑。
- 定时任务因并发策略跳过 → 创建 execution 记录，`status=skipped`，`host_total=0`，`error_summary` 说明原因，便于在历史中看到"这次为什么没跑"。
- `partial` 只在主机维度出现；单主机内的步骤失败不产生 partial。

### 10.2 提交与快照

提交执行（tool/flow/adhoc）时在一个事务内：

1. 校验参数与目标主机（过滤停用主机；全部停用则拒绝）。
2. 解析工具最新版本、编译流程为 `plan_json`（阶段/步骤/工具版本/参数映射）。
3. 创建 execution、每个目标主机一条 host_run、全部 step_run（初始 `queued`）。
4. 返回 execution ID，HTTP 立即返回，不等待执行。

`plan_json` 是执行快照：即使工具/流程随后修改，历史执行仍能准确展示当时配置。敏感参数值存 `sensitive_params_enc`，展示时脱敏。

重跑（rerun）：使用原执行的参数值（含解密后的敏感参数）与主机集合创建新执行；工具/流程重新解析最新版本，不复制旧的 `plan_json`。

### 10.3 并发与排队

- 全局步骤信号量 `max_concurrent_steps`（默认 20），FIFO 等待队列。
- 主机执行槽位 `max_concurrent_host_runs`（默认 50），限制同时打开的 SSH 连接数。
- 每台主机跨执行独占：同一 host_id 同时只有一个 host_run 处于 running；其他等待。同一 host_run 内并行步骤不受此限。
- 单个 host_run 内，阶段串行；阶段内步骤并行，各自竞争全局步骤槽位。
- 100 台主机的单工具执行：50 个 host_run 并发连接，20 个步骤并发执行，其余排队。

### 10.4 实时输出（SSE）

- `GET /api/v1/executions/:id/stream`，支持 `Last-Event-ID`。
- 连接后先订阅 hub，再按 `after_seq` 回放 JSONL 中已有事件，按 seq 去重后继续推送实时事件；保证"先订阅后回放"不丢事件。
- 事件类型：`state`（execution/host_run/step_run 状态变更）、`log`（数据块，带 stream 标签）、`end`（终态）。
- 心跳注释每 20s 一次；EventSource 自动重连。
- 前端日志查看：stdout 正常色、stderr 红色；虚拟滚动；支持按主机/步骤过滤、暂停滚动、下载。
- 历史执行：`GET /executions/:id/events?after_seq=&limit=` 分页读取 JSONL；`GET /executions/:id/logs/download` 导出纯文本。

### 10.5 保留与清理

- 每天 03:00（配置时区）清理超过 `retention_days`（默认 90）的 execution 及其 host_runs、step_runs、JSONL 日志；transfer 元数据同期限；审计日志默认 180 天。
- 清理任务本身写审计（删除数量），单条审计不逐条记录被删执行。

## 11. 流程

### 11.1 模型

- 流程 = 有序阶段（stage）列表；阶段内多个步骤（step）并行；步骤引用工具 + 参数映射。
- 流程级参数声明与工具一致（含 sensitive）。
- `failure_policy`：`abort`（默认，任一阶段失败后不再开始后续阶段）或 `continue`（继续开始后续阶段）。
- `always_run` 阶段：即使之前阶段失败也执行（典型用于清理），但整体取消或流程超时后不再执行。
- 阶段内某步骤失败：同阶段其他步骤继续跑完，不互相中止；该阶段标记失败。
- 流程编辑 UI 为"阶段列表/泳道"，不是 DAG 画布。

### 11.2 参数映射

每个步骤对工具参数逐项配置：

- `flow`：引用流程参数（`${param_name}`），执行时取值。
- `literal`：固定值写入流程定义。
- 未配置的工具有默认值则用默认值；必填且未映射/无默认 → 保存流程时校验失败。

需要保密的固定值应声明为 sensitive 流程参数再引用，不写 literal。

### 11.3 校验

保存时：工具存在且启用、必填工具参数已满足、引用的流程参数存在、名称非空且全局唯一、阶段/步骤序号连续、`failure_policy` 合法、超时/重试范围合法。流程被定时任务引用时只能停用不能删除。

### 11.4 执行

提交时把流程编译为计划快照（解析最新工具版本）；每台主机一个 host_run，独立跑完整流程；主机之间互不影响。host_run 状态：所有已执行步骤成功 → `success`；任一步骤失败 → `failed`（`skipped` 步骤不影响）；取消 → `cancelled`。

## 12. 定时任务

- 目标：单个工具或单个流程；**固定主机集合**（保存时快照，不随分组/标签动态变化），执行时过滤停用主机。
- cron：标准 5 段表达式，按配置时区解析；界面同时提供"每 N 分钟 / 每小时 / 每天 X 点 / 每周几"的图形快捷方式，并预览未来 5 次触发时间。
- 并发策略：
  - `skip`：同一 schedule 已有 `queued`/`running` 的执行时，本次触发创建 `skipped` 执行记录并写审计。
  - `queue`：正常入队，按创建时间顺序执行。
- 错过触发：服务停机期间错过的触发不补跑；启动时重新计算 `next_run_at`。
- 手动"立即执行"：创建 `source=manual` 的执行（`schedule_id` 记录来源），不检查 skip 策略，但仍受主机锁与并发槽位限制。
- 参数：保存时填写，敏感值加密存储；修改后影响后续触发。
- 启用/停用即时生效（调度器重建该条目）；删除时保留历史执行。
- 通知配置：选择渠道（可多选）与触发条件（仅失败 / 总是），见第 15 节。

## 13. 临时命令

- 入口独立页面；输入 shell 命令（多行）、选择多台主机、可选超时（默认 300s）与 sudo。
- 执行管线与工具完全一致（单文件临时脚本 + env + 日志），`execution.kind=adhoc`，无工具引用。
- 命令正文保存在 execution（用于历史查看与重跑）；审计只记录命令摘要（前 200 字符）。
- 权限：operator 及以上。

## 14. 文件分发与下载

### 14.1 上传/分发

- 浏览器 multipart 上传到服务端临时目录（上限 512MB，配置），然后并发（≤10）SFTP 分发到选定主机。
- 选项：远端目录、文件名（默认原名）、权限（默认 0644）、覆盖策略（`overwrite` 默认 / `fail_if_exists`）、是否自动创建父目录（默认是）。
- 每个目标独立状态与字节进度；前端 1s 轮询任务详情（v1 不使用 SSE）。
- 支持取消：停止未开始目标，中断进行中传输，删除服务端临时文件。
- 任务结束删除临时文件；历史只保留元数据与结果。

### 14.2 下载

- 选择一台主机 + 远端路径 → 先 stat 展示大小，再流式下载（默认上限 200MB，超过拒绝并提示用 SFTP）。
- 每次下载生成一条 `direction=download` 的 transfer 记录（单目标）并写审计。
- 权限：operator 及以上。

## 15. 通知

- 渠道类型：通用 Webhook（POST JSON）、飞书机器人、钉钉机器人（加签）、企业微信机器人。
- 渠道配置中的 URL/密钥加密存储，只写不读；支持"测试发送"。
- v1 通知挂在**定时任务**上：选择渠道 + 触发条件（仅失败 / 总是）。手动执行不自动通知（后续版本可挂到流程）。
- 内容：任务名、执行结果、主机统计、开始/结束时间、失败主机前 5 台、执行详情链接（依赖 `base_url` 设置）。
- 投递：超时 10s，失败重试 3 次（1s/5s/25s 退避），每次投递写 `notify_deliveries`（含请求/响应摘要，脱敏）。
- 通知失败不影响执行结果，只记录投递状态。

## 16. 认证与安全

- 密码：bcrypt（cost 12）；长度 ≥8 且包含字母与数字；不强制轮换。
- 会话：服务端 `sessions` 表，token 仅存 SHA-256 哈希；Cookie `spool_session`，HttpOnly、SameSite=Lax、`Secure` 可配置（HTTPS 时开启）；有效期 12h；登录成功后重新生成 token（防会话固定）。
- CSRF：SameSite=Lax + 对修改类请求校验 `Origin`/`Referer` 与 Host 一致。
- 登录限流：按 IP+用户名，5 次失败锁定 5 分钟（内存计数，重启清零）。
- 初始账号：首次启动创建 `admin`，随机密码打印到 stdout，`must_change_password=true`。
- `must_change_password=true` 的用户，除 `GET /auth/me`、`PUT /auth/password`、`POST /auth/logout` 外，所有接口返回 403；前端强制跳转改密页。
- 管理员重置他人密码后，对方下次登录必须修改；admin 可强制注销某用户全部会话。
- 凭据/渠道密钥：AES-256-GCM，AAD=记录 ID；主密钥 0600；不写入日志、不返回接口。
- SQL 全部参数化；前端不使用 `dangerouslySetInnerHTML` 渲染脚本/日志；日志按纯文本转义。
- 安全响应头：CSP `default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'`、`X-Frame-Options: DENY`、`X-Content-Type-Options: nosniff`、`Referrer-Policy: same-origin`。
- TLS：支持配置证书直接启用 HTTPS，或由反向代理终止；`base_url` 用于通知链接与 Cookie Secure 判断。

## 17. 审计

记录动作：登录成功/失败/登出、主机增删改/测试/指纹更新、凭据增删改、工具创建/更新/回滚/删除/复制、流程增删改、定时任务增删改/启停/手动触发/跳过、执行提交/取消、传输提交/取消、用户增删改/重置密码/强制下线、渠道增删改/测试、系统设置修改、执行记录被保留策略清理（汇总）。

- 表只增不改；不提供编辑/删除 API；admin 可在界面筛选查询（时间、用户、动作、资源）。
- 摘要 JSON 中敏感值一律以 `******` 替换；不记录密码、凭据正文、敏感参数值。
- 保留 180 天（可配置），由清理任务删除。

## 18. API 约定

- 前缀 `/api/v1`，JSON；认证走 Cookie。
- 错误响应：`{"code":"validation_failed","message":"...","details":[...]}`；HTTP 状态语义化（400/401/403/404/409/422/500）。
- 分页：`page`（默认 1）、`page_size`（默认 20，最大 100）；响应 `{"items":[],"total":n,"page":1,"page_size":20}`。
- 时间：响应 ISO8601（带时区偏移，按配置时区展示）；存储 UTC 毫秒。
- 审计由中间件在成功修改后写入（含登录）。

主要资源：

```
POST   /auth/login | /auth/logout | GET /auth/me | PUT /auth/password
GET    /users | POST /users | PUT /users/:id | POST /users/:id/reset-password | DELETE /users/:id | POST /users/:id/logout-all
GET    /hosts | POST /hosts | GET/PUT/DELETE /hosts/:id
POST   /hosts/:id/test | PUT /hosts/:id/fingerprint
GET    /host-groups | POST /host-groups | PUT/DELETE /host-groups/:id
GET    /credentials | POST /credentials | PUT/DELETE /credentials/:id
GET    /tools | POST /tools | GET/PUT/DELETE /tools/:id | POST /tools/:id/copy
GET    /tools/:id/versions | GET /tools/:id/versions/:no | GET /tools/:id/diff?from=&to= | POST /tools/:id/rollback
GET    /flows | POST /flows | GET/PUT/DELETE /flows/:id | POST /flows/:id/copy
GET    /executions | POST /executions | GET /executions/:id | POST /executions/:id/cancel | POST /executions/:id/rerun
POST   /executions/:id/host-runs/:hostRunId/cancel
GET    /executions/:id/events?after_seq=&limit= | GET /executions/:id/stream (SSE) | GET /executions/:id/logs/download
GET    /schedules | POST /schedules | GET/PUT/DELETE /schedules/:id | POST /schedules/:id/run
GET    /transfers | POST /transfers/upload | GET /transfers/:id | POST /transfers/:id/cancel
GET    /transfers/download?host_id=&path=
GET    /notify-channels | POST /notify-channels | PUT/DELETE /notify-channels/:id | POST /notify-channels/:id/test
GET    /audit
GET    /settings | PUT /settings
GET    /version
```

执行列表筛选参数：`kind`、`status`、`ref_id`、`schedule_id`、`host_id`、`created_by`、`from`、`to`、`keyword`。

## 19. 前端信息架构

| 路由 | 页面 | 关键交互 |
|---|---|---|
| /login | 登录 | 错误提示、锁定提示 |
| / | 首页 | 最近执行列表、快捷入口（不做指标卡片） |
| /hosts | 主机列表 | 分组/标签/状态/关键字筛选，批量勾选后"执行工具/临时命令/分发文件" |
| /hosts/:id | 主机详情 | 探测结果、指纹、测试按钮、该主机执行历史 |
| /hosts/groups | 分组管理 | CRUD |
| /credentials | 凭据管理 | CRUD，密钥只写 |
| /tools | 工具列表 | 筛选、复制、启停 |
| /tools/:id | 工具详情/编辑器 | 脚本编辑、参数、准备命令、超时、sudo、版本历史与 diff、回滚、执行入口 |
| /flows | 流程列表 | 筛选、复制 |
| /flows/:id | 流程编辑器 | 阶段/步骤增删与拖拽、参数映射、失败策略、`always_run`、只读预览、执行入口 |
| /executions | 执行历史 | 按类型/状态/对象/操作人/时间筛选 |
| /executions/:id | 执行详情 | 总览、主机列表、步骤时间线、实时日志（SSE）、下载、重跑、取消 |
| /schedules | 定时任务列表 | 启停、下次运行、立即执行 |
| /schedules/:id | 定时任务编辑器 | cron 图形快捷 + 表达式、未来 5 次预览、主机选择、参数、并发策略、通知 |
| /adhoc | 临时命令 | 命令编辑、主机多选、超时/sudo |
| /files | 文件分发 | 上传、目标、选项、进度；"从主机下载"、历史 |
| /users | 用户管理（admin） | CRUD、重置密码、强制下线 |
| /settings | 系统设置（admin） | base_url、默认通知渠道、审计入口、关于 |
| /audit | 审计日志（admin） | 筛选、分页 |

通用组件：主机选择器（分组/标签树 + 搜索 + 多选 + 已选计数）、执行状态徽标、日志查看器（虚拟滚动、stdout/stderr 着色、过滤、暂停跟随）、参数表单（按声明动态渲染）、危险操作确认框。

## 20. 配置与部署

### 20.1 目录

```
/opt/spool/spool              二进制
/etc/spool/config.yaml        配置（0600）
/var/lib/spool/spool.db       SQLite（WAL）
/var/lib/spool/master.key     主密钥（0600，首次启动生成）
/var/lib/spool/logs/          执行事件日志
/var/lib/spool/tmp/           传输暂存
/var/lib/spool/spool.lock     单实例锁
```

### 20.2 配置示例

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
execution:
  max_concurrent_steps: 20
  max_concurrent_host_runs: 50
  default_step_timeout_sec: 300
  step_output_limit_mb: 20
  remote_base_dir: /tmp
  ssh_connect_timeout_sec: 10
  ssh_keepalive_sec: 15
  retention_days: 90
security:
  session_ttl_hours: 12
  login_max_failures: 5
  login_lockout_minutes: 5
  host_key_policy: tofu   # tofu | strict
transfer:
  max_upload_mb: 512
  max_download_mb: 200
  concurrency: 10
audit:
  retention_days: 180
notify:
  timeout_sec: 10
  retries: 3
```

运行时设置（`settings` 表，界面可改）：`base_url`（覆盖配置，用于通知链接）、`notify_default_channel_ids`。

### 20.3 部署与升级

- systemd 单元：独立运行用户、`LimitNOFILE`、`Restart=on-failure`、`KillMode=mixed`（保证优雅停机）。
- 升级：替换二进制 → 重启；启动自动执行迁移；停机时等待运行中步骤最多 30s，未完成则取消并标记 `interrupted`。
- 备份：停止写入（或使用 `sqlite3 .backup`）+ 复制 `spool.db`、`master.key`、`logs/`；文档说明密钥丢失等于凭据作废。
- 构建产物：`spool-linux-amd64`、`spool-linux-arm64`。

## 21. 非功能目标

| 项 | 目标 |
|---|---|
| 规模 | ≤100 台主机，同时 20 个远程步骤 |
| API 延迟 | 本机 p95 < 200ms（列表分页 ≤100 条） |
| 首行日志 | 执行开始后 1s 内在页面出现 |
| 100 台单工具扇出 | 平台侧调度与连接在 60s 内完成（不含脚本自身耗时） |
| 恢复 | 进程重启后数据一致，运行中执行标记 `interrupted`，不自动重跑 |
| 数据安全 | 凭据与敏感参数密文存储；接口不返回明文 |
| 可运维性 | 单二进制；备份=数据目录；迁移自动 |

## 22. 测试策略

- 单元测试（Go）：参数校验与 env 转义、AES-GCM 往返与 AAD 防换绑、cron 下次运行计算、执行状态派生、流程编译与校验、输出截断、JSONL 事件编解码、指纹变化判定。
- 集成测试：用 `gliderlabs/ssh` + `pkg/sftp` 服务端搭一个假 Linux 主机，端到端验证：上传、env 注入、stdout/stderr、退出码、超时、取消（进程组 kill）、清理、重试。SSH 指纹校验单独覆盖。
- Store 测试：临时 SQLite 文件 + 迁移，覆盖事务与级联删除。
- API 测试：httptest + 真实 store（内存/临时文件），覆盖认证/角色/审计写入。
- 前端：Vitest 覆盖参数表单、日志解析、状态徽标；Playwright 冒烟：登录 → 建主机（假 SSH）→ 运行工具 → 看到实时输出 → 历史可查。
- CI（GitHub Actions）：`go vet`、`go test ./...`、前端 `npm ci && npm run build`、`make build`。

## 23. 里程碑

| 里程碑 | 内容 | 验收 |
|---|---|---|
| M0 骨架 | 仓库初始化、Makefile、配置、store/迁移、登录/会话、SPA 壳、CI | 登录后可进首页；`go test` 与前端构建通过 |
| M1 主机与凭据 | 主机/分组/凭据 CRUD、加密、测试与探测、指纹、审计基础 | 添加真实主机可测通；凭据不可读回；审计有记录 |
| M2 工具与执行 | 工具/版本/参数、执行服务（SSH/SFTP/env/日志/状态机）、多主机执行、SSE 实时与历史、取消/超时/重试 | 假 SSH 主机端到端通过；真实主机跑通 Shell/Python 工具 |
| M3 流程 | 流程编辑器（后端+前端）、编译、阶段并行、失败策略、`always_run` | 多主机流程正确并行/中止/继续；历史可追溯版本 |
| M4 定时任务 | cron、并发策略、手动执行、未来时间预览、通知渠道与投递 | 定时触发、skip/queue 行为正确；失败通知送达 |
| M5 附加与收尾 | 临时命令、文件分发/下载、用户管理、设置、审计页面、打包与文档 | Playwright 冒烟通过；单二进制部署文档可用 |

## 24. 关键取舍记录

1. **单实例而非多实例**：100 台规模下单实例足够，换取部署与一致性简单；队列设计保留拆分空间。
2. **同一主机跨执行串行**：避免并发装包/改配置互相破坏；牺牲少量吞吐。
3. **流程不做条件分支/传值**：v1 用失败策略 + `always_run` 覆盖大部分场景，避免表达式引擎与画布复杂度。
4. **定时任务固定主机集合**：可预期优先，避免"新机器被自动纳入生产任务"的意外。
5. **凭据只写不读**：牺牲"运维想查看密码"的便利，换取不可导出。
6. **单文件脚本 + 准备命令**：牺牲复杂依赖打包，换分发模型简单可靠。
7. **日志 JSONL 单文件**：便于按序回放与 SSE 一致；存储开销略高于纯文本，下载时转换。
8. **通知只挂定时任务**：v1 聚焦无人值守场景，手动执行不打扰。
9. **文件分发独立于流程**：v1 不引入流程步骤类型，避免执行计划模型膨胀。
10. **不补跑错过的定时任务**：避免服务恢复时突发风暴；运维可手动立即执行。

## 25. 术语

- **执行（execution）**：一次用户/定时任务发起的批量操作，包含多台主机。
- **主机任务（host_run）**：某台主机上的一次完整执行（单工具或整套流程）。
- **步骤（step_run）**：主机任务中的一个脚本执行单元。
- **阶段（stage）**：流程中的一组并行步骤，阶段之间串行。
- **host lock**：同一主机跨执行的独占锁。
