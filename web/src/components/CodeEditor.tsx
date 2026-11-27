import CodeMirror from "@uiw/react-codemirror";
import { python } from "@codemirror/lang-python";
import { StreamLanguage, indentUnit } from "@codemirror/language";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import type { ToolType } from "../api/tools";

interface Props {
  language: ToolType;
  value?: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
}

// oneDark at 12px renders lines at 16.8px; 4px content padding top/bottom.
const lineHeight = 16.8;
const contentPadding = 8;
const minEditorHeight = 320;
const maxVisibleLines = 500;

export default function CodeEditor({ language, value, onChange, readOnly }: Props) {
  const extensions = [language === "python" ? python() : StreamLanguage.define(shell), indentUnit.of("    ")];
  const lineCount = Math.max(1, (value ?? "").split("\n").length);
  const visibleLines = Math.min(lineCount, maxVisibleLines);
  const height = `${Math.max(minEditorHeight, Math.ceil(visibleLines * lineHeight + contentPadding))}px`;
  return (
    <div style={{ border: "1px solid #1f2329", borderRadius: 6, overflow: "hidden" }}>
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
