import CodeMirror from "@uiw/react-codemirror";
import { python } from "@codemirror/lang-python";
import { StreamLanguage, indentUnit } from "@codemirror/language";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { App as AntApp, Button, Tooltip } from "antd";
import { CopyOutlined, DownloadOutlined } from "@ant-design/icons";
import type { ToolType } from "../api/tools";

interface Props {
  language: ToolType;
  value?: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  downloadName?: string;
}

// oneDark at 12px renders lines at 16.8px; 4px content padding top/bottom.
const lineHeight = 16.8;
const contentPadding = 8;
const minEditorHeight = 320;
const maxVisibleLines = 500;

export default function CodeEditor({
  language,
  value,
  onChange,
  readOnly,
  downloadName,
}: Props) {
  const { message } = AntApp.useApp();
  const extensions = [language === "python" ? python() : StreamLanguage.define(shell), indentUnit.of("    ")];
  const lineCount = Math.max(1, (value ?? "").split("\n").length);
  const visibleLines = Math.min(lineCount, maxVisibleLines);
  const height = `${Math.max(minEditorHeight, Math.ceil(visibleLines * lineHeight + contentPadding))}px`;

  const copyScript = async () => {
    const text = value ?? "";
    try {
      if (!navigator.clipboard?.writeText) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(text);
      message.success("已复制脚本内容");
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = text;
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      try {
        document.execCommand("copy");
        message.success("已复制脚本内容");
      } catch {
        message.error("复制失败");
      } finally {
        textarea.remove();
      }
    }
  };

  const downloadScript = () => {
    const extension = language === "python" ? "py" : "sh";
    const fallback = `script.${extension}`;
    const name = (downloadName?.trim() || fallback).replace(/[\\/:*?"<>|]+/g, "_");
    const url = URL.createObjectURL(new Blob([value ?? ""], { type: "text/plain;charset=utf-8" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = name;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  return (
    <div style={{ position: "relative", border: "1px solid #1f2329", borderRadius: 6, overflow: "hidden" }}>
      <div
        style={{
          position: "absolute",
          top: 8,
          right: 8,
          zIndex: 5,
          display: "flex",
          gap: 2,
          padding: 2,
          border: "1px solid #30363d",
          borderRadius: 6,
          background: "rgba(22, 27, 34, 0.92)",
        }}
      >
        <Tooltip title="复制内容">
          <Button
            type="text"
            size="small"
            aria-label="复制脚本内容"
            icon={<CopyOutlined />}
            disabled={!value}
            onClick={() => void copyScript()}
            style={{ color: value ? "#c9d1d9" : "#5c6370" }}
          />
        </Tooltip>
        <Tooltip title="下载代码">
          <Button
            type="text"
            size="small"
            aria-label="下载脚本"
            icon={<DownloadOutlined />}
            disabled={!value}
            onClick={downloadScript}
            style={{ color: value ? "#c9d1d9" : "#5c6370" }}
          />
        </Tooltip>
      </div>
      <CodeMirror
        value={value ?? ""}
        height={height}
        theme="dark"
        extensions={extensions}
        onChange={(v) => onChange?.(v)}
        editable={!readOnly}
        basicSetup={{ lineNumbers: true, foldGutter: false, highlightActiveLine: true, autocompletion: false }}
        style={{ fontSize: 12 }}
      />
    </div>
  );
}
