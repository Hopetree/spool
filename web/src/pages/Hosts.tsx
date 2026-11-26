import { useState } from "react";
import {
  Alert,
  App as AntApp,
  Button,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import { listCredentials, type Credential } from "../api/credentials";
import { listGroups } from "../api/groups";
import {
  createHost,
  deleteHost,
  listHosts,
  setHostFingerprint,
  testHost,
  updateHost,
  type Host,
  type HostInput,
  type HostTestResult,
} from "../api/hosts";
import { useAuthStore } from "../stores/auth";

interface Filters {
  group_id?: string;
  tag?: string;
  enabled?: string;
  keyword?: string;
}

interface KeyIssue {
  hostId: string;
  hostName: string;
  expected: string;
  actual: string;
  keyType: string;
}

export default function Hosts() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const canOperate = user?.role === "admin" || user?.role === "operator";

  const [filters, setFilters] = useState<Filters>({});
  const [page, setPage] = useState(1);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Host | null>(null);
  const [testResult, setTestResult] = useState<HostTestResult | null>(null);
  const [keyIssue, setKeyIssue] = useState<KeyIssue | null>(null);
  const [form] = Form.useForm<HostInput>();

  const enabled = filters.enabled === "true" ? true : filters.enabled === "false" ? false : undefined;
  const queryParams = { group_id: filters.group_id, tag: filters.tag, enabled, keyword: filters.keyword };

  const { data, isLoading } = useQuery({
    queryKey: ["hosts", queryParams],
    queryFn: () => listHosts({ ...queryParams, page: 1, page_size: 100 }),
  });
  const hosts = data?.items ?? [];

  const { data: groups = [] } = useQuery({ queryKey: ["host-groups"], queryFn: listGroups });
  const { data: credentials = [] } = useQuery({
    queryKey: ["credentials"],
    queryFn: listCredentials,
    enabled: isAdmin,
  });

  const save = useMutation({
    mutationFn: (values: HostInput) => (editing ? updateHost(editing.id, values) : createHost(values)),
    onSuccess: () => {
      message.success("已保存");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["hosts"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "保存失败"),
  });

  const remove = useMutation({
    mutationFn: deleteHost,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["hosts"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  const test = useMutation({
    mutationFn: testHost,
    onSuccess: (res) => {
      setTestResult(res);
      void qc.invalidateQueries({ queryKey: ["hosts"] });
    },
    onError: (err, hostID) => {
      if (err instanceof ApiError && err.code === "host_key_mismatch") {
        const details = (err.details ?? {}) as Record<string, unknown>;
        const target = hosts.find((h) => h.id === hostID);
        setKeyIssue({
          hostId: hostID,
          hostName: target?.name ?? hostID,
          expected: String(details.expected ?? ""),
          actual: String(details.actual ?? ""),
          keyType: String(details.key_type ?? ""),
        });
        return;
      }
      message.error(err instanceof ApiError ? err.message : "测试失败");
    },
  });

  const trust = useMutation({
    mutationFn: (v: KeyIssue) => setHostFingerprint(v.hostId, v.actual, v.keyType),
    onSuccess: () => {
      message.success("指纹已更新，请重新测试");
      setKeyIssue(null);
      void qc.invalidateQueries({ queryKey: ["hosts"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "更新指纹失败"),
  });

  const openEdit = (host: Host | null) => {
    setEditing(host);
    if (host) {
      form.setFieldsValue({
        name: host.name,
        address: host.address,
        port: host.port,
        group_id: host.group_id ?? undefined,
        credential_id: host.credential_id,
        enabled: host.enabled,
        note: host.note,
        tags: host.tags,
      });
    } else {
      form.resetFields();
      form.setFieldsValue({ port: 22, enabled: true, tags: [] });
    }
    setOpen(true);
  };

  const credentialOptions = (credentials as Credential[]).map((c) => ({ value: c.id, label: c.name }));

  return (
    <div>
      <Space wrap style={{ marginBottom: 16 }}>
        <Select
          allowClear
          placeholder="分组"
          style={{ width: 150 }}
          value={filters.group_id}
          onChange={(v) => setFilters((f) => ({ ...f, group_id: v }))}
          options={groups.map((g) => ({ value: g.id, label: g.name }))}
        />
        <Input
          allowClear
          placeholder="标签"
          style={{ width: 130 }}
          value={filters.tag}
          onChange={(e) => setFilters((f) => ({ ...f, tag: e.target.value }))}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 110 }}
          value={filters.enabled}
          onChange={(v) => setFilters((f) => ({ ...f, enabled: v }))}
          options={[
            { value: "true", label: "启用" },
            { value: "false", label: "停用" },
          ]}
        />
        <Input.Search
          allowClear
          placeholder="名称或地址"
          style={{ width: 200 }}
          onSearch={(v) => setFilters((f) => ({ ...f, keyword: v }))}
        />
        {isAdmin && (
          <Button type="primary" onClick={() => openEdit(null)}>
            新增主机
          </Button>
        )}
        <Link to="/hosts/groups">
          <Button>分组管理</Button>
        </Link>
      </Space>

      <Table<Host>
        rowKey="id"
        loading={isLoading}
        dataSource={hosts}
        pagination={{ current: page, pageSize: 20, onChange: setPage, showSizeChanger: false }}
        columns={[
          { title: "名称", dataIndex: "name" },
          {
            title: "地址",
            render: (_, r) => `${r.address}:${r.port}`,
          },
          { title: "分组", dataIndex: "group_name" },
          {
            title: "标签",
            dataIndex: "tags",
            render: (tags: string[]) => (tags.length ? tags.map((t) => <Tag key={t}>{t}</Tag>) : "—"),
          },
          {
            title: "状态",
            dataIndex: "enabled",
            render: (v: boolean) => (v ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>),
          },
          {
            title: "最近测试",
            render: (_, r) =>
              r.last_test_at ? (
                <Space size={4}>
                  {dayjs(r.last_test_at).format("MM-DD HH:mm")}
                  <Tag color={r.last_test_ok ? "green" : "red"}>{r.last_test_ok ? "通过" : "失败"}</Tag>
                </Space>
              ) : (
                "—"
              ),
          },
          {
            title: "指纹",
            render: (_, r) => (r.fingerprint ? <Tag color="green">已信任</Tag> : <Tag color="orange">待确认</Tag>),
          },
          {
            title: "操作",
            render: (_, r) => (
              <Space>
                {canOperate && (
                  <Button size="small" loading={test.isPending && test.variables === r.id} onClick={() => test.mutate(r.id)}>
                    测试
                  </Button>
                )}
                {isAdmin && (
                  <Button size="small" onClick={() => openEdit(r)}>
                    编辑
                  </Button>
                )}
                {isAdmin && (
                  <Popconfirm title="确认删除该主机？" onConfirm={() => remove.mutate(r.id)}>
                    <Button size="small" danger>
                      删除
                    </Button>
                  </Popconfirm>
                )}
              </Space>
            ),
          },
        ]}
      />

      <Modal
        open={open}
        title={editing ? "编辑主机" : "新增主机"}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={save.isPending}
        destroyOnClose
        width={560}
      >
        <Form<HostInput> form={form} layout="vertical" onFinish={(values) => save.mutate(values)}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}>
            <Input />
          </Form.Item>
          <Space style={{ display: "flex" }} align="start">
            <Form.Item name="address" label="地址" rules={[{ required: true, message: "请输入地址" }]} style={{ flex: 1 }}>
              <Input placeholder="IP 或域名" />
            </Form.Item>
            <Form.Item name="port" label="端口" rules={[{ required: true }]} style={{ width: 110 }}>
              <InputNumber min={1} max={65535} style={{ width: "100%" }} />
            </Form.Item>
          </Space>
          <Form.Item name="group_id" label="分组">
            <Select allowClear options={groups.map((g) => ({ value: g.id, label: g.name }))} placeholder="不分组" />
          </Form.Item>
          <Form.Item name="credential_id" label="凭据" rules={[{ required: true, message: "请选择凭据" }]}>
            <Select options={credentialOptions} placeholder="选择凭据" showSearch optionFilterProp="label" />
          </Form.Item>
          <Form.Item name="tags" label="标签">
            <Select mode="tags" tokenSeparators={[","]} placeholder="回车添加标签" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="note" label="备注">
            <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={!!testResult}
        title="连接测试成功"
        onCancel={() => setTestResult(null)}
        footer={<Button onClick={() => setTestResult(null)}>关闭</Button>}
        width={620}
      >
        {testResult && (
          <Descriptions
            column={1}
            size="small"
            bordered
            items={[
              { key: "user", label: "登录用户", children: testResult.capability.user },
              { key: "os", label: "系统", children: `${testResult.capability.distro || testResult.capability.os} ${testResult.capability.kernel}` },
              { key: "arch", label: "架构", children: testResult.capability.arch },
              { key: "bash", label: "Bash", children: testResult.capability.bash_version || "未找到" },
              { key: "python", label: "Python", children: testResult.capability.python_version || "未找到" },
              { key: "sftp", label: "SFTP", children: testResult.capability.sftp ? "可用" : "不可用" },
              { key: "sudo", label: "免密 sudo", children: testResult.capability.sudo ? "可用" : "不可用" },
              { key: "tools", label: "setsid / timeout", children: `${testResult.capability.setsid ? "有" : "无"} / ${testResult.capability.timeout ? "有" : "无"}` },
              { key: "latency", label: "握手耗时", children: `${testResult.latency_ms} ms` },
              { key: "fp", label: "主机指纹", children: testResult.fingerprint || "—" },
              { key: "fpnote", label: "指纹记录", children: testResult.fingerprint_recorded ? "本次已首次记录" : "已存在" },
            ]}
          />
        )}
      </Modal>

      <Modal
        open={!!keyIssue}
        title="主机指纹需要确认"
        onCancel={() => setKeyIssue(null)}
        width={560}
        footer={[
          <Button key="cancel" onClick={() => setKeyIssue(null)}>
            取消
          </Button>,
          isAdmin ? (
            <Button
              key="trust"
              type="primary"
              loading={trust.isPending}
              onClick={() => keyIssue && trust.mutate(keyIssue)}
            >
              信任并更新指纹
            </Button>
          ) : null,
        ]}
      >
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          message={
            keyIssue?.expected
              ? "指纹与已记录值不一致，连接已被拒绝"
              : "首次连接，确认指纹后才能信任（strict 模式）"
          }
        />
        <Descriptions
          column={1}
          size="small"
          bordered
          items={[
            { key: "host", label: "主机", children: keyIssue?.hostName },
            { key: "expected", label: "期望指纹", children: keyIssue?.expected || "（无）" },
            { key: "actual", label: "实际指纹", children: keyIssue?.actual },
            { key: "type", label: "密钥类型", children: keyIssue?.keyType },
          ]}
        />
      </Modal>
    </div>
  );
}
