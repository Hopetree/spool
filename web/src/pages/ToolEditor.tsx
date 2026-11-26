import { useEffect, useMemo, useState } from "react";
import {
  App as AntApp,
  Button,
  Card,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import dayjs from "dayjs";
import { diffLines } from "diff";
import { ApiError } from "../api/client";
import {
  createTool,
  getTool,
  listVersions,
  rollbackTool,
  updateTool,
  type ParamType,
  type ToolInput,
  type ToolType,
  type ToolVersion,
} from "../api/tools";
import ExecuteToolModal from "../components/ExecuteToolModal";
import { useAuthStore } from "../stores/auth";

interface ParamForm {
  name: string;
  label?: string;
  type: ParamType;
  required?: boolean;
  default?: string;
  optionsText?: string;
  description?: string;
  sensitive?: boolean;
}

interface FormValues {
  name: string;
  description?: string;
  type: ToolType;
  interpreter?: string;
  content: string;
  prepare_command?: string;
  timeout_sec?: number;
  sudo?: boolean;
  change_note?: string;
  params?: ParamForm[];
}

const paramTypeOptions: { value: ParamType; label: string }[] = [
  { value: "string", label: "字符串" },
  { value: "int", label: "整数" },
  { value: "bool", label: "布尔" },
  { value: "select", label: "枚举" },
];

export default function ToolEditor() {
  const { id } = useParams();
  const isNew = !id;
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const canRun = isAdmin || user?.role === "operator";

  const [form] = Form.useForm<FormValues>();
  const [execOpen, setExecOpen] = useState(false);
  const [versionsOpen, setVersionsOpen] = useState(false);
  const [diffOpen, setDiffOpen] = useState(false);
  const [fromNo, setFromNo] = useState<number>();
  const [toNo, setToNo] = useState<number>();

  const { data: detail, isLoading } = useQuery({
    queryKey: ["tool", id],
    queryFn: () => getTool(id!),
    enabled: !!id,
  });
  const { data: versions = [] } = useQuery({
    queryKey: ["tool-versions", id],
    queryFn: () => listVersions(id!),
    enabled: !!id && versionsOpen,
  });

  useEffect(() => {
    if (!detail) return;
    form.setFieldsValue({
      name: detail.name,
      description: detail.description,
      type: detail.version.type,
      interpreter: detail.version.interpreter,
      content: detail.version.content,
      prepare_command: detail.version.prepare_command,
      timeout_sec: detail.version.timeout_sec,
      sudo: detail.version.sudo,
      change_note: "",
      params: detail.version.params.map((p) => ({ ...p, optionsText: p.options.join(", ") })),
    });
  }, [detail, form]);

  const save = useMutation({
    mutationFn: (values: FormValues) => {
      const input: ToolInput = {
        name: values.name,
        description: values.description ?? "",
        type: values.type,
        interpreter: values.interpreter ?? "",
        content: values.content,
        prepare_command: values.prepare_command ?? "",
        timeout_sec: values.timeout_sec ?? 300,
        sudo: !!values.sudo,
        change_note: values.change_note ?? "",
        params: (values.params ?? []).map((p) => ({
          name: p.name,
          label: p.label ?? "",
          type: p.type,
          required: !!p.required,
          default: p.default ?? "",
          options: (p.optionsText ?? "")
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean),
          description: p.description ?? "",
          sensitive: !!p.sensitive,
        })),
      };
      return isNew ? createTool(input) : updateTool(id!, input);
    },
    onSuccess: (created) => {
      message.success("已保存");
      void qc.invalidateQueries({ queryKey: ["tools"] });
      void qc.invalidateQueries({ queryKey: ["tool", created.id] });
      void qc.invalidateQueries({ queryKey: ["tool-versions", created.id] });
      if (isNew) navigate(`/tools/${created.id}`, { replace: true });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const rollback = useMutation({
    mutationFn: (no: number) => rollbackTool(id!, no),
    onSuccess: () => {
      message.success("已回滚并生成新版本");
      void qc.invalidateQueries({ queryKey: ["tool", id] });
      void qc.invalidateQueries({ queryKey: ["tool-versions", id] });
      void qc.invalidateQueries({ queryKey: ["tools"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "回滚失败"),
  });

  const fromV = versions.find((v) => v.version_no === fromNo);
  const toV = versions.find((v) => v.version_no === toNo);
  const diffParts = useMemo(() => (fromV && toV ? diffLines(fromV.content, toV.content) : []), [fromV, toV]);

  const openDiff = () => {
    if (versions.length >= 2) {
      setFromNo(versions[1].version_no);
      setToNo(versions[0].version_no);
    } else if (versions.length === 1) {
      setFromNo(versions[0].version_no);
      setToNo(versions[0].version_no);
    }
    setDiffOpen(true);
  };

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate("/tools")}>返回列表</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {isNew ? "新建工具" : detail?.name ?? "工具"}
        </Typography.Title>
        {detail && <Tag>v{detail.version_no}</Tag>}
        {detail && !detail.enabled && <Tag color="orange">已停用</Tag>}
        {!isNew && canRun && <Button onClick={() => setExecOpen(true)}>执行</Button>}
        {!isNew && (
          <Button onClick={() => setVersionsOpen(true)}>
            版本历史
          </Button>
        )}
        {isAdmin && (
          <Button type="primary" loading={save.isPending} onClick={() => form.submit()}>
            保存为新版本
          </Button>
        )}
      </Space>

      <Form<FormValues>
        form={form}
        layout="vertical"
        disabled={!isAdmin}
        onFinish={(values) => save.mutate(values)}
        initialValues={{ type: "shell", interpreter: "/bin/bash", timeout_sec: 300, sudo: false, params: [] }}
      >
        <Card title="基本信息" style={{ marginBottom: 16 }} loading={isLoading}>
          <Space wrap align="start">
            <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]} style={{ width: 240 }}>
              <Input />
            </Form.Item>
            <Form.Item name="type" label="类型" style={{ width: 130 }}>
              <Select
                options={[
                  { value: "shell", label: "Shell" },
                  { value: "python", label: "Python" },
                ]}
              />
            </Form.Item>
            <Form.Item name="interpreter" label="解释器" style={{ width: 200 }}>
              <Input placeholder="/bin/bash 或 python3" />
            </Form.Item>
            <Form.Item name="timeout_sec" label="超时（秒）" style={{ width: 140 }}>
              <InputNumber min={1} max={86400} style={{ width: "100%" }} />
            </Form.Item>
            <Form.Item name="sudo" label="sudo -n 执行" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
        </Card>

        <Card title="脚本" style={{ marginBottom: 16 }}>
          <Form.Item name="content" label="脚本内容" rules={[{ required: true, message: "请输入脚本内容" }]}>
            <Input.TextArea
              autoSize={{ minRows: 10, maxRows: 24 }}
              style={{ fontFamily: "Menlo, Consolas, monospace", fontSize: 12 }}
              placeholder={"# 参数以环境变量注入\n# echo $ENV"}
            />
          </Form.Item>
          <Form.Item name="prepare_command" label="准备命令（可选）" extra="在脚本前执行，失败则中止；与脚本同一日志。">
            <Input.TextArea
              autoSize={{ minRows: 2, maxRows: 6 }}
              style={{ fontFamily: "Menlo, Consolas, monospace", fontSize: 12 }}
              placeholder="python3 -m venv .venv && . .venv/bin/activate && pip install -r ..."
            />
          </Form.Item>
        </Card>

        <Card title="参数" style={{ marginBottom: 16 }}>
          <Form.List name="params">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...rest }) => (
                  <Card
                    key={key}
                    size="small"
                    title={`参数 ${name + 1}`}
                    style={{ marginBottom: 12 }}
                    extra={
                      isAdmin && (
                        <Button size="small" danger onClick={() => remove(name)}>
                          删除
                        </Button>
                      )
                    }
                  >
                    <Space wrap align="start">
                      <Form.Item {...rest} name={[name, "name"]} label="变量名" rules={[{ required: true, message: "必填" }]}>
                        <Input style={{ width: 150 }} placeholder="ENV" />
                      </Form.Item>
                      <Form.Item {...rest} name={[name, "label"]} label="显示名">
                        <Input style={{ width: 140 }} />
                      </Form.Item>
                      <Form.Item {...rest} name={[name, "type"]} label="类型" initialValue="string">
                        <Select style={{ width: 110 }} options={paramTypeOptions} />
                      </Form.Item>
                      <Form.Item {...rest} name={[name, "required"]} label="必填" valuePropName="checked">
                        <Switch />
                      </Form.Item>
                      <Form.Item {...rest} name={[name, "sensitive"]} label="敏感" valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    </Space>
                    <Space wrap align="start">
                      <Form.Item {...rest} name={[name, "default"]} label="默认值">
                        <Input style={{ width: 180 }} />
                      </Form.Item>
                      <Form.Item {...rest} name={[name, "optionsText"]} label="候选项（逗号分隔）">
                        <Input style={{ width: 240 }} placeholder="prod, staging" />
                      </Form.Item>
                      <Form.Item {...rest} name={[name, "description"]} label="说明">
                        <Input style={{ width: 260 }} />
                      </Form.Item>
                    </Space>
                  </Card>
                ))}
                {isAdmin && (
                  <Button block onClick={() => add({ type: "string", required: false, sensitive: false })}>
                    添加参数
                  </Button>
                )}
              </>
            )}
          </Form.List>
        </Card>

        {!isNew && (
          <Card title="版本说明">
            <Form.Item name="change_note" label="本次变更说明">
              <Input placeholder="例如：修复日志路径" maxLength={200} />
            </Form.Item>
          </Card>
        )}
      </Form>

      <Drawer
        open={versionsOpen}
        onClose={() => setVersionsOpen(false)}
        title="版本历史"
        width={720}
        extra={<Button onClick={openDiff}>对比版本</Button>}
      >
        <Table<ToolVersion>
          rowKey="id"
          dataSource={versions}
          pagination={false}
          columns={[
            { title: "版本", dataIndex: "version_no", render: (v: number) => `v${v}` },
            { title: "说明", dataIndex: "change_note" },
            { title: "创建时间", dataIndex: "created_at", render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm") },
            {
              title: "操作",
              render: (_, r) => (
                <Space>
                  <Button
                    size="small"
                    onClick={() => {
                      setFromNo(r.version_no);
                      setToNo(r.version_no === 1 ? 1 : r.version_no - 1);
                      setDiffOpen(true);
                    }}
                  >
                    对比
                  </Button>
                  <Popconfirm title={`回滚到 v${r.version_no}？`} onConfirm={() => rollback.mutate(r.version_no)}>
                    <Button size="small" disabled={r.version_no === detail?.version_no}>
                      回滚
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Drawer>

      <Modal open={diffOpen} onCancel={() => setDiffOpen(false)} footer={null} width={900} title="版本对比">
        <Space style={{ marginBottom: 12 }}>
          <Select
            style={{ width: 160 }}
            placeholder="基准版本"
            value={fromNo}
            onChange={setFromNo}
            options={versions.map((v) => ({ value: v.version_no, label: `v${v.version_no}` }))}
          />
          <span>→</span>
          <Select
            style={{ width: 160 }}
            placeholder="目标版本"
            value={toNo}
            onChange={setToNo}
            options={versions.map((v) => ({ value: v.version_no, label: `v${v.version_no}` }))}
          />
        </Space>
        <div
          style={{
            maxHeight: 520,
            overflow: "auto",
            fontFamily: "Menlo, Consolas, monospace",
            fontSize: 12,
            whiteSpace: "pre-wrap",
          }}
        >
          {diffParts.length === 0 ? (
            <Typography.Text type="secondary">请选择两个版本</Typography.Text>
          ) : (
            diffParts.map((part, idx) => (
              <div
                key={idx}
                style={{
                  background: part.added ? "#e6ffec" : part.removed ? "#ffebe9" : undefined,
                  color: part.added ? "#116329" : part.removed ? "#a40e26" : undefined,
                  padding: "2px 6px",
                }}
              >
                {part.value
                  .split("\n")
                  .filter((line, i, arr) => !(i === arr.length - 1 && line === ""))
                  .map((line) => `${part.added ? "+" : part.removed ? "-" : " "}${line}`)
                  .join("\n")}
              </div>
            ))
          )}
        </div>
      </Modal>

      <ExecuteToolModal toolID={id ?? null} open={execOpen} onClose={() => setExecOpen(false)} />
    </div>
  );
}
