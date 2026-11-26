import { useEffect, useMemo, useState } from "react";
import {
  App as AntApp,
  AutoComplete,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import dayjs from "dayjs";
import { diffLines } from "diff";
import { ApiError } from "../api/client";
import {
  copyTool,
  createTool,
  deleteTool,
  deleteToolVersion,
  getTool,
  getToolReferences,
  listToolCategories,
  listVersions,
  rollbackTool,
  setToolEnabled,
  updateTool,
  type ParamType,
  type ToolInput,
  type ToolReferenceFlow,
  type ToolReferenceSchedule,
  type ToolType,
  type ToolVersion,
} from "../api/tools";
import ExecuteToolModal from "../components/ExecuteToolModal";
import ExecutionTable from "../components/ExecutionTable";
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
  const canRun = isAdmin || user?.role === "operator";

  const [form] = Form.useForm<FormValues>();
  const [tab, setTab] = useState("definition");
  const [execOpen, setExecOpen] = useState(false);
  const [diffOpen, setDiffOpen] = useState(false);
  const [fromNo, setFromNo] = useState<number>();
  const [toNo, setToNo] = useState<number>();

  const { data: detail, isLoading } = useQuery({
    queryKey: ["tool", id],
    queryFn: () => getTool(id!),
    enabled: !!id,
  });
  const { data: categories = [] } = useQuery({
    queryKey: ["tool-categories"],
    queryFn: listToolCategories,
    enabled: isAdmin,
  });
  const { data: versions = [] } = useQuery({
    queryKey: ["tool-versions", id],
    queryFn: () => listVersions(id!),
    enabled: !!id,
  });
  const { data: refs, isLoading: refsLoading } = useQuery({
    queryKey: ["tool-references", id],
    queryFn: () => getToolReferences(id!),
    enabled: !!id && tab === "references",
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
      params: detail.version.params.map((p) => ({ ...p, optionsText: p.options.join(", ") })),
    });
  }, [detail, form]);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["tools"] });
    void qc.invalidateQueries({ queryKey: ["tool-categories"] });
  };

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
      invalidate();
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
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "回滚失败"),
  });

  const removeVersion = useMutation({
    mutationFn: (no: number) => deleteToolVersion(id!, no),
    onSuccess: () => {
      message.success("版本已删除");
      void qc.invalidateQueries({ queryKey: ["tool-versions", id] });
      void qc.invalidateQueries({ queryKey: ["tool", id] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除版本失败"),
  });

  const clone = useMutation({
    mutationFn: () => copyTool(id!),
    onSuccess: (created) => {
      message.success("已克隆");
      invalidate();
      navigate(`/tools/${created.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "克隆失败"),
  });

  const removeTool = useMutation({
    mutationFn: () => deleteTool(id!),
    onSuccess: () => {
      message.success("工具已删除");
      invalidate();
      navigate("/tools");
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  const toggleEnabled = useMutation({
    mutationFn: (value: boolean) => setToolEnabled(id!, value),
    onSuccess: () => {
      message.success("已更新");
      void qc.invalidateQueries({ queryKey: ["tool", id] });
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "更新失败"),
  });

  const fromV = versions.find((v) => v.version_no === fromNo);
  const toV = versions.find((v) => v.version_no === toNo);
  const diffParts = useMemo(() => (fromV && toV ? diffLines(fromV.content, toV.content) : []), [fromV, toV]);

  const openDiff = (a?: number, b?: number) => {
    if (a && b) {
      setFromNo(a);
      setToNo(b);
    } else if (versions.length >= 2) {
      setFromNo(versions[1].version_no);
      setToNo(versions[0].version_no);
    } else if (versions.length === 1) {
      setFromNo(versions[0].version_no);
      setToNo(versions[0].version_no);
    }
    setDiffOpen(true);
  };

  const categoryOptions = categories.filter((c) => c.path).map((c) => ({ value: c.path }));

  const definitionTab = (
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
      <Card title="基本信息" style={{ marginBottom: 16 }} loading={isLoading}>
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
  );

  const historyTab = id ? (
    <ExecutionTable kind="tool" refID={id} />
  ) : (
    <Card>保存后可查看执行历史</Card>
  );

  const versionsTab = (
    <Table<ToolVersion>
      rowKey="id"
      dataSource={versions}
      pagination={false}
      columns={[
        {
          title: "版本",
          dataIndex: "version_no",
          width: 100,
          render: (v: number) => (
            <Space size={4}>
              v{v}
              {v === detail?.version_no && <Tag color="green">当前</Tag>}
            </Space>
          ),
        },
        { title: "说明", dataIndex: "change_note" },
        { title: "创建人", dataIndex: "created_by", width: 140 },
        {
          title: "创建时间",
          dataIndex: "created_at",
          width: 170,
          render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm"),
        },
        {
          title: "操作",
          width: 240,
          render: (_, r) => (
            <Space>
              <Button size="small" onClick={() => openDiff(r.version_no === 1 ? 1 : r.version_no - 1, r.version_no)}>
                对比
              </Button>
              {isAdmin && (
                <Popconfirm
                  title={`回滚到 v${r.version_no}？`}
                  onConfirm={() => rollback.mutate(r.version_no)}
                  disabled={r.version_no === detail?.version_no}
                >
                  <Button size="small" disabled={r.version_no === detail?.version_no}>
                    回滚
                  </Button>
                </Popconfirm>
              )}
              {isAdmin && (
                <Popconfirm
                  title={`删除版本 v${r.version_no}？`}
                  description="当前版本或有执行历史的版本不能删除。"
                  onConfirm={() => removeVersion.mutate(r.version_no)}
                  disabled={r.version_no === detail?.version_no}
                >
                  <Button size="small" danger disabled={r.version_no === detail?.version_no}>
                    删除
                  </Button>
                </Popconfirm>
              )}
            </Space>
          ),
        },
      ]}
    />
  );

  const referencesTab = (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Card title="流程引用" loading={refsLoading}>
        <Table<ToolReferenceFlow>
          rowKey={(r) => `${r.flow_id}-${r.stage_name}-${r.step_name}`}
          dataSource={refs?.flows ?? []}
          pagination={false}
          size="small"
          columns={[
            {
              title: "流程",
              dataIndex: "flow_name",
              render: (v: string, r: ToolReferenceFlow) => (
                <Space>
                  <a onClick={() => navigate(`/flows/${r.flow_id}`)}>{v}</a>
                  {r.flow_enabled ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>}
                </Space>
              ),
            },
            { title: "阶段", dataIndex: "stage_name" },
            { title: "步骤", dataIndex: "step_name" },
          ]}
        />
      </Card>
      <Card title="定时任务引用" loading={refsLoading}>
        <Table<ToolReferenceSchedule>
          rowKey="id"
          dataSource={refs?.schedules ?? []}
          pagination={false}
          size="small"
          columns={[
            {
              title: "定时任务",
              dataIndex: "name",
              render: (v: string, r: ToolReferenceSchedule) => (
                <Space>
                  <a onClick={() => navigate(`/schedules/${r.id}`)}>{v}</a>
                  {r.enabled ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>}
                </Space>
              ),
            },
            { title: "cron", dataIndex: "cron_expr" },
          ]}
        />
      </Card>
    </Space>
  );

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate("/tools")}>返回列表</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {isNew ? "新建工具" : detail?.name ?? "工具"}
        </Typography.Title>
        {!isNew && <Tag>{detail?.category || "默认"}</Tag>}
        {detail && <Tag>v{detail.version_no}</Tag>}
        {detail && !detail.enabled && <Tag color="orange">已停用</Tag>}
        {!isNew && canRun && <Button onClick={() => setExecOpen(true)}>执行</Button>}
        {!isNew && isAdmin && (
          <Button loading={clone.isPending} onClick={() => clone.mutate()}>
            克隆
          </Button>
        )}
        {!isNew && isAdmin && (
          <Space size={4}>
            <span>启用</span>
            <Switch
              checked={detail?.enabled}
              loading={toggleEnabled.isPending}
              onChange={(v) => toggleEnabled.mutate(v)}
            />
          </Space>
        )}
        {!isNew && isAdmin && (
          <Popconfirm title="确认删除该工具？" onConfirm={() => removeTool.mutate()}>
            <Button danger loading={removeTool.isPending}>
              删除工具
            </Button>
          </Popconfirm>
        )}
        {isAdmin && tab === "definition" && (
          <Button type="primary" loading={save.isPending} onClick={() => form.submit()}>
            保存为新版本
          </Button>
        )}
      </Space>

      <Tabs
        activeKey={tab}
        onChange={setTab}
        items={[
          { key: "definition", label: "定义", children: definitionTab },
          { key: "history", label: "执行历史", children: historyTab },
          { key: "versions", label: "版本列表", children: !isNew ? versionsTab : <Card>保存后可查看版本</Card> },
          { key: "references", label: "引用对象", children: !isNew ? referencesTab : <Card>保存后可查看引用</Card> },
        ]}
      />

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
