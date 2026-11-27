import { useEffect, type ReactNode } from "react";
import { App as AntApp, Form, Input, Modal, Select } from "antd";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { ApiError } from "../api/client";
import { runTool } from "../api/executions";
import { listHosts } from "../api/hosts";
import { getTool, type ToolParam } from "../api/tools";

interface Props {
  toolID: string | null;
  open: boolean;
  onClose: () => void;
  initialHostIDs?: string[];
  initialParams?: Record<string, string>;
  rerun?: boolean;
}

interface FormValues {
  host_ids: string[];
  params: Record<string, string>;
}

function paramInput(p: ToolParam): ReactNode {
  if (p.type === "bool") {
    return (
      <Select
        options={[
          { value: "true", label: "true" },
          { value: "false", label: "false" },
        ]}
      />
    );
  }
  if (p.type === "select") {
    return <Select options={(p.options ?? []).map((o) => ({ value: o, label: o }))} />;
  }
  if (p.sensitive) {
    return <Input.Password />;
  }
  if (p.type === "int") {
    return <Input inputMode="numeric" placeholder="整数" />;
  }
  return <Input />;
}

export default function ExecuteToolModal({
  toolID,
  open,
  onClose,
  initialHostIDs,
  initialParams,
  rerun,
}: Props) {
  const [form] = Form.useForm<FormValues>();
  const navigate = useNavigate();
  const { message } = AntApp.useApp();

  const { data: tool } = useQuery({
    queryKey: ["tool", toolID],
    queryFn: () => getTool(toolID!),
    enabled: open && !!toolID,
  });
  const { data: hostPage } = useQuery({
    queryKey: ["hosts", "execute"],
    queryFn: () => listHosts({ page: 1, page_size: 100 }),
    enabled: open,
  });
  const hosts = (hostPage?.items ?? []).filter((h) => h.enabled);

  const run = useMutation({
    mutationFn: runTool,
    onSuccess: (ex) => {
      message.success("已提交执行");
      onClose();
      navigate(`/executions/${ex.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "提交失败"),
  });

  useEffect(() => {
    if (!open) return;
    const defaults: Record<string, string> = {};
    tool?.version.params.forEach((p) => {
      if (p.default) defaults[p.name] = p.default;
    });
    form.resetFields();
    form.setFieldsValue({
      host_ids: initialHostIDs ?? tool?.host_param?.default_host_ids ?? [],
      params: { ...defaults, ...(initialParams ?? {}) },
    });
  }, [open, tool, form, initialHostIDs, initialParams]);

  const params = tool?.version.params ?? [];

  return (
    <Modal
      open={open}
      title={`${rerun ? "再次执行" : "执行"}工具${tool ? `：${tool.name} v${tool.version.version_no}` : ""}`}
      onCancel={onClose}
      onOk={() => form.submit()}
      okText="执行"
      confirmLoading={run.isPending}
      width={640}
      destroyOnClose
    >
      <Form<FormValues>
        form={form}
        layout="vertical"
        onFinish={(values) => {
          if (!toolID) return;
          run.mutate({
            kind: "tool",
            ref_id: toolID,
            params: values.params ?? {},
            host_ids: values.host_ids ?? [],
          });
        }}
      >
        <Form.Item name="host_ids" label="目标主机" rules={[{ required: true, message: "请选择主机" }]}>
          <Select
            mode="multiple"
            optionFilterProp="label"
            placeholder="选择一台或多台主机"
            options={hosts.map((h) => ({ value: h.id, label: `${h.name} (${h.address})` }))}
          />
        </Form.Item>
        {params.map((p) => (
          <Form.Item
            key={p.name}
            name={["params", p.name]}
            label={p.label || p.name}
            extra={p.description || (p.sensitive ? "敏感参数，不会在历史中显示" : undefined)}
            rules={p.required ? [{ required: true, message: `请输入 ${p.label || p.name}` }] : []}
          >
            {paramInput(p)}
          </Form.Item>
        ))}
      </Form>
    </Modal>
  );
}
