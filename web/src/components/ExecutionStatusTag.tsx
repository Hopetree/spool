import { Tag } from "antd";

const colors: Record<string, string> = {
  queued: "default",
  running: "processing",
  success: "green",
  partial: "orange",
  failed: "red",
  cancelled: "default",
  interrupted: "magenta",
  skipped: "default",
};

export default function ExecutionStatusTag({ status }: { status: string }) {
  return <Tag color={colors[status] ?? "default"}>{status}</Tag>;
}
