import { useState } from "react";
import { Input, Select, Space, Table } from "antd";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { listExecutions, type Execution } from "../api/executions";
import { listSchedules } from "../api/schedules";
import ExecutionStatusTag from "./ExecutionStatusTag";

interface Props {
  kind?: "tool" | "flow" | "adhoc";
  refID?: string;
  scheduleID?: string;
  scheduledOnly?: boolean;
  showSchedule?: boolean;
  showFilters?: boolean;
  showKind?: boolean;
  pageSize?: number;
}

const statusOptions = [
  { value: "queued", label: "排队中" },
  { value: "running", label: "运行中" },
  { value: "success", label: "成功" },
  { value: "partial", label: "部分成功" },
  { value: "failed", label: "失败" },
  { value: "cancelled", label: "已取消" },
  { value: "interrupted", label: "已中断" },
];

const kindLabels: Record<string, string> = { tool: "工具", flow: "流程", adhoc: "临时命令" };

function duration(ex: Execution): string {
  if (!ex.started_at) return "—";
  const sec = Math.max(0, Math.round(((ex.finished_at ?? Date.now()) - ex.started_at) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

export default function ExecutionTable({
  kind,
  refID,
  scheduleID,
  scheduledOnly,
  showSchedule,
  showFilters,
  showKind,
  pageSize = 20,
}: Props) {
  const navigate = useNavigate();
  const [status, setStatus] = useState<string>();
  const [keyword, setKeyword] = useState("");
  const [range, setRange] = useState("all");

  const { data: schedulePage } = useQuery({
    queryKey: ["schedules", "names"],
    queryFn: () => listSchedules(),
    enabled: !!showSchedule,
  });
  const scheduleName = (id: string) => schedulePage?.items.find((s) => s.id === id)?.name ?? id;

  const { data, isLoading } = useQuery({
    queryKey: [
      "executions",
      kind ?? "",
      refID ?? "",
      scheduleID ?? "",
      scheduledOnly ? "scheduled" : "",
      status,
      keyword,
      range,
    ],
    queryFn: () =>
      listExecutions({
        kind,
        ref_id: refID,
        schedule_id: scheduleID,
        scheduled: scheduledOnly,
        status,
        keyword,
        from: range === "all" ? undefined : dayjs().subtract(Number(range), "hour").valueOf(),
        page: 1,
        page_size: 100,
      }),
    refetchInterval: (query) => {
      const items = query.state.data?.items ?? [];
      return items.some((item) => item.status === "queued" || item.status === "running") ? 3000 : false;
    },
  });

  return (
    <div>
      {showFilters && (
        <Space style={{ marginBottom: 16 }} wrap>
          <Select
            allowClear
            placeholder="状态"
            style={{ width: 130 }}
            value={status}
            onChange={setStatus}
            options={statusOptions}
          />
          <Select
            style={{ width: 130 }}
            value={range}
            onChange={setRange}
            options={[
              { value: "all", label: "全部时间" },
              { value: "24", label: "近 24 小时" },
              { value: "168", label: "近 7 天" },
              { value: "720", label: "近 30 天" },
            ]}
          />
          <Input.Search allowClear placeholder="名称关键字" style={{ width: 240 }} onSearch={setKeyword} />
        </Space>
      )}
      <Table<Execution>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={data?.items ?? []}
        pagination={{ pageSize, showSizeChanger: false }}
        onRow={(r) => ({ onClick: () => navigate(`/executions/${r.id}`), style: { cursor: "pointer" } })}
        columns={[
          {
            title: showSchedule ? "执行对象" : "执行",
            width: showSchedule ? 170 : undefined,
            ellipsis: true,
            render: (_, r) => (
              <Link to={`/executions/${r.id}`} onClick={(e) => e.stopPropagation()}>
                {r.name || r.id}
              </Link>
            ),
          },
          ...(showSchedule
            ? [
                {
                  title: "定时任务",
                  width: 180,
                  ellipsis: true,
                  render: (_: unknown, r: Execution) =>
                    r.schedule_id ? (
                      <Link to={`/schedules/${r.schedule_id}`} onClick={(e) => e.stopPropagation()}>
                        {scheduleName(r.schedule_id)}
                      </Link>
                    ) : (
                      "—"
                    ),
                },
                {
                  title: "触发",
                  dataIndex: "source",
                  width: 70,
                  render: (v: string) => (v === "schedule" ? "定时" : "手动"),
                },
              ]
            : []),
          ...(showKind
            ? [
                {
                  title: "类型",
                  dataIndex: "kind",
                  width: 80,
                  render: (v: string) => kindLabels[v] ?? v,
                },
              ]
            : []),
          {
            title: "状态",
            dataIndex: "status",
            width: 90,
            render: (v: string) => <ExecutionStatusTag status={v} />,
          },
          {
            title: "主机",
            width: 70,
            render: (_, r) => `${r.host_success}/${r.host_total}`,
          },
          {
            title: "操作人",
            width: 120,
            render: (_, r) => r.created_by_name || r.created_by || "—",
          },
          {
            title: "开始时间",
            dataIndex: "created_at",
            width: 160,
            render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm:ss"),
          },
          { title: "耗时", width: 70, render: (_, r) => duration(r) },
          { title: "摘要", dataIndex: "error_summary", ellipsis: true },
        ]}
      />
    </div>
  );
}
