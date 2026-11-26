import { request } from "./client";

export type ChannelType = "webhook" | "feishu" | "dingtalk" | "wecom";

export interface Channel {
  id: string;
  name: string;
  type: ChannelType;
  url: string;
  has_secret: boolean;
  enabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface ChannelInput {
  name: string;
  type: ChannelType;
  url: string;
  secret?: string;
  enabled: boolean;
}

export async function listChannels(): Promise<Channel[]> {
  const res = await request<{ items: Channel[] }>("GET", "/notify-channels");
  return res.items;
}

export function createChannel(input: ChannelInput): Promise<Channel> {
  return request<Channel>("POST", "/notify-channels", input);
}

export function updateChannel(id: string, input: ChannelInput): Promise<Channel> {
  return request<Channel>("PUT", `/notify-channels/${id}`, input);
}

export function deleteChannel(id: string): Promise<void> {
  return request<void>("DELETE", `/notify-channels/${id}`);
}

export function testChannel(id: string): Promise<{ ok: boolean; response: string }> {
  return request<{ ok: boolean; response: string }>("POST", `/notify-channels/${id}/test`);
}
