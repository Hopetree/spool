import { request } from "./client";
import type { ToolParam } from "./tools";

export type FailurePolicy = "abort" | "continue";

export interface FlowMapping {
  source: "flow" | "literal";
  value: string;
}

export interface FlowStep {
  id: string;
  index: number;
  name: string;
  tool_id: string;
  tool_name: string;
  tool_type: string;
  mapping: Record<string, FlowMapping>;
  timeout_sec: number;
  retry_count: number;
  retry_delay_sec: number;
  enabled: boolean;
}

export interface FlowStage {
  id: string;
  index: number;
  name: string;
  always_run: boolean;
  steps: FlowStep[];
}

export interface FlowSummary {
  id: string;
  name: string;
  description: string;
  params: ToolParam[];
  failure_policy: FailurePolicy;
  max_duration_sec: number;
  enabled: boolean;
  stage_count: number;
  step_count: number;
  created_at: number;
  updated_at: number;
}

export interface FlowDetail extends FlowSummary {
  stages: FlowStage[];
}

export interface FlowStepInput {
  name: string;
  tool_id: string;
  mapping: Record<string, FlowMapping>;
  timeout_sec: number;
  retry_count: number;
  retry_delay_sec: number;
  enabled: boolean;
}

export interface FlowStageInput {
  name: string;
  always_run: boolean;
  steps: FlowStepInput[];
}

export interface FlowInput {
  name: string;
  description: string;
  params: ToolParam[];
  failure_policy: FailurePolicy;
  max_duration_sec: number;
  stages: FlowStageInput[];
}

export interface FlowList {
  items: FlowSummary[];
  total: number;
  page: number;
  page_size: number;
}

function qs(params: object): string {
  const sp = new URLSearchParams();
  Object.entries(params as Record<string, string | number | boolean | undefined | null>).forEach(([k, v]) => {
    if (v !== undefined && v !== "" && v !== null) sp.set(k, String(v));
  });
  const s = sp.toString();
  return s ? `?${s}` : "";
}

export function listFlows(filters: { keyword?: string; enabled?: boolean } = {}): Promise<FlowList> {
  return request<FlowList>("GET", `/flows${qs({ ...filters, page: 1, page_size: 100 })}`);
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
