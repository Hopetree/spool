import { useState } from "react";
import { App as AntApp, Button, Form, Input, Modal, Table } from "antd";
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
import PageHeader from "../components/PageHeader";
import RowActions from "../components/RowActions";

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
      <PageHeader
        title="分组管理"
        subtitle={`共 ${data.length} 个分组`}
        extra={
          <Button type="primary" onClick={openCreate}>
            新增分组
          </Button>
        }
      />
      <Table<HostGroup>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={data}
        columns={[
          { title: "名称", dataIndex: "name" },
          { title: "描述", dataIndex: "description" },
          { title: "主机数", dataIndex: "host_count" },
          {
            title: "操作",
            width: 120,
            render: (_, record) => (
              <RowActions
                inline={2}
                actions={[
                  { key: "edit", label: "编辑", onClick: () => openEdit(record) },
                  {
                    key: "delete",
                    label: "删除",
                    danger: true,
                    onClick: () => remove.mutate(record.id),
                    confirm: { title: "确认删除该分组？", description: "组内主机将变为未分组。" },
                  },
                ]}
              />
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
