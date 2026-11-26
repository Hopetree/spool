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

export interface ExecutionDetail extends Execution {
  params: Record<string, string>;
  host_runs: HostRun[];
  step_runs: StepRun[];
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

export function runTool(input: RunToolInput): Promise<Execution> {
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
): Promise<{ items: ExecutionEvent[]; status: ExecutionStatus }> {
  return request<{ items: ExecutionEvent[]; status: ExecutionStatus }>(
    "GET",
    `/executions/${id}/events?after_seq=${afterSeq}&limit=${limit}`,
  );
}

export function logDownloadUrl(id: string, hostRunID?: string, stepRunID?: string): string {
  const sp = new URLSearchParams();
  if (hostRunID) sp.set("host_run", hostRunID);
  if (stepRunID) sp.set("step_run", stepRunID);
  const s = sp.toString();
  return `/api/v1/executions/${id}/logs/download${s ? `?${s}` : ""}`;
}
