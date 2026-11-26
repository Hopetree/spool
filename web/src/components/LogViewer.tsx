import { useEffect, useRef, useState } from "react";
import { App as AntApp, Button, Checkbox, Input, Space } from "antd";
import { CopyOutlined, DownloadOutlined } from "@ant-design/icons";
import { listEvents, logDownloadUrl, type ExecutionEvent } from "../api/executions";

interface Line {
  seq: number;
  stream: "stdout" | "stderr";
  data: string;
}

function highlight(text: string, query: string) {
  if (!query) return text;
  const idx = text.toLowerCase().indexOf(query.toLowerCase());
  if (idx < 0) return text;
  return (
    <>
      {text.slice(0, idx)}
      <mark style={{ background: "#ffd666", color: "#111", borderRadius: 2 }}>{text.slice(idx, idx + query.length)}</mark>
      {text.slice(idx + query.length)}
    </>
  );
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
  const { message } = AntApp.useApp();
  const [lines, setLines] = useState<Line[]>([]);
  const [follow, setFollow] = useState(true);
  const [wrap, setWrap] = useState(true);
  const [lineNumbers, setLineNumbers] = useState(true);
  const [query, setQuery] = useState("");
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

  const visible = lines
    .map((line, idx) => ({ ...line, no: idx + 1 }))
    .filter((line) => !query || line.data.toLowerCase().includes(query.toLowerCase()));

  const copyAll = async () => {
    try {
      await navigator.clipboard.writeText(lines.map((l) => l.data).join(""));
      message.success("已复制全部日志");
    } catch {
      message.error("复制失败");
    }
  };

  return (
    <div>
      <Space style={{ marginBottom: 8 }} wrap size={8}>
        <Input
          size="small"
          allowClear
          placeholder="搜索日志"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          style={{ width: 180 }}
        />
        <Button size="small" icon={<CopyOutlined />} onClick={copyAll}>
          复制
        </Button>
        <Button
          size="small"
          icon={<DownloadOutlined />}
          href={logDownloadUrl(executionId, hostRunID, stepRunID)}
          target="_blank"
        >
          下载
        </Button>
        <Checkbox checked={lineNumbers} onChange={(e) => setLineNumbers(e.target.checked)}>
          行号
        </Checkbox>
        <Checkbox checked={wrap} onChange={(e) => setWrap(e.target.checked)}>
          换行
        </Checkbox>
        <Checkbox checked={follow} onChange={(e) => setFollow(e.target.checked)}>
          自动滚动
        </Checkbox>
        <span style={{ color: "#888" }}>
          {query ? `${visible.length} / ${lines.length} 段` : `${lines.length} 段输出`}
        </span>
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
        ) : visible.length === 0 ? (
          <span style={{ color: "#6b7a90" }}>没有匹配的日志</span>
        ) : (
          visible.map((line) => (
            <div
              key={line.seq}
              style={{
                display: "flex",
                gap: 10,
                whiteSpace: wrap ? "pre-wrap" : "pre",
                wordBreak: wrap ? "break-all" : "normal",
                color: line.stream === "stderr" ? "#ff9e9e" : "#d7e3f4",
              }}
            >
              {lineNumbers && (
                <span style={{ color: "#4c5d78", userSelect: "none", textAlign: "right", minWidth: 32, flex: "none" }}>
                  {line.no}
                </span>
              )}
              <span style={{ minWidth: 0 }}>{highlight(line.data, query)}</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
