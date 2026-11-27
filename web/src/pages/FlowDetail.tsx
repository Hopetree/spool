import { useState } from "react";
import {
  App as AntApp,
  Button,
  Card,
  Descriptions,
  Dropdown,
  Space,
  Tag,
} from "antd";
import { DownOutlined } from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { copyFlow, deleteFlow, getFlow, setFlowEnabled } from "../api/flows";
import ExecuteFlowModal from "../components/ExecuteFlowModal";
import ExecutionTable from "../components/ExecutionTable";
import FlowGraph from "../components/FlowGraph";
import PageHeader from "../components/PageHeader";
import { useAuthStore } from "../stores/auth";

export default function FlowDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { message, modal } = AntApp.useApp();
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
      <PageHeader
        back={{ label: "返回列表", onClick: () => navigate("/flows") }}
        title={detail?.name ?? "流程"}
        subtitle={detail ? `${detail.nodes.length} 个节点 · ${detail.edge_count} 条连线` : undefined}
        crumbs={[{ title: "流程", to: "/flows" }, { title: "流程详情" }]}
        extra={
          <>
            {canRun && (
              <Button type="primary" onClick={() => setExecOpen(true)}>
                执行
              </Button>
            )}
            {isAdmin && <Button onClick={() => navigate(`/flows/${id}/edit`)}>编辑</Button>}
            {isAdmin && (
              <Dropdown
                trigger={["click"]}
                menu={{
                  items: [
                    { key: "copy", label: "克隆" },
                    { key: "enabled", label: detail?.enabled ? "停用" : "启用" },
                    { type: "divider" },
                    { key: "delete", label: "删除流程", danger: true },
                  ],
                  onClick: ({ key }) => {
                    if (key === "copy") clone.mutate();
                    if (key === "enabled") toggle.mutate(!detail?.enabled);
                    if (key === "delete") {
                      modal.confirm({
                        title: "确认删除该流程？",
                        okText: "删除",
                        okButtonProps: { danger: true },
                        onOk: () => remove.mutate(),
                      });
                    }
                  },
                }}
              >
                <Button>
                  更多 <DownOutlined />
                </Button>
              </Dropdown>
            )}
          </>
        }
      />

      <Card loading={isLoading} style={{ marginBottom: 16 }}>
        {detail && (
          <Descriptions
            column={3}
            size="small"
            items={[
              { key: "category", label: "分类", children: detail.category || "默认" },
              { key: "desc", label: "描述", children: detail.description || "—" },
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

      <Card title="流程图" style={{ marginBottom: 16 }} loading={isLoading}>
        {detail && <FlowGraph nodes={detail.nodes} edges={detail.edges} height={480} />}
      </Card>

      <Card title="执行历史">{id && <ExecutionTable kind="flow" refID={id} showFilters />}</Card>

      <ExecuteFlowModal flowID={id ?? null} open={execOpen} onClose={() => setExecOpen(false)} />
    </div>
  );
}
