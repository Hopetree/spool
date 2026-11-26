import { create } from "zustand";
import { request } from "../api/client";

export type Role = "admin" | "operator" | "viewer";

export interface User {
  id: string;
  username: string;
  display_name: string;
  role: Role;
  must_change_password: boolean;
}

interface AuthState {
  user: User | null;
  status: "loading" | "authed" | "anon";
  fetchMe: () => Promise<void>;
  login: (username: string, password: string) => Promise<User>;
  logout: () => Promise<void>;
  changePassword: (oldPassword: string, newPassword: string) => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  status: "loading",
  fetchMe: async () => {
    try {
      const user = await request<User>("GET", "/auth/me");
      set({ user, status: "authed" });
    } catch {
      set({ user: null, status: "anon" });
    }
  },
  login: async (username, password) => {
    const user = await request<User>("POST", "/auth/login", { username, password });
    set({ user, status: "authed" });
    return user;
  },
  logout: async () => {
    try {
      await request<void>("POST", "/auth/logout");
    } finally {
      set({ user: null, status: "anon" });
    }
  },
  changePassword: async (oldPassword, newPassword) => {
    await request<void>("PUT", "/auth/password", {
      old_password: oldPassword,
      new_password: newPassword,
    });
    set((state) => ({
      user: state.user ? { ...state.user, must_change_password: false } : null,
    }));
  },
}));
