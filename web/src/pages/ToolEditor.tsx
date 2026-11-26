import { useEffect } from "react";
import {
  App as AntApp,
  AutoComplete,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import {
  createTool,
  getTool,
  listToolCategories,
  updateTool,
  type ParamType,
  type ToolInput,
  type ToolType,
} from "../api/tools";
import CodeEditor from "../components/CodeEditor";
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
  category?: string;
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

  const [form] = Form.useForm<FormValues>();
  const scriptType = (Form.useWatch("type", form) ?? "shell") as ToolType;

  const { data: detail } = useQuery({
    queryKey: ["tool", id],
    queryFn: () => getTool(id!),
    enabled: !!id,
  });
  const { data: categories = [] } = useQuery({
    queryKey: ["tool-categories"],
    queryFn: listToolCategories,
    enabled: isAdmin,
  });

  useEffect(() => {
    if (!detail) return;
    form.setFieldsValue({
      name: detail.name,
      category: detail.category,
      description: detail.description,
      type: detail.version.type,
      interpreter: detail.version.interpreter,
      content: detail.version.content,
      prepare_command: detail.version.prepare_command,
      timeout_sec: detail.version.timeout_sec,
      sudo: detail.version.sudo,
      change_note: "",
      params: detail.version.params.map((p) => ({ ...p, optionsText: (p.options ?? []).join(", ") })),
    });
  }, [detail, form]);

  const save = useMutation({
    mutationFn: (values: FormValues) => {
      const input: ToolInput = {
        name: values.name,
        category: values.category ?? "",
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
      void qc.invalidateQueries({ queryKey: ["tool-categories"] });
      void qc.invalidateQueries({ queryKey: ["tool", created.id] });
      void qc.invalidateQueries({ queryKey: ["tool-versions", created.id] });
      navigate(`/tools/${created.id}`, { replace: true });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const categoryOptions = categories.filter((c) => c.path).map((c) => ({ value: c.path }));

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate(isNew ? "/tools" : `/tools/${id}`)}>返回</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {isNew ? "新建工具" : `编辑工具：${detail?.name ?? ""}`}
        </Typography.Title>
        <Button type="primary" loading={save.isPending} onClick={() => form.submit()}>
          保存为新版本
        </Button>
      </Space>

      <Form<FormValues>
        form={form}
        layout="vertical"
        disabled={!isAdmin}
        onFinish={(values) => save.mutate(values)}
        initialValues={{
          type: "shell",
          interpreter: "/bin/bash",
          timeout_sec: 300,
          sudo: false,
          params: [],
          category: "",
        }}
      >
        <Card title="基本信息" style={{ marginBottom: 16 }}>
          <Space wrap align="start">
            <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]} style={{ width: 220 }}>
              <Input />
            </Form.Item>
            <Form.Item
              name="category"
              label="分类"
              extra="格式：运维/部署/应用；留空归入“默认”"
              style={{ width: 260 }}
            >
              <AutoComplete
                options={categoryOptions}
                placeholder="如：运维/部署/应用"
                filterOption={(input, option) =>
                  String(option?.value ?? "").toLowerCase().includes(input.toLowerCase())
                }
              />
            </Form.Item>
            <Form.Item name="type" label="类型" style={{ width: 120 }}>
              <Select
                options={[
                  { value: "shell", label: "Shell" },
                  { value: "python", label: "Python" },
                ]}
              />
            </Form.Item>
            <Form.Item name="interpreter" label="解释器" style={{ width: 190 }}>
              <Input placeholder="/bin/bash 或 python3" />
            </Form.Item>
            <Form.Item name="timeout_sec" label="超时（秒）" style={{ width: 130 }}>
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
            <CodeEditor language={scriptType} readOnly={!isAdmin} height="360px" />
          </Form.Item>
          <Form.Item name="prepare_command" label="准备命令（可选）" extra="在脚本前执行，失败则中止；与脚本同一日志。">
            <Input.TextArea
              autoSize={{ minRows: 2, maxRows: 6 }}
              style={{ fontFamily: "Menlo, Consolas, monospace", fontSize: 12 }}
              placeholder="python3 -m venv .venv && . .venv/bin/activate && pip install requests"
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
    </div>
  );
}
