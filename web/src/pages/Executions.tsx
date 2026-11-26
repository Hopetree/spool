import { useState } from "react";
import { Input, Select, Space, Table } from "antd";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import dayjs from "dayjs";
import { listExecutions, type Execution } from "../api/executions";
import ExecutionStatusTag from "../components/ExecutionStatusTag";

function duration(ex: Execution): string {
  if (!ex.started_at) return "—";
  const end = ex.finished_at ?? Date.now();
  const sec = Math.max(0, Math.round((end - ex.started_at) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

export default function Executions() {
  const [kind, setKind] = useState<string>();
  const [status, setStatus] = useState<string>();
  const [keyword, setKeyword] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["executions", { kind, status, keyword }],
    queryFn: () => listExecutions({ kind, status, keyword, page: 1, page_size: 100 }),
    refetchInterval: (query) => {
      const items = query.state.data?.items ?? [];
      return items.some((item) => item.status === "queued" || item.status === "running") ? 3000 : false;
    },
  });
  const items = data?.items ?? [];

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Select
          allowClear
          placeholder="类型"
          style={{ width: 120 }}
          value={kind}
          onChange={setKind}
          options={[{ value: "tool", label: "工具" }]}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 130 }}
          value={status}
          onChange={setStatus}
          options={["queued", "running", "success", "partial", "failed", "cancelled", "interrupted"].map((s) => ({
            value: s,
            label: s,
          }))}
        />
        <Input.Search allowClear placeholder="名称关键字" style={{ width: 220 }} onSearch={setKeyword} />
      </Space>

      <Table<Execution>
        rowKey="id"
        loading={isLoading}
        dataSource={items}
        pagination={{ pageSize: 20, showSizeChanger: false }}
        columns={[
          {
            title: "名称",
            render: (_, r) => <Link to={`/executions/${r.id}`}>{r.name || r.id}</Link>,
          },
          { title: "类型", dataIndex: "kind", width: 90 },
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
          { title: "操作人", dataIndex: "created_by", width: 140 },
          {
            title: "开始时间",
            dataIndex: "created_at",
            width: 170,
            render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm:ss"),
          },
          { title: "耗时", width: 90, render: (_, r) => duration(r) },
          {
            title: "摘要",
            dataIndex: "error_summary",
            ellipsis: true,
          },
        ]}
      />
    </div>
  );
}
