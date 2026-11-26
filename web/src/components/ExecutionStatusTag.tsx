import { Tag } from "antd";
import { statusColors, statusLabels } from "./status";

export default function ExecutionStatusTag({ status }: { status: string }) {
  return <Tag color={statusColors[status] ?? "default"}>{statusLabels[status] ?? (status || "—")}</Tag>;
}
