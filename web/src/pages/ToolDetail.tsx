import { useMemo, useState } from "react";
import {
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import dayjs from "dayjs";
import { diffLines } from "diff";
import { ApiError } from "../api/client";
import {
  copyTool,
  deleteTool,
  deleteToolVersion,
  getTool,
  getToolReferences,
  listVersions,
  rollbackTool,
  setToolEnabled,
  type ToolReferenceFlow,
  type ToolParam,
  type ToolReferenceSchedule,
  type ToolVersion,
} from "../api/tools";
import CodeEditor from "../components/CodeEditor";
import ExecuteToolModal from "../components/ExecuteToolModal";
import ExecutionTable from "../components/ExecutionTable";
import { useAuthStore } from "../stores/auth";

export default function ToolDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const canRun = isAdmin || user?.role === "operator";

  const [execOpen, setExecOpen] = useState(false);
  const [diffOpen, setDiffOpen] = useState(false);
  const [fromNo, setFromNo] = useState<number>();
  const [toNo, setToNo] = useState<number>();

  const { data: detail, isLoading } = useQuery({
    queryKey: ["tool", id],
    queryFn: () => getTool(id!),
    enabled: !!id,
  });
  const { data: versions = [] } = useQuery({
    queryKey: ["tool-versions", id],
    queryFn: () => listVersions(id!),
    enabled: !!id,
  });
  const { data: refs, isLoading: refsLoading } = useQuery({
    queryKey: ["tool-references", id],
    queryFn: () => getToolReferences(id!),
    enabled: !!id,
  });

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["tools"] });
    void qc.invalidateQueries({ queryKey: ["tool-categories"] });
  };

  const rollback = useMutation({
    mutationFn: (no: number) => rollbackTool(id!, no),
    onSuccess: () => {
      message.success("已回滚并生成新版本");
      void qc.invalidateQueries({ queryKey: ["tool", id] });
      void qc.invalidateQueries({ queryKey: ["tool-versions", id] });
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "回滚失败"),
  });

  const removeVersion = useMutation({
    mutationFn: (no: number) => deleteToolVersion(id!, no),
    onSuccess: () => {
      message.success("版本已删除");
      void qc.invalidateQueries({ queryKey: ["tool-versions", id] });
      void qc.invalidateQueries({ queryKey: ["tool", id] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除版本失败"),
  });

  const clone = useMutation({
    mutationFn: () => copyTool(id!),
    onSuccess: (created) => {
      message.success("已克隆");
      invalidate();
      navigate(`/tools/${created.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "克隆失败"),
  });

  const removeTool = useMutation({
    mutationFn: () => deleteTool(id!),
    onSuccess: () => {
      message.success("工具已删除");
      invalidate();
      navigate("/tools");
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  const toggleEnabled = useMutation({
    mutationFn: (value: boolean) => setToolEnabled(id!, value),
    onSuccess: () => {
      message.success("已更新");
      void qc.invalidateQueries({ queryKey: ["tool", id] });
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "更新失败"),
  });

  const fromV = versions.find((v) => v.version_no === fromNo);
  const toV = versions.find((v) => v.version_no === toNo);
  const diffParts = useMemo(() => (fromV && toV ? diffLines(fromV.content, toV.content) : []), [fromV, toV]);

  const openDiff = (a?: number, b?: number) => {
    if (a && b) {
      setFromNo(a);
      setToNo(b);
    } else if (versions.length >= 2) {
      setFromNo(versions[1].version_no);
      setToNo(versions[0].version_no);
    } else if (versions.length === 1) {
      setFromNo(versions[0].version_no);
      setToNo(versions[0].version_no);
    }
    setDiffOpen(true);
  };

  const versionsTab = (
    <Table<ToolVersion>
      rowKey="id"
      dataSource={versions}
      pagination={false}
      columns={[
        {
          title: "版本",
          dataIndex: "version_no",
          width: 110,
          render: (v: number) => (
            <Space size={4}>
              v{v}
              {v === detail?.version_no && <Tag color="green">当前</Tag>}
            </Space>
          ),
        },
        { title: "说明", dataIndex: "change_note" },
        {
          title: "创建人",
          width: 160,
          render: (_, r) => r.created_by_name || r.created_by || "—",
        },
        {
          title: "创建时间",
          dataIndex: "created_at",
          width: 170,
          render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm"),
        },
        {
          title: "操作",
          width: 240,
          render: (_, r) => (
            <Space>
              <Button size="small" onClick={() => openDiff(r.version_no === 1 ? 1 : r.version_no - 1, r.version_no)}>
                对比
              </Button>
              {isAdmin && (
                <Popconfirm
                  title={`回滚到 v${r.version_no}？`}
                  onConfirm={() => rollback.mutate(r.version_no)}
                  disabled={r.version_no === detail?.version_no}
                >
                  <Button size="small" disabled={r.version_no === detail?.version_no}>
                    回滚
                  </Button>
                </Popconfirm>
              )}
              {isAdmin && (
                <Popconfirm
                  title={`删除版本 v${r.version_no}？`}
                  description="当前版本或有执行历史的版本不能删除。"
                  onConfirm={() => removeVersion.mutate(r.version_no)}
                  disabled={r.version_no === detail?.version_no}
                >
                  <Button size="small" danger disabled={r.version_no === detail?.version_no}>
                    删除
                  </Button>
                </Popconfirm>
              )}
            </Space>
          ),
        },
      ]}
    />
  );

  const referencesTab = (
    <Space direction="vertical" size={16} style={{ width: "100%" }}>
      <Card title="流程引用" loading={refsLoading}>
        <Table<ToolReferenceFlow>
          rowKey={(r) => `${r.flow_id}-${r.stage_name}-${r.step_name}`}
          dataSource={refs?.flows ?? []}
          pagination={false}
          size="small"
          columns={[
            {
              title: "流程",
              dataIndex: "flow_name",
              render: (v: string, r: ToolReferenceFlow) => (
                <Space>
                  <a onClick={() => navigate(`/flows/${r.flow_id}`)}>{v}</a>
                  {r.flow_enabled ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>}
                </Space>
              ),
            },
            { title: "阶段", dataIndex: "stage_name" },
            { title: "步骤", dataIndex: "step_name" },
          ]}
        />
      </Card>
      <Card title="定时任务引用" loading={refsLoading}>
        <Table<ToolReferenceSchedule>
          rowKey="id"
          dataSource={refs?.schedules ?? []}
          pagination={false}
          size="small"
          columns={[
            {
              title: "定时任务",
              dataIndex: "name",
              render: (v: string, r: ToolReferenceSchedule) => (
                <Space>
                  <a onClick={() => navigate(`/schedules/${r.id}`)}>{v}</a>
                  {r.enabled ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>}
                </Space>
              ),
            },
            { title: "cron", dataIndex: "cron_expr" },
          ]}
        />
      </Card>
    </Space>
  );

  const scriptTab = detail ? (
        <Card title="脚本与参数">
          <Typography.Text type="secondary">脚本内容</Typography.Text>
          <div style={{ marginTop: 8, marginBottom: 16 }}>
            <CodeEditor
              language={detail.version.type}
              value={detail.version.content}
              readOnly
              height="280px"
            />
          </div>
          <Typography.Text type="secondary">准备命令</Typography.Text>
          <pre
            style={{
              margin: "8px 0 16px",
              padding: 8,
              background: "#fafafa",
              borderRadius: 4,
              fontSize: 12,
              whiteSpace: "pre-wrap",
            }}
          >
            {detail.version.prepare_command || "（无）"}
          </pre>
          <Typography.Text type="secondary">参数</Typography.Text>
          <Table<ToolParam>
            rowKey="name"
            dataSource={detail.version.params}
            pagination={false}
            size="small"
            style={{ marginTop: 8 }}
            columns={[
              { title: "变量名", dataIndex: "name" },
              { title: "显示名", dataIndex: "label" },
              { title: "类型", dataIndex: "type", width: 90 },
              {
                title: "必填",
                dataIndex: "required",
                width: 80,
                render: (v: boolean) => (v ? "是" : "否"),
              },
              { title: "默认值", dataIndex: "default" },
              {
                title: "候选项",
                render: (_, p) => (p.options ?? []).join(", ") || "—",
              },
              {
                title: "敏感",
                dataIndex: "sensitive",
                width: 80,
                render: (v: boolean) => (v ? <Tag color="red">是</Tag> : "否"),
              },
              { title: "说明", dataIndex: "description", ellipsis: true },
            ]}
          />
        </Card>
  ) : null;

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate("/tools")}>返回列表</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {detail?.name ?? "工具"}
        </Typography.Title>
        {detail && (
          <Typography.Text type="secondary">
            {detail.category || "默认"} · v{detail.version_no}
          </Typography.Text>
        )}
        {canRun && <Button onClick={() => setExecOpen(true)}>执行</Button>}
        {isAdmin && <Button onClick={() => navigate(`/tools/${id}/edit`)}>编辑</Button>}
        {isAdmin && (
          <Button loading={clone.isPending} onClick={() => clone.mutate()}>
            克隆
          </Button>
        )}
        {isAdmin && (
          <Space size={4}>
            <span>启用</span>
            <Switch
              checked={detail?.enabled}
              loading={toggleEnabled.isPending}
              onChange={(v) => toggleEnabled.mutate(v)}
            />
          </Space>
        )}
        {isAdmin && (
          <Popconfirm title="确认删除该工具？" onConfirm={() => removeTool.mutate()}>
            <Button danger loading={removeTool.isPending}>
              删除工具
            </Button>
          </Popconfirm>
        )}
      </Space>

      <Card loading={isLoading} style={{ marginBottom: 16 }}>
        {detail && (
          <Descriptions
            column={3}
            size="small"
            items={[
              { key: "desc", label: "描述", children: detail.description || "—" },
              { key: "type", label: "类型", children: detail.version.type },
              { key: "interp", label: "解释器", children: detail.version.interpreter },
              { key: "timeout", label: "默认超时", children: `${detail.version.timeout_sec}s` },
              { key: "sudo", label: "sudo -n", children: detail.version.sudo ? "是" : "否" },
              { key: "params", label: "参数数量", children: detail.version.params.length },
              {
                key: "updated",
                label: "更新时间",
                children: dayjs(detail.updated_at).format("YYYY-MM-DD HH:mm"),
              },
            ]}
          />
        )}
      </Card>

      <Tabs
        defaultActiveKey="history"
        items={[
          {
            key: "history",
            label: "执行历史",
            children: id ? <ExecutionTable kind="tool" refID={id} showFilters /> : <Card>—</Card>,
          },
          { key: "versions", label: "版本列表", children: versionsTab },
          { key: "references", label: "引用对象", children: referencesTab },
          { key: "script", label: "脚本与参数", children: scriptTab },
        ]}
      />

      <Modal open={diffOpen} onCancel={() => setDiffOpen(false)} footer={null} width={900} title="版本对比">
        <Space style={{ marginBottom: 12 }}>
          <Select
            style={{ width: 160 }}
            placeholder="基准版本"
            value={fromNo}
            onChange={setFromNo}
            options={versions.map((v) => ({ value: v.version_no, label: `v${v.version_no}` }))}
          />
          <span>→</span>
          <Select
            style={{ width: 160 }}
            placeholder="目标版本"
            value={toNo}
            onChange={setToNo}
            options={versions.map((v) => ({ value: v.version_no, label: `v${v.version_no}` }))}
          />
        </Space>
        <div
          style={{
            maxHeight: 520,
            overflow: "auto",
            fontFamily: "Menlo, Consolas, monospace",
            fontSize: 12,
            whiteSpace: "pre-wrap",
          }}
        >
          {diffParts.length === 0 ? (
            <Typography.Text type="secondary">请选择两个版本</Typography.Text>
          ) : (
            diffParts.map((part, idx) => (
              <div
                key={idx}
                style={{
                  background: part.added ? "#e6ffec" : part.removed ? "#ffebe9" : undefined,
                  color: part.added ? "#116329" : part.removed ? "#a40e26" : undefined,
                  padding: "2px 6px",
                }}
              >
                {part.value
                  .split("\n")
                  .filter((line, i, arr) => !(i === arr.length - 1 && line === ""))
                  .map((line) => `${part.added ? "+" : part.removed ? "-" : " "}${line}`)
                  .join("\n")}
              </div>
            ))
          )}
        </div>
      </Modal>

      <ExecuteToolModal toolID={id ?? null} open={execOpen} onClose={() => setExecOpen(false)} />
    </div>
  );
}
