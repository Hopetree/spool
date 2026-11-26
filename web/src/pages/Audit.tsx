import { useState } from "react";
import { Input, Space, Table, Tag, Tooltip, Typography } from "antd";
import { useQuery } from "@tanstack/react-query";
import dayjs from "dayjs";
import { listAudit, type AuditRecord } from "../api/audit";

function pretty(json: string): string {
  try {
    return JSON.stringify(JSON.parse(json), null, 2);
  } catch {
    return json;
  }
}

export default function Audit() {
  const [action, setAction] = useState("");
  const [actor, setActor] = useState("");
  const [page, setPage] = useState(1);

  const { data, isLoading } = useQuery({
    queryKey: ["audit", { action, actor, page }],
    queryFn: () => listAudit({ action, actor_id: actor, page, page_size: 20 }),
  });

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Input.Search allowClear placeholder="操作，如 host.create" style={{ width: 240 }} onSearch={setAction} />
        <Input.Search allowClear placeholder="用户 ID" style={{ width: 240 }} onSearch={setActor} />
        <Typography.Text type="secondary">审计日志只读，保留 180 天</Typography.Text>
      </Space>
      <Table<AuditRecord>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={data?.items ?? []}
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
          { title: "用户", dataIndex: "username_snapshot", width: 140 },
          {
            title: "操作",
            dataIndex: "action",
            width: 190,
            render: (v: string) => <Tag>{v}</Tag>,
          },
          {
            title: "资源",
            width: 220,
            render: (_, r) => (
              <Space size={4}>
                <Tag>{r.resource_type || "—"}</Tag>
                <span>{r.resource_name || r.resource_id || ""}</span>
              </Space>
            ),
          },
          {
            title: "摘要",
            dataIndex: "summary_json",
            ellipsis: true,
            render: (v: string) => (
              <Tooltip title={<pre style={{ margin: 0, whiteSpace: "pre-wrap" }}>{pretty(v)}</pre>}>
                <span>{v === "{}" ? "—" : v}</span>
              </Tooltip>
            ),
          },
          { title: "IP", dataIndex: "ip", width: 130 },
        ]}
      />
    </div>
  );
}
