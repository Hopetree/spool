import { useEffect, useState } from "react";
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Empty,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
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
import ExecutionStatusTag from "../components/ExecutionStatusTag";
import LogViewer from "../components/LogViewer";

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

  const { data, isLoading } = useQuery({
    queryKey: ["execution", id],
    queryFn: () => getExecution(id!),
    enabled: !!id,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "queued" || status === "running" ? 1500 : false;
    },
  });

  useEffect(() => {
    const hosts = data?.host_runs ?? [];
    setSelectedHost((prev) => {
      if (prev && hosts.some((h) => h.id === prev)) return prev;
      return hosts[0]?.id;
    });
  }, [data]);

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
  const currentHost = data?.host_runs.find((h) => h.id === selectedHost);
  const backPath =
    data?.kind === "flow" ? "/flows/executions" : data?.kind === "adhoc" ? "/adhoc/history" : "/tools/executions";

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate(backPath)}>返回列表</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {data?.name || "执行详情"}
        </Typography.Title>
        {data && <ExecutionStatusTag status={data.status} />}
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
      </Space>

      <Card loading={isLoading} style={{ marginBottom: 16 }}>
        {data && (
          <Descriptions
            column={3}
            size="small"
            items={[
              { key: "id", label: "执行 ID", children: data.id },
              { key: "operator", label: "操作人", children: data.created_by || "—" },
              { key: "source", label: "来源", children: data.source },
              { key: "created", label: "创建时间", children: dayjs(data.created_at).format("YYYY-MM-DD HH:mm:ss") },
              { key: "started", label: "开始时间", children: data.started_at ? dayjs(data.started_at).format("HH:mm:ss") : "—" },
              { key: "duration", label: "耗时", children: duration(data.started_at, data.finished_at) },
              {
                key: "hosts",
                label: "主机统计",
                children: `成功 ${data.host_success} / 失败 ${data.host_failed} / 取消 ${data.host_cancelled} / 共 ${data.host_total}`,
              },
              { key: "summary", label: "摘要", children: data.error_summary || "—" },
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
        <Card
          title={
            <Space>
              主机与输出
              <Typography.Text type="secondary" style={{ fontWeight: 400, fontSize: 12 }}>
                共 {data?.host_runs.length ?? 0} 台
              </Typography.Text>
            </Space>
          }
          style={{ marginBottom: 16 }}
        >
          {!data || data.host_runs.length === 0 ? (
            <Empty description="没有主机执行记录" />
          ) : (
            <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
              <div style={{ width: 320, flex: "0 0 320px", maxHeight: 600, overflow: "auto" }}>
                {data.host_runs.map((h) => {
                  const step = stepByHost.get(h.id);
                  const selected = h.id === selectedHost;
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
                })}
              </div>

              <div style={{ flex: 1, minWidth: 0 }}>
                {currentHost && (
                  <div
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      marginBottom: 8,
                      gap: 8,
                    }}
                  >
                    <Space>
                      <Typography.Text strong>{currentHost.host_name}</Typography.Text>
                      <ExecutionStatusTag status={currentHost.status} />
                      {currentHost.error_message && (
                        <Typography.Text type="danger" style={{ fontSize: 12 }}>
                          {currentHost.error_message}
                        </Typography.Text>
                      )}
                    </Space>
                    <Button size="small" href={logDownloadUrl(id, currentHost.id)} target="_blank">
                      下载该主机日志
                    </Button>
                  </div>
                )}
                {currentHost?.error_message && (
                  <Alert type="error" showIcon message={currentHost.error_message} style={{ marginBottom: 8 }} />
                )}
                <LogViewer executionId={id} active={!!active} hostRunID={selectedHost} />
              </div>
            </div>
          )}
        </Card>
      )}

      {!singleStep && (
        <>
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
                    <Button size="small" href={id ? logDownloadUrl(id, r.host_run_id, r.id) : "#"} target="_blank">
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
    </div>
  );
}
