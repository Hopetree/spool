import { useState } from "react";
import { App as AntApp, Button, Form, Input, Modal, Popconfirm, Space, Table } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import {
  createGroup,
  deleteGroup,
  listGroups,
  updateGroup,
  type GroupInput,
  type HostGroup,
} from "../api/groups";

export default function Groups() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<HostGroup | null>(null);
  const [form] = Form.useForm<GroupInput>();

  const { data = [], isLoading } = useQuery({ queryKey: ["host-groups"], queryFn: listGroups });

  const save = useMutation({
    mutationFn: (values: GroupInput) =>
      editing ? updateGroup(editing.id, values) : createGroup(values),
    onSuccess: () => {
      message.success("已保存");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["host-groups"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const remove = useMutation({
    mutationFn: deleteGroup,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["host-groups"] });
      void qc.invalidateQueries({ queryKey: ["hosts"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    setOpen(true);
  };

  const openEdit = (group: HostGroup) => {
    setEditing(group);
    form.setFieldsValue({ name: group.name, description: group.description });
    setOpen(true);
  };

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" onClick={openCreate}>
          新增分组
        </Button>
      </Space>
      <Table<HostGroup>
        rowKey="id"
        loading={isLoading}
        dataSource={data}
        columns={[
          { title: "名称", dataIndex: "name" },
          { title: "描述", dataIndex: "description" },
          { title: "主机数", dataIndex: "host_count" },
          {
            title: "操作",
            render: (_, record) => (
              <Space>
                <Button size="small" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Popconfirm
                  title="确认删除该分组？"
                  description="组内主机将变为未分组。"
                  onConfirm={() => remove.mutate(record.id)}
                >
                  <Button size="small" danger>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        open={open}
        title={editing ? "编辑分组" : "新增分组"}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={save.isPending}
        destroyOnClose
      >
        <Form<GroupInput> form={form} layout="vertical" onFinish={(values) => save.mutate(values)}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
