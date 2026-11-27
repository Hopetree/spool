import { request } from "./client";

export interface Capability {
  user: string;
  os: string;
  kernel: string;
  arch: string;
  distro: string;
  bash_path: string;
  bash_version: string;
  python_path: string;
  python_version: string;
  sftp: boolean;
  sudo: boolean;
  setsid: boolean;
  timeout: boolean;
  latency_ms: number;
  fingerprint: string;
  key_type: string;
  probed_at: number;
}

export interface Host {
  id: string;
  name: string;
  address: string;
  port: number;
  group_id: string | null;
  group_name: string;
  credential_id?: string;
  credential_name?: string;
  enabled: boolean;
  fingerprint: string;
  fingerprint_type: string;
  capability: Capability;
  last_test_at: number | null;
  last_test_ok: boolean | null;
  note: string;
  tags: string[];
  created_at: number;
  updated_at: number;
}

export interface HostInput {
  name: string;
  address: string;
  port: number;
  group_id?: string;
  credential_id: string;
  enabled: boolean;
  note?: string;
  tags: string[];
}

export interface HostTestResult {
  ok: boolean;
  latency_ms: number;
  fingerprint: string;
  key_type: string;
  fingerprint_recorded: boolean;
  capability: Capability;
}

export interface HostList {
  items: Host[];
  total: number;
  page: number;
  page_size: number;
}

export interface HostFilters {
  group_id?: string;
  tag?: string;
  enabled?: boolean;
  keyword?: string;
  page?: number;
  page_size?: number;
}

function qs(params: HostFilters): string {
  const sp = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== "" && v !== null) sp.set(k, String(v));
  });
  const s = sp.toString();
  return s ? `?${s}` : "";
}

export function listHosts(filters: HostFilters = {}): Promise<HostList> {
  return request<HostList>("GET", `/hosts${qs(filters)}`);
}

export interface HostImportRow {
  name: string;
  address: string;
  port: number;
  tags: string[];
  note: string;
  enabled?: boolean;
}

export interface HostImportResult {
  created: number;
  failed: number;
  errors: { row: number; message: string }[];
}

export function importHosts(input: {
  group_id: string;
  credential_id: string;
  rows: HostImportRow[];
}): Promise<HostImportResult> {
  return request<HostImportResult>("POST", "/hosts/import", input);
}

export function getHost(id: string): Promise<Host> {
  return request<Host>("GET", `/hosts/${id}`);
}

export function createHost(input: HostInput): Promise<Host> {
  return request<Host>("POST", "/hosts", input);
}

export function updateHost(id: string, input: HostInput): Promise<Host> {
  return request<Host>("PUT", `/hosts/${id}`, input);
}

export function deleteHost(id: string): Promise<void> {
  return request<void>("DELETE", `/hosts/${id}`);
}

export function testHost(id: string): Promise<HostTestResult> {
  return request<HostTestResult>("POST", `/hosts/${id}/test`);
}

export function setHostFingerprint(id: string, fingerprint: string, keyType: string): Promise<Host> {
  return request<Host>("PUT", `/hosts/${id}/fingerprint`, {
    fingerprint,
    key_type: keyType,
  });
}
