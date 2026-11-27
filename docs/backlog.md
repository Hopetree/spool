# 待办需求

本文件记录尚未排期、但已明确方向的需求。当前只做设计约束，不实现。

## MCP 接入

**状态：待排期（等平台核心功能完善后再实施）**

### 背景与目标

希望任意支持 MCP 的 Agent（Claude Desktop、Cursor、Cline、自研 Agent 等）接入 Spool 后，能够：

1. 自助理解 Spool 是什么、能做什么、如何调用；
2. 查询主机、工具、流程、定时任务与执行记录；
3. 发起工具/流程/临时命令执行，并跟踪状态、读取输出；
4. 在权限范围内完成日常运维操作，所有动作可审计、可追溯。

同时平台需要一个“MCP 接入”页面，展示接入配置与已实现能力，配置可直接复制，能力说明与 MCP Server 实际暴露的 `tools/list` 保持一致，相当于一份可交互的接入文档。

MCP 功能本身暂不实现；当前开发只需保证后续能平滑接入。

### 范围（暂定）

#### 读取类能力

| MCP 工具（暂定名） | 说明 | 最低角色 |
|---|---|---|
| `spool_overview` | 平台概览、当前用户、可用能力摘要 | viewer |
| `hosts_list` / `hosts_get` | 主机、分组、标签、状态、环境能力；不含凭据明文 | viewer |
| `host_groups_list` | 主机分组与数量 | viewer |
| `credentials_list` | 凭据元数据（名称、用户名、类型），绝不含密码/私钥 | viewer |
| `tools_list` / `tools_get` | 工具、分类、参数定义、当前版本 | viewer |
| `tools_versions_list` | 版本列表与版本内容（敏感参数定义脱敏） | viewer |
| `tools_references` | 工具被哪些流程/定时任务引用 | viewer |
| `flows_list` / `flows_get` | 流程、分类、节点与连线、参数定义 | viewer |
| `schedules_list` / `schedules_get` | 定时任务、cron、目标、下次运行 | viewer |
| `schedules_next_runs` | 下次运行时间预览 | viewer |
| `executions_list` / `executions_get` | 按资产类型、状态、时间、关键字查询执行与主机结果 | viewer |
| `executions_events` | 分页读取执行事件/输出，支持按主机与节点过滤 | viewer |
| `executions_log_url` | 获取日志下载地址或内容（受大小限制） | viewer |
| `transfers_list` / `transfers_get` | 文件分发记录与状态 | viewer |
| `audit_list` | 审计日志查询 | admin |
| `users_list` | 用户列表（不含密码哈希/会话） | admin |

#### 执行与操作类能力

| MCP 工具（暂定名） | 说明 | 最低角色 |
|---|---|---|
| `tools_run` | 选择工具、主机集合、参数并发起执行，返回 execution_id | operator |
| `flows_run` | 选择流程、主机集合、流程参数并发起执行，返回 execution_id | operator |
| `adhoc_run` | 对一批主机执行临时 shell 命令，返回 execution_id | operator |
| `executions_cancel` | 取消执行或单主机执行 | operator |
| `executions_rerun` | 重跑历史执行 | operator |
| `executions_wait` | 在限定时间内等待执行结束并返回摘要（可选，避免 Agent 自行轮询） | operator |
| `schedules_run` | 手动触发定时任务 | operator |
| `hosts_test` | SSH 连通性与环境探测 | operator |
| `tools_create` / `tools_update` / `tools_copy` | 创建/更新版本/克隆工具 | admin |
| `tools_enable` / `tools_delete` | 启停/删除工具 | admin |
| `flows_create` / `flows_update` / `flows_copy` | 创建/更新/克隆流程 | admin |
| `schedules_create` / `schedules_update` / `schedules_enable` / `schedules_delete` | 定时任务管理 | admin |
| `hosts_create` / `hosts_update` / `hosts_import` | 主机管理（凭据只引用 ID） | admin |
| `transfers_upload` | 文件分发（后续单独评估大文件/二进制传输方式） | operator |

原则：先只读、再执行、最后管理类写操作；删除、批量执行、文件分发等高风险操作必须显式授权并写入审计。用户、凭据密钥、会话等安全敏感数据默认不通过 MCP 暴露写能力。

### MCP Server 设计要点

- 传输：Streamable HTTP + JSON-RPC 2.0，端点暂定 `/mcp`；请求无状态，便于测试和未来扩展。
- 发现：`initialize` 返回平台说明与使用指引；`tools/list` 提供完整 JSON Schema、中文描述、调用示例、权限要求与只读/破坏性注解；可用 MCP resources 提供快速上手、参数说明、错误码等文档。
- 结果：优先返回结构化 `structuredContent`；执行类工具统一返回 `execution_id`，状态与日志通过查询工具获取，避免长时间阻塞。
- 错误：机器可读错误码 + 可操作的中文提示（如权限不足、主机离线、参数缺失、执行不存在）。
- 幂等：创建/触发类工具接受 `idempotency_key`，避免 Agent 重试造成重复执行。
- 限流：按 Token 限制请求频率与并发执行数；大输出分页/截断，完整日志通过下载资源获取。
- 审计：每次 `tools/call` 记录真实用户、Token、客户端信息、工具名、参数摘要（敏感值脱敏）、结果状态；审计来源标记为 `mcp`。

### 认证与权限

- 使用独立 MCP Token（Bearer），不使用浏览器 Cookie；Token 绑定用户，只在创建时显示一次，数据库存哈希。
- Token 具备 scope：`read` / `execute` / `admin`，并与用户角色取交集；可设置过期时间、随时吊销。
- 记录 `last_used_at`、最近使用 IP/客户端，支持一键吊销全部 Token。
- 只读 Token 不能执行；viewer 角色即使持有 execute scope 也不能执行。
- 危险操作（删除、批量执行、文件分发）要求 admin 角色或显式高危 scope。
- 后续可在 service 层统一鉴权，避免只依赖 HTTP 中间件。

### 接入页面

位置：系统管理 → MCP 接入（暂定）。

内容：

1. **启用与端点**：MCP 开关、接入地址（`base_url` + `/mcp`）、协议版本、健康状态。
2. **Token 管理**：创建（名称/scope/过期时间）、一次性复制、吊销、最近使用。
3. **复制接入配置**：提供 Claude Desktop、Cursor、Cline 与通用 JSON 的配置片段，一键复制；含认证头示例。
4. **能力文档**：按读取/执行/管理分组展示工具名称、用途、所需角色、参数 JSON Schema、示例调用与返回示例；数据直接来自 MCP 工具注册表，保证与 `tools/list` 一致。
5. **快速验证**：展示 `initialize` / `tools/list` 的示例请求与响应，便于联调；可选“测试连接”。

### 当前开发需要遵守的约束

MCP 尚未实现，但后续接入不应重写业务逻辑。当前功能开发遵循：

1. **业务逻辑放 service 层**：HTTP handler 只做绑定、鉴权、响应；新增能力不依赖 `gin.Context`、Cookie、CSRF 等 Web 概念。
2. **输入校验放 service 层**：提供可复用的输入结构与 `Validate`，HTTP 与未来 MCP 共用同一套校验。
3. **稳定 DTO 与错误码**：返回稳定 ID、RFC3339 时间、明确枚举；错误包含机器可读 code，避免只返回展示文案。
4. **权限可复用**：明确每个操作所需角色；逐步将权限判断从纯 HTTP 中间件下沉到可复用的授权层。
5. **审计统一**：所有写操作通过统一审计入口，保留 actor、来源、资源与结果；未来增加 `source=web|mcp` 与客户端信息。
6. **敏感数据边界**：凭据/敏感参数永不回显；MCP 工具描述与返回中不得出现密钥；敏感参数传入时也要脱敏审计。
7. **长任务模式**：创建执行立即返回 ID，状态/事件/日志通过查询接口获取；保持现有 SSE + 事件表能力可供 MCP 轮询或订阅。
8. **能力清单同步**：新增平台能力时，在本文件的能力矩阵中补充“是否计划 MCP 暴露、所需权限、工具名”，避免上线时遗漏。

### 非目标（当前）

- 不做 MCP 的 UI 自动化/浏览器控制能力；
- 不通过 MCP 暴露监控能力（平台本身不含监控）；
- 不允许 MCP 读取凭据明文或会话 Token；
- 不在本期实现多租户或跨平台联邦。

### 分阶段实施（未来）

| 阶段 | 内容 | 验收 |
|---|---|---|
| M6-0（现在） | 仅保留设计与开发约束，不写 MCP 代码 | 新功能满足 service 层复用要求 |
| M6-1 | Token、MCP Server 骨架、只读工具、审计、接入页文档 | Agent 可初始化、发现并查询主机/工具/流程/执行 |
| M6-2 | 执行类工具、状态/日志跟踪、取消/重跑、幂等与限流 | Agent 可发起执行并获取完整结果，审计可追溯 |
| M6-3 | 管理类写工具、文件分发、危险操作注解与权限细化 | 权限矩阵测试通过 |
| M6-4 | 客户端兼容性、协议一致性测试、文档与示例完善 | Claude Desktop/Cursor/Cline 至少各完成一次真实联调 |

### 验收标准（整体）

- 任意标准 MCP 客户端凭 Token 可完成：初始化 → 查看能力 → 查询资源 → 发起执行 → 跟踪状态/读取输出。
- 无权限、过期/吊销 Token、越权角色均被明确拒绝，且不泄露资源是否存在等敏感信息。
- 所有 MCP 调用按真实用户写入审计，来源标记正确，敏感参数不落日志。
- 接入页复制出的配置可直接使用；页面能力清单与 `tools/list` 完全一致。
- 凭据明文、敏感参数、用户密码哈希在任何 MCP 响应中均不可见。
