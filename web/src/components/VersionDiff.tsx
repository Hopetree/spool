import { useMemo, type CSSProperties } from "react";
import { diffLines, structuredPatch, type Hunk } from "diff";

export type DiffMode = "unified" | "split";

export interface DiffStats {
  added: number;
  deleted: number;
}

function splitLines(value: string): string[] {
  if (!value) return [];
  const lines = value.split("\n");
  if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop();
  return lines;
}

export function diffStats(oldText: string, newText: string): DiffStats {
  let added = 0;
  let deleted = 0;
  for (const part of diffLines(oldText, newText)) {
    const count = splitLines(part.value).length;
    if (part.added) added += count;
    else if (part.removed) deleted += count;
  }
  return { added, deleted };
}

export function mergeDiffStats(...stats: DiffStats[]): DiffStats {
  return stats.reduce(
    (sum, item) => ({ added: sum.added + item.added, deleted: sum.deleted + item.deleted }),
    { added: 0, deleted: 0 },
  );
}

type UnifiedRow =
  | { type: "hunk"; text: string }
  | { type: "line"; kind: "context" | "add" | "del"; oldNo?: number; newNo?: number; text: string };

interface SplitSide {
  no: number;
  text: string;
}

type SplitRow = { type: "hunk"; text: string } | { type: "line"; old?: SplitSide; new?: SplitSide };

function buildUnified(hunks: Hunk[]): UnifiedRow[] {
  const rows: UnifiedRow[] = [];
  for (const hunk of hunks) {
    rows.push({
      type: "hunk",
      text: `@@ -${hunk.oldStart},${hunk.oldLines} +${hunk.newStart},${hunk.newLines} @@`,
    });
    let oldNo = hunk.oldStart;
    let newNo = hunk.newStart;
    for (const raw of hunk.lines) {
      const prefix = raw[0];
      const text = raw.slice(1);
      if (prefix === "+") {
        rows.push({ type: "line", kind: "add", newNo: newNo++, text });
      } else if (prefix === "-") {
        rows.push({ type: "line", kind: "del", oldNo: oldNo++, text });
      } else if (prefix === " ") {
        rows.push({ type: "line", kind: "context", oldNo: oldNo++, newNo: newNo++, text });
      }
      // "\ No newline at end of file" markers are not user-facing.
    }
  }
  return rows;
}

function buildSplit(hunks: Hunk[]): SplitRow[] {
  const rows: SplitRow[] = [];
  for (const hunk of hunks) {
    rows.push({
      type: "hunk",
      text: `@@ -${hunk.oldStart},${hunk.oldLines} +${hunk.newStart},${hunk.newLines} @@`,
    });
    let oldNo = hunk.oldStart;
    let newNo = hunk.newStart;
    let i = 0;
    while (i < hunk.lines.length) {
      const raw = hunk.lines[i];
      const prefix = raw[0];
      if (prefix === " ") {
        const text = raw.slice(1);
        rows.push({ type: "line", old: { no: oldNo++, text }, new: { no: newNo++, text } });
        i += 1;
        continue;
      }
      if (prefix !== "-" && prefix !== "+") {
        i += 1;
        continue;
      }

      const removed: string[] = [];
      const added: string[] = [];
      while (i < hunk.lines.length) {
        const line = hunk.lines[i];
        const linePrefix = line[0];
        if (linePrefix === " ") break;
        if (linePrefix === "-") removed.push(line.slice(1));
        if (linePrefix === "+") added.push(line.slice(1));
        i += 1;
      }
      const count = Math.max(removed.length, added.length);
      for (let j = 0; j < count; j += 1) {
        const oldText = removed[j];
        const newText = added[j];
        rows.push({
          type: "line",
          old: oldText !== undefined ? { no: oldNo++, text: oldText } : undefined,
          new: newText !== undefined ? { no: newNo++, text: newText } : undefined,
        });
      }
    }
  }
  return rows;
}

const addBg = "#e6ffec";
const addFg = "#116329";
const delBg = "#ffebe9";
const delFg = "#a40e26";
const emptyBg = "#fafafa";

const cell: CSSProperties = {
  padding: "0 8px",
  verticalAlign: "top",
  fontFamily: "Menlo, Consolas, monospace",
  fontSize: 12,
  lineHeight: "20px",
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
};

const lineNo: CSSProperties = {
  ...cell,
  width: 48,
  minWidth: 48,
  color: "#8c8c8c",
  textAlign: "right",
  userSelect: "none",
  padding: "0 6px",
};

const hunkCell: CSSProperties = {
  ...cell,
  padding: "4px 8px",
  background: "#f6f8fa",
  color: "#57606a",
  fontSize: 11,
  lineHeight: "18px",
  borderTop: "1px solid #eaeef2",
  borderBottom: "1px solid #eaeef2",
};

function EmptyDiff() {
  return <div style={{ padding: "16px 12px", color: "#8c8c8c", fontSize: 12 }}>两个版本内容一致</div>;
}

export default function VersionDiff({
  oldText,
  newText,
  mode,
}: {
  oldText: string;
  newText: string;
  mode: DiffMode;
}) {
  const hunks = useMemo(
    () => structuredPatch("", "", oldText, newText, "", "", { context: 3 }).hunks,
    [oldText, newText],
  );
  const unifiedRows = useMemo(() => buildUnified(hunks), [hunks]);
  const splitRows = useMemo(() => buildSplit(hunks), [hunks]);

  if (mode === "unified") {
    if (unifiedRows.length === 0) return <EmptyDiff />;
    return (
      <div style={{ overflowX: "auto" }}>
        <table style={{ width: "100%", borderCollapse: "collapse", tableLayout: "fixed" }}>
          <colgroup>
            <col style={{ width: 48 }} />
            <col style={{ width: 48 }} />
            <col style={{ width: 24 }} />
            <col />
          </colgroup>
          <tbody>
            {unifiedRows.map((row, index) =>
              row.type === "hunk" ? (
                <tr key={`h-${index}`}>
                  <td colSpan={4} style={hunkCell}>
                    {row.text}
                  </td>
                </tr>
              ) : (
                <tr
                  key={`l-${index}`}
                  style={{
                    background: row.kind === "add" ? addBg : row.kind === "del" ? delBg : undefined,
                    color: row.kind === "add" ? addFg : row.kind === "del" ? delFg : "#262626",
                  }}
                >
                  <td style={lineNo}>{row.oldNo ?? ""}</td>
                  <td style={lineNo}>{row.newNo ?? ""}</td>
                  <td style={{ ...cell, width: 24, padding: 0, textAlign: "center", userSelect: "none" }}>
                    {row.kind === "add" ? "+" : row.kind === "del" ? "-" : " "}
                  </td>
                  <td style={cell}>{row.text || " "}</td>
                </tr>
              ),
            )}
          </tbody>
        </table>
      </div>
    );
  }

  if (splitRows.length === 0) return <EmptyDiff />;
  return (
    <div style={{ overflowX: "auto" }}>
      <table style={{ width: "100%", borderCollapse: "collapse", tableLayout: "fixed" }}>
        <colgroup>
          <col style={{ width: 48 }} />
          <col />
          <col style={{ width: 48 }} />
          <col />
        </colgroup>
        <tbody>
          {splitRows.map((row, index) => {
            if (row.type === "hunk") {
              return (
                <tr key={`h-${index}`}>
                  <td colSpan={4} style={hunkCell}>
                    {row.text}
                  </td>
                </tr>
              );
            }
            const same = !!row.old && !!row.new && row.old.text === row.new.text;
            const oldBg = row.old ? (same ? undefined : delBg) : emptyBg;
            const newBg = row.new ? (same ? undefined : addBg) : emptyBg;
            return (
              <tr key={`l-${index}`}>
                <td style={{ ...lineNo, background: oldBg }}>{row.old?.no ?? ""}</td>
                <td
                  style={{
                    ...cell,
                    background: oldBg,
                    color: row.old && !same ? delFg : "#262626",
                  }}
                >
                  {row.old?.text || " "}
                </td>
                <td
                  style={{
                    ...lineNo,
                    background: newBg,
                    padding: "0 6px",
                    borderLeft: "1px solid #f0f0f0",
                  }}
                >
                  {row.new?.no ?? ""}
                </td>
                <td
                  style={{
                    ...cell,
                    background: newBg,
                    color: row.new && !same ? addFg : "#262626",
                  }}
                >
                  {row.new?.text || " "}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
