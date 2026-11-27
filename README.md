# Spool

内网 Linux 运维平台：无 Agent、SSH 直连，把日常操作沉淀为带版本的脚本工具，支持多主机批量执行、实时输出、DAG 流程编排与定时任务。

> 当前状态：**M0–M5 核心功能完成**，并在其上完成 DAG 流程图编排、工具/流程分类、主机批量导入、执行历史增强与界面统一。设计文档见 [docs/superpowers/specs](docs/superpowers/specs/2026-11-26-spool-design.md)，待办需求见 [docs/backlog.md](docs/backlog.md)。

## 功能概览

### 主机与凭据

- 主机分组、标签、启用状态，按分组/标签/关键字筛选；左侧分组栏显示数量
- CSV 批量导入：模板下载、逐行校验、文件内重名检查、导入结果与错误行号；支持导出
- 凭据独立管理（密码/私钥），AES-256-GCM 加密存储，接口只写不读
- SSH 连通性测试与环境探测：系统、Bash、Python、SFTP、免密 sudo、setsid/timeout
- 主机指纹 TOFU 首次记录、指纹变化拒绝连接、管理员显式更新

### 工具与版本

- Shell/Python 单文件脚本；参数声明（类型/必填/默认/枚举/敏感）、准备命令、sudo、超时
- 每个工具内置一个必填的“执行主机”主机类型参数，可配置显示名称与默认主机；直接执行工具时默认回填该默认主机
- 参数以环境变量注入；敏感参数独立加密、不回显、不写审计
- 不可变版本：每次保存生成新版本，支持版本对比、回滚、删除版本、克隆、停用/删除工具
- 斜杠分类 `运维/部署/应用`，留空归入“默认”；工具列表左侧分类树（搜索/计数/最近访问）+ 右侧列表/卡片视图，支持按 Shell/Python 类型、状态筛选
- 最近访问按账号记录在浏览器本地（最多 6 个），不上传服务端
- CodeMirror 6 脚本编辑：Shell/Python 语法高亮、行号
- 工具详情聚合该工具的执行历史、版本列表、引用对象（流程/定时任务）与常用操作

### 执行

- 多主机批量执行，SSH/SFTP 下发脚本与环境文件，实时输出（SSE）、断线回放、历史回看、日志下载
- 工具/临时命令执行详情：左侧主机成功/失败/退出码，右侧按主机过滤的实时输出，打开即直出
- 取消、超时、步骤级重试；主机级状态与错误摘要
- SSH 连接或主机不可用会标记对应主机和步骤失败，并返回 `ssh_error` / `host_unavailable` 错误码；只有流程分支未触发的节点才标记为跳过
- 并发控制：全局步骤/主机上限、同一主机跨任务串行、单步输出 20MB 截断
- 状态：queued/running/success/partial/failed/cancelled/interrupted；服务重启后未完成执行标记 interrupted
- 临时命令：不建工具直接对一批主机执行 shell，保留执行记录与实时输出；历史支持状态/时间/命令搜索与服务端分页
- 执行历史按资产类型隔离：工具、流程、临时命令、定时任务各自独立；资产详情只显示自身记录

### 流程编排（DAG）

- React Flow 流程图编辑器：双击画布或从节点句柄拖到空白处直接建节点；节点即工具，连线即执行顺序
- 流程编辑按“基本信息 → 流程设计 → 参数设置”三步组织；超时、重试在节点配置中完成，执行主机与普通参数统一在第三步配置来源
- 条件分支：`always` / `success` / `failure` / `output_contains` / `output_not_contains`
- 并行分支：一个节点可有多条出边，满足条件的下游同时执行；汇合为 OR-join（任一入边触发即可执行）
- 失败被非 `always` 条件边匹配时视为已处理，不会让流程失败；未被处理的失败导致对应主机执行失败
- 参数按节点工具配置，来源支持工具默认值、固定值、执行时输入、引用上游节点参数；执行输入的输入名与显示标题流程内唯一，后续节点通过引用复用
- 执行时输入不继承工具默认值，执行表单默认留空；需要默认值时应选择“工具默认值”来源
- 引用在上游祖先节点间编译期解析，支持同类型参数复用；敏感值沿引用链传播，遵循同一套加密与脱敏规则
- 执行主机是工具内置的主机类型参数，来源支持工具默认值、固定主机、执行时选择、引用上游参数；节点只在其解析后的主机上执行，与其他参数遵循同一套引用和唯一性规则
- 旧流程如缺少执行主机参数配置，保存或执行时会提示补齐
- 支持流程最大执行时长；流程执行详情展示执行图，点击节点查看该节点的工具执行详情与输出
- 流程分类、复制、启停、删除
- 旧版阶段流程数据自动迁移为节点图

### 定时任务

- 5 段 cron + 常用周期，中文人类可读描述与下次运行时间预览
- 目标为工具或流程；工具始终使用任务指定的主机，流程使用流程内主机参数的值；并发策略（跳过/排队），手动立即执行
- 错过触发不补跑；支持启停与删除
- 独立的任务历史页：全部定时任务执行情况，单任务详情内也可查看自身历史

### 通知、文件与系统管理

- 通知：Webhook / 飞书 / 钉钉 / 企业微信机器人，失败或每次都通知，加签密钥加密，投递重试与投递记录
- 文件分发：上传文件并并发 SFTP 分发到多台主机（目录/文件名/权限/覆盖策略/进度/取消）
- 文件下载：从主机流式下载
- 用户：管理员/操作员/只读三级角色，创建/编辑/停用、重置密码（强制改密）、强制下线、删除，保护最后一个管理员
- 审计日志：筛选查询人为操作（登录、主机、凭据、工具、流程、执行、定时任务管理/手动触发、通知、用户、传输等）；自动调度执行不写入审计

### 界面与使用

- 一级/二级侧边栏，默认只展开当前分区；工作台提供统计、近 7 天执行趋势（悬浮查看每日明细）、快捷操作与可筛选的执行历史
- 资源统一为“列表 → 详情（历史/版本/引用）→ 编辑（配置）”：工具、流程、定时任务结构一致
- 统一 PageHeader（面包屑/标题/操作）、表格行内文字链 + “更多”菜单、浅色状态标签
- 列表支持搜索、状态与时间范围过滤、分页；不同资产的执行历史分开查看

## 技术栈

- 后端：Go 1.26、Gin、sqlx、goose、SQLite（modernc，纯 Go，无 CGO）、bcrypt、log/slog、robfig/cron、x/crypto/ssh、pkg/sftp
- 加密：AES-256-GCM；凭据与敏感参数使用独立主密钥 `master.key`（0600）
- 前端：React 18、TypeScript、Vite、Ant Design 5、TanStack Query、Zustand、React Router、CodeMirror 6、@xyflow/react
- 交付：前端构建产物 `go:embed` 进二进制，单文件部署

## 本地开发

```bash
# 后端（默认监听 :8080，数据目录 ./data）
make dev-api

# 前端（Vite dev server，:5173，代理 /api 到 :8080）
make dev-web
```

首次启动会在后端 stdout 打印初始管理员账号与随机密码，登录后强制修改。

可选：复制配置模板后按需修改。

```bash
cp config.example.yaml config.yaml
```

## 构建

```bash
make build          # 产物 bin/spool，前端已嵌入
make test           # go test ./... + 前端 vitest
```

`make build` 会先构建前端并复制到 `internal/web/dist`，再以 `CGO_ENABLED=0` 编译单二进制。

## 部署

```bash
useradd --system --home /var/lib/spool --shell /usr/sbin/nologin spool
install -d -o spool -g spool -m 0755 /opt/spool /var/lib/spool /etc/spool
install -o spool -g spool -m 0755 bin/spool /opt/spool/spool
install -o spool -g spool -m 0600 config.example.yaml /etc/spool/config.yaml
install -m 0644 deploy/spool.service /etc/systemd/system/spool.service
systemctl daemon-reload && systemctl enable --now spool
```

初始密码：

```bash
journalctl -u spool -n 50 | grep "Initial password"
```

首次登录后必须修改密码。

数据目录默认 `/var/lib/spool`：

```
spool.db      SQLite（WAL）
master.key    凭据/敏感参数加密主密钥（0600）
logs/         执行日志（JSONL，按执行 ID）
tmp/          传输暂存
spool.lock    单实例锁
```

备份：使用 `sqlite3 /var/lib/spool/spool.db ".backup '/backup/spool.db'"`，并复制 `logs/` 与 `master.key`。**丢失 `master.key` 会导致所有已存凭据与敏感参数作废**，需与数据库一并妥善备份。

## 配置

完整示例见 [config.example.yaml](config.example.yaml)。

| 键 | 默认 | 说明 |
|---|---|---|
| `server.listen` | `:8080` | 监听地址 |
| `server.base_url` | 空 | 对外访问地址，用于通知链接与 Origin 校验 |
| `server.cookie_secure` | `false` | HTTPS 部署时置 `true` |
| `server.tls.cert/key` | 空 | 同时配置则直接启用 HTTPS |
| `data_dir` | `./data` | 数据目录 |
| `timezone` | `Asia/Shanghai` | 展示与调度时区 |
| `logging.level` | `info` | debug/info/warn/error |
| `security.session_ttl_hours` | `12` | 会话有效期 |
| `security.login_max_failures` | `5` | 登录失败锁定阈值 |
| `security.login_lockout_minutes` | `5` | 锁定时长 |
| `security.host_key_policy` | `tofu` | 主机指纹策略（tofu/strict） |
| `execution.max_concurrent_steps` | `20` | 全局并行步骤上限 |
| `execution.max_concurrent_host_runs` | `50` | 全局并行主机执行上限 |
| `execution.default_step_timeout_sec` | `300` | 默认步骤超时 |
| `execution.step_output_limit_mb` | `20` | 单步输出截断阈值 |
| `execution.retention_days` | `90` | 执行记录保留天数（清理任务待实现） |
| `transfer.max_upload_mb` | `512` | 单次分发文件大小上限 |
| `transfer.max_download_mb` | `200` | 单次下载文件大小上限 |
| `transfer.concurrency` | `10` | 分发并发数 |

环境变量可覆盖：`SPOOL_LISTEN`、`SPOOL_BASE_URL`、`SPOOL_DATA_DIR`、`SPOOL_LOG_LEVEL`、`SPOOL_TIMEZONE`。

## 安全说明

- 会话为服务端会话表 + HttpOnly Cookie（SameSite=Lax），登录后重新生成 token
- 修改类请求校验 Origin，跨站请求直接拒绝
- 密码 bcrypt cost 12；连续失败锁定
- 凭据与敏感参数只写不读，接口不回显明文；审计不记录敏感值
- 响应包含 CSP、X-Frame-Options、nosniff 等安全头
- 生产环境建议启用 HTTPS（直接配置证书或反向代理），并设置 `server.cookie_secure: true`

## 目录结构

```
cmd/spool                启动与装配
internal/config          配置
internal/store           SQLite、迁移
internal/auth            密码、会话、引导管理员
internal/audit           审计写入
internal/credential      凭据加密与主密钥
internal/host            主机、分组、SSH 探测
internal/tool            工具与版本
internal/flow            流程 DAG、参数来源与引用解析、编译
internal/exec            执行引擎、图调度、SSE 事件、日志
internal/schedule        定时任务
internal/notify          通知渠道与投递
internal/transfer        文件分发/下载
internal/api             HTTP 路由与中间件
internal/web             前端产物嵌入
web/                     React 应用
docs/                    设计文档、调研与待办
deploy/                  systemd 单元
```

## 待办需求

### MCP 接入（暂不实施）

目标：把平台大部分能力以 MCP 工具暴露，任意支持 MCP 的 Agent 接入后能自助发现并理解平台能力；页面提供“接入配置 + 已实现能力”文档，配置可直接复制。

暂定范围：

- MCP Server（Streamable HTTP + JSON-RPC），提供 `initialize`、`tools/list`、`tools/call`，并给出平台说明与工具级 JSON Schema
- 能力覆盖主机查询、工具/版本/参数查询、工具执行与状态/日志跟踪、流程查询与执行、定时任务查询/触发/历史、执行历史/取消/重跑、临时命令等；写操作按角色与 scope 限制
- 独立访问 Token（绑定用户与角色、只显示一次、可吊销、记录最近使用），所有 MCP 调用按真实用户写审计
- 页面：接入地址与 Token 管理、Claude Desktop/Cursor/Cline 等可直接复制的配置、能力清单（名称/用途/权限/参数/示例）与联调状态
- 工具描述遵循 MCP 注解（只读/破坏性提示），敏感字段永不返回

当前不做实现，仅作为待办；详细设计、能力矩阵与分阶段验收见 [docs/backlog.md](docs/backlog.md)。

**对当前开发的要求**：新增能力优先放在 service 层并保持与 HTTP/UI 解耦，参数校验、权限、审计、稳定 JSON DTO 可被未来的 MCP 层复用；长任务保持“创建返回 ID + 查询状态/日志”的模式。

### 工具执行导入导出（暂不实施）

支持按条件导出工具执行记录与结果（可选日志），并评估导入历史记录或批量执行任务的方案；导入导出均需处理权限、敏感参数脱敏、逐行校验与异步大文件。具体方向、格式与字段待确认，详见 [docs/backlog.md](docs/backlog.md)。当前不实现接口与页面。

### 平台环境变量管理（暂不实施）

平台统一维护环境变量，工具执行和流程中的工具节点自动注入，供脚本通过 `$VAR` / `os.environ` 读取；支持敏感值加密、只写不读、审计不记录值。优先级、临时命令是否注入、保留变量名与作用域边界待确认，详见 [docs/backlog.md](docs/backlog.md)。当前不实现。

## 路线图

| 里程碑 | 内容 |
|---|---|
| M0（已完成） | 骨架、登录、会话、审计基础、前端外壳、CI |
| M1（已完成） | 主机、分组/标签、凭据加密、连通性测试与环境探测、指纹 |
| M2（已完成） | 工具与不可变版本、参数、SSH/SFTP 执行、实时输出（SSE）、历史、取消/超时/重试 |
| M3（已完成） | 流程：DAG 节点图、条件/并行分支、节点参数来源与引用、执行图与节点输出 |
| M4（已完成） | 定时任务（cron、并发策略）、通知渠道 |
| M5（已完成） | 临时命令、文件分发/下载、用户管理、审计页面、打包 |
| M5+（已完成） | 工具/流程分类、批量导入、执行历史隔离与增强、界面统一 |
| M6（待排期） | MCP Server、能力清单、接入配置页 |
| 待排期 | 工具执行记录/结果导入导出 |
| 待排期 | 平台环境变量管理与工具注入 |
| 收尾（待排期） | 执行记录自动清理、运行时设置页、前端分包与日志虚拟滚动 |
