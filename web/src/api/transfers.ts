import { parseApiError, request } from "./client";

export interface Transfer {
  id: string;
  direction: "upload" | "download";
  file_name: string;
  size_bytes: number;
  target_total: number;
  status: string;
  created_by: string;
  created_at: number;
  finished_at: number | null;
}

export interface TransferTarget {
  id: string;
  transfer_id: string;
  host_id: string;
  host_name: string;
  remote_path: string;
  status: string;
  bytes_done: number;
  error: string;
  started_at: number | null;
  finished_at: number | null;
}

export interface TransferList {
  items: Transfer[];
  total: number;
  page: number;
  page_size: number;
}

export function listTransfers(page = 1, pageSize = 50): Promise<TransferList> {
  return request<TransferList>("GET", `/transfers?page=${page}&page_size=${pageSize}`);
}

export function getTransfer(id: string): Promise<{ transfer: Transfer; targets: TransferTarget[] }> {
  return request<{ transfer: Transfer; targets: TransferTarget[] }>("GET", `/transfers/${id}`);
}

export async function uploadTransfer(form: FormData): Promise<Transfer> {
  const res = await fetch("/api/v1/transfers/upload", {
    method: "POST",
    body: form,
    credentials: "same-origin",
  });
  if (!res.ok) throw await parseApiError(res);
  return (await res.json()) as Transfer;
}

export function cancelTransfer(id: string): Promise<void> {
  return request<void>("POST", `/transfers/${id}/cancel`);
}

export function downloadUrl(hostID: string, path: string): string {
  const sp = new URLSearchParams({ host_id: hostID, path });
  return `/api/v1/transfers/download?${sp.toString()}`;
}
