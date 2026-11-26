import CodeMirror from "@uiw/react-codemirror";
import { python } from "@codemirror/lang-python";
import { StreamLanguage } from "@codemirror/language";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import type { ToolType } from "../api/tools";

interface Props {
  language: ToolType;
  value?: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  height?: string;
}

export default function CodeEditor({ language, value, onChange, readOnly, height = "320px" }: Props) {
  const extensions = language === "python" ? [python()] : [StreamLanguage.define(shell)];
  return (
    <div style={{ border: "1px solid #d9d9d9", borderRadius: 6, overflow: "hidden" }}>
      <CodeMirror
        value={value ?? ""}
        height={height}
        extensions={extensions}
        onChange={(v) => onChange?.(v)}
        editable={!readOnly}
        basicSetup={{ lineNumbers: true, foldGutter: false, highlightActiveLine: true, autocompletion: false }}
        style={{ fontSize: 12 }}
      />
    </div>
  );
}
