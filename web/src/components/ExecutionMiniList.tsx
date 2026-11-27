import { List } from "antd";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { listExecutions, type Execution } from "../api/executions";
import ExecutionStatusTag from "./ExecutionStatusTag";

function duration(ex: Execution): string {
  if (!ex.started_at) return "—";
  const sec = Math.max(0, Math.round(((ex.finished_at ?? Date.now()) - ex.started_at) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

/** Compact execution list for side panels. */
export default function ExecutionMiniList({
  kind,
  refID,
  limit = 6,
}: {
  kind?: "tool" | "flow" | "adhoc";
  refID?: string;
  limit?: number;
}) {
  const navigate = useNavigate();
  const { data, isLoading } = useQuery({
    queryKey: ["executions", "mini", kind ?? "", refID ?? ""],
    queryFn: () => listExecutions({ kind, ref_id: refID, page: 1, page_size: limit }),
    refetchInterval: (query) => {
      const items = query.state.data?.items ?? [];
      return items.some((item) => item.status === "queued" || item.status === "running") ? 3000 : 15000;
    },
  });

  return (
    <List<Execution>
      loading={isLoading}
      dataSource={data?.items ?? []}
      locale={{ emptyText: "暂无执行记录" }}
      split
      renderItem={(ex) => (
        <List.Item
          style={{ padding: "8px 0", cursor: "pointer" }}
          onClick={() => navigate(`/executions/${ex.id}`)}
        >
          <div style={{ width: "100%", minWidth: 0 }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
              <span
                style={{
                  fontWeight: 500,
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                }}
              >
                {ex.name || ex.id}
              </span>
              <ExecutionStatusTag status={ex.status} />
            </div>
            <div style={{ fontSize: 12, color: "#8c8c8c", marginTop: 2 }}>
              {dayjs(ex.created_at).format("MM-DD HH:mm")} · 主机 {ex.host_success}/{ex.host_total} ·{" "}
              {duration(ex)}
            </div>
          </div>
        </List.Item>
      )}
    />
  );
}
