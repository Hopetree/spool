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

const labels: Record<string, string> = {
  queued: "排队中",
  running: "运行中",
  success: "成功",
  partial: "部分成功",
  failed: "失败",
  cancelled: "已取消",
  interrupted: "已中断",
  skipped: "已跳过",
};

export default function ExecutionStatusTag({ status }: { status: string }) {
  return <Tag color={colors[status] ?? "default"}>{labels[status] ?? (status || "—")}</Tag>;
}
