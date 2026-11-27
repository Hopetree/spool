import { ApiError, parseApiError, request } from "./client";

export interface Transfer {
  id: string;
  direction: "upload" | "download";
  file_name: string;
  size_bytes: number;
  target_total: number;
  status: string;
  remote_path: string;
  created_by: string;
  created_by_name?: string;
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
  let res: Response;
  try {
    res = await fetch("/api/v1/transfers/upload", {
      method: "POST",
      body: form,
      credentials: "same-origin",
    });
  } catch {
    throw new ApiError(0, "network_error", "上传失败，请检查网络连接后重试");
  }
  if (!res.ok) throw await parseApiError(res);
  try {
    return (await res.json()) as Transfer;
  } catch {
    throw new ApiError(res.status, "invalid_response", "服务端返回异常，请稍后重试");
  }
}

export function cancelTransfer(id: string): Promise<void> {
  return request<void>("POST", `/transfers/${id}/cancel`);
}

export interface TransferDownload {
  blob: Blob;
  fileName: string;
}

function downloadFileName(disposition: string | null, remotePath: string): string {
  if (disposition) {
    const encoded = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
    if (encoded) {
      try {
        return decodeURIComponent(encoded[1]);
      } catch {
        // Fall through to unencoded filename.
      }
    }
    const quoted = /filename="([^"]*)"/i.exec(disposition);
    if (quoted?.[1]) return quoted[1];
    const plain = /filename=([^;]+)/i.exec(disposition);
    if (plain?.[1]) return plain[1].trim();
  }
  return remotePath.split("/").pop() || "download";
}

export async function downloadTransfer(hostID: string, remotePath: string): Promise<TransferDownload> {
  const sp = new URLSearchParams({ host_id: hostID, path: remotePath });
  let res: Response;
  try {
    res = await fetch(`/api/v1/transfers/download?${sp.toString()}`, { credentials: "same-origin" });
  } catch {
    throw new ApiError(0, "network_error", "无法连接服务器，请检查网络后重试");
  }
  if (!res.ok) throw await parseApiError(res);
  let blob: Blob;
  try {
    blob = await res.blob();
  } catch {
    throw new ApiError(0, "download_failed", "下载中断，请稍后重试");
  }
  return { blob, fileName: downloadFileName(res.headers.get("Content-Disposition"), remotePath) };
}
