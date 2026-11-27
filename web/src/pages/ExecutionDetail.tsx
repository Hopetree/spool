import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  App as AntApp,
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
  Typography,
} from "antd";
import { SearchOutlined } from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import dayjs from "dayjs";
import {
  cancelExecution,
  cancelHostRun,
  getExecution,
  logDownloadUrl,
  rerunExecution,
  type HostRun,
  type StepRun,
} from "../api/executions";
import { ApiError } from "../api/client";
import { listHosts } from "../api/hosts";
import { listTools } from "../api/tools";
import ExecutionGraph from "../components/ExecutionGraph";
import PageHeader from "../components/PageHeader";
import ExecutionStatusTag from "../components/ExecutionStatusTag";
import LogViewer from "../components/LogViewer";
import { aggregateStatus } from "../components/status";

function duration(start: number | null, end: number | null): string {
  if (!start) return "—";
  const sec = Math.max(0, Math.round(((end ?? Date.now()) - start) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

function bytes(n: number): string {
  if (n < 1024) return `${n}B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`;
  return `${(n / 1024 / 1024).toFixed(1)}MB`;
}

export default function ExecutionDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const [selectedHost, setSelectedHost] = useState<string>();
  const [hostQuery, setHostQuery] = useState("");
  const [hostStatusFilter, setHostStatusFilter] = useState<string>();
  const [targetsOpen, setTargetsOpen] = useState(false);
  const [selectedNodeID, setSelectedNodeID] = useState<string>();
  const [selectedStepID, setSelectedStepID] = useState<string>();

  const { data, isLoading } = useQuery({
    queryKey: ["execution", id],
    queryFn: () => getExecution(id!),
    enabled: !!id,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "queued" || status === "running" ? 1500 : false;
    },
  });

  const { data: hostPage, isLoading: hostsLoading } = useQuery({
    queryKey: ["hosts", "execution-targets"],
    queryFn: () => listHosts({ page: 1, page_size: 100 }),
    enabled: targetsOpen,
  });
  const hostByID = new Map((hostPage?.items ?? []).map((host) => [host.id, host]));

  useEffect(() => {
    const hosts = data?.host_runs ?? [];
    setSelectedHost((prev) => {
      if (prev && hosts.some((h) => h.id === prev)) return prev;
      return hosts[0]?.id;
    });
  }, [data]);

  const cancel = useMutation({    mutationFn: () => cancelExecution(id!),
    onSuccess: () => {
      message.success("已请求取消");
      void qc.invalidateQueries({ queryKey: ["execution", id] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "取消失败"),
  });

  const rerun = useMutation({
    mutationFn: () => rerunExecution(id!),
    onSuccess: (ex) => {
      message.success("已重新提交");
      void qc.invalidateQueries({ queryKey: ["executions"] });
      navigate(`/executions/${ex.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "重跑失败"),
  });

  const cancelHost = useMutation({
    mutationFn: (hostRunID: string) => cancelHostRun(id!, hostRunID),
    onSuccess: () => {
      message.success("已请求取消该主机");
      void qc.invalidateQueries({ queryKey: ["execution", id] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "取消失败"),
  });

  const active = data?.status === "queued" || data?.status === "running";
  const singleStep = data?.kind === "tool" || data?.kind === "adhoc";
  const hostName = (hostRunID: string) => data?.host_runs.find((h) => h.id === hostRunID)?.host_name ?? hostRunID;
  const stepByHost = new Map<string, StepRun>();
  (data?.step_runs ?? []).forEach((s) => {
    if (!stepByHost.has(s.host_run_id)) stepByHost.set(s.host_run_id, s);
  });
  const hostCounts = {
    running: (data?.host_runs ?? []).filter((h) => h.status === "queued" || h.status === "running").length,
    success: (data?.host_runs ?? []).filter((h) => h.status === "success").length,
    failed: (data?.host_runs ?? []).filter((h) => h.status === "failed").length,
    cancelled: (data?.host_runs ?? []).filter((h) => h.status === "cancelled").length,
  };
  const visibleHosts = (data?.host_runs ?? []).filter((h) => {
    if (hostQuery && !h.host_name.toLowerCase().includes(hostQuery.toLowerCase())) return false;
    if (!hostStatusFilter) return true;
    if (hostStatusFilter === "running") return h.status === "running" || h.status === "queued";
    return h.status === hostStatusFilter;
  });
  const selectedVisibleHost = visibleHosts.find((h) => h.id === selectedHost) ?? visibleHosts[0];
  const hostFilters = [
    { key: "running", label: "执行中", count: hostCounts.running, color: "#1677ff" },
    { key: "success", label: "成功", count: hostCounts.success, color: "#52c41a" },
    { key: "failed", label: "失败", count: hostCounts.failed, color: "#ff4d4f" },
    ...(hostCounts.cancelled > 0
      ? [{ key: "cancelled", label: "取消", count: hostCounts.cancelled, color: "#8c8c8c" }]
      : []),
  ];
  const copyHostNames = async () => {
    try {
      await navigator.clipboard.writeText((data?.host_runs ?? []).map((h) => h.host_name).join("\n"));
      message.success("已复制主机名");
    } catch {
      message.error("复制失败");
    }
  };
  const toolDetailPath = data?.kind === "tool" && data.ref_id ? `/tools/${data.ref_id}` : "";
  const backPath =
    data?.kind === "flow"
      ? "/flows/executions"
      : data?.kind === "adhoc"
        ? "/adhoc"
        : toolDetailPath || "/tools/executions";

  const graph = data?.graph;
  // Old plan snapshots do not carry tool names; resolve them for display.
  const { data: toolPage } = useQuery({
    queryKey: ["tools", "execution-graph-names"],
    queryFn: () => listTools({ page: 1, page_size: 100 }),
    enabled: data?.kind === "flow",
  });
  const displayGraph = useMemo(() => {
    if (!graph) return graph;
    const names = new Map((toolPage?.items ?? []).map((t) => [t.id, t.name]));
    return {
      ...graph,
      nodes: graph.nodes.map((n) => ({
        ...n,
        tool_name: n.tool_name || (n.tool_id ? names.get(n.tool_id) : undefined) || "",
      })),
    };
  }, [graph, toolPage]);
  const showGraph = data?.kind === "flow" && !!displayGraph && displayGraph.nodes.length > 0;
  const nodeStatuses = useMemo(() => {
    const byNode = new Map<string, string[]>();
    for (const s of data?.step_runs ?? []) {
      if (!s.node_id) continue;
      const list = byNode.get(s.node_id) ?? [];
      list.push(s.status);
      byNode.set(s.node_id, list);
    }
    const out: Record<string, string> = {};
    byNode.forEach((statuses, nodeID) => {
      out[nodeID] = aggregateStatus(statuses);
    });
    return out;
  }, [data]);
  const graphNode = displayGraph?.nodes.find((n) => n.id === selectedNodeID);
  const nodeSteps = (data?.step_runs ?? []).filter((s) => s.node_id === selectedNodeID);
  const activeNodeStep = nodeSteps.find((s) => s.id === selectedStepID) ?? nodeSteps[0];

  const openNode = (nodeID: string) => {
    if (displayGraph?.nodes.find((n) => n.id === nodeID)?.type !== "tool") return;
    setSelectedNodeID(nodeID);
    setSelectedStepID(undefined);
  };

  // Keep a tool node selected so the detail panel always has content. Failures
  // win over the first node so a problem run opens on the failing step.
  useEffect(() => {
    if (!displayGraph || data?.kind !== "flow") return;
    const toolNodes = displayGraph.nodes.filter((n) => n.type === "tool");
    if (toolNodes.length === 0) return;
    setSelectedNodeID((prev) => {
      if (prev && toolNodes.some((n) => n.id === prev)) return prev;
      const preferred =
        toolNodes.find((n) => nodeStatuses[n.id] === "failed") ??
        toolNodes.find((n) => nodeStatuses[n.id] === "running") ??
        toolNodes[0];
      return preferred.id;
    });
  }, [displayGraph, data?.kind, nodeStatuses]);

  return (
    <div>
      <PageHeader
        title={data?.name || "执行详情"}
        subtitle={data ? <ExecutionStatusTag status={data.status} /> : undefined}
        crumbs={
          data?.kind === "tool"
            ? [
                { title: "工具", to: "/tools" },
                { title: "执行历史", to: "/tools/executions" },
                { title: "执行详情" },
              ]
            : [
                {
                  title: data?.kind === "flow" ? "流程执行历史" : data?.kind === "adhoc" ? "临时命令" : "工具执行历史",
                  to: backPath,
                },
                { title: "执行详情" },
              ]
        }
        extra={
          <>
            {active && (
              <Popconfirm title="确认取消本次执行？" onConfirm={() => cancel.mutate()}>
                <Button danger loading={cancel.isPending}>
                  取消执行
                </Button>
              </Popconfirm>
            )}
            <Button onClick={() => rerun.mutate()} loading={rerun.isPending}>
              重新执行
            </Button>
            <Button href={id ? logDownloadUrl(id) : "#"} target="_blank">
              下载全部日志
            </Button>
          </>
        }
      />

      <Card loading={isLoading} style={{ marginBottom: 16 }}>
        {data && (
          <Descriptions
            column={3}
            size="small"
            items={[
              {
                key: "object",
                label: data.kind === "tool" ? "工具名称" : data.kind === "flow" ? "流程名称" : "执行对象",
                children:
                  data.kind === "tool" && toolDetailPath ? (
                    <Link to={toolDetailPath}>{data.name}</Link>
                  ) : (
                    data.name
                  ),
              },
              ...(data.kind === "tool"
                ? [
                    {
                      key: "version",
                      label: "工具版本",
                      children: data.tool_version_no ? `v${data.tool_version_no}` : "—",
                    },
                  ]
                : []),
              { key: "operator", label: "创建用户", children: data.created_by_name || data.created_by || "—" },
              { key: "created", label: "创建时间", children: dayjs(data.created_at).format("YYYY-MM-DD HH:mm:ss") },
              {
                key: "started",
                label: "开始时间",
                children: data.started_at ? dayjs(data.started_at).format("YYYY-MM-DD HH:mm:ss") : "—",
              },
              { key: "duration", label: "耗时", children: duration(data.started_at, data.finished_at) },
              {
                key: "source",
                label: "来源",
                children: data.source === "manual" ? "手动" : data.source === "schedule" ? "定时任务" : data.source,
              },
              {
                key: "hosts",
                label: "执行目标",
                children: (
                  <Button type="link" size="small" style={{ padding: 0 }} onClick={() => setTargetsOpen(true)}>
                    查看主机（{data.host_total}）
                  </Button>
                ),
              },
              {
                key: "params",
                label: "参数",
                children:
                  Object.keys(data.params).length > 0 ? (
                    <Space wrap>
                      {Object.entries(data.params).map(([k, v]) => (
                        <Tag key={k}>
                          {k}={v || "（空）"}
                        </Tag>
                      ))}
                    </Space>
                  ) : (
                    "—"
                  ),
              },
              { key: "summary", label: "摘要", children: data.error_summary || "—", span: 3 },
            ]}
          />
        )}
      </Card>

      {data?.kind === "adhoc" && data.command && (
        <Card title="命令" style={{ marginBottom: 16 }}>
          <pre style={{ margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-all", fontSize: 12 }}>
            {data.command}
          </pre>
        </Card>
      )}

      {id && singleStep && (
        <Card style={{ marginBottom: 16 }} styles={{ body: { padding: 16 } }}>
          {!data || data.host_runs.length === 0 ? (
            <Empty description="没有主机执行记录" />
          ) : (
            <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
              <div style={{ width: 340, flex: "0 0 340px", maxHeight: 640, overflow: "auto" }}>
                <Input
                  allowClear
                  size="small"
                  prefix={<SearchOutlined style={{ color: "#bfbfbf" }} />}
                  placeholder="搜索主机名"
                  value={hostQuery}
                  onChange={(e) => setHostQuery(e.target.value)}
                  style={{ marginBottom: 10 }}
                />
                <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 12 }}>
                  {hostFilters.map((filter) => {
                    const checked = hostStatusFilter === filter.key;
                    return (
                      <span
                        key={filter.key}
                        title={checked ? "再次点击取消筛选" : `只看${filter.label}的主机`}
                        onClick={() => setHostStatusFilter(checked ? undefined : filter.key)}
                        style={{
                          display: "inline-flex",
                          alignItems: "center",
                          gap: 6,
                          padding: "3px 10px",
                          borderRadius: 14,
                          border: `1px solid ${checked ? filter.color : "#e8e8e8"}`,
                          background: checked ? `${filter.color}14` : "#fafafa",
                          color: checked ? filter.color : "#595959",
                          cursor: "pointer",
                          fontSize: 12,
                          lineHeight: "18px",
                          userSelect: "none",
                        }}
                      >
                        <span style={{ width: 6, height: 6, borderRadius: 3, background: filter.color }} />
                        {filter.label} ({filter.count})
                      </span>
                    );
                  })}
                </div>
                {visibleHosts.length === 0 ? (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的主机" />
                ) : (
                  visibleHosts.map((h) => {
                    const step = stepByHost.get(h.id);
                    const selected = h.id === selectedVisibleHost?.id;
                    return (
                      <div
                        key={h.id}
                        onClick={() => setSelectedHost(h.id)}
                        style={{
                          padding: "10px 12px",
                          marginBottom: 8,
                          borderRadius: 6,
                          cursor: "pointer",
                          border: selected ? "1px solid #1677ff" : "1px solid #f0f0f0",
                          background: selected ? "#f0f7ff" : "#fff",
                        }}
                      >
                        <div style={{ display: "flex", justifyContent: "space-between", gap: 8, alignItems: "center" }}>
                          <strong
                            title={h.host_name}
                            style={{
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                            }}
                          >
                            {h.host_name}
                          </strong>
                          <ExecutionStatusTag status={h.status} />
                        </div>
                        <div style={{ fontSize: 12, color: "#8c8c8c", marginTop: 4 }}>
                          耗时 {duration(h.started_at, h.finished_at)}
                          {step?.exit_code !== null && step?.exit_code !== undefined && ` · 退出码 ${step.exit_code}`}
                          {step?.truncated && " · 输出已截断"}
                        </div>
                        {h.error_message && (
                          <div
                            style={{
                              fontSize: 12,
                              color: "#cf1322",
                              marginTop: 4,
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                            }}
                            title={h.error_message}
                          >
                            {h.error_message}
                          </div>
                        )}
                        {(h.status === "running" || h.status === "queued") && (
                          <div style={{ marginTop: 6 }}>
                            <Popconfirm
                              title="取消该主机的执行？"
                              onConfirm={(e) => {
                                e?.stopPropagation();
                                cancelHost.mutate(h.id);
                              }}
                            >
                              <Button size="small" danger onClick={(e) => e.stopPropagation()}>
                                取消该主机
                              </Button>
                            </Popconfirm>
                          </div>
                        )}
                      </div>
                    );
                  })
                )}
                <Button block style={{ marginTop: 4 }} onClick={copyHostNames}>
                  复制主机名
                </Button>
              </div>

              <div style={{ flex: 1, minWidth: 0 }}>
                {selectedVisibleHost?.error_message && (
                  <Alert
                    type="error"
                    showIcon
                    message={selectedVisibleHost.error_message}
                    style={{ marginBottom: 8 }}
                  />
                )}
                {selectedVisibleHost ? (
                  <LogViewer
                    executionId={id}
                    active={!!active}
                    hostRunID={selectedVisibleHost.id}
                    heading="执行日志"
                    showJumpControls
                  />
                ) : (
                  <Empty description="没有匹配的主机" />
                )}
              </div>
            </div>
          )}
        </Card>
      )}

      {!singleStep && (
        <>
          {showGraph && displayGraph && (
            <Card
              title="执行流程图"
              extra={
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  节点颜色表示执行结果，灰色虚线为未执行分支；点击节点查看该节点的工具执行输出
                </Typography.Text>
              }
              style={{ marginBottom: 16 }}
            >
              <ExecutionGraph
                graph={displayGraph}
                statusByNode={nodeStatuses}
                height={360}
                onNodeClick={openNode}
              />
            </Card>
          )}

          {!showGraph && (
          <Card title="主机" style={{ marginBottom: 16 }}>
            <Table<HostRun>
              rowKey="id"
              dataSource={data?.host_runs ?? []}
              pagination={false}
              size="small"
              onRow={(r) => ({ onClick: () => setSelectedHost(r.id), style: { cursor: "pointer" } })}
              columns={[
                { title: "主机", dataIndex: "host_name" },
                { title: "状态", dataIndex: "status", render: (v: string) => <ExecutionStatusTag status={v} /> },
                { title: "耗时", render: (_, r) => duration(r.started_at, r.finished_at) },
                { title: "错误", dataIndex: "error_message", ellipsis: true },
                {
                  title: "操作",
                  width: 130,
                  render: (_, r) =>
                    (r.status === "running" || r.status === "queued") && (
                      <Popconfirm title="取消该主机的执行？" onConfirm={() => cancelHost.mutate(r.id)}>
                        <Button size="small" danger loading={cancelHost.isPending}>
                          取消
                        </Button>
                      </Popconfirm>
                    ),
                },
              ]}
            />
          </Card>
          )}

          {showGraph &&
            (graphNode ? (
              <Card
                title={`节点执行详情：${graphNode.name}`}
                extra={
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    运行中的执行会实时刷新
                  </Typography.Text>
                }
                style={{ marginBottom: 16 }}
              >
                {graphNode.type === "start" ? (
                  <Alert type="info" showIcon message="开始节点只用于触发后续分支，没有工具执行记录。" />
                ) : (
                  <>
                    <Descriptions
                      size="small"
                      column={4}
                      items={[
                        { key: "tool", label: "工具", children: graphNode.tool_name || graphNode.tool_id || "—" },
                        {
                          key: "version",
                          label: "版本",
                          children: graphNode.tool_version_no ? `v${graphNode.tool_version_no}` : "—",
                        },
                        { key: "type", label: "类型", children: graphNode.tool_type || "—" },
                        {
                          key: "hosts",
                          label: "执行主机",
                          children: `${nodeSteps.filter((s) => s.status !== "skipped").length} / ${nodeSteps.length}`,
                        },
                      ]}
                    />
                    {nodeSteps.length === 0 ? (
                      <Empty description="该节点没有执行记录" />
                    ) : (
                      <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
                        <div style={{ width: 320, flex: "0 0 320px", maxHeight: 480, overflow: "auto" }}>
                          {nodeSteps.map((step) => {
                            const selected = step.id === activeNodeStep?.id;
                            return (
                              <div
                                key={step.id}
                                onClick={() => setSelectedStepID(step.id)}
                                style={{
                                  padding: "10px 12px",
                                  marginBottom: 8,
                                  borderRadius: 6,
                                  cursor: "pointer",
                                  border: selected ? "1px solid #1677ff" : "1px solid #f0f0f0",
                                  background: selected ? "#f0f7ff" : "#fff",
                                }}
                              >
                                <div
                                  style={{
                                    display: "flex",
                                    justifyContent: "space-between",
                                    gap: 8,
                                    alignItems: "center",
                                  }}
                                >
                                  <strong
                                    style={{
                                      overflow: "hidden",
                                      textOverflow: "ellipsis",
                                      whiteSpace: "nowrap",
                                    }}
                                  >
                                    {hostName(step.host_run_id)}
                                  </strong>
                                  <ExecutionStatusTag status={step.status} />
                                </div>
                                <div style={{ fontSize: 12, color: "#8c8c8c", marginTop: 4 }}>
                                  耗时 {duration(step.started_at, step.finished_at)}
                                  {step.exit_code !== null &&
                                    step.exit_code !== undefined &&
                                    ` · 退出码 ${step.exit_code}`}
                                  {step.attempt > 1 && ` · 尝试 ${step.attempt} 次`}
                                  {step.truncated && " · 输出已截断"}
                                </div>
                                {step.error_message && (
                                  <div
                                    style={{
                                      fontSize: 12,
                                      color: "#cf1322",
                                      marginTop: 4,
                                      overflow: "hidden",
                                      textOverflow: "ellipsis",
                                      whiteSpace: "nowrap",
                                    }}
                                    title={step.error_message}
                                  >
                                    {step.error_message}
                                  </div>
                                )}
                                {(step.status === "running" || step.status === "queued") && (
                                  <div style={{ marginTop: 6 }}>
                                    <Popconfirm
                                      title="取消该主机的执行？"
                                      onConfirm={(e) => {
                                        e?.stopPropagation();
                                        cancelHost.mutate(step.host_run_id);
                                      }}
                                    >
                                      <Button size="small" danger onClick={(e) => e.stopPropagation()}>
                                        取消该主机
                                      </Button>
                                    </Popconfirm>
                                  </div>
                                )}
                              </div>
                            );
                          })}
                        </div>
                        <div style={{ flex: 1, minWidth: 0 }}>
                          {activeNodeStep ? (
                            <>
                              <div
                                style={{
                                  display: "flex",
                                  justifyContent: "space-between",
                                  alignItems: "center",
                                  gap: 8,
                                  marginBottom: 8,
                                }}
                              >
                                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                                  {hostName(activeNodeStep.host_run_id)} 的节点输出 ·{" "}
                                  {bytes(activeNodeStep.log_bytes)}
                                </Typography.Text>
                                <Button
                                  size="small"
                                  href={logDownloadUrl(id!, activeNodeStep.host_run_id, activeNodeStep.id)}
                                  target="_blank"
                                >
                                  下载该节点日志
                                </Button>
                              </div>
                              {activeNodeStep.error_message && (
                                <Alert
                                  type="error"
                                  showIcon
                                  message={activeNodeStep.error_message}
                                  style={{ marginBottom: 8 }}
                                />
                              )}
                              <LogViewer
                                executionId={id!}
                                active={!!active}
                                hostRunID={activeNodeStep.host_run_id}
                                stepRunID={activeNodeStep.id}
                              />
                            </>
                          ) : (
                            <Empty description="请选择一台主机查看输出" />
                          )}
                        </div>
                      </div>
                    )}
                  </>
                )}
              </Card>
            ) : (
              <Card style={{ marginBottom: 16 }}>
                <Empty description="点击流程图中的节点查看该节点的执行输出" />
              </Card>
            ))}

          {!showGraph && (
            <>
              <Card title="步骤" style={{ marginBottom: 16 }}>
                <Table<StepRun>
                  rowKey="id"
                  dataSource={data?.step_runs ?? []}
                  pagination={false}
                  size="small"
                  columns={[
                    { title: "主机", render: (_, r) => hostName(r.host_run_id) },
                    { title: "步骤", dataIndex: "name" },
                    { title: "状态", dataIndex: "status", render: (v: string) => <ExecutionStatusTag status={v} /> },
                    {
                      title: "退出码",
                      dataIndex: "exit_code",
                      width: 90,
                      render: (v: number | null) => (v === null ? "—" : v),
                    },
                    { title: "尝试", dataIndex: "attempt", width: 70 },
                    { title: "耗时", render: (_, r) => duration(r.started_at, r.finished_at) },
                    {
                      title: "输出",
                      render: (_, r) => (
                        <Space size={4}>
                          <span>{bytes(r.log_bytes)}</span>
                          {r.truncated && <Tag color="orange">已截断</Tag>}
                        </Space>
                      ),
                    },
                    { title: "错误", dataIndex: "error_message", ellipsis: true },
                    {
                      title: "日志",
                      width: 90,
                      render: (_, r) => (
                        <Button
                          size="small"
                          href={id ? logDownloadUrl(id, r.host_run_id, r.id) : "#"}
                          target="_blank"
                        >
                          下载
                        </Button>
                      ),
                    },
                  ]}
                />
              </Card>

              {id && (
                <Card title="实时输出">
                  <LogViewer executionId={id} active={!!active} />
                </Card>
              )}
            </>
          )}
        </>
      )}

      <Modal
        open={targetsOpen}
        onCancel={() => setTargetsOpen(false)}
        title={`执行目标（${data?.host_total ?? 0} 台）`}
        footer={null}
        width={860}
      >
        <Table<HostRun>
          rowKey="id"
          size="small"
          loading={hostsLoading}
          dataSource={data?.host_runs ?? []}
          pagination={false}
          scroll={{ y: 420 }}
          columns={[
            { title: "主机", dataIndex: "host_name", width: 160, ellipsis: true },
            {
              title: "地址",
              width: 160,
              render: (_, r) => {
                const host = hostByID.get(r.host_id);
                return host ? `${host.address}:${host.port}` : "—";
              },
            },
            {
              title: "分组",
              width: 120,
              ellipsis: true,
              render: (_, r) => hostByID.get(r.host_id)?.group_name || "—",
            },
            {
              title: "标签",
              width: 200,
              render: (_, r) => {
                const tags = hostByID.get(r.host_id)?.tags ?? [];
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
            {
              title: "状态",
              width: 100,
              render: (_, r) => <ExecutionStatusTag status={r.status} />,
            },
            { title: "耗时", width: 90, render: (_, r) => duration(r.started_at, r.finished_at) },
            { title: "错误", dataIndex: "error_message", ellipsis: true },
          ]}
        />
      </Modal>
    </div>
  );
}
