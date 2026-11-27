import cronstrue from "cronstrue/i18n";

/** Human readable Chinese description for a 5-field cron expression. */
export function describeCron(expr: string): string {
  if (!expr.trim()) return "";
  try {
    return cronstrue.toString(expr, { locale: "zh_CN" });
  } catch {
    return "";
  }
}
