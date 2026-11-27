import { useState } from "react";
import { Input, List, Pagination, Select, Space } from "antd";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { listExecutions, type Execution } from "../api/executions";
import ExecutionStatusTag from "./ExecutionStatusTag";

const statusOptions = [
  { value: "queued", label: "排队中" },
  { value: "running", label: "运行中" },
  { value: "success", label: "成功" },
  { value: "partial", label: "部分成功" },
  { value: "failed", label: "失败" },
  { value: "cancelled", label: "已取消" },
  { value: "interrupted", label: "已中断" },
];

function duration(ex: Execution): string {
  if (!ex.started_at) return "—";
  const sec = Math.max(0, Math.round(((ex.finished_at ?? Date.now()) - ex.started_at) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

/** Ad-hoc execution history with server-side filtering and pagination. */
export default function AdhocHistoryPanel({ pageSize = 6 }: { pageSize?: number }) {
  const navigate = useNavigate();
  const [status, setStatus] = useState<string>();
  const [keyword, setKeyword] = useState("");
  const [page, setPage] = useState(1);

  const { data, isLoading } = useQuery({
    queryKey: ["executions", "adhoc-panel", status ?? "", keyword, page],
    queryFn: () =>
      listExecutions({ kind: "adhoc", status, keyword, page, page_size: pageSize }),
    refetchInterval: (query) => {
      const items = query.state.data?.items ?? [];
      return items.some((item) => item.status === "queued" || item.status === "running") ? 3000 : 15000;
    },
  });

  const items = data?.items ?? [];
  const total = data?.total ?? 0;

  const reset = (next: () => void) => {
    next();
    setPage(1);
  };

  return (
    <div>
      <Space style={{ marginBottom: 8 }} wrap size={8}>
        <Select
          size="small"
          allowClear
          placeholder="状态"
          style={{ width: 100 }}
          value={status}
          onChange={(v) => reset(() => setStatus(v))}
          options={statusOptions}
        />
        <Input.Search
          size="small"
          allowClear
          placeholder="搜索命令"
          style={{ width: 240 }}
          onSearch={(v) => reset(() => setKeyword(v))}
        />
      </Space>

      <List<Execution>
        loading={isLoading}
        dataSource={items}
        locale={{ emptyText: status || keyword ? "没有匹配的执行记录" : "暂无执行记录" }}
        split
        renderItem={(ex) => (
          <List.Item
            style={{ padding: "8px 0", cursor: "pointer" }}
            onClick={() => navigate(`/executions/${ex.id}`)}
          >
            <div style={{ width: "100%", minWidth: 0 }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
                <span
                  title={ex.command || ex.name}
                  style={{
                    fontFamily: "Menlo, Consolas, monospace",
                    fontSize: 12,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                >
                  {(ex.command || ex.name).replace(/\s+/g, " ").slice(0, 100)}
                </span>
                <ExecutionStatusTag status={ex.status} />
              </div>
              <div style={{ fontSize: 12, color: "#8c8c8c", marginTop: 2 }}>
                {ex.created_by_name || ex.created_by || "—"} · {dayjs(ex.created_at).format("MM-DD HH:mm")} · 主机{" "}
                {ex.host_success}/{ex.host_total} · {duration(ex)}
              </div>
              {ex.error_summary && (
                <div
                  style={{
                    fontSize: 12,
                    color: "#cf1322",
                    marginTop: 2,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                  title={ex.error_summary}
                >
                  {ex.error_summary}
                </div>
              )}
            </div>
          </List.Item>
        )}
      />

      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 8,
          marginTop: 4,
        }}
      >
        <span style={{ fontSize: 12, color: "#8c8c8c" }}>共 {total} 条</span>
        <Pagination
          size="small"
          current={page}
          pageSize={pageSize}
          total={total}
          showSizeChanger={false}
          hideOnSinglePage={false}
          onChange={setPage}
        />
      </div>
    </div>
  );
}
