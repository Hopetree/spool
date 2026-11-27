import { useEffect, useState, type ReactNode } from "react";
import {
  App as AntApp,
  Button,
  Card,
  Form,
  Input,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import { getFlow, listFlows } from "../api/flows";
import { listHosts } from "../api/hosts";
import { listChannels } from "../api/notify";
import {
  createSchedule,
  getSchedule,
  previewCron,
  updateSchedule,
  type ConcurrencyPolicy,
  type NotifyEvents,
  type ScheduleInput,
  type TargetKind,
} from "../api/schedules";
import { getTool, listTools, type ToolParam } from "../api/tools";
import PageHeader from "../components/PageHeader";
import { describeCron } from "../utils/cron";
import { useAuthStore } from "../stores/auth";

interface FormValues {
  name: string;
  target_kind: TargetKind;
  target_ref_id: string;
  params?: Record<string, string>;
  host_ids: string[];
  cron_expr: string;
  concurrency_policy: ConcurrencyPolicy;
  notify_channel_ids?: string[];
  notify_events: NotifyEvents;
  enabled: boolean;
}

const presets = [
  { value: "*/5 * * * *", label: "每 5 分钟" },
  { value: "0 * * * *", label: "每小时" },
  { value: "0 2 * * *", label: "每天 02:00" },
  { value: "0 9 * * 1", label: "每周一 09:00" },
];

function parseHostIDs(value: string | undefined): string[] {
  try {
    const parsed = JSON.parse(value || "[]");
    return Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === "string") : [];
  } catch {
    return [];
  }
}

function HostParamSelect({
  value,
  onChange,
  hosts,
}: {
  value?: string;
  onChange?: (value: string) => void;
  hosts: { id: string; name: string; address: string }[];
}) {
  return (
    <Select
      mode="multiple"
      allowClear
      optionFilterProp="label"
      placeholder="选择执行主机"
      value={parseHostIDs(value)}
      onChange={(ids) => onChange?.(JSON.stringify(ids))}
      options={hosts.map((h) => ({ value: h.id, label: `${h.name} (${h.address})` }))}
    />
  );
}

function paramInput(p: ToolParam, editing: boolean, hosts: { id: string; name: string; address: string }[]): ReactNode {
  if (p.type === "host") {
    return <HostParamSelect hosts={hosts} />;
  }
  if (p.type === "bool") {
    return (
      <Select
        allowClear
        options={[
          { value: "true", label: "true" },
          { value: "false", label: "false" },
        ]}
      />
    );
  }
  if (p.type === "select") {
    return <Select allowClear options={(p.options ?? []).map((o) => ({ value: o, label: o }))} />;
  }
  if (p.sensitive) {
    return <Input.Password placeholder={editing ? "留空保持不变" : undefined} />;
  }
  if (p.type === "int") {
    return <Input inputMode="numeric" placeholder="整数" />;
  }
  return <Input />;
}

export default function ScheduleEditor() {
  const { id } = useParams();
  const isNew = !id;
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";

  const [form] = Form.useForm<FormValues>();
  const [times, setTimes] = useState<number[]>([]);

  const { data: detail } = useQuery({
    queryKey: ["schedule", id],
    queryFn: () => getSchedule(id!),
    enabled: !!id,
  });
  const { data: toolPage } = useQuery({ queryKey: ["tools", "schedule-editor"], queryFn: () => listTools({ page: 1, page_size: 100 }) });
  const { data: flowPage } = useQuery({ queryKey: ["flows", "schedule-editor"], queryFn: () => listFlows() });
  const { data: hostPage } = useQuery({ queryKey: ["hosts", "schedule-editor"], queryFn: () => listHosts({ page: 1, page_size: 100 }) });
  const { data: channels = [] } = useQuery({ queryKey: ["notify-channels"], queryFn: listChannels, enabled: isAdmin });

  const kind = Form.useWatch("target_kind", form);
  const targetRef = Form.useWatch("target_ref_id", form);
  const cronExpr = Form.useWatch("cron_expr", form);

  const { data: toolDetail } = useQuery({
    queryKey: ["tool", targetRef],
    queryFn: () => getTool(targetRef!),
    enabled: kind === "tool" && !!targetRef,
  });
  const { data: flowDetail } = useQuery({
    queryKey: ["flow", targetRef],
    queryFn: () => getFlow(targetRef!),
    enabled: kind === "flow" && !!targetRef,
  });
  const params: ToolParam[] = kind === "tool" ? toolDetail?.version.params ?? [] : flowDetail?.params ?? [];
  const hostsRequired = kind !== "flow";

  const tools = (toolPage?.items ?? []).filter((t) => t.enabled);
  const flows = (flowPage?.items ?? []).filter((f) => f.enabled);
  const hosts = hostPage?.items ?? [];

  useEffect(() => {
    if (!detail) return;
    form.setFieldsValue({
      name: detail.name,
      target_kind: detail.target_kind,
      target_ref_id: detail.target_ref_id,
      params: { ...detail.params },
      host_ids: detail.host_ids,
      cron_expr: detail.cron_expr,
      concurrency_policy: detail.concurrency_policy,
      notify_channel_ids: detail.notify_channel_ids,
      notify_events: detail.notify_events,
      enabled: detail.enabled,
    });
  }, [detail, form]);

  useEffect(() => {
    if (params.length === 0) return;
    const current = form.getFieldValue("params") ?? {};
    const next: Record<string, string> = { ...current };
    let changed = false;
    params.forEach((p) => {
      if (!p.sensitive && next[p.name] === undefined && p.default) {
        next[p.name] = p.default;
        changed = true;
      }
    });
    if (changed) form.setFieldsValue({ params: next });
  }, [params, form]);

  const preview = useMutation({
    mutationFn: (expr: string) => previewCron(expr, 5),
    onSuccess: (items) => setTimes(items),
    onError: (err) => message.error(err instanceof ApiError ? err.message : "cron 表达式不合法"),
  });

  const save = useMutation({
    mutationFn: (values: FormValues) => {
      const input: ScheduleInput = {
        name: values.name,
        target_kind: values.target_kind,
        target_ref_id: values.target_ref_id,
        params: values.params ?? {},
        host_ids: values.host_ids ?? [],
        cron_expr: values.cron_expr,
        concurrency_policy: values.concurrency_policy,
        notify_channel_ids: values.notify_channel_ids ?? [],
        notify_events: values.notify_events,
        enabled: values.enabled,
      };
      return isNew ? createSchedule(input) : updateSchedule(id!, input);
    },
    onSuccess: (created) => {
      message.success("已保存");
      void qc.invalidateQueries({ queryKey: ["schedules"] });
      navigate(`/schedules/${created.id}`, { replace: true });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  if (!isAdmin) {
    return (
      <Card>
        <Typography.Title level={4}>定时任务仅管理员可编辑</Typography.Title>
        <Button onClick={() => navigate(isNew ? "/schedules" : `/schedules/${id}`)}>返回</Button>
      </Card>
    );
  }

  return (
    <div>
      <PageHeader
        back={{ label: "返回", onClick: () => navigate(isNew ? "/schedules" : `/schedules/${id}`) }}
        title={isNew ? "新建定时任务" : detail?.name ?? "定时任务"}
        crumbs={[{ title: "自动化", to: "/schedules" }, { title: isNew ? "新建定时任务" : "编辑定时任务" }]}
        extra={
          <Button type="primary" loading={save.isPending} onClick={() => form.submit()}>
            保存
          </Button>
        }
      />

      <Form<FormValues>
        form={form}
        layout="vertical"
        onFinish={(values) => save.mutate(values)}
        initialValues={{
          target_kind: "tool",
          concurrency_policy: "skip",
          notify_events: "failure",
          enabled: true,
          cron_expr: "0 2 * * *",
        }}
      >
        <Card title="基本信息" style={{ marginBottom: 16 }}>
          <Space wrap align="start">
            <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]} style={{ width: 240 }}>
              <Input />
            </Form.Item>
            <Form.Item name="target_kind" label="执行对象类型" style={{ width: 140 }}>
              <Select
                onChange={() => form.setFieldsValue({ target_ref_id: "", params: {} })}
                options={[
                  { value: "tool", label: "工具" },
                  { value: "flow", label: "流程" },
                ]}
              />
            </Form.Item>
            <Form.Item name="target_ref_id" label="执行对象" rules={[{ required: true, message: "请选择执行对象" }]} style={{ width: 260 }}>
              <Select
                showSearch
                optionFilterProp="label"
                placeholder="选择工具或流程"
                onChange={() => form.setFieldsValue({ params: {} })}
                options={
                  kind === "flow"
                    ? flows.map((f) => ({ value: f.id, label: f.name }))
                    : tools.map((t) => ({ value: t.id, label: `${t.name} (v${t.version_no})` }))
                }
              />
            </Form.Item>
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
        </Card>

        <Card title="调度" style={{ marginBottom: 16 }}>
          <Space wrap align="start">
            <Form.Item
              name="cron_expr"
              label="cron 表达式（5 段）"
              rules={[{ required: true, message: "请输入 cron" }]}
              extra={cronExpr ? describeCron(cronExpr) : undefined}
              style={{ width: 260 }}
            >
              <Input placeholder="0 2 * * *" />
            </Form.Item>
            <Form.Item label="常用周期" style={{ width: 160 }}>
              <Select
                placeholder="选择后填入 cron"
                options={presets}
                onChange={(v) => form.setFieldsValue({ cron_expr: v })}
              />
            </Form.Item>
            <Button style={{ marginTop: 30 }} loading={preview.isPending} onClick={() => preview.mutate(cronExpr ?? "")}>
              预览下次
            </Button>
            <Form.Item name="concurrency_policy" label="并发策略" style={{ width: 180 }}>
              <Select
                options={[
                  { value: "skip", label: "上次未完成则跳过" },
                  { value: "queue", label: "排队执行" },
                ]}
              />
            </Form.Item>
          </Space>
          {times.length > 0 && (
            <Space wrap>
              {times.map((t) => (
                <Tag key={t}>{dayjs(t).format("YYYY-MM-DD HH:mm")}</Tag>
              ))}
            </Space>
          )}
        </Card>

        <Card title="目标与参数" style={{ marginBottom: 16 }}>
          {hostsRequired && (
            <Form.Item
              name="host_ids"
              label="目标主机"
              rules={[{ required: true, message: "请选择主机" }]}
            >
              <Select
                mode="multiple"
                optionFilterProp="label"
                placeholder="选择一台或多台主机"
                options={hosts.map((h) => ({
                  value: h.id,
                  label: `${h.name} (${h.address})${h.enabled ? "" : " [停用]"}`,
                }))}
              />
            </Form.Item>
          )}
          {params.length === 0 ? (
            <Typography.Text type="secondary">该执行对象没有参数</Typography.Text>
          ) : (
            params.map((p) => (
              <Form.Item
                key={p.name}
                name={["params", p.name]}
                label={
                  <Space size={4}>
                    {p.label || p.name}
                    {p.sensitive && <Tag color="red">敏感</Tag>}
                  </Space>
                }
                extra={p.description || (p.sensitive ? "敏感值保存后不再显示，留空表示沿用原值" : undefined)}
                rules={
                  p.type === "host"
                    ? [{
                        validator: (_, value) =>
                          parseHostIDs(value).length > 0
                            ? Promise.resolve()
                            : Promise.reject(new Error("请选择执行主机")),
                      }]
                    : p.required && !p.sensitive
                      ? [{ required: true, message: `请输入 ${p.label || p.name}` }]
                      : []
                }
              >
                {paramInput(p, !!id, hosts)}
              </Form.Item>
            ))
          )}
        </Card>

        <Card title="通知">
          <Space wrap align="start">
            <Form.Item name="notify_channel_ids" label="通知渠道" style={{ minWidth: 320 }}>
              <Select
                mode="multiple"
                optionFilterProp="label"
                placeholder="可选，多渠道"
                options={channels.map((c) => ({ value: c.id, label: `${c.name} (${c.type})` }))}
              />
            </Form.Item>
            <Form.Item name="notify_events" label="通知时机" style={{ width: 160 }}>
              <Select
                options={[
                  { value: "failure", label: "仅失败时" },
                  { value: "always", label: "每次都通知" },
                ]}
              />
            </Form.Item>
          </Space>
        </Card>
      </Form>
    </div>
  );
}
