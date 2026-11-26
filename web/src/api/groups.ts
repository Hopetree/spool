import { request } from "./client";

export interface HostGroup {
  id: string;
  name: string;
  description: string;
  host_count: number;
  created_at: number;
  updated_at: number;
}

export interface GroupInput {
  name: string;
  description?: string;
}

export async function listGroups(): Promise<HostGroup[]> {
  const res = await request<{ items: HostGroup[] }>("GET", "/host-groups");
  return res.items;
}

export function createGroup(input: GroupInput): Promise<HostGroup> {
  return request<HostGroup>("POST", "/host-groups", input);
}

export function updateGroup(id: string, input: GroupInput): Promise<HostGroup> {
  return request<HostGroup>("PUT", `/host-groups/${id}`, input);
}

export function deleteGroup(id: string): Promise<void> {
  return request<void>("DELETE", `/host-groups/${id}`);
}
