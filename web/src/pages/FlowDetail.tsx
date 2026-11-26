import { useState } from "react";
import {
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import {
  copyFlow,
  deleteFlow,
  getFlow,
  setFlowEnabled,
  type FlowStage,
} from "../api/flows";
import ExecuteFlowModal from "../components/ExecuteFlowModal";
import ExecutionTable from "../components/ExecutionTable";
import { useAuthStore } from "../stores/auth";

const policyLabel: Record<string, string> = { abort: "失败即中止后续阶段", continue: "失败后继续后续阶段" };

export default function FlowDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const canRun = isAdmin || user?.role === "operator";
  const [execOpen, setExecOpen] = useState(false);

  const { data: detail, isLoading } = useQuery({
    queryKey: ["flow", id],
    queryFn: () => getFlow(id!),
    enabled: !!id,
  });

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["flows"] });
    void qc.invalidateQueries({ queryKey: ["flow", id] });
  };

  const clone = useMutation({
    mutationFn: () => copyFlow(id!),
    onSuccess: (created) => {
      message.success("已克隆");
      invalidate();
      navigate(`/flows/${created.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "克隆失败"),
  });

  const toggle = useMutation({
    mutationFn: (value: boolean) => setFlowEnabled(id!, value),
    onSuccess: () => {
      message.success("已更新");
      invalidate();
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "更新失败"),
  });

  const remove = useMutation({
    mutationFn: () => deleteFlow(id!),
    onSuccess: () => {
      message.success("流程已删除");
      invalidate();
      navigate("/flows");
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Button onClick={() => navigate("/flows")}>返回列表</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {detail?.name ?? "流程"}
        </Typography.Title>
        {detail && (
          <Typography.Text type="secondary">
            {detail.stage_count} 个阶段 · {detail.step_count} 个步骤
          </Typography.Text>
        )}
        {canRun && <Button onClick={() => setExecOpen(true)}>执行</Button>}
        {isAdmin && <Button onClick={() => navigate(`/flows/${id}/edit`)}>编辑</Button>}
        {isAdmin && (
          <Button loading={clone.isPending} onClick={() => clone.mutate()}>
            克隆
          </Button>
        )}
        {isAdmin && (
          <Space size={4}>
            <span>启用</span>
            <Switch checked={detail?.enabled} loading={toggle.isPending} onChange={(v) => toggle.mutate(v)} />
          </Space>
        )}
        {isAdmin && (
          <Popconfirm title="确认删除该流程？" onConfirm={() => remove.mutate()}>
            <Button danger loading={remove.isPending}>
              删除流程
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
              { key: "policy", label: "失败策略", children: policyLabel[detail.failure_policy] ?? detail.failure_policy },
              { key: "duration", label: "单机最大时长", children: `${detail.max_duration_sec}s` },
              {
                key: "params",
                label: "流程参数",
                children:
                  detail.params.length > 0 ? (
                    <Space wrap>
                      {detail.params.map((p) => (
                        <Tag key={p.name}>
                          {p.name}
                          {p.sensitive ? "（敏感）" : ""}
                        </Tag>
                      ))}
                    </Space>
                  ) : (
                    "—"
                  ),
              },
            ]}
          />
        )}
      </Card>

      <Card title="阶段与步骤" style={{ marginBottom: 16 }} loading={isLoading}>
        <Table<FlowStage>
          rowKey="id"
          dataSource={detail?.stages ?? []}
          pagination={false}
          size="small"
          columns={[
            { title: "序号", dataIndex: "index", width: 80 },
            { title: "阶段", dataIndex: "name" },
            {
              title: "始终执行",
              dataIndex: "always_run",
              width: 100,
              render: (v: boolean) => (v ? <Tag color="orange">是</Tag> : "否"),
            },
            {
              title: "步骤",
              render: (_, r) => (
                <Space wrap>
                  {r.steps.map((s) => (
                    <Tag key={s.id}>{s.name}</Tag>
                  ))}
                </Space>
              ),
            },
            { title: "步骤数", width: 90, render: (_, r) => r.steps.length },
          ]}
        />
      </Card>

      <Card title="执行历史">
        {id && <ExecutionTable kind="flow" refID={id} showFilters />}
      </Card>

      <ExecuteFlowModal flowID={id ?? null} open={execOpen} onClose={() => setExecOpen(false)} />
    </div>
  );
}
