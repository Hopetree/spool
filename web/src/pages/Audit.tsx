import { useState, type CSSProperties } from "react";
import { Card, Select, Space, Table, Tag, Tooltip, Typography } from "antd";
import { useQuery } from "@tanstack/react-query";
import PageHeader from "../components/PageHeader";
import dayjs from "dayjs";
import { listAudit, type AuditRecord } from "../api/audit";
import { listUsers } from "../api/users";

const actionLabels: Record<string, string> = {
  login: "登录",
  login_failed: "登录失败",
  logout: "退出登录",
  password_change: "修改密码",
  "host.create": "新建主机",
  "host.update": "编辑主机",
  "host.delete": "删除主机",
  "host.test": "连通性测试",
  "host.import": "导入主机",
  "host.fingerprint_update": "更新主机指纹",
  "credential.create": "新建凭据",
  "credential.update": "编辑凭据",
  "credential.delete": "删除凭据",
  "group.create": "新建分组",
  "group.update": "编辑分组",
  "group.delete": "删除分组",
  "tool.create": "新建工具",
  "tool.update": "更新工具",
  "tool.delete": "删除工具",
  "tool.enabled": "启停工具",
  "tool.copy": "克隆工具",
  "tool.rollback": "回滚版本",
  "tool.version_delete": "删除版本",
  "flow.create": "新建流程",
  "flow.update": "更新流程",
  "flow.delete": "删除流程",
  "flow.enabled": "启停流程",
  "flow.copy": "克隆流程",
  "execution.start": "发起执行",
  "execution.cancel": "取消执行",
  "execution.cancel_host": "取消主机执行",
  "execution.rerun": "重跑执行",
  "execution.download": "下载执行日志",
  "schedule.create": "新建定时任务",
  "schedule.update": "编辑定时任务",
  "schedule.delete": "删除定时任务",
  "schedule.enabled": "启停定时任务",
  "schedule.trigger": "手动触发任务",
  "notify.create": "新建通知渠道",
  "notify.update": "编辑通知渠道",
  "notify.delete": "删除通知渠道",
  "notify.test": "测试通知渠道",
  "user.create": "新建用户",
  "user.update": "编辑用户",
  "user.delete": "删除用户",
  "user.reset_password": "重置密码",
  "user.logout_all": "强制下线",
  "transfer.upload": "文件分发",
  "transfer.download": "文件下载",
  "transfer.cancel": "取消传输",
  "envar.create": "新建环境变量",
  "envar.update": "编辑环境变量",
  "envar.delete": "删除环境变量",
};

const actionOptions = Object.entries(actionLabels)
  .map(([value, label]) => ({ value, label }))
  .sort((a, b) => a.label.localeCompare(b.label, "zh-CN"));

const resourceLabels: Record<string, string> = {
  host: "主机",
  host_group: "主机分组",
  credential: "凭据",
  group: "分组",
  tool: "工具",
  flow: "流程",
  execution: "执行",
  schedule: "定时任务",
  notify_channel: "通知渠道",
  notify: "通知渠道",
  user: "用户",
  transfer: "传输",
  platform_env: "环境变量",
};

const summaryLabels: Record<string, string> = {
  role: "角色",
  type: "类型",
  version: "版本",
  enabled: "状态",
  from: "原版本",
  new_version: "新版本",
  source: "来源",
  cron: "计划",
  target_kind: "目标类型",
  execution_id: "执行",
  nodes: "节点",
  edges: "连线",
  reason: "原因",
  address: "地址",
  port: "端口",
  created: "新建",
  failed: "失败",
  group_id: "分组",
  ok: "结果",
  fingerprint_recorded: "记录指纹",
  fingerprint: "指纹",
  key_type: "密钥类型",
  hosts: "主机",
  remote_dir: "目录",
  size: "大小",
  host_id: "主机",
  path: "路径",
  auth_type: "认证方式",
  command: "命令",
  kind: "类型",
  ref_id: "引用",
  source_execution: "源执行",
  host_run: "主机执行",
  step_run: "步骤执行",
  elapsed_ms: "耗时",
  error: "错误",
};

const summaryOrder = [
  "ok",
  "created",
  "failed",
  "error",
  "kind",
  "target_kind",
  "type",
  "role",
  "hosts",
  "command",
  "remote_dir",
  "path",
  "size",
  "address",
  "port",
  "cron",
  "enabled",
  "version",
  "from",
  "new_version",
  "reason",
  "execution_id",
  "elapsed_ms",
  "host_id",
  "auth_type",
  "source",
  "ref_id",
  "source_execution",
  "host_run",
  "step_run",
  "group_id",
  "fingerprint_recorded",
  "fingerprint",
  "key_type",
];
const summaryRank = new Map(summaryOrder.map((key, i) => [key, i]));

const opaqueID = /^[0-9A-HJKMNP-TV-Z]{26}$/;

function readableName(value: string): string {
  const name = value.trim();
  return opaqueID.test(name) ? "" : name;
}

function pretty(json: string): string {
  try {
    return JSON.stringify(JSON.parse(json), null, 2);
  } catch {
    return json;
  }
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n}B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`;
  return `${(n / 1024 / 1024).toFixed(1)}MB`;
}

function formatSummaryValue(key: string, value: unknown): string {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "string" && opaqueID.test(value) && key !== "command" && key !== "path" && key !== "remote_dir") {
    return "—";
  }
  if (key === "role") {
    return ({ admin: "管理员", operator: "操作员", viewer: "只读" } as Record<string, string>)[String(value)] ?? String(value);
  }
  if (key === "kind" || key === "target_kind") {
    return ({ tool: "工具", flow: "流程", adhoc: "临时命令" } as Record<string, string>)[String(value)] ?? String(value);
  }
  if (key === "type") {
    return (
      ({
        shell: "Shell",
        python: "Python",
        webhook: "Webhook",
        feishu: "飞书",
        dingtalk: "钉钉",
        wecom: "企业微信",
      } as Record<string, string>)[String(value)] ?? String(value)
    );
  }
  if (key === "auth_type") {
    return ({ password: "密码", private_key: "私钥" } as Record<string, string>)[String(value)] ?? String(value);
  }
  if (key === "enabled") return value ? "启用" : "停用";
  if (key === "ok") return value ? "成功" : "失败";
  if (key === "fingerprint_recorded") return value ? "是" : "否";
  if (key === "reason") {
    return (
      ({
        invalid_credentials: "用户名或密码错误",
        disabled: "用户已停用",
      } as Record<string, string>)[String(value)] ?? String(value)
    );
  }
  if (key === "size" && typeof value === "number") return formatBytes(value);
  if (key === "elapsed_ms" && typeof value === "number") return `${value}ms`;
  if (key === "hosts" && typeof value === "number") return `${value} 台`;
  if (Array.isArray(value)) return value.map(String).join("、");
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function summaryEntries(json: string): [string, unknown][] {
  try {
    const parsed: unknown = JSON.parse(json);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return [];
    return Object.entries(parsed as Record<string, unknown>).sort(
      ([a], [b]) => (summaryRank.get(a) ?? summaryOrder.length) - (summaryRank.get(b) ?? summaryOrder.length),
    );
  } catch {
    return [];
  }
}

function summaryText(json: string): string {
  const entries = summaryEntries(json);
  if (entries.length === 0) return "—";
  const parts = entries
    .slice(0, 4)
    .map(([key, value]) => `${summaryLabels[key] ?? key} ${formatSummaryValue(key, value)}`);
  if (entries.length > 4) parts.push(`+${entries.length - 4}`);
  return parts.join(" · ");
}

function ellipsisStyle(): CSSProperties {
  return { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", minWidth: 0 };
}

export default function Audit() {
  const [action, setAction] = useState("");
  const [actor, setActor] = useState("");
  const [page, setPage] = useState(1);

  const { data: users = [] } = useQuery({
    queryKey: ["users", "audit-filter"],
    queryFn: listUsers,
  });
  const userOptions = users.map((u) => ({
    value: u.id,
    label: u.display_name ? `${u.display_name}（${u.username}）` : u.username,
  }));

  const { data, isLoading } = useQuery({
    queryKey: ["audit", { action, actor, page }],
    queryFn: () => listAudit({ action, actor_id: actor, page, page_size: 20 }),
  });

  return (
    <div>
      <PageHeader title="审计日志" subtitle="只读，保留 180 天" />
      <Card size="small" style={{ marginBottom: 16 }} styles={{ body: { padding: 12 } }}>
        <Space wrap>
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="全部操作"
            style={{ width: 220 }}
            value={action || undefined}
            onChange={(v) => {
              setAction(v ?? "");
              setPage(1);
            }}
            options={actionOptions}
          />
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="全部用户"
            style={{ width: 240 }}
            value={actor || undefined}
            onChange={(v) => {
              setActor(v ?? "");
              setPage(1);
            }}
            options={userOptions}
          />
          <Typography.Text type="secondary">审计日志只读，保留 180 天</Typography.Text>
        </Space>
      </Card>
      <Table<AuditRecord>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={data?.items ?? []}
        tableLayout="fixed"
        scroll={{ x: 1100 }}
        pagination={{
          current: page,
          pageSize: 20,
          total: data?.total ?? 0,
          onChange: setPage,
          showSizeChanger: false,
        }}
        columns={[
          {
            title: "时间",
            dataIndex: "ts",
            width: 170,
            render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm:ss"),
          },
          { title: "用户", dataIndex: "username_snapshot", width: 130, ellipsis: true },
          {
            title: "操作",
            dataIndex: "action",
            width: 150,
            render: (v: string) => (
              <Tooltip title={v}>
                <Tag>{actionLabels[v] ?? v}</Tag>
              </Tooltip>
            ),
          },
          {
            title: "资源",
            width: 260,
            render: (_, r) => {
              const type = (resourceLabels[r.resource_type] ?? r.resource_type) || "";
              const name = readableName(r.resource_name);
              if (!type && !name) return <span style={{ color: "#bfbfbf" }}>—</span>;
              return (
                <div style={{ display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
                  {type && (
                    <Tag style={{ flex: "none", marginInlineEnd: 0 }}>{type}</Tag>
                  )}
                  {name ? (
                    <span title={name} style={{ ...ellipsisStyle(), fontSize: 13 }}>
                      {name}
                    </span>
                  ) : (
                    <span style={{ color: "#bfbfbf" }}>已删除</span>
                  )}
                </div>
              );
            },
          },
          {
            title: "摘要",
            render: (_, r) => {
              const json = r.summary_display || r.summary_json;
              const text = summaryText(json);
              if (text === "—") return <span style={{ color: "#bfbfbf" }}>—</span>;
              return (
                <Tooltip title={<pre style={{ margin: 0, whiteSpace: "pre-wrap" }}>{pretty(json)}</pre>}>
                  <span style={{ display: "block", ...ellipsisStyle() }}>{text}</span>
                </Tooltip>
              );
            },
          },
          { title: "IP", dataIndex: "ip", width: 120, ellipsis: true },
        ]}
      />
    </div>
  );
}
