import { useState } from "react";
import { App as AntApp, Button, Form, Input, Modal, Select, Table, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import {
  createCredential,
  deleteCredential,
  listCredentials,
  updateCredential,
  type AuthType,
  type Credential,
  type CredentialInput,
} from "../api/credentials";
import PageHeader from "../components/PageHeader";
import RowActions from "../components/RowActions";

const typeLabel: Record<AuthType, string> = { password: "密码", private_key: "私钥" };

export default function Credentials() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Credential | null>(null);
  const [form] = Form.useForm<CredentialInput>();
  const authType = Form.useWatch("auth_type", form);

  const { data = [], isLoading } = useQuery({ queryKey: ["credentials"], queryFn: listCredentials });

  const save = useMutation({
    mutationFn: (values: CredentialInput) =>
      editing ? updateCredential(editing.id, values) : createCredential(values),
    onSuccess: () => {
      message.success("已保存");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["credentials"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const remove = useMutation({
    mutationFn: deleteCredential,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["credentials"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ auth_type: "password" });
    setOpen(true);
  };

  const openEdit = (cred: Credential) => {
    setEditing(cred);
    form.setFieldsValue({
      name: cred.name,
      auth_type: cred.auth_type,
      username: cred.username,
      description: cred.description,
      secret: "",
      passphrase: "",
    });
    setOpen(true);
  };

  return (
    <div>
      <PageHeader
        title="凭据"
        subtitle={`共 ${data.length} 个`}
        extra={
          <Button type="primary" onClick={openCreate}>
            新增凭据
          </Button>
        }
      />
      <Table<Credential>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={data}
        columns={[
          { title: "名称", dataIndex: "name" },
          {
            title: "类型",
            dataIndex: "auth_type",
            render: (value: AuthType) => <Tag>{typeLabel[value]}</Tag>,
          },
          { title: "登录用户名", dataIndex: "username" },
          {
            title: "私钥口令",
            dataIndex: "has_passphrase",
            render: (value: boolean) => (value ? "有" : "无"),
          },
          { title: "描述", dataIndex: "description" },
          {
            title: "操作",
            width: 130,
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
                    confirm: { title: "确认删除该凭据？", description: "被主机引用时无法删除。" },
                  },
                ]}
              />
            ),
          },
        ]}
      />

      <Modal
        open={open}
        title={editing ? "编辑凭据" : "新增凭据"}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={save.isPending}
        destroyOnClose
      >
        <Form<CredentialInput> form={form} layout="vertical" onFinish={(values) => save.mutate(values)}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="auth_type" label="认证方式" rules={[{ required: true }]}>
            <Select
              options={[
                { value: "password", label: "密码" },
                { value: "private_key", label: "私钥" },
              ]}
            />
          </Form.Item>
          <Form.Item name="username" label="登录用户名" rules={[{ required: true, message: "请输入用户名" }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="secret"
            label={authType === "private_key" ? "私钥内容" : "密码"}
            rules={editing ? [] : [{ required: true, message: "请输入密码或私钥" }]}
            extra={editing ? "留空表示不修改" : undefined}
          >
            <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} />
          </Form.Item>
          {authType === "private_key" && (
            <Form.Item name="passphrase" label="私钥口令" extra={editing ? "留空表示不修改" : undefined}>
              <Input.Password />
            </Form.Item>
          )}
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
