import { useState } from "react";
import { App as AntApp, Button, Input, Popconfirm, Select, Space, Switch, Table, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import { copyTool, deleteTool, listTools, setToolEnabled, type ToolSummary } from "../api/tools";
import ExecuteToolModal from "../components/ExecuteToolModal";
import { useAuthStore } from "../stores/auth";

export default function Tools() {
  const qc = useQueryClient();
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const canRun = isAdmin || user?.role === "operator";

  const [keyword, setKeyword] = useState("");
  const [enabled, setEnabled] = useState<string>();
  const [execTool, setExecTool] = useState<ToolSummary | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["tools", { keyword, enabled }],
    queryFn: () =>
      listTools({
        keyword,
        enabled: enabled === "true" ? true : enabled === "false" ? false : undefined,
        page: 1,
        page_size: 100,
      }),
  });
  const tools = data?.items ?? [];

  const toggle = useMutation({
    mutationFn: (v: { id: string; value: boolean }) => setToolEnabled(v.id, v.value),
    onSuccess: () => {
      message.success("已更新");
      void qc.invalidateQueries({ queryKey: ["tools"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "更新失败"),
  });

  const copy = useMutation({
    mutationFn: copyTool,
    onSuccess: () => {
      message.success("已复制");
      void qc.invalidateQueries({ queryKey: ["tools"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "复制失败"),
  });

  const remove = useMutation({
    mutationFn: deleteTool,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["tools"] });
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
          <Button type="primary" onClick={() => navigate("/tools/new")}>
            新建工具
          </Button>
        )}
      </Space>

      <Table<ToolSummary>
        rowKey="id"
        loading={isLoading}
        dataSource={tools}
        columns={[
          {
            title: "名称",
            render: (_, r) => <Link to={`/tools/${r.id}`}>{r.name}</Link>,
          },
          {
            title: "类型",
            dataIndex: "type",
            render: (v: string) => <Tag color={v === "python" ? "blue" : "geekblue"}>{v}</Tag>,
          },
          { title: "版本", dataIndex: "version_no", render: (v: number) => `v${v}` },
          { title: "参数", dataIndex: "param_count" },
          {
            title: "状态",
            dataIndex: "enabled",
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
            render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm"),
          },
          {
            title: "操作",
            render: (_, r) => (
              <Space>
                {canRun && (
                  <Button size="small" onClick={() => setExecTool(r)}>
                    执行
                  </Button>
                )}
                {isAdmin && (
                  <Button size="small" onClick={() => navigate(`/tools/${r.id}`)}>
                    编辑
                  </Button>
                )}
                {isAdmin && (
                  <Button size="small" loading={copy.isPending} onClick={() => copy.mutate(r.id)}>
                    复制
                  </Button>
                )}
                {isAdmin && (
                  <Popconfirm
                    title="确认删除该工具？"
                    description="已有执行历史的工具无法删除，只能停用。"
                    onConfirm={() => remove.mutate(r.id)}
                  >
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

      <ExecuteToolModal toolID={execTool?.id ?? null} open={!!execTool} onClose={() => setExecTool(null)} />
    </div>
  );
}
