# Spool UI/UX 参考调研：开源运维与工作流平台

> 调研日期：2026-11-26
> 目的：为 Spool 的页面设计与排版优化提供参考，重点看「无 Agent / SSH 直连」和「脚本 → 流程 → 定时任务 → 执行历史」这条主链路。

## 1. 结论摘要

Spool 的功能骨架已经完整，当前主要差距不在功能，而在**信息层级和页面编排**：

1. **缺少统一的页面头部**：列表页大多直接是工具栏 + 表格，没有页面标题/面包屑，进入后缺少位置感；参考项目几乎都有「面包屑 + 标题 + 主操作」这一行。
2. **操作按钮过重**：列表行内 4 个带边框按钮会撑高行高、分散注意力；常见做法是文字链（编辑/删除）+「更多」下拉。
3. **筛选与表格混在一起**：参考项目通常把筛选条做成独立卡片，表格卡片内只放「新建 / 状态切换 / 刷新」工具栏。
4. **执行详情可以更有层次**：Kestra 的「状态摘要 + 元数据 chips + 页签（拓扑/甘特/日志）」是同类产品里最清晰的结构；Spool 已有流程图 + 节点面板，可以再补「概览信息」和「全部节点日志/耗时」视图。
5. **日志查看器缺少基础工具**：搜索、复制、下载、换行/时间戳切换、行号，是同类产品的标配。
6. **首页缺少趋势**：目前是统计卡 + 最近执行；参考项目会放执行趋势、成功率、即将运行的定时任务、主机健康度。
7. **状态标签偏重**：实心绿/红标签视觉噪音大；参考项目多用浅底 + 圆点/勾选图标的轻量 pill。
8. **导航可折叠**：Spug/Kestra/Argo 都支持侧边栏收成图标栏；Spool 目前固定 220px。

## 2. 调研对象对比

| 项目 | 技术栈 | 与 Spool 重合的能力 | 最值得借鉴的页面 |
| --- | --- | --- | --- |
| **Spug** | Python/Django + React | 无 Agent 主机管理、批量执行、在线终端、文件分发、应用发布、流水线、任务计划、监控/告警、凭据、RBAC/LDAP | 主机列表（左树右表）、执行任务（表单 + 执行记录双栏）、流水线、任务计划 |
| **Semaphore UI** | Go + Vue | Inventory、Task Template、Schedule、执行历史，Ansible/Terraform/PowerShell | 极简项目侧栏、表格内联 Run、任务历史 |
| **Windmill** | Rust + Svelte | 多语言脚本、DAG Flow、Schedule/Webhook、实时日志、Worker、Git 同步 | 脚本编辑器、Flow 编辑器、Runs 列表与单次运行日志 |
| **Kestra** | Java + Vue | YAML 声明式 Flow、DAG 拓扑、触发器、执行历史、Gantt/日志 | 执行详情（状态头 + 页签 + 拓扑/甘特/日志） |
| **Argo Workflows** | Go + React | Kubernetes 原生 DAG、节点状态、节点详情/日志 | 全屏 DAG、圆形状态节点、画布悬浮工具栏 |
| **AWX / Ansible Controller** | Python + React | Inventory、Job Template、Workflow、审批节点、RBAC | Workflow Visualizer（节点图 + 右侧详情） |
| **JumpServer** | Python + Vue | 堡垒机：资产树、账号、会话录像、命令审计、Web 终端 | 资产树导航、会话/审计列表 |
| **mayfly-go** | Go + Vue | 机器管理、脚本执行、Cron、文件管理、数据库运维 | 资源树统一入口、机器终端/文件管理器 |
| **Rundeck** | Java + Vue | 项目/节点、Job、Workflow 步骤、执行历史、ACL | Job 定义页、执行跟随页（按节点看日志） |
| **n8n**（编辑体验参考） | TypeScript + Vue | 节点式流程编辑器、执行视图 | 画布双击加节点、节点面板、执行时节点高亮 |

## 3. 各项目界面观察

### 3.1 Spug（最接近的功能形态）

实拍页面（本次调研已登录官方 demo）：

- 工作台：`/tmp/spool-ui-ref/spug-workbench.png`
- 主机管理：`/tmp/spool-ui-ref/spug-hosts.png`
- 执行任务：`/tmp/spool-ui-ref/spug-exec-task.png`
- 流水线：`/tmp/spool-ui-ref/spug-pipeline.png`
- 流水线编辑器：`/tmp/spool-ui-ref/spug-pipeline-editor.png`
- 执行记录：`/tmp/spool-ui-ref/spug-pipeline-records.png`
- 任务计划：`/tmp/spool-ui-ref/spug-schedules.png`

观察：

- **外壳**：深色侧栏（图标 + 文字 + 二级菜单箭头）+ 顶部工具条（意见反馈、通知、消息、语言、用户）+ 面包屑。Spool 已去掉顶部 Header，可以只借鉴面包屑，不恢复整条工具条。
- **列表页三段式**：页面标题行（标题 + 主操作）→ 筛选条 → 表格卡片。主机页是「左分组树卡片 + 右表格卡片」，表格工具栏含搜索、新建下拉、导出、验证、`全部/未验证` 分段筛选、刷新、列设置、全屏。
- **行内操作是文字链**：`编辑 / 删除` 蓝色/红色文字，不用按钮；操作多时用「更多」下拉。行高明显更小。
- **状态是浅色标签**：`已验证` 浅绿底、`未激活` 浅灰底，视觉很轻。
- **执行任务页**：左侧表单（目标主机、Shell/Python 切换、代码编辑器、开始执行），右侧「执行记录」常驻，提交后记录直接出现在右边，不用跳页。
- **流水线编辑器**：节点是较大的白色卡片（图标 + 名称），垂直排列，画布无网格；顶部只有「执行/返回列表」。分支能力弱于 Spool，但单链路可读性好。
- **执行记录**：右侧抽屉内表格（序号/状态/触发方式/执行人/开始时间/耗时/操作），而不是跳转新页面。
- **任务计划**：筛选卡片 + 列表卡片；操作是 `详情 / 编辑 / 更多`；状态用「全部/已激活/未激活」分段筛选。

远程参考图：
- https://raw.githubusercontent.com/openspug/spug/4.0/docs/img/4.0/en/host.jpg
- https://raw.githubusercontent.com/openspug/spug/4.0/docs/img/4.0/en/host-exec.jpg
- https://raw.githubusercontent.com/openspug/spug/4.0/docs/img/4.0/en/pipeline.jpg
- https://raw.githubusercontent.com/openspug/spug/4.0/docs/img/4.0/en/pipeline-console.jpg
- https://raw.githubusercontent.com/openspug/spug/4.0/docs/img/4.0/en/schedule.jpg

### 3.2 Kestra（执行详情的最佳参考）

截图：

- 执行列表：`/tmp/spool-ui-ref/kestra-executions-overview-small.png`
- 执行详情（Overview）：`/tmp/spool-ui-ref/kestra-execution-results-overview-small.png`
- Gantt：`/tmp/spool-ui-ref/kestra-execution-gantt-view-small.png`
- 日志：`/tmp/spool-ui-ref/kestra-execution-logs-view-small.png`
- 欢迎页/侧栏：`/tmp/spool-ui-ref/kestra-welcome-small.png`

观察：

- **面包屑 + 页签**：`Executions / namespace / flow / execution-id`，页签 `Overview / Gantt / Logs / Input/Output / Metrics / Dependencies / Audit Logs`。
- **状态头**：左上大号状态（Success 绿色），旁边流程名、执行 ID（可复制）、命名空间、调度、相对时间；下方一排元数据 chips（团队、类型、触发来源、correlation id、revision、duration、attempts、task 数）。
- **拓扑**：深色画布，节点是带图标的胶囊卡，旁边有耗时；触发器单独一组；边是细虚线，整体非常克制。
- **Gantt**：每个任务一行，时间条 + attempt/status chip + 最近一条日志，可展开；适合快速定位慢任务。
- **Logs**：按任务分组，组头有状态/耗时，组内日志行；支持级别过滤、Collapse all、Temporal view、复制/下载。
- **列表页**：顶部执行趋势面积图 + 紧凑表格（ID/开始/结束/耗时/命名空间/Flow/Labels/State/Triggers），状态是浅色 pill。

远程参考图：
- https://kestra.io/cdn-cgi/image/onerror=redirect,width=3020,height=1660,format=auto/_astro/executions-overview.CDoBoPZy.png
- https://kestra.io/cdn-cgi/image/onerror=redirect,width=3020,height=1660,format=auto/_astro/execution-results-overview.DA0zoky4.png
- https://kestra.io/cdn-cgi/image/onerror=redirect,width=3020,height=1660,format=auto/_astro/execution-gantt-view.WoBN6eV4.png
- https://kestra.io/cdn-cgi/image/onerror=redirect,width=3020,height=1660,format=auto/_astro/execution-logs-view.D1gbv8l8.png

### 3.3 Argo Workflows（DAG 状态表达）

截图：`/tmp/spool-ui-ref/argo-ui-small.png`

观察：

- **全屏 DAG**：画布占满内容区，节点是圆形状态图标 + 下方任务名，成功=绿色勾、运行=蓝色、失败=红色；节点多时也不会拥挤。
- **悬浮工具栏**：左上角浮层放过滤、方向、缩放、适配、搜索；不占用画布高度。
- **顶部动作条**：`RESUBMIT / DELETE / LOGS / SHARE / WORKFLOW LINK` 统一放左上，右侧是辅助图标。
- 节点详情/日志通常通过点击节点后的侧面板或独立页展示。

远程参考图：https://argo-workflows.readthedocs.io/en/latest/assets/screenshot.png

### 3.4 Semaphore UI（极简执行型界面）

截图：`/tmp/spool-ui-ref/semaphore.png`

观察：

- 左侧深青色侧栏，顶部是项目选择器，菜单只有 Dashboard / Task Templates / Inventory / Environment / Key Store / Playbook Repositories。
- 表格列头全大写、行内直接 `▶ RUN`，没有多余按钮；新建模板是右上角唯一主按钮。
- 信息密度高、装饰少，适合「任务模板 + 历史」这种执行型产品。

远程参考图：https://user-images.githubusercontent.com/914224/134777345-8789d9e4-ff0d-439c-b80e-ddc56b74fcee.png

### 3.5 其他项目的可取点

- **AWX Workflow Visualizer**：节点按类型着色（Job Template / Project Sync / Inventory Sync / Approval），点击节点在右侧编辑，支持缩放/自动布局；审批节点是显式的「暂停点」。Spool 若以后做人工审批，可参考这种节点类型。
- **JumpServer**：以「资产树」为第一入口，资产 → 账号 → 会话 → 命令审计，层级非常清楚。Spool 的主机/凭据可以借鉴资产树与「最近会话」。
- **mayfly-go**：资源树统一入口，机器、数据库、Redis 等都挂在树上；机器详情内是终端/文件/进程/监控 tab。适合 Spool 未来扩数据库/文件运维时的导航方式。
- **Rundeck**：Job 定义页用「Workflow 步骤列表 + 每步节点/选项」，执行页按节点分组展示日志；比纯 DAG 更适合运维人员阅读步骤。
- **n8n**：画布双击打开节点搜索、节点右侧固定 Inspector、执行时节点按状态高亮并可在节点上直接看输出；Spool 的流程图 + 节点面板可以进一步做成「常驻右侧 Inspector」。

## 4. 可直接借鉴到 Spool 的模式

### 4.1 外壳与导航

- 侧边栏支持**收成图标栏**（宽度 64px），悬停显示 tooltip；折叠按钮放在品牌区右侧。
- 每个页面顶部统一为：`面包屑（一级 / 二级 / 当前）` + `页面标题` + 右侧主操作；列表页不再只有工具栏。
- 可选的全局搜索（主机名 / 工具名 / 流程名），`⌘K` 打开；不恢复整条顶部 Header。

### 4.2 列表页统一模板

```
[面包屑]
[标题 + 描述]                          [主操作按钮]
[筛选卡片：搜索 | 分组/分类 | 状态 | 重置]
[表格卡片 工具栏：新建 | 全部/启用/停用 分段 | 刷新 | 列设置]
[表格：名称(链接) | 关键字段 | 状态 pill | 更新时间 | 操作(文字链 + 更多)]
[分页]
```

具体到 Spool：

- 主机：左分组树 + 右表格，保留；行内操作改 `测试 / 编辑 / 更多(删除)`。
- 工具：左分类树 + 右表格，保留；行内操作改 `执行 / 编辑 / 更多(克隆、删除)`。
- 流程：列表增加「最近执行 / 成功率」列；行内 `执行 / 编辑 / 更多(克隆、删除)`。
- 定时任务：增加「下次运行」相对时间、启停开关、`立即执行 / 编辑 / 更多`。
- 执行历史：统一「执行名称 | 类型 | 状态 | 主机 | 操作人 | 开始时间 | 耗时 | 摘要」，摘要列只保留一行 ellipsis。

### 4.3 详情页统一模板

```
[返回] [标题] [状态 pill]                    [执行/编辑/克隆/更多]
[摘要 Descriptions 或状态头 + 元数据 chips]
[Tabs：概览 | 执行历史 | 版本 | 引用 | 脚本/配置]
```

Spool 的工具详情已经接近这个结构；流程详情可以把「流程图」放在概览 tab，执行历史单独 tab；定时任务详情同理。

### 4.4 流程执行详情（结合 Kestra + 当前 Spool）

建议结构：

```
[返回] 流程名 [状态]                          [重新执行] [取消] [下载日志]
[状态头：状态 + 执行ID/操作人/来源/耗时/主机统计 + 开始时间]
[Tabs]
  概览：流程图 + 选中节点面板（当前实现）
  全部日志：按节点分组的可折叠日志（Kestra Logs 模式）
  耗时：每节点/主机耗时条（Kestra Gantt 简化版）
```

- 保留当前「点击节点看该节点输出」的交互。
- 「全部日志」页签按节点分组，组头显示节点名/状态/耗时，支持搜索、复制、下载、折叠全部。
- 「耗时」页签用横向条形图展示每个节点在各主机的耗时，快速定位慢节点。
- 状态头参考 Kestra 的 chips：`来源=手动 | 主机 4 | 成功 4 | 失败 0 | 耗时 3s`。

### 4.5 日志查看器

统一为一个带工具栏的组件：

- 搜索/高亮、复制全部、下载、自动滚动开关、换行开关、行号、时间戳（如事件带 ts）。
- 深色背景 + 等宽字体（现有），但增加行号槽和 `stderr` 红色区分。
- 按节点查看时标题显示 `节点 / 主机`，避免用户误以为是全局日志。

### 4.6 流程图编辑器

- 保留双击/拖线新建节点（已实现）。
- 增加**节点面板/快捷选择**：双击画布直接弹出工具搜索列表，选中即建节点，而不是先建空节点再在抽屉里选工具。
- 在连线上增加 `+` 插入节点（n8n/Windmill 模式），适合串行链路。
- 节点配置从抽屉改为**右侧常驻 Inspector**（可折叠），选中节点即可编辑，画布不跳转。
- 画布左上浮层放「自动布局 / 适配 / 缩放」，参考 Argo 的悬浮工具栏。

### 4.7 首页工作台

参考 Kestra 执行列表与 Spug 工作台，建议：

- 顶部 4 个统计卡（主机/工具/流程/定时任务），保留。
- 增加「最近 7 天执行趋势」条形/面积图 + 成功率。
- 「最近执行」保留，增加类型/状态筛选。
- 「即将运行」列出未来 24h 的定时任务（名称、cron、下次运行）。
- 主机健康：最近测试失败/未测试的主机列表。

### 4.8 视觉语言

- **状态 pill**：浅色底 + 彩色文字 + 小圆点/勾选图标；避免大面积实心绿/红。图节点仍用颜色边框。
- **操作**：文字链为主，危险操作红色文字；行内最多 2 个文字链 + 「更多」。
- **表格**：`size="small"`（已做）、行高 40–44px、名称列加粗、关键列固定宽度、长文本 ellipsis。
- **卡片**：减少嵌套卡片；筛选与表格分离；卡片间距 12–16px。
- **图标**：页面标题/统计卡使用线性图标；状态用图标 + 文本，不只靠颜色。
- **空状态**：简短说明 + 主操作按钮（如「还没有工具，新建第一个」）。
- **深色模式**：可作为 P2；日志区已经是深色，浅色主题下保持统一。

## 5. Spool 现状问题清单（按页面）

| 页面 | 现状 | 建议 |
| --- | --- | --- |
| 全局 | 无面包屑；列表页无标题；侧栏不可折叠 | 页面头统一；侧栏折叠；可选全局搜索 |
| 工作台 | 统计卡 + 快捷操作 + 最近执行 | 增加 7 天趋势/成功率、即将运行、主机健康 |
| 主机列表 | 筛选与表格同卡片；按钮式操作 | 筛选条独立；文字链 + 更多；批量选择 |
| 工具列表 | 左树右表；按钮式操作；行内 4 按钮 | 文字链 + 更多；分类树支持折叠全部；列设置 |
| 工具详情 | 页签结构已接近参考 | 版本列表改时间线；脚本页签加复制/下载 |
| 流程列表 | 无最近执行/成功率；按钮式操作 | 增加最近执行列；文字链 + 更多 |
| 流程编辑器 | 双击/拖线建节点；抽屉配置 | 工具选择前置；右侧 Inspector；连线 + 插入 |
| 流程执行详情 | 状态头较弱；无全部日志/耗时视图 | 状态头 + chips；增加「全部日志」「耗时」页签 |
| 定时任务 | 表格 + 详情页 | 增加下次运行相对时间、cron 人类可读、启停开关 |
| 执行历史 | 统一表格，信息够用 | 状态分段筛选；类型/操作人筛选；保存筛选 |
| 日志查看器 | 自动滚动 + 下载 | 搜索/复制/行号/换行/时间戳 |
| 状态标签 | 实心色块 | 浅色 pill + 图标 |

## 6. 分阶段建议

### P0（低成本、立刻提升观感）

1. 统一页面头部：面包屑 + 标题 + 主操作（主机/工具/流程/定时任务/执行历史）。
2. 列表行操作改文字链 + 「更多」；筛选条与表格卡片分离。
3. 状态 pill 改浅色 + 圆点/图标。
4. 日志查看器工具栏：搜索、复制、下载、行号、换行开关。
5. 首页增加 7 天执行趋势 + 成功率（用现有 executions 数据即可）。

### P1（结构性优化）

1. 侧栏折叠为图标栏 + 全局搜索。
2. 流程执行详情增加「全部日志（按节点分组）」与「耗时」页签，状态头改 Kestra 式 chips。
3. 流程编辑器：双击画布直接选工具、右侧常驻 Inspector、连线 + 插入节点。
4. 工具/流程列表增加最近执行、成功率、负责人等运营字段。
5. 定时任务：cron 人类可读描述 + 未来 5 次运行预览。

### P2（体验加分）

1. 深色模式 / 紧凑模式切换。
2. 保存筛选、列设置、批量操作。
3. 键盘快捷键（⌘K 搜索、G 然后 H 跳主机等）。
4. 执行对比（两次运行节点状态/耗时对比）。

## 7. 不建议照搬

- **Spug 的顶部工具条**：Spool 去 Header 是为了省空间，恢复会走回头路；只借鉴面包屑。
- **Argo 的纯圆形节点**：Spool 的节点需要展示工具名/状态，矩形卡片更合适；可借鉴其「状态优先」的简洁配色。
- **Kestra 的深色企业级外壳**：信息密度很高，但 Spool 是内网轻量工具，浅色更友好。
- **过多页签**：Kestra 有 8 个页签，Spool 保持 3–4 个即可（概览/日志/耗时）。
- **复杂审批/指标**：当前阶段不需要，避免过度设计。

## 8. 参考资料

### 项目与文档

- Spug：https://github.com/openspug/spug ；demo https://demo.spug.cc （admin / spug.cc）
- Semaphore UI：https://github.com/semaphoreui/semaphore ；https://semaphoreui.com
- Windmill：https://github.com/windmill-labs/windmill ；https://www.windmill.dev/docs
- Kestra：https://github.com/kestra-io/kestra ；https://kestra.io/docs/ui
- Argo Workflows：https://github.com/argoproj/argo-workflows ；https://argo-workflows.readthedocs.io
- AWX：https://github.com/ansible/awx
- JumpServer：https://github.com/jumpserver/jumpserver
- mayfly-go：https://github.com/dromara/mayfly-go
- Rundeck：https://github.com/rundeck/rundeck
- n8n：https://github.com/n8n-io/n8n

### 本次截图（临时目录，供本机查看）

`/tmp/spool-ui-ref/` 下：

- `spug-workbench.png`、`spug-hosts.png`、`spug-exec-task.png`
- `spug-pipeline.png`、`spug-pipeline-editor.png`、`spug-pipeline-records.png`
- `spug-schedules.png`
- `kestra-executions-overview-small.png`、`kestra-execution-results-overview-small.png`
- `kestra-execution-gantt-view-small.png`、`kestra-execution-logs-view-small.png`
- `argo-ui-small.png`、`semaphore.png`
