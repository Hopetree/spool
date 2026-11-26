import { request } from "./client";

export type ParamType = "string" | "int" | "bool" | "select";
export type ToolType = "shell" | "python";

export interface ToolParam {
  name: string;
  label: string;
  type: ParamType;
  required: boolean;
  default: string;
  options: string[];
  description: string;
  sensitive: boolean;
}

export interface ToolSummary {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  version_no: number;
  type: ToolType;
  interpreter: string;
  timeout_sec: number;
  sudo: boolean;
  content_hash: string;
  param_count: number;
  params: ToolParam[];
  created_at: number;
  updated_at: number;
}

export interface ToolVersion {
  id: string;
  tool_id: string;
  version_no: number;
  type: ToolType;
  interpreter: string;
  content: string;
  params: ToolParam[];
  prepare_command: string;
  timeout_sec: number;
  sudo: boolean;
  content_hash: string;
  change_note: string;
  created_by: string;
  created_at: number;
}

export interface ToolDetail extends ToolSummary {
  version: ToolVersion;
}

export interface ToolInput {
  name: string;
  description: string;
  type: ToolType;
  interpreter: string;
  content: string;
  prepare_command: string;
  params: ToolParam[];
  timeout_sec: number;
  sudo: boolean;
  change_note: string;
}

export interface ToolList {
  items: ToolSummary[];
  total: number;
  page: number;
  page_size: number;
}

export interface ToolFilters {
  keyword?: string;
  enabled?: boolean;
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

export function listTools(filters: ToolFilters = {}): Promise<ToolList> {
  return request<ToolList>("GET", `/tools${qs(filters)}`);
}

export function getTool(id: string): Promise<ToolDetail> {
  return request<ToolDetail>("GET", `/tools/${id}`);
}

export function createTool(input: ToolInput): Promise<ToolDetail> {
  return request<ToolDetail>("POST", "/tools", input);
}

export function updateTool(id: string, input: ToolInput): Promise<ToolDetail> {
  return request<ToolDetail>("PUT", `/tools/${id}`, input);
}

export function setToolEnabled(id: string, enabled: boolean): Promise<void> {
  return request<void>("POST", `/tools/${id}/enabled`, { enabled });
}

export function deleteTool(id: string): Promise<void> {
  return request<void>("DELETE", `/tools/${id}`);
}

export function copyTool(id: string): Promise<ToolDetail> {
  return request<ToolDetail>("POST", `/tools/${id}/copy`);
}

export async function listVersions(id: string): Promise<ToolVersion[]> {
  const res = await request<{ items: ToolVersion[] }>("GET", `/tools/${id}/versions`);
  return res.items;
}

export function rollbackTool(id: string, versionNo: number, note = ""): Promise<ToolDetail> {
  return request<ToolDetail>("POST", `/tools/${id}/rollback`, { version_no: versionNo, note });
}
