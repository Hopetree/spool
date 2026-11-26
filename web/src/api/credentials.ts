import { request } from "./client";

export type AuthType = "password" | "private_key";

export interface Credential {
  id: string;
  name: string;
  auth_type: AuthType;
  username: string;
  description: string;
  has_passphrase: boolean;
  created_at: number;
  updated_at: number;
}

export interface CredentialInput {
  name: string;
  auth_type: AuthType;
  username: string;
  secret?: string;
  passphrase?: string;
  description?: string;
}

export async function listCredentials(): Promise<Credential[]> {
  const res = await request<{ items: Credential[] }>("GET", "/credentials");
  return res.items;
}

export function createCredential(input: CredentialInput): Promise<Credential> {
  return request<Credential>("POST", "/credentials", input);
}

export function updateCredential(id: string, input: CredentialInput): Promise<Credential> {
  return request<Credential>("PUT", `/credentials/${id}`, input);
}

export function deleteCredential(id: string): Promise<void> {
  return request<void>("DELETE", `/credentials/${id}`);
}
