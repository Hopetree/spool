import { useState } from "react";
import { App as AntApp, Button, Card, Input, Select, Space, Switch, Table } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import { copyFlow, deleteFlow, listFlows, setFlowEnabled, type FlowSummary } from "../api/flows";
import ExecuteFlowModal from "../components/ExecuteFlowModal";
import PageHeader from "../components/PageHeader";
import RowActions from "../components/RowActions";
import { useAuthStore } from "../stores/auth";

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
      <PageHeader
        title="流程列表"
        subtitle={`共 ${flows.length} 个`}
        extra={
          isAdmin && (
            <Button type="primary" onClick={() => navigate("/flows/new")}>
              新建流程
            </Button>
          )
        }
      />

      <Card size="small" style={{ marginBottom: 16 }} styles={{ body: { padding: 12 } }}>
        <Space wrap>
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
        </Space>
      </Card>

      <Table<FlowSummary>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={flows}
        columns={[
          { title: "名称", render: (_, r) => <Link to={`/flows/${r.id}`}>{r.name}</Link> },
          { title: "工具节点", dataIndex: "node_count", width: 90 },
          { title: "连线", dataIndex: "edge_count", width: 80 },
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
            width: 160,
            render: (_, r) => (
              <RowActions
                inline={2}
                actions={[
                  ...(canRun ? [{ key: "run", label: "执行", onClick: () => setExecFlow(r) }] : []),
                  ...(isAdmin
                    ? [{ key: "edit", label: "编辑", onClick: () => navigate(`/flows/${r.id}/edit`) }]
                    : []),
                  ...(isAdmin
                    ? [
                        { key: "copy", label: "复制", onClick: () => copy.mutate(r.id) },
                        {
                          key: "delete",
                          label: "删除",
                          danger: true,
                          onClick: () => remove.mutate(r.id),
                          confirm: { title: "确认删除该流程？" },
                        },
                      ]
                    : []),
                ]}
              />
            ),
          },
        ]}
      />

      <ExecuteFlowModal flowID={execFlow?.id ?? null} open={!!execFlow} onClose={() => setExecFlow(null)} />
    </div>
  );
}
