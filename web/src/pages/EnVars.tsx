import { useState } from "react";
import { App as AntApp, Button, Form, Input, Modal, Switch, Table, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import {
  createEnvVar,
  deleteEnvVar,
  listEnvVars,
  updateEnvVar,
  type EnvVar,
  type EnvVarInput,
} from "../api/envars";
import PageHeader from "../components/PageHeader";
import RowActions from "../components/RowActions";

export default function EnVars() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<EnvVar | null>(null);
  const [form] = Form.useForm<EnvVarInput>();

  const { data, isLoading } = useQuery({ queryKey: ["envars"], queryFn: listEnvVars });
  const items = data?.items ?? [];

  const save = useMutation({
    mutationFn: (values: EnvVarInput) =>
      editing ? updateEnvVar(editing.id, values) : createEnvVar(values),
    onSuccess: () => {
      message.success("已保存");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["envars"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const remove = useMutation({
    mutationFn: deleteEnvVar,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["envars"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ sensitive: false });
    setOpen(true);
  };

  const openEdit = (item: EnvVar) => {
    setEditing(item);
    form.setFieldsValue({
      name: item.name,
      value: "",
      description: item.description,
      sensitive: item.sensitive,
    });
    setOpen(true);
  };

  return (
    <div>
      <PageHeader
        title="环境变量"
        subtitle={`共 ${items.length} 个`}
        crumbs={[{ title: "系统管理", to: "/users" }, { title: "环境变量" }]}
        extra={
          <Button type="primary" onClick={openCreate}>
            新增变量
          </Button>
        }
      />
      <Table<EnvVar>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={items}
        columns={[
          { title: "变量名", dataIndex: "name" },
          { title: "说明", dataIndex: "description", ellipsis: true },
          {
            title: "敏感",
            dataIndex: "sensitive",
            width: 90,
            render: (v: boolean) => (v ? <Tag color="red">敏感</Tag> : "否"),
          },
          { title: "更新人", dataIndex: "updated_by_name", width: 140 },
          {
            title: "更新时间",
            dataIndex: "updated_at",
            width: 170,
            render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm"),
          },
          {
            title: "操作",
            width: 130,
            render: (_, r) => (
              <RowActions
                inline={2}
                actions={[
                  { key: "edit", label: "编辑", onClick: () => openEdit(r) },
                  {
                    key: "delete",
                    label: "删除",
                    danger: true,
                    onClick: () => remove.mutate(r.id),
                    confirm: { title: `确认删除环境变量 ${r.name}？` },
                  },
                ]}
              />
            ),
          },
        ]}
      />

      <Modal
        open={open}
        title={editing ? `编辑环境变量：${editing.name}` : "新增环境变量"}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={save.isPending}
        destroyOnClose
        width={560}
      >
        <Form<EnvVarInput> form={form} layout="vertical" onFinish={(values) => save.mutate(values)}>
          <Form.Item
            name="name"
            label="变量名"
            rules={[{ required: true, message: "请输入变量名" }]}
            extra="大写字母、数字和下划线；不能以 SPOOL_ 开头，不能使用 PATH/HOME 等保留名称"
          >
            <Input placeholder="APP_URL" disabled={!!editing} />
          </Form.Item>
          <Form.Item
            name="value"
            label="变量值"
            rules={editing ? [] : [{ required: true, message: "请输入变量值" }]}
            extra={editing ? "留空表示保持原值；保存后不再显示明文" : "保存后不再显示明文"}
          >
            <Input.Password autoComplete="new-password" placeholder={editing ? "留空保持原值" : "变量值"} />
          </Form.Item>
          <Form.Item name="description" label="说明">
            <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder="用途说明" />
          </Form.Item>
          <Form.Item name="sensitive" label="敏感" valuePropName="checked" extra="敏感变量同样只写不读，审计不记录值">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
