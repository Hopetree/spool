import { useMemo } from "react";
import { Background, Controls, MarkerType, ReactFlow } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { conditionLabel, type EdgeCondition } from "../api/flows";
import type { ExecutionGraphData } from "../api/executions";
import { flowNodeTypes, makeRFEdge, type FlowRFNode } from "./FlowGraphParts";

interface Position {
  x: number;
  y: number;
}

/** Old snapshots have no layout; fall back to a simple layered layout. */
function nodePositions(graph: ExecutionGraphData): Map<string, Position> {
  const out = new Map<string, Position>();
  if (graph.nodes.some((n) => n.x !== 0 || n.y !== 0) || graph.nodes.length === 0) {
    graph.nodes.forEach((n) => out.set(n.id, { x: n.x, y: n.y }));
    return out;
  }
  const next = new Map<string, string[]>();
  const indegree = new Map<string, number>();
  graph.nodes.forEach((n) => {
    next.set(n.id, []);
    indegree.set(n.id, 0);
  });
  graph.edges.forEach((e) => {
    next.get(e.from)?.push(e.to);
    indegree.set(e.to, (indegree.get(e.to) ?? 0) + 1);
  });
  const layer = new Map<string, number>();
  const queue = graph.nodes.filter((n) => (indegree.get(n.id) ?? 0) === 0).map((n) => n.id);
  if (queue.length === 0) queue.push(graph.nodes[0].id);
  queue.forEach((id) => layer.set(id, 0));
  while (queue.length > 0) {
    const id = queue.shift()!;
    for (const to of next.get(id) ?? []) {
      if (!layer.has(to)) {
        layer.set(to, (layer.get(id) ?? 0) + 1);
        queue.push(to);
      }
    }
  }
  const rows = new Map<number, number>();
  for (const n of graph.nodes) {
    const l = layer.get(n.id) ?? 0;
    const row = rows.get(l) ?? 0;
    rows.set(l, row + 1);
    out.set(n.id, { x: 40 + l * 250, y: 40 + row * 110 });
  }
  return out;
}

export default function ExecutionGraph({
  graph,
  statusByNode,
  height = 420,
  onNodeClick,
}: {
  graph: ExecutionGraphData;
  statusByNode: Record<string, string>;
  height?: number;
  onNodeClick?: (nodeID: string) => void;
}) {
  // Polling replaces the graph/status objects on every tick; keep the React
  // Flow nodes stable unless the visible content actually changed.
  const graphKey = useMemo(
    () => JSON.stringify({ nodes: graph.nodes, edges: graph.edges, status: statusByNode }),
    [graph, statusByNode],
  );

  const rfNodes = useMemo<FlowRFNode[]>(() => {
    const positions = nodePositions(graph);
    return graph.nodes.map((n) => ({
      id: n.id,
      type: n.type,
      position: positions.get(n.id) ?? { x: 0, y: 0 },
      data: {
        kind: n.type,
        name: n.type === "start" ? "开始" : n.name,
        toolID: n.tool_id ?? "",
        toolName: n.tool_name ?? "",
        toolType: n.tool_type ?? "",
        hostIDs: n.host_ids ?? [],
        mapping: {},
        timeoutSec: 0,
        retryCount: 0,
        retryDelaySec: 0,
        status: n.type === "tool" ? statusByNode[n.id] : undefined,
      },
      draggable: false,
      connectable: false,
      deletable: false,
      style: onNodeClick ? { cursor: "pointer" } : undefined,
    }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [graphKey]);

  const rfEdges = useMemo(
    () =>
      graph.edges.map((e, i) => {
        const condition = (e.condition in conditionLabel ? e.condition : "always") as EdgeCondition;
        const edge = makeRFEdge({
          id: `${e.from}-${e.to}-${i}`,
          source: e.from,
          target: e.to,
          condition,
          value: e.value ?? "",
        });
        // A skipped target means this branch was not taken in this run.
        if (statusByNode[e.to] === "skipped") {
          return {
            ...edge,
            style: { ...edge.style, stroke: "#bfbfbf", strokeDasharray: "4 4", opacity: 0.7 },
            markerEnd: { type: MarkerType.ArrowClosed, color: "#bfbfbf" },
            labelStyle: { fontSize: 11, fill: "#bfbfbf" },
          };
        }
        return edge;
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [graphKey],
  );

  return (
    <div style={{ height }}>
      <ReactFlow
        className="light"
        nodes={rfNodes}
        edges={rfEdges}
        nodeTypes={flowNodeTypes}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        onNodeClick={(_, n) => onNodeClick?.(n.id)}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
