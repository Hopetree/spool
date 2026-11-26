import { App as AntApp, Button, Card, Form, Input, InputNumber, Select, Space, Switch } from "antd";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { ApiError } from "../api/client";
import { runAdhoc } from "../api/executions";
import { listHosts } from "../api/hosts";
import PageHeader from "../components/PageHeader";

interface FormValues {
  host_ids: string[];
  command: string;
  timeout_sec: number;
  sudo: boolean;
}

export default function Adhoc() {
  const [form] = Form.useForm<FormValues>();
  const navigate = useNavigate();
  const { message } = AntApp.useApp();
  const { data: hostPage } = useQuery({
    queryKey: ["hosts", "adhoc"],
    queryFn: () => listHosts({ page: 1, page_size: 100 }),
  });
  const hosts = (hostPage?.items ?? []).filter((h) => h.enabled);

  const run = useMutation({
    mutationFn: runAdhoc,
    onSuccess: (ex) => {
      message.success("已提交");
      navigate(`/executions/${ex.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "提交失败"),
  });

  return (
    <div>
      <PageHeader
        title="临时命令"
        subtitle="不创建工具，直接在一批主机上执行 shell 命令；执行记录与输出同样保留"
      />
      <Card>
      <Form<FormValues>
        form={form}
        layout="vertical"
        initialValues={{ timeout_sec: 300, sudo: false }}
        onFinish={(values) => run.mutate({ kind: "adhoc", ...values })}
      >
        <Form.Item name="host_ids" label="目标主机" rules={[{ required: true, message: "请选择主机" }]}>
          <Select
            mode="multiple"
            optionFilterProp="label"
            placeholder="选择一台或多台主机"
            options={hosts.map((h) => ({ value: h.id, label: `${h.name} (${h.address})` }))}
          />
        </Form.Item>
        <Form.Item name="command" label="命令" rules={[{ required: true, message: "请输入命令" }]}>
          <Input.TextArea
            autoSize={{ minRows: 8, maxRows: 20 }}
            style={{ fontFamily: "Menlo, Consolas, monospace", fontSize: 12 }}
            placeholder={"# 例如\nuname -a\nsystemctl status nginx --no-pager"}
          />
        </Form.Item>
        <Space wrap align="start">
          <Form.Item name="timeout_sec" label="超时（秒）" style={{ width: 140 }}>
            <InputNumber min={1} max={86400} style={{ width: "100%" }} />
          </Form.Item>
          <Form.Item name="sudo" label="sudo -n 执行" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Space>
        <Button type="primary" htmlType="submit" loading={run.isPending}>
          执行
        </Button>
      </Form>
      </Card>
    </div>
  );
}
