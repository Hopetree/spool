import { useEffect, useState } from "react";
import {
  App as AntApp,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import {
  createFlow,
  getFlow,
  updateFlow,
  type FailurePolicy,
  type FlowInput,
  type FlowMapping,
} from "../api/flows";
import { listTools, type ParamType, type ToolSummary } from "../api/tools";

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
  failure_policy: FailurePolicy;
  max_duration_sec?: number;
  params?: ParamForm[];
}

interface StepState {
  name: string;
  tool_id: string;
  mapping: Record<string, FlowMapping>;
  timeout_sec: number;
  retry_count: number;
  retry_delay_sec: number;
  enabled: boolean;
}

interface StageState {
  name: string;
  always_run: boolean;
  steps: StepState[];
}

const paramTypeOptions = [
  { value: "string", label: "字符串" },
  { value: "int", label: "整数" },
  { value: "bool", label: "布尔" },
  { value: "select", label: "枚举" },
];

const emptyStep = (): StepState => ({
  name: "",
  tool_id: "",
  mapping: {},
  timeout_sec: 0,
  retry_count: 0,
  retry_delay_sec: 0,
  enabled: true,
});

const emptyStage = (n: number): StageState => ({
  name: `阶段 ${n}`,
  always_run: false,
  steps: [emptyStep()],
});

function MappingEditor({
  tool,
  step,
  flowParams,
  onChange,
}: {
  tool: ToolSummary;
  step: StepState;
  flowParams: ParamForm[];
  onChange: (mapping: Record<string, FlowMapping>) => void;
}) {
  if (tool.params.length === 0) {
    return <Typography.Text type="secondary">该工具没有参数</Typography.Text>;
  }
  const setMapping = (name: string, m?: FlowMapping) => {
    const next = { ...step.mapping };
    if (m) next[name] = m;
    else delete next[name];
    onChange(next);
  };
  return (
    <div style={{ marginTop: 8 }}>
      {tool.params.map((p) => {
        const m = step.mapping[p.name];
        const source: string = m?.source ?? "default";
        const eligible = flowParams.filter((fp) => !p.sensitive || fp.sensitive);
        return (
          <div
            key={p.name}
            style={{ display: "flex", gap: 12, alignItems: "center", padding: "6px 0", borderTop: "1px solid #f0f0f0" }}
          >
            <div style={{ width: 190 }}>
              {p.name}
              {p.sensitive && <Tag color="red" style={{ marginLeft: 6 }}>敏感</Tag>}
            </div>
            <Select
              style={{ width: 120 }}
              value={source}
              onChange={(next: string) => {
                if (next === "default") {
                  setMapping(p.name);
                  return;
                }
                if (next === "flow") {
                  setMapping(p.name, { source: "flow", value: eligible[0]?.name ?? "" });
                  return;
                }
                setMapping(p.name, { source: "literal", value: "" });
              }}
              options={[
                { value: "default", label: "默认值" },
                { value: "flow", label: "流程参数" },
                { value: "literal", label: "固定值", disabled: p.sensitive },
              ]}
            />
            <div style={{ flex: 1 }}>
              {source === "flow" && (
                <Select
                  style={{ width: "100%" }}
                  value={m?.value || undefined}
                  placeholder="选择流程参数"
                  onChange={(v) => setMapping(p.name, { source: "flow", value: v })}
                  options={eligible.map((fp) => ({
                    value: fp.name,
                    label: `${fp.name}${fp.sensitive ? "（敏感）" : ""}`,
                  }))}
                />
              )}
              {source === "literal" && (
                <Input
                  value={m?.value ?? ""}
                  placeholder={p.sensitive ? "敏感参数请改用敏感流程参数" : "固定值"}
                  onChange={(e) => setMapping(p.name, { source: "literal", value: e.target.value })}
                />
              )}
              {source === "default" && (
                <Typography.Text type="secondary">
                  {p.default ? `默认值：${p.default}` : p.required ? "必填，必须映射" : "无默认值"}
                </Typography.Text>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default function FlowEditor() {
  const { id } = useParams();
  const isNew = !id;
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";

  const [form] = Form.useForm<FormValues>();
  const [stages, setStages] = useState<StageState[]>([]);

  const { data: detail, isLoading } = useQuery({
    queryKey: ["flow", id],
    queryFn: () => getFlow(id!),
    enabled: !!id,
  });
  const { data: toolPage } = useQuery({
    queryKey: ["tools", "flow-editor"],
    queryFn: () => listTools({ page: 1, page_size: 100 }),
  });
  const tools = (toolPage?.items ?? []).filter((t) => t.enabled);
  const watchedParams = Form.useWatch("params", form) as ParamForm[] | undefined;
  const flowParams = watchedParams ?? [];

  useEffect(() => {
    if (detail) {
      form.setFieldsValue({
        name: detail.name,
        description: detail.description,
        failure_policy: detail.failure_policy,
        max_duration_sec: detail.max_duration_sec,
        params: detail.params.map((p) => ({ ...p, optionsText: (p.options ?? []).join(", ") })),
      });
      setStages(
        detail.stages.map((stage) => ({
          name: stage.name,
          always_run: stage.always_run,
          steps: stage.steps.map((step) => ({
            name: step.name,
            tool_id: step.tool_id,
            mapping: step.mapping ?? {},
            timeout_sec: step.timeout_sec,
            retry_count: step.retry_count,
            retry_delay_sec: step.retry_delay_sec,
            enabled: step.enabled,
          })),
        })),
      );
      return;
    }
    if (isNew) {
      setStages((prev) => (prev.length ? prev : [emptyStage(1)]));
    }
  }, [detail, isNew, form]);

  const save = useMutation({
    mutationFn: (values: FormValues) => {
      if (stages.length === 0) throw new Error("请至少添加一个阶段");
      stages.forEach((stage, si) => {
        if (stage.steps.length === 0) throw new Error(`阶段 ${si + 1} 至少需要一个步骤`);
        stage.steps.forEach((step) => {
          if (!step.name.trim()) throw new Error(`阶段 ${si + 1} 有步骤未填写名称`);
          if (!step.tool_id) throw new Error(`步骤 ${step.name} 未选择工具`);
        });
      });
      const input: FlowInput = {
        name: values.name,
        description: values.description ?? "",
        failure_policy: values.failure_policy,
        max_duration_sec: values.max_duration_sec ?? 3600,
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
        stages: stages.map((stage) => ({
          name: stage.name,
          always_run: stage.always_run,
          steps: stage.steps.map((step) => ({
            name: step.name.trim(),
            tool_id: step.tool_id,
            mapping: step.mapping,
            timeout_sec: step.timeout_sec,
            retry_count: step.retry_count,
            retry_delay_sec: step.retry_delay_sec,
            enabled: step.enabled,
          })),
        })),
      };
      return isNew ? createFlow(input) : updateFlow(id!, input);
    },
    onSuccess: (created) => {
      message.success("已保存");
      void qc.invalidateQueries({ queryKey: ["flows"] });
      void qc.invalidateQueries({ queryKey: ["flow", created.id] });
      navigate(`/flows/${created.id}`, { replace: true });
    },
    onError: (err) =>
      message.error(err instanceof ApiError ? err.message : err instanceof Error ? err.message : "保存失败"),
  });

  const updateStage = (si: number, patch: Partial<StageState>) =>
    setStages((prev) => prev.map((s, i) => (i === si ? { ...s, ...patch } : s)));
  const updateStep = (si: number, ti: number, patch: Partial<StepState>) =>
    setStages((prev) =>
      prev.map((s, i) => (i === si ? { ...s, steps: s.steps.map((x, j) => (j === ti ? { ...x, ...patch } : x)) } : s)),
    );
  const addStage = () => setStages((prev) => [...prev, emptyStage(prev.length + 1)]);
  const addStep = (si: number) =>
    setStages((prev) => prev.map((s, i) => (i === si ? { ...s, steps: [...s.steps, emptyStep()] } : s)));
  const moveStage = (si: number, dir: number) =>
    setStages((prev) => {
      const j = si + dir;
      if (j < 0 || j >= prev.length) return prev;
      const next = [...prev];
      [next[si], next[j]] = [next[j], next[si]];
      return next;
    });
  const moveStep = (si: number, ti: number, dir: number) =>
    setStages((prev) =>
      prev.map((s, i) => {
        if (i !== si) return s;
        const j = ti + dir;
        if (j < 0 || j >= s.steps.length) return s;
        const steps = [...s.steps];
        [steps[ti], steps[j]] = [steps[j], steps[ti]];
        return { ...s, steps };
      }),
    );

  if (!isAdmin) {
    return (
      <Card>
        <Typography.Title level={4}>流程定义仅管理员可编辑</Typography.Title>
        <Space>
          <Button onClick={() => navigate(isNew ? "/flows" : `/flows/${id}`)}>返回</Button>
        </Space>
      </Card>
    );
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate(isNew ? "/flows" : `/flows/${id}`)}>返回</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {isNew ? "新建流程" : `编辑流程：${detail?.name ?? ""}`}
        </Typography.Title>
        <Button type="primary" loading={save.isPending} onClick={() => form.submit()}>
          保存流程
        </Button>
      </Space>

      <Form<FormValues>
        form={form}
        layout="vertical"
        onFinish={(values) => save.mutate(values)}
        initialValues={{ failure_policy: "abort", max_duration_sec: 3600, params: [] }}
      >
        <Card title="基本信息" style={{ marginBottom: 16 }} loading={isLoading}>
          <Space wrap align="start">
            <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]} style={{ width: 260 }}>
              <Input />
            </Form.Item>
            <Form.Item name="failure_policy" label="失败策略" style={{ width: 180 }}>
              <Select
                options={[
                  { value: "abort", label: "失败即中止后续阶段" },
                  { value: "continue", label: "失败后继续后续阶段" },
                ]}
              />
            </Form.Item>
            <Form.Item name="max_duration_sec" label="单机最大时长（秒）" style={{ width: 180 }}>
              <InputNumber min={1} max={86400} style={{ width: "100%" }} />
            </Form.Item>
          </Space>
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
        </Card>

        <Card title="流程参数" style={{ marginBottom: 16 }}>
          <Form.List name="params">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...rest }) => (
                  <Space key={key} wrap align="start" style={{ display: "flex", marginBottom: 8 }}>
                    <Form.Item {...rest} name={[name, "name"]} label="变量名" rules={[{ required: true, message: "必填" }]}>
                      <Input style={{ width: 140 }} placeholder="ENV" />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "label"]} label="显示名">
                      <Input style={{ width: 130 }} />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "type"]} label="类型" initialValue="string">
                      <Select style={{ width: 100 }} options={paramTypeOptions} />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "required"]} label="必填" valuePropName="checked">
                      <Switch />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "sensitive"]} label="敏感" valuePropName="checked">
                      <Switch />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "default"]} label="默认值">
                      <Input style={{ width: 150 }} />
                    </Form.Item>
                    <Form.Item {...rest} name={[name, "optionsText"]} label="候选项">
                      <Input style={{ width: 160 }} placeholder="逗号分隔" />
                    </Form.Item>
                    <Button danger style={{ marginTop: 30 }} onClick={() => remove(name)}>
                      删除
                    </Button>
                  </Space>
                ))}
                <Button block onClick={() => add({ type: "string", required: false, sensitive: false })}>
                  添加流程参数
                </Button>
              </>
            )}
          </Form.List>
        </Card>

        <Card
          title="阶段（阶段内并行，阶段间串行）"
          extra={
            <Button type="dashed" onClick={addStage}>
              添加阶段
            </Button>
          }
        >
          {stages.map((stage, si) => (
            <Card
              key={si}
              size="small"
              style={{ marginBottom: 16 }}
              title={
                <Space>
                  <Input
                    value={stage.name}
                    onChange={(e) => updateStage(si, { name: e.target.value })}
                    style={{ width: 180 }}
                  />
                  <Switch
                    checked={stage.always_run}
                    onChange={(checked) => updateStage(si, { always_run: checked })}
                  />
                  <span>始终执行</span>
                </Space>
              }
              extra={
                <Space>
                  <Button size="small" disabled={si === 0} onClick={() => moveStage(si, -1)}>
                    上移
                  </Button>
                  <Button size="small" disabled={si === stages.length - 1} onClick={() => moveStage(si, 1)}>
                    下移
                  </Button>
                  <Button
                    size="small"
                    danger
                    disabled={stages.length === 1}
                    onClick={() => setStages((prev) => prev.filter((_, i) => i !== si))}
                  >
                    删除阶段
                  </Button>
                </Space>
              }
            >
              {stage.steps.map((step, ti) => {
                const tool = tools.find((t) => t.id === step.tool_id);
                return (
                  <Card
                    key={ti}
                    type="inner"
                    size="small"
                    style={{ marginBottom: 12 }}
                    title={`步骤 ${ti + 1}`}
                    extra={
                      <Space>
                        <Button size="small" disabled={ti === 0} onClick={() => moveStep(si, ti, -1)}>
                          上移
                        </Button>
                        <Button size="small" disabled={ti === stage.steps.length - 1} onClick={() => moveStep(si, ti, 1)}>
                          下移
                        </Button>
                        <Button
                          size="small"
                          danger
                          disabled={stage.steps.length === 1}
                          onClick={() =>
                            updateStage(si, { steps: stage.steps.filter((_, j) => j !== ti) })
                          }
                        >
                          删除
                        </Button>
                      </Space>
                    }
                  >
                    <Space wrap align="start">
                      <Space direction="vertical" size={2}>
                        <span>步骤名称</span>
                        <Input
                          value={step.name}
                          placeholder="如：部署"
                          style={{ width: 160 }}
                          onChange={(e) => updateStep(si, ti, { name: e.target.value })}
                        />
                      </Space>
                      <Space direction="vertical" size={2}>
                        <span>工具</span>
                        <Select
                          style={{ width: 210 }}
                          placeholder="选择工具"
                          showSearch
                          optionFilterProp="label"
                          value={step.tool_id || undefined}
                          onChange={(v) => updateStep(si, ti, { tool_id: v, mapping: {} })}
                          options={tools.map((t) => ({ value: t.id, label: `${t.name} (v${t.version_no})` }))}
                        />
                      </Space>
                      <Space direction="vertical" size={2}>
                        <span>超时（秒，0=工具默认）</span>
                        <InputNumber
                          min={0}
                          max={86400}
                          value={step.timeout_sec}
                          onChange={(v) => updateStep(si, ti, { timeout_sec: v ?? 0 })}
                        />
                      </Space>
                      <Space direction="vertical" size={2}>
                        <span>重试</span>
                        <InputNumber
                          min={0}
                          max={3}
                          value={step.retry_count}
                          onChange={(v) => updateStep(si, ti, { retry_count: v ?? 0 })}
                        />
                      </Space>
                      <Space direction="vertical" size={2}>
                        <span>间隔（秒）</span>
                        <InputNumber
                          min={0}
                          max={300}
                          value={step.retry_delay_sec}
                          onChange={(v) => updateStep(si, ti, { retry_delay_sec: v ?? 0 })}
                        />
                      </Space>
                      <Space direction="vertical" size={2}>
                        <span>启用</span>
                        <Switch
                          checked={step.enabled}
                          onChange={(checked) => updateStep(si, ti, { enabled: checked })}
                        />
                      </Space>
                    </Space>
                    {tool && (
                      <MappingEditor
                        tool={tool}
                        step={step}
                        flowParams={flowParams}
                        onChange={(mapping) => updateStep(si, ti, { mapping })}
                      />
                    )}
                  </Card>
                );
              })}
              <Button block type="dashed" onClick={() => addStep(si)}>
                添加步骤
              </Button>
            </Card>
          ))}
        </Card>
      </Form>


    </div>
  );
}
