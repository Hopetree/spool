import { statusColors, statusLabels } from "./status";

/**
 * Lightweight status pill: pale background, colored text and a status dot.
 * Solid blocks were too loud in tables and headers.
 */
export default function ExecutionStatusTag({ status }: { status: string }) {
  const color = statusColors[status] ?? "#8c8c8c";
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 5,
        padding: "0 8px",
        lineHeight: "20px",
        borderRadius: 10,
        fontSize: 12,
        whiteSpace: "nowrap",
        color,
        background: `${color}14`,
        border: `1px solid ${color}33`,
      }}
    >
      <span style={{ width: 6, height: 6, borderRadius: 3, background: color, flex: "none" }} />
      {statusLabels[status] ?? (status || "—")}
    </span>
  );
}
