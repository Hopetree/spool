import { request } from "./client";

export interface AuditRecord {
  id: string;
  ts: number;
  user_id: string;
  username_snapshot: string;
  action: string;
  resource_type: string;
  resource_id: string;
  resource_name: string;
  summary_json: string;
  summary_display?: string;
  ip: string;
  user_agent: string;
}

export interface AuditFilters {
  action?: string;
  actor_id?: string;
  from?: number;
  to?: number;
  page?: number;
  page_size?: number;
}

export interface AuditPage {
  items: AuditRecord[];
  total: number;
  page: number;
  page_size: number;
}

export function listAudit(filters: AuditFilters = {}): Promise<AuditPage> {
  const sp = new URLSearchParams();
  Object.entries(filters).forEach(([k, v]) => {
    if (v !== undefined && v !== "") sp.set(k, String(v));
  });
  return request<AuditPage>("GET", `/audit?${sp.toString()}`);
}
