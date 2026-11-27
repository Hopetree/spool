import type { CSSProperties } from "react";
import {
  Handle,
  MarkerType,
  Position,
  type Edge,
  type Node,
  type NodeProps,
  type NodeTypes,
} from "@xyflow/react";
import {
  conditionLabel,
  type EdgeCondition,
  type FlowEdge,
  type FlowMapping,
  type FlowNode,
  type FlowNodeType,
} from "../api/flows";
import { statusColors, statusLabel } from "./status";

/** Data carried by an editable flow node. */
export interface FlowNodeData extends Record<string, unknown> {
  kind: FlowNodeType;
  name: string;
  toolID: string;
  toolName: string;
  toolType: string;
  hostIDs: string[];
  mapping: Record<string, FlowMapping>;
  timeoutSec: number;
  retryCount: number;
  retryDelaySec: number;
  /** Execution status; only set when rendering a run graph. */
  status?: string;
}

/** Data carried by an editable flow edge. */
export interface FlowEdgeData extends Record<string, unknown> {
  condition: EdgeCondition;
  value: string;
}

export type FlowRFNode = Node<FlowNodeData, FlowNodeType>;
export type FlowRFEdge = Edge<FlowEdgeData>;

export const edgeColor: Record<string, string> = {
  always: "#8c8c8c",
  success: "#52c41a",
  failure: "#ff4d4f",
  output_contains: "#1677ff",
  output_not_contains: "#fa8c16",
};

export const edgeConditionOptions: { value: EdgeCondition; label: string }[] = (
  Object.keys(conditionLabel) as EdgeCondition[]
).map((c) => ({ value: c, label: conditionLabel[c] }));

function boxStyle(selected: boolean | undefined, start: boolean, status?: string): CSSProperties {
  const statusColor = status ? statusColors[status] : undefined;
  return {
    width: 168,
    padding: "8px 10px",
    borderRadius: 8,
    border: `1px solid ${selected ? "#1677ff" : statusColor ?? (start ? "#91caff" : "#d9d9d9")}`,
    background: start ? "#f0f7ff" : "#fff",
    boxShadow: selected ? "0 0 0 2px rgba(22,119,255,0.18)" : "0 1px 2px rgba(0,0,0,0.04)",
    fontSize: 12,
    lineHeight: 1.4,
  };
}

function StartNode({ selected }: NodeProps<FlowRFNode>) {
  return (
    <div style={{ ...boxStyle(selected, true), display: "flex", alignItems: "center", gap: 6 }}>
      <span style={{ width: 8, height: 8, borderRadius: 4, background: "#1677ff", flex: "none" }} />
      <span style={{ fontWeight: 600 }}>开始</span>
      <Handle type="source" position={Position.Right} style={{ width: 8, height: 8, background: "#1677ff" }} />
    </div>
  );
}

function ToolNode({ data, selected }: NodeProps<FlowRFNode>) {
  const color = data.status ? statusColors[data.status] : undefined;
  return (
    <div style={boxStyle(selected, false, data.status)}>
      <Handle type="target" position={Position.Left} style={{ width: 8, height: 8 }} />
      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <span
          style={{
            fontWeight: 600,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            flex: 1,
          }}
        >
          {data.name || "未命名节点"}
        </span>
        {color && (
          <span
            style={{
              flex: "none",
              fontSize: 10,
              lineHeight: "16px",
              padding: "0 5px",
              borderRadius: 8,
              color,
              background: `${color}1a`,
            }}
          >
            {statusLabel(data.status ?? "")}
          </span>
        )}
      </div>
      <div
        style={{
          fontSize: 11,
          color: "#8c8c8c",
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        {data.toolName ? `工具 · ${data.toolName}` : data.toolID ? `工具 · ${data.toolID}` : "未选择工具"}
      </div>
      <div style={{ fontSize: 11, color: "#8c8c8c" }}>
        {data.hostIDs.length > 0 ? `${data.hostIDs.length} 台执行主机` : "默认执行主机"}
      </div>
      <Handle type="source" position={Position.Right} style={{ width: 8, height: 8 }} />
    </div>
  );
}

export const flowNodeTypes: NodeTypes = { start: StartNode, tool: ToolNode };

export function makeRFEdge(input: {
  id: string;
  source: string;
  target: string;
  condition: EdgeCondition;
  value: string;
}): FlowRFEdge {
  const color = edgeColor[input.condition] ?? "#8c8c8c";
  return {
    id: input.id,
    source: input.source,
    target: input.target,
    data: { condition: input.condition, value: input.value },
    label: conditionLabel[input.condition] + (input.value ? `：${input.value}` : ""),
    labelStyle: { fontSize: 11 },
    labelBgPadding: [4, 2],
    style: { stroke: color, strokeWidth: 1.5 },
    markerEnd: { type: MarkerType.ArrowClosed, color },
  };
}

/** Recomputes label/style/marker of an edge from its data. */
export function restyleEdge(edge: FlowRFEdge): FlowRFEdge {
  return makeRFEdge({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    condition: edge.data?.condition ?? "success",
    value: edge.data?.value ?? "",
  });
}

export function flowToRFNodes(nodes: FlowNode[], interactive = false): FlowRFNode[] {
  return nodes.map((n) => ({
    id: n.id,
    type: n.type,
    position: { x: n.x, y: n.y },
    data: {
      kind: n.type,
      name: n.type === "start" ? "开始" : n.name,
      toolID: n.tool_id,
      toolName: n.tool_name,
      toolType: n.tool_type,
      hostIDs: n.host_ids ?? [],
      mapping: n.mapping ?? {},
      timeoutSec: n.timeout_sec,
      retryCount: n.retry_count,
      retryDelaySec: n.retry_delay_sec,
    },
    draggable: interactive,
    connectable: interactive,
    deletable: interactive && n.type !== "start",
  }));
}

export function flowToRFEdges(edges: FlowEdge[]): FlowRFEdge[] {
  return edges.map((e) =>
    makeRFEdge({ id: e.id, source: e.from, target: e.to, condition: e.condition, value: e.value }),
  );
}
