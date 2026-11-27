import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  App as AntApp,
  Button,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
} from "antd";
import { DownloadOutlined, UploadOutlined } from "@ant-design/icons";
import type { Credential } from "../api/credentials";
import type { HostGroup } from "../api/groups";
import { importHosts, type HostImportResult } from "../api/hosts";

interface ParsedRow {
  key: string;
  line: number;
  name: string;
  address: string;
  port: number;
  tags: string[];
  note: string;
  enabled: boolean;
  error?: string;
}

const templateHeader = "主机名称,地址,端口,标签,备注";
const templateRows = ["web-01,10.0.0.11,22,prod|web,示例主机", "web-02,10.0.0.12,22,prod|web,"];

/** Minimal RFC4180-ish CSV parser: commas, quotes and newlines in cells. */
function parseCSV(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let field = "";
  let quoted = false;
  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i];
    if (quoted) {
      if (ch === '"') {
        if (text[i + 1] === '"') {
          field += '"';
          i += 1;
        } else {
          quoted = false;
        }
      } else {
        field += ch;
      }
      continue;
    }
    if (ch === '"') {
      quoted = true;
    } else if (ch === ",") {
      row.push(field);
      field = "";
    } else if (ch === "\n") {
      row.push(field);
      rows.push(row);
      row = [];
      field = "";
    } else if (ch !== "\r") {
      field += ch;
    }
  }
  if (field !== "" || row.length > 0) {
    row.push(field);
    rows.push(row);
  }
  return rows.filter((r) => r.some((c) => c.trim() !== ""));
}

function splitTags(raw: string): string[] {
  return raw
    .split(/[|;、]/)
    .map((s) => s.trim())
    .filter(Boolean);
}

function parseBool(raw: string): boolean {
  const v = raw.trim().toLowerCase();
  return !["0", "false", "否", "停用", "no"].includes(v);
}

export default function HostImportModal({
  open,
  onClose,
  groups,
  credentials,
  onImported,
}: {
  open: boolean;
  onClose: () => void;
  groups: HostGroup[];
  credentials: Credential[];
  onImported: () => void;
}) {
  const { message } = AntApp.useApp();
  const [groupID, setGroupID] = useState<string>();
  const [credentialID, setCredentialID] = useState<string>();
  const [rows, setRows] = useState<ParsedRow[]>([]);
  const [fileName, setFileName] = useState("");
  const [result, setResult] = useState<HostImportResult>();
  const [importing, setImporting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setRows([]);
    setFileName("");
    setResult(undefined);
    setGroupID(undefined);
    setCredentialID(credentials.length === 1 ? credentials[0].id : undefined);
  }, [open, credentials]);

  const invalidCount = useMemo(() => rows.filter((r) => r.error).length, [rows]);
  const canImport = rows.length > 0 && invalidCount === 0 && !!credentialID && !importing;

  const downloadTemplate = () => {
    const content = `\uFEFF${templateHeader}\n${templateRows.join("\n")}\n`;
    const url = URL.createObjectURL(new Blob([content], { type: "text/csv;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "主机导入模板.csv";
    a.click();
    URL.revokeObjectURL(url);
  };

  const loadFile = async (file: File) => {
    setResult(undefined);
    setFileName(file.name);
    const text = await file.text();
    const table = parseCSV(text);
    if (table.length === 0) {
      setRows([]);
      message.error("文件内容为空");
      return;
    }
    const normalized = table[0].map((c) => c.replace(/^\uFEFF/, "").trim());
    const hasHeader = normalized.some((c) => ["主机名称", "名称", "name", "主机名"].includes(c.toLowerCase()));
    const header = hasHeader ? normalized : ["主机名称", "地址", "端口", "标签", "备注", "启用"];
    const dataRows = hasHeader ? table.slice(1) : table;
    const index = (names: string[]) => header.findIndex((h) => names.includes(h.toLowerCase()));
    const nameIdx = index(["主机名称", "名称", "主机名", "name"]);
    const addrIdx = index(["地址", "ip", "address", "主机地址"]);
    const portIdx = index(["端口", "port"]);
    const tagIdx = index(["标签", "tags"]);
    const noteIdx = index(["备注", "note"]);
    const enabledIdx = index(["启用", "enabled"]);

    const parsed: ParsedRow[] = dataRows.map((cells, i) => {
      const name = (cells[nameIdx >= 0 ? nameIdx : 0] ?? "").trim();
      const address = (cells[addrIdx >= 0 ? addrIdx : 1] ?? "").trim();
      const portRaw = (cells[portIdx] ?? "").trim();
      const port = portRaw === "" ? 22 : Number(portRaw);
      const tags = tagIdx >= 0 ? splitTags(cells[tagIdx] ?? "") : [];
      const note = noteIdx >= 0 ? (cells[noteIdx] ?? "").trim() : "";
      const enabled = enabledIdx >= 0 ? parseBool(cells[enabledIdx] ?? "true") : true;
      let error = "";
      if (!name) error = "主机名称不能为空";
      else if (!address) error = "地址不能为空";
      else if (!Number.isInteger(port) || port < 1 || port > 65535) error = "端口必须是 1-65535";
      return { key: `${i}-${name}`, line: i + 2, name, address, port, tags, note, enabled, error };
    });

    const seen = new Map<string, number>();
    for (const row of parsed) {
      const key = row.name.toLowerCase();
      if (key && seen.has(key)) {
        row.error = row.error || `与第 ${seen.get(key)} 行主机名称重复`;
      } else if (key) {
        seen.set(key, row.line);
      }
    }
    setRows(parsed);
  };

  const submit = async () => {
    if (!credentialID) return;
    setImporting(true);
    try {
      const res = await importHosts({
        group_id: groupID ?? "",
        credential_id: credentialID,
        rows: rows.map((r) => ({
          name: r.name,
          address: r.address,
          port: r.port,
          tags: r.tags,
          note: r.note,
          enabled: r.enabled,
        })),
      });
      setResult(res);
      onImported();
      message.success(`导入完成：成功 ${res.created} 台，失败 ${res.failed} 台`);
      if (res.failed === 0) onClose();
    } catch {
      message.error("导入失败，请稍后重试");
    } finally {
      setImporting(false);
    }
  };

  return (
    <Modal
      open={open}
      title="批量导入主机"
      width={760}
      onCancel={onClose}
      footer={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={importing} disabled={!canImport} onClick={submit}>
            导入主机
          </Button>
        </Space>
      }
      destroyOnClose
    >
      <Space direction="vertical" size={12} style={{ width: "100%" }}>
        <div>
          <Typography.Text strong>模板下载：</Typography.Text>
          <Button type="link" icon={<DownloadOutlined />} style={{ padding: 0 }} onClick={downloadTemplate}>
            主机导入模板.csv
          </Button>
          <div style={{ fontSize: 12, color: "#8c8c8c" }}>
            列：主机名称、地址、端口（默认 22）、标签（用 | 分隔）、备注；密码/密钥使用下方选择的凭据，不写入文件。
          </div>
        </div>

        <Space wrap align="start">
          <div>
            <div style={{ fontSize: 12, color: "#8c8c8c", marginBottom: 4 }}>
              <span style={{ color: "#ff4d4f" }}>* </span>登录凭据
            </div>
            <Select
              style={{ width: 240 }}
              placeholder="选择导入主机使用的凭据"
              value={credentialID}
              onChange={setCredentialID}
              options={credentials.map((c) => ({ value: c.id, label: `${c.name}（${c.username}）` }))}
            />
          </div>
          <div>
            <div style={{ fontSize: 12, color: "#8c8c8c", marginBottom: 4 }}>默认分组</div>
            <Select
              style={{ width: 200 }}
              allowClear
              placeholder="不分组"
              value={groupID}
              onChange={setGroupID}
              options={groups.map((g) => ({ value: g.id, label: g.name }))}
            />
          </div>
          <div>
            <div style={{ fontSize: 12, color: "#8c8c8c", marginBottom: 4 }}>
              <span style={{ color: "#ff4d4f" }}>* </span>导入数据
            </div>
            <Upload
              accept=".csv,text/csv"
              showUploadList={false}
              beforeUpload={(file) => {
                void loadFile(file as File);
                return false;
              }}
            >
              <Button icon={<UploadOutlined />}>{fileName ? "重新上传" : "点击上传"}</Button>
            </Upload>
          </div>
        </Space>

        {fileName && (
          <div style={{ fontSize: 12, color: "#595959" }}>
            已解析 {fileName}：共 {rows.length} 行
            {invalidCount > 0 && <Tag color="red" style={{ marginLeft: 8 }}>{invalidCount} 行有误</Tag>}
          </div>
        )}

        {rows.length > 0 && (
          <Table<ParsedRow>
            rowKey="key"
            size="small"
            dataSource={rows}
            pagination={false}
            scroll={{ y: 260 }}
            columns={[
              { title: "行", dataIndex: "line", width: 48 },
              { title: "主机名称", dataIndex: "name", width: 130, ellipsis: true },
              { title: "地址", dataIndex: "address", width: 130, ellipsis: true },
              { title: "端口", dataIndex: "port", width: 60 },
              {
                title: "标签",
                dataIndex: "tags",
                width: 120,
                ellipsis: true,
                render: (tags: string[]) => tags.join(", ") || "—",
              },
              { title: "备注", dataIndex: "note", ellipsis: true },
              {
                title: "校验",
                width: 150,
                render: (_, r) =>
                  r.error ? (
                    <Typography.Text type="danger" style={{ fontSize: 12 }}>
                      {r.error}
                    </Typography.Text>
                  ) : (
                    <Tag color="green">正常</Tag>
                  ),
              },
            ]}
          />
        )}

        {result && (
          <Alert
            type={result.failed > 0 ? "warning" : "success"}
            showIcon
            message={`导入完成：成功 ${result.created} 台，失败 ${result.failed} 台`}
            description={
              result.errors.length > 0 && (
                <div style={{ maxHeight: 120, overflow: "auto", fontSize: 12 }}>
                  {result.errors.map((e) => (
                    <div key={`${e.row}-${e.message}`}>
                      第 {e.row} 行：{e.message}
                    </div>
                  ))}
                </div>
              )
            }
          />
        )}
      </Space>
    </Modal>
  );
}
