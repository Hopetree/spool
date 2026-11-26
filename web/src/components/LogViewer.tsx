import { useEffect, useRef, useState } from "react";
import { Checkbox, Space } from "antd";
import { listEvents, type ExecutionEvent, type ExecutionStatus } from "../api/executions";
import ExecutionStatusTag from "./ExecutionStatusTag";

interface Line {
  seq: number;
  stream: "stdout" | "stderr";
  data: string;
}

export default function LogViewer({
  executionId,
  active,
  hostRunID,
  stepRunID,
}: {
  executionId: string;
  active: boolean;
  hostRunID?: string;
  stepRunID?: string;
}) {
  const [lines, setLines] = useState<Line[]>([]);
  const [status, setStatus] = useState<ExecutionStatus>();
  const [follow, setFollow] = useState(true);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    let source: EventSource | undefined;
    setLines([]);

    const append = (ev: ExecutionEvent) => {
      if (cancelled || ev.type !== "log" || !ev.data) return;
      setLines((prev) => {
        if (prev.length >= 20000) return prev;
        return [...prev, { seq: ev.seq, stream: ev.stream ?? "stdout", data: ev.data ?? "" }];
      });
    };

    const load = async () => {
      try {
        const res = await listEvents(executionId, 0, 20000, hostRunID, stepRunID);
        if (cancelled) return;
        setStatus(res.status);
        setLines(
          res.items
            .filter((ev) => ev.type === "log" && ev.data)
            .map((ev) => ({ seq: ev.seq, stream: ev.stream ?? "stdout", data: ev.data ?? "" })),
        );
        if (!active) return;
        const last = res.items.length ? res.items[res.items.length - 1].seq : 0;
        const suffix = hostRunID ? `&host_run=${hostRunID}` : "";
        const stepSuffix = stepRunID ? `&step_run=${stepRunID}` : "";
        source = new EventSource(`/api/v1/executions/${executionId}/stream?after_seq=${last}${suffix}${stepSuffix}`);
        source.addEventListener("log", (e) => append(JSON.parse((e as MessageEvent).data) as ExecutionEvent));
        source.addEventListener("state", (e) => {
          const ev = JSON.parse((e as MessageEvent).data) as ExecutionEvent;
          if (ev.entity === "execution" && ev.status) setStatus(ev.status as ExecutionStatus);
        });
        source.addEventListener("end", () => source?.close());
      } catch {
        // keep the viewer empty; the page shows request errors elsewhere
      }
    };
    void load();
    return () => {
      cancelled = true;
      source?.close();
    };
  }, [executionId, active, hostRunID, stepRunID]);

  useEffect(() => {
    if (follow && boxRef.current) {
      boxRef.current.scrollTop = boxRef.current.scrollHeight;
    }
  }, [lines, follow]);

  return (
    <div>
      <Space style={{ marginBottom: 8 }}>
        <ExecutionStatusTag status={status ?? ""} />
        <Checkbox checked={follow} onChange={(e) => setFollow(e.target.checked)}>
          自动滚动
        </Checkbox>
        <span style={{ color: "#888" }}>{lines.length} 段输出</span>
      </Space>
      <div
        ref={boxRef}
        style={{
          maxHeight: 520,
          overflow: "auto",
          background: "#0b1020",
          color: "#d7e3f4",
          fontFamily: "Menlo, Consolas, monospace",
          fontSize: 12,
          lineHeight: 1.5,
          padding: 12,
          borderRadius: 6,
        }}
      >
        {lines.length === 0 ? (
          <span style={{ color: "#6b7a90" }}>暂无输出</span>
        ) : (
          lines.map((line) => (
            <div
              key={line.seq}
              style={{ whiteSpace: "pre-wrap", wordBreak: "break-all", color: line.stream === "stderr" ? "#ff9e9e" : "#d7e3f4" }}
            >
              {line.data}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
