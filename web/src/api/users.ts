import { request } from "./client";

export type Role = "admin" | "operator" | "viewer";

export interface UserView {
  id: string;
  username: string;
  display_name: string;
  role: Role;
  enabled: boolean;
  must_change_password: boolean;
  last_login_at: number | null;
  created_at: number;
  updated_at: number;
}

export interface CreateUserInput {
  username: string;
  display_name: string;
  role: Role;
  password: string;
}

export async function listUsers(): Promise<UserView[]> {
  const res = await request<{ items: UserView[] }>("GET", "/users");
  return res.items;
}

export function createUser(input: CreateUserInput): Promise<UserView> {
  return request<UserView>("POST", "/users", input);
}

export function updateUser(
  id: string,
  input: { display_name: string; role: Role; enabled: boolean },
): Promise<UserView> {
  return request<UserView>("PUT", `/users/${id}`, input);
}

export function resetUserPassword(id: string, password: string): Promise<void> {
  return request<void>("POST", `/users/${id}/reset-password`, { password });
}

export function logoutAllSessions(id: string): Promise<void> {
  return request<void>("POST", `/users/${id}/logout-all`);
}

export function deleteUser(id: string): Promise<void> {
  return request<void>("DELETE", `/users/${id}`);
}
