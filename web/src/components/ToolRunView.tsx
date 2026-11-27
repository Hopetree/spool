import { useState } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Input,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
} from "antd";
import { SearchOutlined } from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import dayjs from "dayjs";
import { listHosts } from "../api/hosts";
import ExecutionStatusTag from "./ExecutionStatusTag";
import LogViewer from "./LogViewer";
import { statusColors, statusLabels } from "./status";

export interface ToolRunHost {
  key: string;
  hostID: string;
  hostName: string;
  status: string;
  startedAt: number | null;
  finishedAt: number | null;
  errorMessage: string;
  failureReason?: string;
  hostRunID: string;
  stepRunID?: string;
  exitCode?: number | null;
  truncated?: boolean;
}

function duration(start: number | null | undefined, end: number | null | undefined): string {
  if (!start) return "—";
  const sec = Math.max(0, Math.round(((end ?? Date.now()) - start) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

function statusGroup(status: string): "running" | "success" | "failed" | "cancelled" | "skipped" | "other" {
  if (status === "running" || status === "queued") return "running";
  if (status === "success") return "success";
  if (status === "failed") return "failed";
  if (status === "cancelled") return "cancelled";
  if (status === "skipped") return "skipped";
  return "other";
}

export function ToolRunInfo({
  objectLabel,
  objectName,
  objectHref,
  showVersion,
  versionNo,
  operator,
  createdAt,
  startedAt,
  finishedAt,
  sourceLabel,
  params,
  summary,
  hosts,
}: {
  objectLabel: string;
  objectName: string;
  objectHref?: string;
  showVersion?: boolean;
  versionNo?: number;
  operator: string;
  createdAt: number;
  startedAt?: number | null;
  finishedAt?: number | null;
  sourceLabel: string;
  params: Record<string, string>;
  summary?: string;
  hosts: ToolRunHost[];
}) {
  const [targetsOpen, setTargetsOpen] = useState(false);
  const { data: hostPage, isLoading } = useQuery({
    queryKey: ["hosts", "execution-targets"],
    queryFn: () => listHosts({ page: 1, page_size: 100 }),
    enabled: targetsOpen,
  });
  const hostByID = new Map((hostPage?.items ?? []).map((host) => [host.id, host]));

  return (
    <>
      <Card style={{ marginBottom: 16 }}>
        <Descriptions
          column={3}
          size="small"
          items={[
            {
              key: "object",
              label: objectLabel,
              children: objectHref ? <Link to={objectHref}>{objectName}</Link> : objectName,
            },
            ...(showVersion
              ? [
                  {
                    key: "version",
                    label: "工具版本",
                    children: versionNo ? `v${versionNo}` : "—",
                  },
                ]
              : []),
            { key: "operator", label: "创建用户", children: operator || "—" },
            { key: "created", label: "创建时间", children: dayjs(createdAt).format("YYYY-MM-DD HH:mm:ss") },
            {
              key: "started",
              label: "开始时间",
              children: startedAt ? dayjs(startedAt).format("YYYY-MM-DD HH:mm:ss") : "—",
            },
            { key: "duration", label: "耗时", children: duration(startedAt, finishedAt) },
            { key: "source", label: "来源", children: sourceLabel || "—" },
            {
              key: "hosts",
              label: "执行目标",
              children: (
                <Button type="link" size="small" style={{ padding: 0 }} onClick={() => setTargetsOpen(true)}>
                  查看主机（{hosts.length}）
                </Button>
              ),
            },
            {
              key: "params",
              label: "参数",
              children:
                Object.keys(params).length > 0 ? (
                  <Space wrap>
                    {Object.entries(params).map(([key, value]) => (
                      <Tag key={key}>
                        {key}={value || "（空）"}
                      </Tag>
                    ))}
                  </Space>
                ) : (
                  "—"
                ),
            },
            { key: "summary", label: "摘要", children: summary || "—", span: 3 },
          ]}
        />
      </Card>

      <Modal
        open={targetsOpen}
        onCancel={() => setTargetsOpen(false)}
        title={`执行目标（${hosts.length} 台）`}
        footer={null}
        width={860}
      >
        <Table<ToolRunHost>
          rowKey="key"
          size="small"
          loading={isLoading}
          dataSource={hosts}
          pagination={false}
          scroll={{ y: 420 }}
          columns={[
            { title: "主机", dataIndex: "hostName", width: 160, ellipsis: true },
            {
              title: "地址",
              width: 160,
              render: (_, host) => {
                const info = hostByID.get(host.hostID);
                return info ? `${info.address}:${info.port}` : "—";
              },
            },
            {
              title: "分组",
              width: 120,
              ellipsis: true,
              render: (_, host) => hostByID.get(host.hostID)?.group_name || "—",
            },
            {
              title: "标签",
              width: 200,
              render: (_, host) => {
                const tags = hostByID.get(host.hostID)?.tags ?? [];
                return tags.length > 0 ? (
                  <Space wrap size={4}>
                    {tags.map((tag) => (
                      <Tag key={tag}>{tag}</Tag>
                    ))}
                  </Space>
                ) : (
                  "—"
                );
              },
            },
            { title: "状态", width: 100, render: (_, host) => <ExecutionStatusTag status={host.status} /> },
            { title: "耗时", width: 90, render: (_, host) => duration(host.startedAt, host.finishedAt) },
            { title: "错误", dataIndex: "errorMessage", ellipsis: true },
          ]}
        />
      </Modal>
    </>
  );
}

export function ToolRunConsole({
  executionId,
  hosts,
  active,
  onCancelHost,
  cancelLoading,
  emptyText = "没有主机执行记录",
}: {
  executionId: string;
  hosts: ToolRunHost[];
  active: boolean;
  onCancelHost?: (hostRunID: string) => void;
  cancelLoading?: boolean;
  emptyText?: string;
}) {
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>();
  const [selectedKey, setSelectedKey] = useState<string>();

  const counts = hosts.reduce(
    (acc, host) => {
      acc[statusGroup(host.status)] += 1;
      return acc;
    },
    { running: 0, success: 0, failed: 0, cancelled: 0, skipped: 0, other: 0 },
  );
  const filters = [
    { key: "running", label: "执行中", count: counts.running, color: statusColors.running },
    { key: "success", label: "成功", count: counts.success, color: statusColors.success },
    { key: "failed", label: "失败", count: counts.failed, color: statusColors.failed },
    ...(counts.cancelled > 0 ? [{ key: "cancelled", label: "取消", count: counts.cancelled, color: statusColors.cancelled }] : []),
    ...(counts.skipped > 0 ? [{ key: "skipped", label: statusLabels.skipped, count: counts.skipped, color: statusColors.skipped }] : []),
    ...(counts.other > 0 ? [{ key: "other", label: "其他", count: counts.other, color: statusColors.interrupted }] : []),
  ];

  const visible = hosts.filter((host) => {
    if (query && !host.hostName.toLowerCase().includes(query.toLowerCase())) return false;
    if (!statusFilter) return true;
    return statusGroup(host.status) === statusFilter;
  });
  const selected =
    visible.find((host) => host.key === selectedKey) ??
    visible.find((host) => host.status !== "skipped") ??
    visible[0];

  if (hosts.length === 0) {
    return (
      <Card style={{ marginBottom: 16 }}>
        <Empty description={emptyText} />
      </Card>
    );
  }

  return (
    <Card style={{ marginBottom: 16 }} styles={{ body: { padding: 16 } }}>
      <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
        <div style={{ width: 340, flex: "0 0 340px", maxHeight: 640, overflow: "auto" }}>
          <Input
            allowClear
            size="small"
            prefix={<SearchOutlined style={{ color: "#bfbfbf" }} />}
            placeholder="搜索主机名"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            style={{ marginBottom: 10 }}
          />
          <div style={{ display: "flex", flexWrap: "nowrap", gap: 4, marginBottom: 12 }}>
            {filters.map((filter) => {
              const checked = statusFilter === filter.key;
              return (
                <span
                  key={filter.key}
                  title={checked ? "再次点击取消筛选" : `只看${filter.label}的主机`}
                  onClick={() => setStatusFilter(checked ? undefined : filter.key)}
                  style={{
                    flex: "1 1 0",
                    minWidth: 0,
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                    gap: 4,
                    padding: "2px 4px",
                    borderRadius: 14,
                    border: `1px solid ${checked ? filter.color : "#e8e8e8"}`,
                    background: checked ? `${filter.color}14` : "#fafafa",
                    color: checked ? filter.color : "#595959",
                    cursor: "pointer",
                    fontSize: 11,
                    lineHeight: "16px",
                    whiteSpace: "nowrap",
                    userSelect: "none",
                    overflow: "hidden",
                  }}
                >
                  <span style={{ width: 5, height: 5, borderRadius: 3, background: filter.color, flex: "none" }} />
                  <span style={{ overflow: "hidden", textOverflow: "ellipsis" }}>
                    {filter.label} ({filter.count})
                  </span>
                </span>
              );
            })}
          </div>
          {visible.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的主机" />
          ) : (
            visible.map((host) => {
              const selectedHost = host.key === selected?.key;
              return (
                <div
                  key={host.key}
                  onClick={() => setSelectedKey(host.key)}
                  style={{
                    padding: "10px 12px",
                    marginBottom: 8,
                    borderRadius: 6,
                    cursor: "pointer",
                    border: selectedHost ? "1px solid #1677ff" : "1px solid #f0f0f0",
                    background: selectedHost ? "#f0f7ff" : "#fff",
                  }}
                >
                  <div style={{ display: "flex", justifyContent: "space-between", gap: 8, alignItems: "center" }}>
                    <strong
                      title={host.hostName}
                      style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                    >
                      {host.hostName}
                    </strong>
                    <ExecutionStatusTag status={host.status} />
                  </div>
                  <div style={{ fontSize: 12, color: "#8c8c8c", marginTop: 4 }}>
                    耗时 {duration(host.startedAt, host.finishedAt)}
                    {host.exitCode !== null && host.exitCode !== undefined && ` · 退出码 ${host.exitCode}`}
                    {host.failureReason && ` · 错误码 ${host.failureReason}`}
                    {host.truncated && " · 输出已截断"}
                  </div>
                  {host.errorMessage && (
                    <div
                      style={{
                        fontSize: 12,
                        color: "#cf1322",
                        marginTop: 4,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                      }}
                      title={host.errorMessage}
                    >
                      {host.errorMessage}
                    </div>
                  )}
                  {(host.status === "running" || host.status === "queued") && onCancelHost && (
                    <div style={{ marginTop: 6 }}>
                      <Popconfirm
                        title="取消该主机的执行？"
                        onConfirm={(event) => {
                          event?.stopPropagation();
                          onCancelHost(host.hostRunID);
                        }}
                      >
                        <Button size="small" danger loading={cancelLoading} onClick={(event) => event.stopPropagation()}>
                          取消该主机
                        </Button>
                      </Popconfirm>
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>

        <div style={{ flex: 1, minWidth: 0 }}>
          {selected?.errorMessage && (
            <Alert
              type="error"
              showIcon
              message={
                selected.failureReason
                  ? `${selected.failureReason}：${selected.errorMessage}`
                  : selected.errorMessage
              }
              style={{ marginBottom: 8 }}
            />
          )}
          {selected ? (
            <LogViewer
              executionId={executionId}
              active={active}
              hostRunID={selected.hostRunID}
              stepRunID={selected.stepRunID}
              heading="执行日志"
              showJumpControls
            />
          ) : (
            <Empty description="没有匹配的主机" />
          )}
        </div>
      </div>
    </Card>
  );
}
