import { request } from "./client";

export interface EnvVar {
  id: string;
  name: string;
  description: string;
  sensitive: boolean;
  created_at: number;
  updated_at: number;
  updated_by_name: string;
}

export interface EnvVarInput {
  name: string;
  value: string;
  description: string;
  sensitive: boolean;
}

export interface EnvVarList {
  items: EnvVar[];
  total: number;
}

export function listEnvVars(): Promise<EnvVarList> {
  return request<EnvVarList>("GET", "/envars");
}

export function createEnvVar(input: EnvVarInput): Promise<EnvVar> {
  return request<EnvVar>("POST", "/envars", input);
}

export function updateEnvVar(id: string, input: EnvVarInput): Promise<EnvVar> {
  return request<EnvVar>("PUT", `/envars/${id}`, input);
}

export function deleteEnvVar(id: string): Promise<void> {
  return request<void>("DELETE", `/envars/${id}`);
}
