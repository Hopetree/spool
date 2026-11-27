import { useEffect, useState } from "react";
import { AppstoreOutlined, CodeOutlined, UnorderedListOutlined } from "@ant-design/icons";
import { App as AntApp, Button, Card, Col, Empty, Input, Pagination, Row, Segmented, Select, Space, Switch, Table, Tag } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import { ApiError } from "../api/client";
import {
  copyTool,
  deleteTool,
  listToolCategories,
  listTools,
  setToolEnabled,
  type ToolSummary,
} from "../api/tools";
import ExecuteToolModal from "../components/ExecuteToolModal";
import CategoryTree from "../components/CategoryTree";
import PageHeader from "../components/PageHeader";
import RowActions from "../components/RowActions";
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
  const [selected, setSelected] = useState<string>("all");
  const [view, setView] = useState<"list" | "card">(() =>
    localStorage.getItem("spool.tools.view") === "card" ? "card" : "list",
  );
  const [execTool, setExecTool] = useState<ToolSummary | null>(null);
  const [cardPage, setCardPage] = useState(1);
  const cardPageSize = 9;

  const { data: categories = [] } = useQuery({ queryKey: ["tool-categories"], queryFn: listToolCategories });

  const filtered = {
    category: selected !== "all" && selected !== "__default__" ? selected : undefined,
    uncategorized: selected === "__default__" ? true : undefined,
  };
  const { data, isLoading } = useQuery({
    queryKey: ["tools", { keyword, enabled, selected }],
    queryFn: () =>
      listTools({
        keyword,
        enabled: enabled === "true" ? true : enabled === "false" ? false : undefined,
        ...filtered,
        page: 1,
        page_size: 100,
      }),
  });
  const tools = data?.items ?? [];
  const pagedTools = tools.slice((cardPage - 1) * cardPageSize, cardPage * cardPageSize);

  useEffect(() => {
    setCardPage(1);
  }, [keyword, enabled, selected]);

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
    onSuccess: (created) => {
      message.success("已克隆");
      void qc.invalidateQueries({ queryKey: ["tools"] });
      void qc.invalidateQueries({ queryKey: ["tool-categories"] });
      navigate(`/tools/${created.id}`);
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "克隆失败"),
  });

  const remove = useMutation({
    mutationFn: deleteTool,
    onSuccess: () => {
      message.success("已删除");
      void qc.invalidateQueries({ queryKey: ["tools"] });
      void qc.invalidateQueries({ queryKey: ["tool-categories"] });
    },
    onError: (err) => message.error(err instanceof ApiError ? err.message : "删除失败"),
  });

  return (
    <div>
      <PageHeader
        title="工具列表"
        subtitle={`共 ${tools.length} 个`}
        extra={
          isAdmin && (
            <Button type="primary" onClick={() => navigate("/tools/new")}>
              新建工具
            </Button>
          )
        }
      />

      <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
        <CategoryTree
          allLabel="全部工具"
          items={categories}
          selected={selected}
          onSelect={setSelected}
        />

        <div style={{ flex: 1, minWidth: 0 }}>
          <Card size="small" style={{ marginBottom: 16 }} styles={{ body: { padding: 12 } }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
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
              <Segmented
                value={view}
                onChange={(value) => {
                  const next = value as "list" | "card";
                  setView(next);
                  localStorage.setItem("spool.tools.view", next);
                }}
                options={[
                  { value: "list", label: "列表", icon: <UnorderedListOutlined /> },
                  { value: "card", label: "卡片", icon: <AppstoreOutlined /> },
                ]}
              />
            </div>
          </Card>

          {view === "list" ? (
          <Table<ToolSummary>
          rowKey="id"
          size="small"
          loading={isLoading}
          dataSource={tools}
          columns={[
            {
              title: "名称",
              width: 180,
              ellipsis: true,
              render: (_, r) => <Link to={`/tools/${r.id}`}>{r.name}</Link>,
            },
            {
              title: "分类",
              dataIndex: "category",
              width: 140,
              ellipsis: true,
              render: (v: string) => (v ? <Tag>{v}</Tag> : <Tag>默认</Tag>),
            },
            {
              title: "类型",
              dataIndex: "type",
              width: 80,
              render: (v: string) => <Tag color={v === "python" ? "blue" : "geekblue"}>{v}</Tag>,
            },
            { title: "版本", dataIndex: "version_no", width: 70, render: (v: number) => `v${v}` },
            {
              title: "状态",
              dataIndex: "enabled",
              width: 80,
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
              width: 150,
              render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm"),
            },
            {
              title: "操作",
              width: 180,
              render: (_, r) => (
                <RowActions
                  inline={2}
                  actions={[
                    ...(canRun ? [{ key: "run", label: "执行", onClick: () => setExecTool(r) }] : []),
                    ...(isAdmin
                      ? [{ key: "edit", label: "编辑", onClick: () => navigate(`/tools/${r.id}/edit`) }]
                      : []),
                    ...(isAdmin
                      ? [
                          {
                            key: "copy",
                            label: "克隆",
                            loading: copy.isPending,
                            onClick: () => copy.mutate(r.id),
                          },
                          {
                            key: "delete",
                            label: "删除",
                            danger: true,
                            onClick: () => remove.mutate(r.id),
                            confirm: {
                              title: "确认删除该工具？",
                              description: "已被流程、定时任务或执行历史使用时无法删除，只能停用。",
                            },
                          },
                        ]
                      : []),
                  ]}
                />
              ),
            },
          ]}
          />
          ) : (
            <Row gutter={[12, 12]}>
              {pagedTools.map((tool) => (
                <Col key={tool.id} xs={24} sm={12} xl={8}>
                  <Card
                    size="small"
                    hoverable
                    styles={{
                      body: {
                        padding: 16,
                        display: "flex",
                        flexDirection: "column",
                        height: "100%",
                        boxSizing: "border-box",
                      },
                    }}
                  >
                    <div style={{ display: "flex", alignItems: "flex-start", gap: 12 }}>
                      <span
                        style={{
                          width: 38,
                          height: 38,
                          borderRadius: 10,
                          flex: "none",
                          display: "inline-flex",
                          alignItems: "center",
                          justifyContent: "center",
                          fontSize: 18,
                          color: tool.type === "python" ? "#1677ff" : "#52c41a",
                          background: tool.type === "python" ? "#e6f4ff" : "#f6ffed",
                        }}
                      >
                        <CodeOutlined />
                      </span>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <Link to={`/tools/${tool.id}`} style={{ fontWeight: 600 }}>
                          {tool.name}
                        </Link>
                        <div
                          style={{
                            fontSize: 12,
                            color: "#8c8c8c",
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          v{tool.version_no} · {tool.category || "默认"}
                        </div>
                      </div>
                      {!tool.enabled && <Tag>停用</Tag>}
                    </div>
                    <div
                      style={{
                        fontSize: 12,
                        color: "#595959",
                        marginTop: 10,
                        lineHeight: 1.6,
                        height: 38,
                        overflow: "hidden",
                      }}
                    >
                      {tool.description || "—"}
                    </div>
                    <div
                      style={{
                        display: "flex",
                        justifyContent: "space-between",
                        alignItems: "center",
                        marginTop: 12,
                      }}
                    >
                      <Tag color={tool.type === "python" ? "blue" : "geekblue"}>{tool.type}</Tag>
                      {canRun && (
                        <Button size="small" onClick={() => setExecTool(tool)}>
                          执行
                        </Button>
                      )}
                    </div>
                  </Card>
                </Col>
              ))}
              {tools.length === 0 && !isLoading && (
                <Col span={24}>
                  <Empty description="没有匹配的工具" />
                </Col>
              )}
            </Row>
          )}
          {view === "card" && tools.length > cardPageSize && (
            <div style={{ display: "flex", justifyContent: "flex-end", marginTop: 12 }}>
              <Pagination
                size="small"
                current={cardPage}
                pageSize={cardPageSize}
                total={tools.length}
                showSizeChanger={false}
                onChange={setCardPage}
              />
            </div>
          )}
        </div>
      </div>

      <ExecuteToolModal toolID={execTool?.id ?? null} open={!!execTool} onClose={() => setExecTool(null)} />
    </div>
  );
}
