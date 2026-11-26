import { useState } from "react";
import { App as AntApp, Button, Form, Input, Modal, Select, Switch, Table, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import {
  createChannel,
  deleteChannel,
  listChannels,
  testChannel,
  updateChannel,
  type Channel,
  type ChannelInput,
  type ChannelType,
} from "../api/notify";
import PageHeader from "../components/PageHeader";
import RowActions from "../components/RowActions";

const typeLabel: Record<ChannelType, string> = {
  webhook: "通用 Webhook",
  feishu: "飞书",
  dingtalk: "钉钉",
  wecom: "企业微信",
};

export default function Channels() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Channel | null>(null);
  const [form] = Form.useForm<ChannelInput>();

  const { data = [], isLoading } = useQuery({ queryKey: ["notify-channels"], queryFn: listChannels });

  const save = useMutation({
    mutationFn: (values: ChannelInput) =>
      editing ? updateChannel(editing.id, values) : createChannel(values),
    onSuccess: () => {
      message.success("已保存");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["notify-channels"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const remove = useMutation({
    mutationFn: deleteChannel,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["notify-channels"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  const test = useMutation({
    mutationFn: testChannel,
    onSuccess: () => message.success("测试消息已发送"),
    onError: (err) => message.error(err instanceof ApiError ? err.message : "发送失败"),
  });

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ type: "webhook", enabled: true });
    setOpen(true);
  };
  const openEdit = (ch: Channel) => {
    setEditing(ch);
    form.setFieldsValue({ name: ch.name, type: ch.type, url: ch.url, enabled: ch.enabled, secret: "" });
    setOpen(true);
  };

  return (
    <div>
      <PageHeader
        title="通知渠道"
        subtitle={`共 ${data.length} 个`}
        extra={
          <Button type="primary" onClick={openCreate}>
            新增渠道
          </Button>
        }
      />
      <Table<Channel>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={data}
        columns={[
          { title: "名称", dataIndex: "name" },
          {
            title: "类型",
            dataIndex: "type",
            render: (v: ChannelType) => <Tag>{typeLabel[v] ?? v}</Tag>,
          },
          { title: "地址", dataIndex: "url", ellipsis: true },
          {
            title: "加签密钥",
            dataIndex: "has_secret",
            width: 100,
            render: (v: boolean) => (v ? "已配置" : "无"),
          },
          {
            title: "启用",
            dataIndex: "enabled",
            width: 80,
            render: (v: boolean) => (v ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
          },
          {
            title: "操作",
            width: 150,
            render: (_, r) => (
              <RowActions
                inline={2}
                actions={[
                  { key: "test", label: "测试", loading: test.isPending, onClick: () => test.mutate(r.id) },
                  { key: "edit", label: "编辑", onClick: () => openEdit(r) },
                  {
                    key: "delete",
                    label: "删除",
                    danger: true,
                    onClick: () => remove.mutate(r.id),
                    confirm: { title: "确认删除该渠道？" },
                  },
                ]}
              />
            ),
          },
        ]}
      />

      <Modal
        open={open}
        title={editing ? "编辑通知渠道" : "新增通知渠道"}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={save.isPending}
        destroyOnClose
        width={560}
      >
        <Form<ChannelInput> form={form} layout="vertical" onFinish={(values) => save.mutate(values)}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="type" label="类型" rules={[{ required: true }]}>
            <Select
              options={(Object.keys(typeLabel) as ChannelType[]).map((t) => ({ value: t, label: typeLabel[t] }))}
            />
          </Form.Item>
          <Form.Item
            name="url"
            label="Webhook 地址"
            rules={[{ required: true, message: "请输入地址" }]}
            extra="企业微信填机器人完整地址；钉钉/飞书填机器人地址"
          >
            <Input placeholder="https://..." />
          </Form.Item>
          <Form.Item name="secret" label="加签密钥" extra={editing ? "留空表示不修改；钉钉/飞书机器人可选" : "钉钉/飞书机器人可选"}>
            <Input.Password />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
