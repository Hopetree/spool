/** "3 分钟后" style description for a future timestamp. */
export function relativeFuture(ts: number): string {
  const mins = Math.max(0, Math.round((ts - Date.now()) / 60000));
  if (mins < 1) return "即将运行";
  if (mins < 60) return `${mins} 分钟后`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours} 小时后`;
  return `${Math.round(hours / 24)} 天后`;
}
