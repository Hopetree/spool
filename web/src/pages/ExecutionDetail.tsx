import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Empty,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
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
import { getFlow } from "../api/flows";
import { listTools } from "../api/tools";
import ExecutionGraph from "../components/ExecutionGraph";
import ExecuteToolModal from "../components/ExecuteToolModal";
import PageHeader from "../components/PageHeader";
import ExecutionStatusTag from "../components/ExecutionStatusTag";
import LogViewer from "../components/LogViewer";
import { ToolRunConsole, ToolRunInfo, type ToolRunHost } from "../components/ToolRunView";
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
  const [rerunOpen, setRerunOpen] = useState(false);
  const [rerunInitial, setRerunInitial] = useState<{ hostIDs: string[]; params: Record<string, string> }>({
    hostIDs: [],
    params: {},
  });
  const [selectedNodeID, setSelectedNodeID] = useState<string>();

  const { data, isLoading } = useQuery({
    queryKey: ["execution", id],
    queryFn: () => getExecution(id!),
    enabled: !!id,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "queued" || status === "running" ? 1500 : false;
    },
  });

  const cancel = useMutation({
    mutationFn: () => cancelExecution(id!),
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
  const sourceLabel =
    data?.source === "manual" ? "手动" : data?.source === "schedule" ? "定时任务" : data?.source || "—";
  const { data: flowDetail } = useQuery({
    queryKey: ["flow", data?.ref_id, "execution-detail"],
    queryFn: () => getFlow(data!.ref_id),
    enabled: data?.kind === "flow" && !!data.ref_id,
  });
  const displayParams = useMemo(() => {
    if (!data) return {};
    if (data.kind !== "flow") return data.params;
    const hostNames = new Set(
      (flowDetail?.params ?? []).filter((param) => param.type === "host").map((param) => param.name),
    );
    return Object.fromEntries(Object.entries(data.params).filter(([name]) => !hostNames.has(name)));
  }, [data, flowDetail]);
  const toolHosts: ToolRunHost[] = (data?.host_runs ?? []).map((run) => {
    const step = stepByHost.get(run.id);
    return {
      key: run.id,
      hostID: run.host_id,
      hostName: run.host_name,
      status: run.status,
      startedAt: run.started_at,
      finishedAt: run.finished_at,
      errorMessage: run.error_message,
      failureReason: run.failure_reason,
      hostRunID: run.id,
      stepRunID: step?.id,
      exitCode: step?.exit_code,
      truncated: step?.truncated,
    };
  });
  const toolDetailPath = data?.kind === "tool" && data.ref_id ? `/tools/${data.ref_id}` : "";

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
  const nodeHosts: ToolRunHost[] = nodeSteps.map((step) => {
    const run = data?.host_runs.find((host) => host.id === step.host_run_id);
    return {
      key: step.id,
      hostID: run?.host_id ?? "",
      hostName: run?.host_name ?? step.host_run_id,
      status: step.status,
      startedAt: step.started_at,
      finishedAt: step.finished_at,
      errorMessage: step.error_message,
      failureReason: step.failure_reason,
      hostRunID: step.host_run_id,
      stepRunID: step.id,
      exitCode: step.exit_code,
      truncated: step.truncated,
    };
  });
  const nodeStarted = nodeSteps.map((step) => step.started_at).filter((value): value is number => !!value);
  const nodeFinished = nodeSteps.map((step) => step.finished_at).filter((value): value is number => !!value);
  const nodeStartedAt = nodeStarted.length ? Math.min(...nodeStarted) : null;
  const nodeFinishedAt = active ? null : nodeFinished.length ? Math.max(...nodeFinished) : null;
  const nodeSummary = nodeSteps.find((step) => step.status === "failed" && step.error_message)?.error_message ?? "";

  const openNode = (nodeID: string) => {
    if (displayGraph?.nodes.find((n) => n.id === nodeID)?.type !== "tool") return;
    setSelectedNodeID(nodeID);
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
                { title: "工具历史", to: "/tools/executions" },
                { title: "执行详情" },
              ]
            : data?.kind === "flow"
              ? [
                  { title: "流程", to: "/flows" },
                  { title: "流程历史", to: "/flows/executions" },
                  { title: "执行详情" },
                ]
              : [{ title: "临时命令", to: "/adhoc" }, { title: "执行详情" }]
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
            {data?.kind === "tool" ? (
              <Button
                disabled={!data?.ref_id}
                onClick={() => {
                  if (!data) return;
                  setRerunInitial({
                    hostIDs: data.host_runs.map((run) => run.host_id),
                    params: data.params,
                  });
                  setRerunOpen(true);
                }}
              >
                再次执行
              </Button>
            ) : (
              <Button onClick={() => rerun.mutate()} loading={rerun.isPending} disabled={!data}>
                重新执行
              </Button>
            )}
            <Button href={id ? logDownloadUrl(id) : "#"} target="_blank">
              下载全部日志
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Card loading style={{ marginBottom: 16 }} />
      ) : data ? (
        <ToolRunInfo
          objectLabel={data.kind === "tool" ? "工具名称" : data.kind === "flow" ? "流程名称" : "执行对象"}
          objectName={data.name}
          objectHref={
            data.kind === "tool" && toolDetailPath
              ? toolDetailPath
              : data.kind === "flow" && data.ref_id
                ? `/flows/${data.ref_id}`
                : undefined
          }
          showVersion={data.kind === "tool"}
          versionNo={data.tool_version_no}
          operator={data.created_by_name || data.created_by || "—"}
          createdAt={data.created_at}
          startedAt={data.started_at}
          finishedAt={data.finished_at}
          sourceLabel={sourceLabel}
          params={displayParams}
          summary={data.error_summary}
          hosts={toolHosts}
        />
      ) : null}

      {data?.kind === "adhoc" && data.command && (
        <Card title="命令" style={{ marginBottom: 16 }}>
          <pre style={{ margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-all", fontSize: 12 }}>
            {data.command}
          </pre>
        </Card>
      )}

      {id && singleStep && (
        <ToolRunConsole
          executionId={id}
          hosts={toolHosts}
          active={!!active}
          onCancelHost={(hostRunID) => cancelHost.mutate(hostRunID)}
          cancelLoading={cancelHost.isPending}
        />
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
              graphNode.type === "start" ? (
                <Card style={{ marginBottom: 16 }}>
                  <Alert type="info" showIcon message="开始节点只用于触发后续分支，没有工具执行记录。" />
                </Card>
              ) : (
                <>
                  <ToolRunInfo
                    objectLabel="工具名称"
                    objectName={graphNode.tool_name || graphNode.name}
                    objectHref={graphNode.tool_id ? `/tools/${graphNode.tool_id}` : undefined}
                    showVersion
                    versionNo={graphNode.tool_version_no}
                    operator={data?.created_by_name || data?.created_by || "—"}
                    createdAt={data?.created_at ?? 0}
                    startedAt={nodeStartedAt}
                    finishedAt={nodeFinishedAt}
                    sourceLabel={sourceLabel}
                    params={graphNode.params ?? {}}
                    summary={nodeSummary}
                    hosts={nodeHosts}
                  />
                  <ToolRunConsole
                    key={graphNode.id}
                    executionId={id ?? ""}
                    hosts={nodeHosts}
                    active={!!active}
                    onCancelHost={(hostRunID) => cancelHost.mutate(hostRunID)}
                    cancelLoading={cancelHost.isPending}
                    emptyText="该节点没有执行记录"
                  />
                </>
              )
            ) : (
              <Card style={{ marginBottom: 16 }}>
                <Empty description="点击流程图中的节点查看该节点的工具执行详情" />
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

      <ExecuteToolModal
        toolID={data?.kind === "tool" ? data.ref_id : null}
        open={rerunOpen}
        onClose={() => setRerunOpen(false)}
        initialHostIDs={rerunInitial.hostIDs}
        initialParams={rerunInitial.params}
        rerun
      />
    </div>
  );
}
