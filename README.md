# Spool

内网 Linux 运维平台：无 agent、SSH 直连，把日常操作沉淀为带版本的脚本工具，支持多主机批量执行、实时输出、流程编排与定时任务。

> 当前状态：**M5 核心功能完成**。登录/审计、主机/凭据/指纹、工具与版本、多主机执行与实时输出、流程编排、定时任务与通知、临时命令、文件分发/下载、用户管理、审计日志均已实现。设计文档见 [docs/superpowers/specs](docs/superpowers/specs/2026-11-26-spool-design.md)。

## 已实现功能（M0-M5）

- 本地账号登录、服务端会话、初始密码强制修改、登录限流
- 操作审计：登录、凭据、主机、分组、连通性测试、指纹更新
- 凭据：密码/私钥，AES-256-GCM 加密，接口只写不读
- 主机：分组、标签、启用状态、按分组/标签/关键字筛选
- SSH 连通性测试与环境探测：系统、Bash、Python、SFTP、免密 sudo、setsid/timeout
- 主机指纹：TOFU 首次记录、指纹变化拒绝连接、管理员显式更新
- 工具：Shell/Python 单文件脚本、参数声明（类型/必填/默认/枚举/敏感）、准备命令、sudo、不可变版本、版本对比/回滚/删除、克隆与停用
- 工具分类：`运维/部署/应用` 形式的目录层级，可留空归入“默认”；工具列表左侧分类树 + 搜索，右侧列表
- 工具详情：执行历史（仅该工具的直接执行）、版本列表、引用对象（流程/定时任务）、克隆、删除工具/版本
- 执行历史按资产类型隔离：工具执行历史、流程执行历史、临时命令历史各自独立；工具/流程详情内只显示自身记录
- 侧边栏采用一级/二级菜单：主机与凭据、工具、流程、自动化、文件分发、系统管理
- 执行：多主机批量、SSH/SFTP 下发、环境变量注入、实时输出（SSE）、历史回看与日志下载、取消/超时/步骤级重试
- 并发控制：全局步骤与主机执行上限、同一主机跨任务串行；单步输出 20MB 截断
- 执行状态：queued/running/success/partial/failed/cancelled/interrupted，服务重启后标记 interrupted
- 流程：阶段串行、阶段内步骤并行、流程参数与工具参数映射（流程参数/固定值/默认值）、失败策略（中止/继续）、`always_run` 清理阶段、单机最大时长、流程复制与启停、流程执行/重跑
- 定时任务：5 段 cron + 常用周期、固定主机集合、并发策略（跳过/排队）、手动立即执行、下次运行预览、错过触发不补跑、启停与删除
- 通知：Webhook / 飞书 / 钉钉 / 企业微信机器人，失败或每次都通知，加签密钥加密存储，投递重试与投递记录
- 临时命令：不建工具直接对一批主机执行 shell，保留执行记录/实时输出，可重跑
- 文件分发：上传文件并并发 SFTP 分发到多台主机（目录/文件名/权限/覆盖策略/进度/取消），支持从主机流式下载
- 用户管理：创建/编辑/停用用户、重置密码（强制改密）、强制下线、删除；保护最后一个管理员
- 审计日志：筛选查询登录、主机、凭据、工具、流程、执行、定时任务、通知、用户、传输等操作

## 技术栈

- 后端：Go 1.26、Gin、sqlx、goose、SQLite（modernc，纯 Go，无 CGO）、bcrypt、log/slog
- 前端：React 18、TypeScript、Vite、Ant Design 5、TanStack Query、Zustand、React Router
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
logs/         执行日志（后续里程碑）
tmp/          传输暂存（后续里程碑）
spool.lock    单实例锁
```

备份：使用 `sqlite3 /var/lib/spool/spool.db ".backup '/backup/spool.db'"`，并复制 `logs/`。后续里程碑会新增 `master.key`（凭据加密主密钥），**丢失后所有已存凭据作废**，需一并备份且妥善保管。

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

环境变量可覆盖：`SPOOL_LISTEN`、`SPOOL_BASE_URL`、`SPOOL_DATA_DIR`、`SPOOL_LOG_LEVEL`、`SPOOL_TIMEZONE`。

## 安全说明

- 会话为服务端会话表 + HttpOnly Cookie（SameSite=Lax），登录后重新生成 token
- 修改类请求校验 Origin，跨站请求直接拒绝
- 密码 bcrypt cost 12；连续失败锁定
- 响应包含 CSP、X-Frame-Options、nosniff 等安全头
- 生产环境建议启用 HTTPS（直接配置证书或反向代理），并设置 `server.cookie_secure: true`

## 目录结构

```
cmd/spool                启动与装配
internal/config          配置
internal/store           SQLite、迁移
internal/auth            密码、会话、引导管理员
internal/audit           审计写入
internal/api             HTTP 路由与中间件
internal/web             前端产物嵌入
web/                     React 应用
docs/superpowers/        设计文档与实施计划
deploy/                  systemd 单元
```

## 后续可选收尾

- 执行/传输记录与日志的自动保留清理（设计默认 90 天）
- 运行时设置页（base_url、默认通知渠道）
- 前端代码分包与更细的日志虚拟滚动

## 路线图

| 里程碑 | 内容 |
|---|---|
| M0（已完成） | 骨架、登录、会话、审计基础、前端外壳、CI |
| M1（已完成） | 主机、分组/标签、凭据加密、连通性测试与环境探测、指纹 |
| M2（已完成） | 工具与不可变版本、参数、SSH/SFTP 执行、实时输出（SSE）、历史、取消/超时/重试 |
| M3（已完成） | 流程：阶段并行、参数映射、失败策略、always_run |
| M4（已完成） | 定时任务（cron、并发策略）、通知渠道 |
| M5（已完成） | 临时命令、文件分发/下载、用户管理、审计页面、打包 |
