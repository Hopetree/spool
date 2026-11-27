import { useState } from "react";
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DownloadOutlined, ImportOutlined } from "@ant-design/icons";
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
import HostImportModal from "../components/HostImportModal";
import PageHeader from "../components/PageHeader";
import RowActions from "../components/RowActions";
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
  const [viewing, setViewing] = useState<Host | null>(null);
  const [importOpen, setImportOpen] = useState(false);
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
    onSuccess: (res, hostID) => {
      setTestResult(res);
      setViewing((prev) =>
        prev && prev.id === hostID
          ? {
              ...prev,
              capability: res.capability,
              fingerprint: res.fingerprint,
              fingerprint_type: res.key_type,
              last_test_at: Date.now(),
              last_test_ok: res.ok,
            }
          : prev,
      );
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

  const exportCSV = () => {
    const header = ["主机名称", "地址", "端口", "分组", "标签", "状态", "备注", "最近测试"];
    const lines = hosts.map((h) =>
      [
        h.name,
        h.address,
        String(h.port),
        h.group_name || "",
        h.tags.join("|"),
        h.enabled ? "启用" : "停用",
        h.note,
        h.last_test_at ? dayjs(h.last_test_at).format("YYYY-MM-DD HH:mm") : "",
      ]
        .map((v) => `"${String(v).replace(/"/g, '""')}"`)
        .join(","),
    );
    const content = `\uFEFF${header.join(",")}\n${lines.join("\n")}\n`;
    const url = URL.createObjectURL(new Blob([content], { type: "text/csv;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "主机列表.csv";
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div>
      <PageHeader
        title="主机列表"
        subtitle={`共 ${data?.total ?? hosts.length} 台`}
        extra={
          <>
            {isAdmin && (
              <Button icon={<DownloadOutlined />} onClick={exportCSV}>
                导出
              </Button>
            )}
            {isAdmin && (
              <Button icon={<ImportOutlined />} onClick={() => setImportOpen(true)}>
                导入
              </Button>
            )}
            {isAdmin && (
              <Button type="primary" onClick={() => openEdit(null)}>
                新增主机
              </Button>
            )}
            <Link to="/hosts/groups">
              <Button>分组管理</Button>
            </Link>
          </>
        }
      />

      <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
        <Card
          size="small"
          title="分组"
          style={{ width: 230, flex: "0 0 230px" }}
          styles={{ body: { padding: 8 } }}
        >
          <div
            onClick={() => setFilters((f) => ({ ...f, group_id: undefined }))}
            style={{
              padding: "7px 10px",
              borderRadius: 6,
              cursor: "pointer",
              background: filters.group_id ? undefined : "#e6f4ff",
              color: filters.group_id ? undefined : "#1677ff",
              display: "flex",
              justifyContent: "space-between",
              marginBottom: 2,
            }}
          >
            <span>全部主机</span>
            <span style={{ color: "#8c8c8c" }}>
              {filters.group_id ? groups.reduce((sum, g) => sum + g.host_count, 0) : data?.total ?? hosts.length}
            </span>
          </div>
          {groups.map((g) => {
            const selected = filters.group_id === g.id;
            return (
              <div
                key={g.id}
                onClick={() => setFilters((f) => ({ ...f, group_id: g.id }))}
                style={{
                  padding: "7px 10px",
                  borderRadius: 6,
                  cursor: "pointer",
                  background: selected ? "#e6f4ff" : undefined,
                  color: selected ? "#1677ff" : undefined,
                  display: "flex",
                  justifyContent: "space-between",
                }}
              >
                <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {g.name}
                </span>
                <span style={{ color: "#8c8c8c" }}>{g.host_count}</span>
              </div>
            );
          })}
        </Card>

        <div style={{ flex: 1, minWidth: 0 }}>
      <Card size="small" style={{ marginBottom: 16 }} styles={{ body: { padding: 12 } }}>
        <Space wrap>
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
        </Space>
      </Card>

      <Table<Host>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={hosts}
        pagination={{ current: page, pageSize: 20, onChange: setPage, showSizeChanger: false }}
        columns={[
          {
            title: "名称",
            dataIndex: "name",
            render: (v: string, r) => <a onClick={() => setViewing(r)}>{v}</a>,
          },
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
            width: 170,
            render: (_, r) => (
              <RowActions
                inline={2}
                actions={[
                  ...(canOperate
                    ? [
                        {
                          key: "test",
                          label: "测试",
                          loading: test.isPending && test.variables === r.id,
                          onClick: () => test.mutate(r.id),
                        },
                      ]
                    : []),
                  ...(isAdmin ? [{ key: "edit", label: "编辑", onClick: () => openEdit(r) }] : []),
                  ...(isAdmin
                    ? [
                        {
                          key: "delete",
                          label: "删除",
                          danger: true,
                          onClick: () => remove.mutate(r.id),
                          confirm: { title: "确认删除该主机？" },
                        },
                      ]
                    : []),
                ]}
              />
            ),
          },
        ]}
      />
        </div>
      </div>

      <Drawer
        open={!!viewing}
        onClose={() => setViewing(null)}
        width={560}
        title={viewing ? <Space>{viewing.name}<Tag>{viewing.enabled ? "启用" : "停用"}</Tag></Space> : "主机详情"}
        extra={
          viewing && (
            <Space>
              {canOperate && (
                <Button
                  loading={test.isPending && test.variables === viewing.id}
                  onClick={() => test.mutate(viewing.id)}
                >
                  测试连接
                </Button>
              )}
              {isAdmin && (
                <Button
                  onClick={() => {
                    const host = viewing;
                    setViewing(null);
                    openEdit(host);
                  }}
                >
                  编辑
                </Button>
              )}
            </Space>
          )
        }
      >
        {viewing && (
          <Space direction="vertical" size={16} style={{ width: "100%" }}>
            <Descriptions
              title="基本信息"
              bordered
              size="small"
              column={1}
              items={[
                { key: "addr", label: "连接地址", children: `${viewing.address}:${viewing.port}` },
                { key: "group", label: "所属分组", children: viewing.group_name || "未分组" },
                {
                  key: "tags",
                  label: "标签",
                  children: viewing.tags.length ? (
                    <Space wrap size={4}>
                      {viewing.tags.map((t) => (
                        <Tag key={t}>{t}</Tag>
                      ))}
                    </Space>
                  ) : (
                    "—"
                  ),
                },
                ...(isAdmin
                  ? [{ key: "credential", label: "登录凭据", children: viewing.credential_name || "—" }]
                  : []),
                { key: "note", label: "备注", children: viewing.note || "—" },
                { key: "created", label: "创建时间", children: dayjs(viewing.created_at).format("YYYY-MM-DD HH:mm") },
                { key: "updated", label: "更新时间", children: dayjs(viewing.updated_at).format("YYYY-MM-DD HH:mm") },
              ]}
            />

            <Descriptions
              title="系统与能力"
              bordered
              size="small"
              column={1}
              items={[
                {
                  key: "probed",
                  label: "探测时间",
                  children: viewing.capability.probed_at
                    ? dayjs(viewing.capability.probed_at).format("YYYY-MM-DD HH:mm:ss")
                    : "尚未探测",
                },
                {
                  key: "os",
                  label: "操作系统",
                  children: viewing.capability.distro || viewing.capability.os || "—",
                },
                {
                  key: "kernel",
                  label: "内核 / 架构",
                  children: viewing.capability.kernel
                    ? `${viewing.capability.kernel} / ${viewing.capability.arch || "—"}`
                    : "—",
                },
                { key: "user", label: "登录用户", children: viewing.capability.user || "—" },
                {
                  key: "bash",
                  label: "Bash",
                  children: viewing.capability.bash_path
                    ? `${viewing.capability.bash_path} ${viewing.capability.bash_version || ""}`.trim()
                    : "—",
                },
                {
                  key: "python",
                  label: "Python",
                  children: viewing.capability.python_path
                    ? `${viewing.capability.python_path} ${viewing.capability.python_version || ""}`.trim()
                    : "—",
                },
                {
                  key: "caps",
                  label: "能力",
                  children: viewing.capability.probed_at ? (
                    <Space wrap size={4}>
                      {(
                        [
                          ["sftp", "SFTP"],
                          ["sudo", "sudo"],
                          ["setsid", "setsid"],
                          ["timeout", "timeout"],
                        ] as const
                      ).map(([key, label]) => (
                        <Tag key={key} color={viewing.capability[key] ? "green" : undefined}>
                          {label}
                          {viewing.capability[key] ? "" : "（不支持）"}
                        </Tag>
                      ))}
                    </Space>
                  ) : (
                    "—"
                  ),
                },
                {
                  key: "latency",
                  label: "探测延迟",
                  children: viewing.capability.probed_at ? `${viewing.capability.latency_ms} ms` : "—",
                },
              ]}
            />

            <Descriptions
              title="安全与状态"
              bordered
              size="small"
              column={1}
              items={[
                {
                  key: "fingerprint",
                  label: "主机指纹",
                  children: viewing.fingerprint ? (
                    <Typography.Text code copyable={{ text: viewing.fingerprint }}>
                      {viewing.fingerprint}
                    </Typography.Text>
                  ) : (
                    <Tag color="orange">待确认</Tag>
                  ),
                },
                { key: "keytype", label: "密钥类型", children: viewing.fingerprint_type || "—" },
                {
                  key: "trust",
                  label: "信任状态",
                  children: viewing.fingerprint ? <Tag color="green">已信任</Tag> : <Tag color="orange">待确认</Tag>,
                },
                {
                  key: "lasttest",
                  label: "最近测试",
                  children: viewing.last_test_at ? (
                    <Space size={6}>
                      {dayjs(viewing.last_test_at).format("YYYY-MM-DD HH:mm:ss")}
                      <Tag color={viewing.last_test_ok ? "green" : "red"}>
                        {viewing.last_test_ok ? "通过" : "失败"}
                      </Tag>
                    </Space>
                  ) : (
                    "—"
                  ),
                },
              ]}
            />
          </Space>
        )}
      </Drawer>

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

      <HostImportModal
        open={importOpen}
        onClose={() => setImportOpen(false)}
        groups={groups}
        credentials={credentials}
        onImported={() => {
          void qc.invalidateQueries({ queryKey: ["hosts"] });
          void qc.invalidateQueries({ queryKey: ["host-groups"] });
        }}
      />
    </div>
  );
}
