import { request } from "./client";

export type TargetKind = "tool" | "flow";
export type ConcurrencyPolicy = "skip" | "queue";
export type NotifyEvents = "failure" | "always";

export interface Schedule {
  id: string;
  name: string;
  target_kind: TargetKind;
  target_ref_id: string;
  target_name: string;
  params: Record<string, string>;
  cron_expr: string;
  concurrency_policy: ConcurrencyPolicy;
  host_ids: string[];
  node_host_ids: Record<string, string[]>;
  notify_channel_ids: string[];
  notify_events: NotifyEvents;
  enabled: boolean;
  last_run_at: number | null;
  next_run_at: number | null;
  created_at: number;
  updated_at: number;
}

export interface ScheduleInput {
  name: string;
  target_kind: TargetKind;
  target_ref_id: string;
  params: Record<string, string>;
  host_ids: string[];
  node_host_ids: Record<string, string[]>;
  cron_expr: string;
  concurrency_policy: ConcurrencyPolicy;
  notify_channel_ids: string[];
  notify_events: NotifyEvents;
  enabled: boolean;
}

export interface ScheduleList {
  items: Schedule[];
  total: number;
  page: number;
  page_size: number;
}

export function listSchedules(filters: { keyword?: string; enabled?: boolean } = {}): Promise<ScheduleList> {
  const sp = new URLSearchParams();
  if (filters.keyword) sp.set("keyword", filters.keyword);
  if (filters.enabled !== undefined) sp.set("enabled", String(filters.enabled));
  sp.set("page_size", "100");
  return request<ScheduleList>("GET", `/schedules?${sp.toString()}`);
}

export function getSchedule(id: string): Promise<Schedule> {
  return request<Schedule>("GET", `/schedules/${id}`);
}

export function createSchedule(input: ScheduleInput): Promise<Schedule> {
  return request<Schedule>("POST", "/schedules", input);
}

export function updateSchedule(id: string, input: ScheduleInput): Promise<Schedule> {
  return request<Schedule>("PUT", `/schedules/${id}`, input);
}

export function setScheduleEnabled(id: string, enabled: boolean): Promise<void> {
  return request<void>("POST", `/schedules/${id}/enabled`, { enabled });
}

export function deleteSchedule(id: string): Promise<void> {
  return request<void>("DELETE", `/schedules/${id}`);
}

export function runSchedule(id: string): Promise<{ id: string }> {
  return request<{ id: string }>("POST", `/schedules/${id}/run`);
}

export async function previewCron(expr: string, count = 5): Promise<number[]> {
  const res = await request<{ items: number[] }>("POST", "/schedules/preview", { cron_expr: expr, count });
  return res.items;
}

export async function nextRuns(id: string, count = 5): Promise<number[]> {
  const res = await request<{ items: number[] }>("GET", `/schedules/${id}/next-runs?count=${count}`);
  return res.items;
}
