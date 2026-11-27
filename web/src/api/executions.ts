import { request } from "./client";

export type ExecutionStatus =
  | "queued"
  | "running"
  | "success"
  | "partial"
  | "failed"
  | "cancelled"
  | "interrupted"
  | "skipped";

export interface Execution {
  id: string;
  kind: string;
  source: string;
  ref_id: string;
  schedule_id: string;
  name: string;
  host_total: number;
  host_success: number;
  host_failed: number;
  host_cancelled: number;
  status: ExecutionStatus;
  failure_reason: string;
  error_summary: string;
  cancel_requested: boolean;
  created_by: string;
  created_by_name?: string;
  command?: string;
  tool_version_no?: number;
  created_at: number;
  started_at: number | null;
  finished_at: number | null;
}

export interface HostRun {
  id: string;
  execution_id: string;
  host_id: string;
  host_name: string;
  status: string;
  failure_reason: string;
  error_message: string;
  started_at: number | null;
  finished_at: number | null;
}

export interface StepRun {
  id: string;
  host_run_id: string;
  node_id: string;
  stage_idx: number;
  step_idx: number;
  name: string;
  tool_id: string;
  tool_version_id: string;
  tool_version_no: number;
  content_hash: string;
  interpreter: string;
  sudo: boolean;
  status: string;
  failure_reason: string;
  exit_code: number | null;
  error_message: string;
  attempt: number;
  started_at: number | null;
  finished_at: number | null;
  log_events: number;
  log_bytes: number;
  truncated: boolean;
  dropped_bytes: number;
}

export interface ExecutionGraphNode {
  id: string;
  type: "start" | "tool";
  name: string;
  tool_id?: string;
  tool_name?: string;
  tool_type?: string;
  tool_version_no?: number;
  host_ids?: string[];
  params?: Record<string, string>;
  x: number;
  y: number;
}

export interface ExecutionGraphEdge {
  from: string;
  to: string;
  condition: string;
  value?: string;
}

export interface ExecutionGraphData {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
}

export interface ExecutionDetail extends Execution {
  params: Record<string, string>;
  command?: string;
  host_runs: HostRun[];
  step_runs: StepRun[];
  graph?: ExecutionGraphData;
}

export interface ExecutionEvent {
  seq: number;
  ts: number;
  type: "log" | "state" | "end";
  entity?: string;
  status?: string;
  host_run?: string;
  step_run?: string;
  stream?: "stdout" | "stderr";
  data?: string;
  message?: string;
}

export interface ExecutionList {
  items: Execution[];
  total: number;
  page: number;
  page_size: number;
}

export interface ExecutionFilters {
  kind?: string;
  status?: string;
  ref_id?: string;
  host_id?: string;
  tool_id?: string;
  schedule_id?: string;
  scheduled?: boolean;
  created_by?: string;
  keyword?: string;
  from?: number;
  to?: number;
  page?: number;
  page_size?: number;
}

function qs(params: object): string {
  const sp = new URLSearchParams();
  Object.entries(params as Record<string, string | number | boolean | undefined | null>).forEach(([k, v]) => {
    if (v !== undefined && v !== "") sp.set(k, String(v));
  });
  const s = sp.toString();
  return s ? `?${s}` : "";
}

export function listExecutions(filters: ExecutionFilters = {}): Promise<ExecutionList> {
  return request<ExecutionList>("GET", `/executions${qs(filters)}`);
}

export function getExecution(id: string): Promise<ExecutionDetail> {
  return request<ExecutionDetail>("GET", `/executions/${id}`);
}

export interface RunToolInput {
  kind: "tool";
  ref_id: string;
  params: Record<string, string>;
  host_ids: string[];
}

export interface RunFlowInput {
  kind: "flow";
  ref_id: string;
  params: Record<string, string>;
  host_ids: string[];
  node_host_ids: Record<string, string[]>;
}

export function runTool(input: RunToolInput): Promise<Execution> {
  return request<Execution>("POST", "/executions", input);
}

export function runFlow(input: RunFlowInput): Promise<Execution> {
  return request<Execution>("POST", "/executions", input);
}

export interface RunAdhocInput {
  kind: "adhoc";
  command: string;
  timeout_sec: number;
  sudo: boolean;
  host_ids: string[];
}

export function runAdhoc(input: RunAdhocInput): Promise<Execution> {
  return request<Execution>("POST", "/executions", input);
}

export function cancelExecution(id: string): Promise<void> {
  return request<void>("POST", `/executions/${id}/cancel`);
}

export function cancelHostRun(executionID: string, hostRunID: string): Promise<void> {
  return request<void>("POST", `/executions/${executionID}/host-runs/${hostRunID}/cancel`);
}

export function rerunExecution(id: string): Promise<Execution> {
  return request<Execution>("POST", `/executions/${id}/rerun`);
}

export async function listEvents(
  id: string,
  afterSeq = 0,
  limit = 5000,
  hostRunID?: string,
  stepRunID?: string,
): Promise<{ items: ExecutionEvent[]; status: ExecutionStatus }> {
  const sp = new URLSearchParams({ after_seq: String(afterSeq), limit: String(limit) });
  if (hostRunID) sp.set("host_run", hostRunID);
  if (stepRunID) sp.set("step_run", stepRunID);
  return request<{ items: ExecutionEvent[]; status: ExecutionStatus }>(
    "GET",
    `/executions/${id}/events?${sp.toString()}`,
  );
}

export function logDownloadUrl(id: string, hostRunID?: string, stepRunID?: string): string {
  const sp = new URLSearchParams();
  if (hostRunID) sp.set("host_run", hostRunID);
  if (stepRunID) sp.set("step_run", stepRunID);
  const s = sp.toString();
  return `/api/v1/executions/${id}/logs/download${s ? `?${s}` : ""}`;
}
