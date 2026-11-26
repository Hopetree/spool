import { useState } from "react";
import { App as AntApp, Button, Input, Popconfirm, Select, Space, Switch, Table, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import { copyFlow, deleteFlow, listFlows, setFlowEnabled, type FlowSummary } from "../api/flows";
import ExecuteFlowModal from "../components/ExecuteFlowModal";
import { useAuthStore } from "../stores/auth";

const policyLabel: Record<string, string> = { abort: "失败即中止", continue: "失败后继续" };

export default function Flows() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const canRun = isAdmin || user?.role === "operator";

  const [keyword, setKeyword] = useState("");
  const [enabled, setEnabled] = useState<string>();
  const [execFlow, setExecFlow] = useState<FlowSummary | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["flows", { keyword, enabled }],
    queryFn: () =>
      listFlows({ keyword, enabled: enabled === "true" ? true : enabled === "false" ? false : undefined }),
  });
  const flows = data?.items ?? [];

  const toggle = useMutation({
    mutationFn: (v: { id: string; value: boolean }) => setFlowEnabled(v.id, v.value),
    onSuccess: () => {
      message.success("已更新");
      void qc.invalidateQueries({ queryKey: ["flows"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "更新失败"),
  });

  const copy = useMutation({
    mutationFn: copyFlow,
    onSuccess: () => {
      message.success("已复制");
      void qc.invalidateQueries({ queryKey: ["flows"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "复制失败"),
  });

  const remove = useMutation({
    mutationFn: deleteFlow,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["flows"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Input.Search allowClear placeholder="名称或描述" style={{ width: 220 }} onSearch={setKeyword} />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 110 }}
          value={enabled}
          onChange={setEnabled}
          options={[
            { value: "true", label: "启用" },
            { value: "false", label: "停用" },
          ]}
        />
        {isAdmin && (
          <Button type="primary" onClick={() => navigate("/flows/new")}>
            新建流程
          </Button>
        )}
      </Space>

      <Table<FlowSummary>
        rowKey="id"
        loading={isLoading}
        dataSource={flows}
        columns={[
          { title: "名称", render: (_, r) => <Link to={`/flows/${r.id}`}>{r.name}</Link> },
          { title: "阶段", dataIndex: "stage_count", width: 80 },
          { title: "步骤", dataIndex: "step_count", width: 80 },
          {
            title: "失败策略",
            dataIndex: "failure_policy",
            width: 130,
            render: (v: string) => <Tag>{policyLabel[v] ?? v}</Tag>,
          },
          {
            title: "状态",
            dataIndex: "enabled",
            width: 90,
            render: (v: boolean, r) => (
              <Switch
                checked={v}
                disabled={!isAdmin}
                loading={toggle.isPending && toggle.variables?.id === r.id}
                onChange={(value) => toggle.mutate({ id: r.id, value })}
              />
            ),
          },
          {
            title: "更新时间",
            dataIndex: "updated_at",
            width: 170,
            render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm"),
          },
          {
            title: "操作",
            render: (_, r) => (
              <Space>
                {canRun && (
                  <Button size="small" onClick={() => setExecFlow(r)}>
                    执行
                  </Button>
                )}
                {isAdmin && (
                  <Button size="small" onClick={() => navigate(`/flows/${r.id}/edit`)}>
                    编辑
                  </Button>
                )}
                {isAdmin && (
                  <Button size="small" loading={copy.isPending} onClick={() => copy.mutate(r.id)}>
                    复制
                  </Button>
                )}
                {isAdmin && (
                  <Popconfirm title="确认删除该流程？" onConfirm={() => remove.mutate(r.id)}>
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

      <ExecuteFlowModal flowID={execFlow?.id ?? null} open={!!execFlow} onClose={() => setExecFlow(null)} />
    </div>
  );
}
