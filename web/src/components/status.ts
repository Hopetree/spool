/** Shared status colors/labels for execution, host run and step run states. */
export const statusColors: Record<string, string> = {
  queued: "#8c8c8c",
  running: "#1677ff",
  success: "#52c41a",
  partial: "#fa8c16",
  failed: "#ff4d4f",
  cancelled: "#8c8c8c",
  interrupted: "#eb2f96",
  skipped: "#bfbfbf",
};

export const statusLabels: Record<string, string> = {
  queued: "排队中",
  running: "运行中",
  success: "成功",
  partial: "部分成功",
  failed: "失败",
  cancelled: "已取消",
  interrupted: "已中断",
  skipped: "已跳过",
};

export function statusLabel(status: string): string {
  return statusLabels[status] ?? status;
}

/** Aggregates per-host statuses of one graph node into one display status. */
export function aggregateStatus(statuses: string[]): string {
  if (statuses.length === 0) return "";
  const has = (s: string) => statuses.includes(s);
  if (has("running")) return "running";
  if (has("failed")) return "failed";
  if (has("cancelled")) return "cancelled";
  if (has("interrupted")) return "interrupted";
  if (has("queued")) return "queued";
  if (has("success")) return "success";
  return "skipped";
}
