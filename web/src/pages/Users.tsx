import { useState } from "react";
import { App as AntApp, Button, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import {
  createUser,
  deleteUser,
  listUsers,
  logoutAllSessions,
  resetUserPassword,
  updateUser,
  type CreateUserInput,
  type Role,
  type UserView,
} from "../api/users";
import { useAuthStore } from "../stores/auth";

const roleLabel: Record<Role, string> = { admin: "管理员", operator: "操作员", viewer: "只读" };
const roleColor: Record<Role, string> = { admin: "red", operator: "blue", viewer: "default" };

export default function Users() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const me = useAuthStore((s) => s.user);
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<UserView | null>(null);
  const [resetting, setResetting] = useState<UserView | null>(null);
  const [createForm] = Form.useForm<CreateUserInput>();
  const [editForm] = Form.useForm<{ display_name: string; role: Role; enabled: boolean }>();
  const [resetForm] = Form.useForm<{ password: string }>();

  const { data = [], isLoading } = useQuery({ queryKey: ["users"], queryFn: listUsers });

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["users"] });

  const create = useMutation({
    mutationFn: createUser,
    onSuccess: () => {
      message.success("已创建");
      setCreateOpen(false);
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "创建失败"),
  });

  const update = useMutation({
    mutationFn: (values: { display_name: string; role: Role; enabled: boolean }) =>
      updateUser(editing!.id, values),
    onSuccess: () => {
      message.success("已保存");
      setEditing(null);
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const reset = useMutation({
    mutationFn: (password: string) => resetUserPassword(resetting!.id, password),
    onSuccess: () => {
      message.success("密码已重置，用户下次登录需修改");
      setResetting(null);
      resetForm.resetFields();
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "重置失败"),
  });

  const forceLogout = useMutation({
    mutationFn: logoutAllSessions,
    onSuccess: () => message.success("已强制下线"),
    onError: (err) => message.error(err instanceof ApiError ? err.message : "操作失败"),
  });

  const remove = useMutation({
    mutationFn: deleteUser,
    onSuccess: () => {
      message.success("已删除");
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" onClick={() => setCreateOpen(true)}>
          新增用户
        </Button>
      </Space>
      <Table<UserView>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={data}
        columns={[
          { title: "用户名", dataIndex: "username" },
          { title: "显示名", dataIndex: "display_name" },
          {
            title: "角色",
            dataIndex: "role",
            render: (v: Role) => <Tag color={roleColor[v]}>{roleLabel[v] ?? v}</Tag>,
          },
          {
            title: "状态",
            dataIndex: "enabled",
            render: (v: boolean) => (v ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
          },
          {
            title: "需改密",
            dataIndex: "must_change_password",
            render: (v: boolean) => (v ? "是" : "否"),
          },
          {
            title: "最近登录",
            dataIndex: "last_login_at",
            render: (v: number | null) => (v ? dayjs(v).format("YYYY-MM-DD HH:mm") : "—"),
          },
          {
            title: "操作",
            width: 270,
            render: (_, r) => (
              <Space size={4} wrap={false}>
                <Button
                  size="small"
                  onClick={() => {
                    setEditing(r);
                    editForm.setFieldsValue({ display_name: r.display_name, role: r.role, enabled: r.enabled });
                  }}
                >
                  编辑
                </Button>
                <Button size="small" onClick={() => setResetting(r)}>
                  重置密码
                </Button>
                <Popconfirm title="强制该用户所有会话下线？" onConfirm={() => forceLogout.mutate(r.id)}>
                  <Button size="small">强制下线</Button>
                </Popconfirm>
                <Popconfirm title="确认删除该用户？" onConfirm={() => remove.mutate(r.id)}>
                  <Button size="small" danger disabled={r.id === me?.id}>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        open={createOpen}
        title="新增用户"
        onCancel={() => setCreateOpen(false)}
        onOk={() => createForm.submit()}
        confirmLoading={create.isPending}
        destroyOnClose
      >
        <Form<CreateUserInput>
          form={createForm}
          layout="vertical"
          initialValues={{ role: "operator" }}
          onFinish={(values) => create.mutate(values)}
        >
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: "请输入用户名" }]}>
            <Input placeholder="3-32 位字母数字 . _ -" />
          </Form.Item>
          <Form.Item name="display_name" label="显示名" rules={[{ required: true, message: "请输入显示名" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="role" label="角色" rules={[{ required: true }]}>
            <Select
              options={(Object.keys(roleLabel) as Role[]).map((r) => ({ value: r, label: roleLabel[r] }))}
            />
          </Form.Item>
          <Form.Item
            name="password"
            label="初始密码"
            rules={[
              { required: true, message: "请输入初始密码" },
              { min: 8, message: "至少 8 位" },
              { pattern: /^(?=.*[A-Za-z])(?=.*\d).+$/, message: "需同时包含字母和数字" },
            ]}
          >
            <Input.Password />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={!!editing}
        title={`编辑用户：${editing?.username ?? ""}`}
        onCancel={() => setEditing(null)}
        onOk={() => editForm.submit()}
        confirmLoading={update.isPending}
        destroyOnClose
      >
        <Form form={editForm} layout="vertical" onFinish={(values) => update.mutate(values)}>
          <Form.Item name="display_name" label="显示名" rules={[{ required: true, message: "请输入显示名" }]}>
            <Input />
          </Form.Item>
          <Form.Item name="role" label="角色" rules={[{ required: true }]}>
            <Select
              options={(Object.keys(roleLabel) as Role[]).map((r) => ({ value: r, label: roleLabel[r] }))}
            />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={!!resetting}
        title={`重置密码：${resetting?.username ?? ""}`}
        onCancel={() => setResetting(null)}
        onOk={() => resetForm.submit()}
        confirmLoading={reset.isPending}
        destroyOnClose
      >
        <Form form={resetForm} layout="vertical" onFinish={(values) => reset.mutate(values.password)}>
          <Form.Item
            name="password"
            label="新密码"
            rules={[
              { required: true, message: "请输入新密码" },
              { min: 8, message: "至少 8 位" },
              { pattern: /^(?=.*[A-Za-z])(?=.*\d).+$/, message: "需同时包含字母和数字" },
            ]}
          >
            <Input.Password />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
