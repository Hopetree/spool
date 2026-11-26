import { useMemo, useState } from "react";
import { App as AntApp, Button, Card, Input, Popconfirm, Select, Space, Switch, Table, Tag, Tree } from "antd";
import type { DataNode } from "antd/es/tree";
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
  type ToolCategory,
  type ToolSummary,
} from "../api/tools";
import ExecuteToolModal from "../components/ExecuteToolModal";
import { useAuthStore } from "../stores/auth";

interface CatNode {
  path: string;
  name: string;
  count: number;
  children: CatNode[];
}

function buildTree(items: ToolCategory[]): CatNode[] {
  const roots: CatNode[] = [];
  for (const item of items) {
    if (!item.path) continue;
    const parts = item.path.split("/");
    let level = roots;
    let acc = "";
    let node: CatNode | undefined;
    for (const part of parts) {
      acc = acc ? `${acc}/${part}` : part;
      node = level.find((n) => n.path === acc);
      if (!node) {
        node = { path: acc, name: part, count: 0, children: [] };
        level.push(node);
      }
      level = node.children;
    }
    if (node) node.count += item.count;
  }
  const aggregate = (node: CatNode): number => {
    node.count += node.children.reduce((sum, child) => sum + aggregate(child), 0);
    return node.count;
  };
  roots.forEach(aggregate);
  roots.sort((a, b) => a.name.localeCompare(b.name));
  return roots;
}

function filterTree(nodes: CatNode[], keyword: string): CatNode[] {
  const q = keyword.trim().toLowerCase();
  if (!q) return nodes;
  const out: CatNode[] = [];
  for (const node of nodes) {
    const children = filterTree(node.children, keyword);
    if (node.name.toLowerCase().includes(q) || node.path.toLowerCase().includes(q) || children.length > 0) {
      out.push({ ...node, children });
    }
  }
  return out;
}

function toTreeData(nodes: CatNode[]): DataNode[] {
  return nodes.map((n) => ({
    key: n.path,
    title: (
      <span>
        {n.name}
        <span style={{ color: "#8c8c8c" }}> ({n.count})</span>
      </span>
    ),
    children: n.children.length ? toTreeData(n.children) : undefined,
  }));
}

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
  const [search, setSearch] = useState("");
  const [execTool, setExecTool] = useState<ToolSummary | null>(null);

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

  const total = categories.reduce((sum, c) => sum + c.count, 0);
  const uncategorized = categories.find((c) => c.path === "")?.count ?? 0;
  const treeData: DataNode[] = useMemo(() => {
    const categoryNodes = toTreeData(filterTree(buildTree(categories), search));
    return [
      {
        key: "all",
        title: (
          <span>
            全部工具<span style={{ color: "#8c8c8c" }}> ({total})</span>
          </span>
        ),
        children: [
          {
            key: "__default__",
            title: (
              <span>
                默认<span style={{ color: "#8c8c8c" }}> ({uncategorized})</span>
              </span>
            ),
          },
          ...categoryNodes,
        ],
      },
    ];
  }, [categories, search, total, uncategorized]);

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
    <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
      <Card size="small" style={{ width: 260, flex: "0 0 260px" }} title="分类">
        <Input.Search allowClear placeholder="搜索分类" style={{ marginBottom: 8 }} onChange={(e) => setSearch(e.target.value)} />
        <Tree
          treeData={treeData}
          defaultExpandAll
          blockNode
          selectedKeys={[selected]}
          onSelect={(keys) => setSelected(String(keys[0] ?? "all"))}
        />
      </Card>

      <div style={{ flex: 1, minWidth: 0 }}>
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
            { title: "名称", render: (_, r) => <Link to={`/tools/${r.id}`}>{r.name}</Link> },
            {
              title: "分类",
              dataIndex: "category",
              render: (v: string) => (v ? <Tag>{v}</Tag> : <Tag>默认</Tag>),
            },
            {
              title: "类型",
              dataIndex: "type",
              width: 90,
              render: (v: string) => <Tag color={v === "python" ? "blue" : "geekblue"}>{v}</Tag>,
            },
            { title: "版本", dataIndex: "version_no", width: 80, render: (v: number) => `v${v}` },
            { title: "参数", dataIndex: "param_count", width: 70 },
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
              width: 160,
              render: (v: number) => dayjs(v).format("YYYY-MM-DD HH:mm"),
            },
            {
              title: "操作",
              render: (_, r) => (
                <Space wrap>
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
                      克隆
                    </Button>
                  )}
                  {isAdmin && (
                    <Popconfirm
                      title="确认删除该工具？"
                      description="已被流程、定时任务或执行历史使用时无法删除，只能停用。"
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
      </div>

      <ExecuteToolModal toolID={execTool?.id ?? null} open={!!execTool} onClose={() => setExecTool(null)} />
    </div>
  );
}
