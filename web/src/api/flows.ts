import { request } from "./client";
import type { ToolCategory, ToolParam } from "./tools";

export type FlowNodeType = "start" | "tool";
export type EdgeCondition =
  | "always"
  | "success"
  | "failure"
  | "output_contains"
  | "output_not_contains";

export type FlowMapping =
  | { source: "literal"; value: string }
  | { source: "input"; input_name: string; input_label?: string; input_sensitive?: boolean }
  | { source: "node"; ref_node_id: string; ref_param: string };

export interface FlowNode {
  id: string;
  type: FlowNodeType;
  name: string;
  tool_id: string;
  tool_name: string;
  tool_type: string;
  host_ids: string[];
  mapping: Record<string, FlowMapping>;
  timeout_sec: number;
  retry_count: number;
  retry_delay_sec: number;
  x: number;
  y: number;
}

export interface FlowEdge {
  id: string;
  from: string;
  to: string;
  condition: EdgeCondition;
  value: string;
}

export interface FlowSummary {
  id: string;
  name: string;
  category: string;
  description: string;
  params: ToolParam[];
  max_duration_sec: number;
  enabled: boolean;
  node_count: number;
  edge_count: number;
  created_at: number;
  updated_at: number;
}

export interface FlowDetail extends FlowSummary {
  nodes: FlowNode[];
  edges: FlowEdge[];
}

export interface FlowNodeInput {
  id: string;
  type: FlowNodeType;
  name: string;
  tool_id: string;
  host_ids: string[];
  mapping: Record<string, FlowMapping>;
  timeout_sec: number;
  retry_count: number;
  retry_delay_sec: number;
  x: number;
  y: number;
}

export interface FlowEdgeInput {
  from: string;
  to: string;
  condition: EdgeCondition;
  value: string;
}

export interface FlowInput {
  name: string;
  category: string;
  description: string;
  params?: ToolParam[];
  max_duration_sec: number;
  nodes: FlowNodeInput[];
  edges: FlowEdgeInput[];
}

export interface FlowList {
  items: FlowSummary[];
  total: number;
  page: number;
  page_size: number;
}

export const conditionLabel: Record<EdgeCondition, string> = {
  always: "总是",
  success: "成功",
  failure: "失败",
  output_contains: "输出包含",
  output_not_contains: "输出不包含",
};

function qs(params: object): string {
  const sp = new URLSearchParams();
  Object.entries(params as Record<string, string | number | boolean | undefined | null>).forEach(([k, v]) => {
    if (v !== undefined && v !== "" && v !== null) sp.set(k, String(v));
  });
  const s = sp.toString();
  return s ? `?${s}` : "";
}

export function listFlows(
  filters: { keyword?: string; enabled?: boolean; category?: string; uncategorized?: boolean } = {},
): Promise<FlowList> {
  return request<FlowList>("GET", `/flows${qs({ ...filters, page: 1, page_size: 100 })}`);
}

export function listFlowCategories(): Promise<ToolCategory[]> {
  return request<ToolCategory[]>("GET", "/flows/categories");
}

export function getFlow(id: string): Promise<FlowDetail> {
  return request<FlowDetail>("GET", `/flows/${id}`);
}

export function createFlow(input: FlowInput): Promise<FlowDetail> {
  return request<FlowDetail>("POST", "/flows", input);
}

export function updateFlow(id: string, input: FlowInput): Promise<FlowDetail> {
  return request<FlowDetail>("PUT", `/flows/${id}`, input);
}

export function setFlowEnabled(id: string, enabled: boolean): Promise<void> {
  return request<void>("POST", `/flows/${id}/enabled`, { enabled });
}

export function deleteFlow(id: string): Promise<void> {
  return request<void>("DELETE", `/flows/${id}`);
}

export function copyFlow(id: string): Promise<FlowDetail> {
  return request<FlowDetail>("POST", `/flows/${id}/copy`);
}
