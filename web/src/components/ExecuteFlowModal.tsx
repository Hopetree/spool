import { useEffect, type ReactNode } from "react";
import { App as AntApp, Form, Input, Modal, Select } from "antd";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { ApiError } from "../api/client";
import { runFlow } from "../api/executions";
import { getFlow } from "../api/flows";
import { listHosts } from "../api/hosts";
import type { ToolParam } from "../api/tools";

interface Props {
  flowID: string | null;
  open: boolean;
  onClose: () => void;
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

export default function ExecuteFlowModal({ flowID, open, onClose }: Props) {
  const [form] = Form.useForm<FormValues>();
  const navigate = useNavigate();
  const { message } = AntApp.useApp();

  const { data: flow } = useQuery({
    queryKey: ["flow", flowID],
    queryFn: () => getFlow(flowID!),
    enabled: open && !!flowID,
  });
  const { data: hostPage } = useQuery({
    queryKey: ["hosts", "execute"],
    queryFn: () => listHosts({ page: 1, page_size: 100 }),
    enabled: open,
  });
  const hosts = (hostPage?.items ?? []).filter((h) => h.enabled);

  const run = useMutation({
    mutationFn: runFlow,
    onSuccess: (ex) => {
      message.success("已提交流程执行");
      onClose();
      navigate(`/executions/${ex.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "提交失败"),
  });

  useEffect(() => {
    if (!open) return;
    const defaults: Record<string, string> = {};
    flow?.params.forEach((p) => {
      if (p.default) defaults[p.name] = p.default;
    });
    form.resetFields();
    form.setFieldsValue({ params: defaults });
  }, [open, flow, form]);

  return (
    <Modal
      open={open}
      title={`执行流程${flow ? `：${flow.name}` : ""}`}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={run.isPending}
      width={640}
      destroyOnClose
    >
      <Form<FormValues>
        form={form}
        layout="vertical"
        onFinish={(values) => {
          if (!flowID) return;
          run.mutate({
            kind: "flow",
            ref_id: flowID,
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
        {(flow?.params ?? []).map((p) => (
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
