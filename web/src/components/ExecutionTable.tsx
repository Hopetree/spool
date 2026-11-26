import { useState } from "react";
import { Input, Select, Space, Table } from "antd";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { listExecutions, type Execution } from "../api/executions";
import ExecutionStatusTag from "./ExecutionStatusTag";

interface Props {
  kind: "tool" | "flow" | "adhoc";
  refID?: string;
  showFilters?: boolean;
  pageSize?: number;
}

function duration(ex: Execution): string {
  if (!ex.started_at) return "—";
  const sec = Math.max(0, Math.round(((ex.finished_at ?? Date.now()) - ex.started_at) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

export default function ExecutionTable({ kind, refID, showFilters, pageSize = 20 }: Props) {
  const navigate = useNavigate();
  const [status, setStatus] = useState<string>();
  const [keyword, setKeyword] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["executions", kind, refID ?? "", status, keyword],
    queryFn: () =>
      listExecutions({
        kind,
        ref_id: refID,
        status,
        keyword,
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
            style={{ width: 140 }}
            value={status}
            onChange={setStatus}
            options={["queued", "running", "success", "partial", "failed", "cancelled", "interrupted"].map((s) => ({
              value: s,
              label: s,
            }))}
          />
          <Input.Search allowClear placeholder="名称关键字" style={{ width: 240 }} onSearch={setKeyword} />
        </Space>
      )}
      <Table<Execution>
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        pagination={{ pageSize, showSizeChanger: false }}
        onRow={(r) => ({ onClick: () => navigate(`/executions/${r.id}`), style: { cursor: "pointer" } })}
        columns={[
          {
            title: "执行",
            render: (_, r) => (
              <Link to={`/executions/${r.id}`} onClick={(e) => e.stopPropagation()}>
                {r.name || r.id}
              </Link>
            ),
          },
          {
            title: "状态",
            dataIndex: "status",
            width: 120,
            render: (v: string) => <ExecutionStatusTag status={v} />,
          },
          {
            title: "主机",
            width: 100,
            render: (_, r) => `${r.host_success}/${r.host_total}`,
          },
          {
            title: "操作人",
            width: 160,
            render: (_, r) => r.created_by_name || r.created_by || "—",
          },
          {
            title: "开始时间",
            dataIndex: "created_at",
            width: 170,
            render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm:ss"),
          },
          { title: "耗时", width: 90, render: (_, r) => duration(r) },
          { title: "摘要", dataIndex: "error_summary", ellipsis: true },
        ]}
      />
    </div>
  );
}
