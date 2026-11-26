import { useState } from "react";
import {
  App as AntApp,
  Button,
  Card,
  Form,
  Input,
  Progress,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
  Upload,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import { listHosts } from "../api/hosts";
import {
  cancelTransfer,
  downloadUrl,
  getTransfer,
  listTransfers,
  uploadTransfer,
  type Transfer,
  type TransferTarget,
} from "../api/transfers";
import { useAuthStore } from "../stores/auth";

interface UploadForm {
  host_ids: string[];
  remote_dir: string;
  remote_name?: string;
  mode: string;
  overwrite: string;
  create_parents: boolean;
}

function bytes(n: number): string {
  if (n < 1024) return `${n}B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`;
  return `${(n / 1024 / 1024).toFixed(1)}MB`;
}

const statusColor: Record<string, string> = {
  queued: "default",
  running: "processing",
  success: "green",
  partial: "orange",
  failed: "red",
  cancelled: "default",
};

export default function Files() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const canOperate = user?.role === "admin" || user?.role === "operator";

  const [file, setFile] = useState<File | null>(null);
  const [activeID, setActiveID] = useState<string | null>(null);
  const [downloadHost, setDownloadHost] = useState<string>();
  const [downloadPath, setDownloadPath] = useState("");
  const [form] = Form.useForm<UploadForm>();

  const { data: hostPage } = useQuery({
    queryKey: ["hosts", "files"],
    queryFn: () => listHosts({ page: 1, page_size: 100 }),
  });
  const hosts = (hostPage?.items ?? []).filter((h) => h.enabled);

  const { data: history } = useQuery({
    queryKey: ["transfers"],
    queryFn: () => listTransfers(1, 50),
    refetchInterval: (query) =>
      (query.state.data?.items ?? []).some((t) => t.status === "queued" || t.status === "running") ? 2000 : false,
  });

  const { data: active } = useQuery({
    queryKey: ["transfer", activeID],
    queryFn: () => getTransfer(activeID!),
    enabled: !!activeID,
    refetchInterval: (query) => {
      const status = query.state.data?.transfer.status;
      return status === "queued" || status === "running" ? 1000 : false;
    },
  });

  const upload = useMutation({
    mutationFn: uploadTransfer,
    onSuccess: (tr) => {
      message.success("已开始分发");
      setActiveID(tr.id);
      setFile(null);
      form.resetFields();
      void qc.invalidateQueries({ queryKey: ["transfers"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "提交失败"),
  });

  const cancel = useMutation({
    mutationFn: cancelTransfer,
    onSuccess: () => {
      message.success("已请求取消");
      void qc.invalidateQueries({ queryKey: ["transfer", activeID] });
      void qc.invalidateQueries({ queryKey: ["transfers"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "取消失败"),
  });

  const doUpload = (values: UploadForm) => {
    if (!file) {
      message.error("请选择文件");
      return;
    }
    const fd = new FormData();
    fd.append("file", file);
    fd.append("host_ids", JSON.stringify(values.host_ids));
    fd.append("remote_dir", values.remote_dir);
    if (values.remote_name) fd.append("remote_name", values.remote_name);
    fd.append("mode", values.mode);
    fd.append("overwrite", values.overwrite);
    fd.append("create_parents", String(values.create_parents));
    upload.mutate(fd);
  };

  const uploadTab = (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Form<UploadForm>
        form={form}
        layout="vertical"
        initialValues={{ mode: "0644", overwrite: "overwrite", create_parents: true }}
        onFinish={doUpload}
      >
        <Form.Item label="文件">
          <Upload.Dragger
            beforeUpload={(f) => {
              setFile(f);
              return false;
            }}
            onRemove={() => setFile(null)}
            maxCount={1}
          >
            <p>点击或拖拽文件到此处</p>
            {file && <p style={{ color: "#1677ff" }}>{file.name}（{bytes(file.size)}）</p>}
          </Upload.Dragger>
        </Form.Item>
        <Form.Item name="host_ids" label="目标主机" rules={[{ required: true, message: "请选择主机" }]}>
          <Select
            mode="multiple"
            optionFilterProp="label"
            placeholder="选择一台或多台主机"
            options={hosts.map((h) => ({ value: h.id, label: `${h.name} (${h.address})` }))}
          />
        </Form.Item>
        <Space wrap align="start">
          <Form.Item name="remote_dir" label="远端目录" rules={[{ required: true, message: "请输入远端目录" }]} style={{ width: 260 }}>
            <Input placeholder="/tmp" />
          </Form.Item>
          <Form.Item name="remote_name" label="远端文件名" style={{ width: 200 }}>
            <Input placeholder="默认使用原文件名" />
          </Form.Item>
          <Form.Item name="mode" label="权限" style={{ width: 120 }}>
            <Input placeholder="0644" />
          </Form.Item>
          <Form.Item name="overwrite" label="覆盖策略" style={{ width: 160 }}>
            <Select
              options={[
                { value: "overwrite", label: "覆盖" },
                { value: "fail_if_exists", label: "已存在则失败" },
              ]}
            />
          </Form.Item>
          <Form.Item name="create_parents" label="自动创建目录" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Space>
        <Button type="primary" htmlType="submit" loading={upload.isPending}>
          开始分发
        </Button>
      </Form>

      {active && (
        <Card
          title={`传输：${active.transfer.file_name}`}
          extra={
            <Space>
              <Tag color={statusColor[active.transfer.status] ?? "default"}>{active.transfer.status}</Tag>
              {(active.transfer.status === "queued" || active.transfer.status === "running") && (
                <Button size="small" danger loading={cancel.isPending} onClick={() => cancel.mutate(active.transfer.id)}>
                  取消
                </Button>
              )}
            </Space>
          }
        >
          <Table<TransferTarget>
            rowKey="id"
            size="small"
            dataSource={active.targets}
            pagination={false}
            columns={[
              { title: "主机", dataIndex: "host_name" },
              { title: "远端路径", dataIndex: "remote_path", ellipsis: true },
              {
                title: "状态",
                dataIndex: "status",
                render: (v: string) => <Tag color={statusColor[v] ?? "default"}>{v}</Tag>,
              },
              {
                title: "进度",
                render: (_, r) => (
                  <Progress
                    percent={
                      active.transfer.size_bytes > 0
                        ? Math.min(100, Math.round((r.bytes_done / active.transfer.size_bytes) * 100))
                        : 0
                    }
                    size="small"
                  />
                ),
              },
              { title: "已传", render: (_, r) => bytes(r.bytes_done) },
              { title: "错误", dataIndex: "error", ellipsis: true },
            ]}
          />
        </Card>
      )}
    </Space>
  );

  const downloadTab = (
    <Card>
      <Typography.Paragraph type="secondary">
        从一台主机下载文件到本地，单文件上限由服务端配置。每次下载都会记录传输历史。
      </Typography.Paragraph>
      <Space wrap align="start">
        <Select
          showSearch
          optionFilterProp="label"
          placeholder="选择主机"
          style={{ width: 260 }}
          value={downloadHost}
          onChange={setDownloadHost}
          options={hosts.map((h) => ({ value: h.id, label: `${h.name} (${h.address})` }))}
        />
        <Input
          placeholder="远端绝对路径，如 /var/log/nginx/access.log"
          style={{ width: 380 }}
          value={downloadPath}
          onChange={(e) => setDownloadPath(e.target.value)}
        />
        <Button
          type="primary"
          disabled={!downloadHost || !downloadPath.startsWith("/")}
          onClick={() => downloadHost && window.open(downloadUrl(downloadHost, downloadPath), "_blank")}
        >
          下载
        </Button>
      </Space>
    </Card>
  );

  const historyTab = (
    <Table<Transfer>
      rowKey="id"
      dataSource={history?.items ?? []}
      size="small"
      onRow={(r) => ({ onClick: () => setActiveID(r.id), style: { cursor: "pointer" } })}
      columns={[
        { title: "文件", dataIndex: "file_name", ellipsis: true },
        {
          title: "方向",
          dataIndex: "direction",
          width: 90,
          render: (v: string) => <Tag>{v === "upload" ? "分发" : "下载"}</Tag>,
        },
        {
          title: "状态",
          dataIndex: "status",
          width: 110,
          render: (v: string) => <Tag color={statusColor[v] ?? "default"}>{v}</Tag>,
        },
        { title: "大小", dataIndex: "size_bytes", width: 100, render: (v: number) => bytes(v) },
        { title: "主机数", dataIndex: "target_total", width: 90 },
        {
          title: "操作人",
          width: 160,
          render: (_, r) => r.created_by_name || r.created_by || "—",
        },
        {
          title: "时间",
          dataIndex: "created_at",
          width: 170,
          render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm:ss"),
        },
      ]}
    />
  );

  return (
    <Tabs
      items={[
        { key: "upload", label: "上传分发", children: canOperate ? uploadTab : <Card>无操作权限</Card> },
        { key: "download", label: "从主机下载", children: canOperate ? downloadTab : <Card>无操作权限</Card> },
        { key: "history", label: "传输历史", children: historyTab },
      ]}
    />
  );
}
