import { useEffect, useMemo, useState, type Key } from "react";
import { Card, Input, Tree } from "antd";
import type { DataNode } from "antd/es/tree";

export interface CategoryItem {
  path: string;
  count: number;
}

interface CatNode {
  path: string;
  name: string;
  count: number;
  children: CatNode[];
}

function buildTree(items: CategoryItem[]): CatNode[] {
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

/**
 * Slash-separated category tree used by the tool and flow libraries.
 * `selected` is "all", "__default__" or a category path.
 */
export default function CategoryTree({
  title = "分类",
  allLabel,
  items,
  selected,
  onSelect,
  width = 260,
}: {
  title?: string;
  allLabel: string;
  items: CategoryItem[];
  selected: string;
  onSelect: (key: string) => void;
  width?: number;
}) {
  const [search, setSearch] = useState("");
  const [expandedKeys, setExpandedKeys] = useState<Key[]>([]);
  const total = items.reduce((sum, c) => sum + c.count, 0);
  const uncategorized = items.find((c) => c.path === "")?.count ?? 0;

  const treeData: DataNode[] = useMemo(() => {
    const categoryNodes = toTreeData(filterTree(buildTree(items), search));
    return [
      {
        key: "all",
        title: (
          <span>
            {allLabel}
            <span style={{ color: "#8c8c8c" }}> ({total})</span>
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
  }, [items, search, total, uncategorized, allLabel]);

  // Categories load asynchronously, so defaultExpandAll cannot cover them.
  useEffect(() => {
    const keys: Key[] = [];
    const walk = (nodes: DataNode[]) => {
      for (const node of nodes) {
        if (node.children?.length) {
          keys.push(node.key);
          walk(node.children as DataNode[]);
        }
      }
    };
    walk(treeData);
    setExpandedKeys(keys);
  }, [treeData]);

  return (
    <Card size="small" style={{ width, flex: `0 0 ${width}px` }} title={title}>
      <Input.Search
        allowClear
        placeholder="搜索分类"
        style={{ marginBottom: 8 }}
        onChange={(e) => setSearch(e.target.value)}
      />
      <Tree
        treeData={treeData}
        expandedKeys={expandedKeys}
        onExpand={(keys) => setExpandedKeys(keys)}
        blockNode
        selectedKeys={[selected]}
        onSelect={(keys) => onSelect(String(keys[0] ?? "all"))}
      />
    </Card>
  );
}
