export interface RecentTool {
  id: string;
  name: string;
  visited_at: number;
}

const maxRecentTools = 6;
const storagePrefix = "spool.recentTools";

function storageKey(userKey: string | undefined): string {
  return `${storagePrefix}.${userKey || "anonymous"}`;
}

/** Recent tools are browser-local only; they are never stored on the server. */
export function loadRecentTools(userKey: string | undefined): RecentTool[] {
  try {
    const raw = localStorage.getItem(storageKey(userKey));
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((item): item is RecentTool => {
        if (!item || typeof item !== "object") return false;
        const record = item as Partial<RecentTool>;
        return typeof record.id === "string" && typeof record.name === "string";
      })
      .slice(0, maxRecentTools);
  } catch {
    return [];
  }
}

export function recordRecentTool(userKey: string | undefined, tool: { id: string; name: string }): void {
  if (!tool.id) return;
  const next: RecentTool[] = [
    { id: tool.id, name: tool.name, visited_at: Date.now() },
    ...loadRecentTools(userKey).filter((item) => item.id !== tool.id),
  ].slice(0, maxRecentTools);
  try {
    localStorage.setItem(storageKey(userKey), JSON.stringify(next));
  } catch {
    // Private mode or full storage: recent tools are best-effort only.
  }
}
