import { createAuthClient } from "better-auth/client";
import {
  organizationClient,
  twoFactorClient,
  adminClient,
} from "better-auth/client/plugins";

const AUTH_BASE_URL = import.meta.env.VITE_AUTH_URL || "http://localhost:3001";

export const authClient = createAuthClient({
  baseURL: AUTH_BASE_URL,
  plugins: [
    organizationClient(),
    twoFactorClient(),
    adminClient(),
  ],
});

export interface WorkspaceInfo {
  id: string;
  name: string;
  slug: string;
  plan: "starter" | "growth" | "enterprise";
  billingProvider: "midtrans" | "stripe";
  monthlyEventQuota: number;
  usedEventsThisMonth: number;
  createdAt: string;
}

export interface WorkspaceMember {
  id: string;
  userId: string;
  name: string;
  email: string;
  image?: string | null;
  role: "owner" | "admin" | "viewer" | "member";
  scopeType: "workspace" | "project";
  projectId?: string | null;
  projectName?: string | null;
  createdAt: string;
}

export interface WorkspaceInvitation {
  id: string;
  email: string;
  role: "owner" | "admin" | "viewer" | "member";
  scopeType: "workspace" | "project";
  projectId?: string | null;
  projectName?: string | null;
  status: string;
  expiresAt: string;
  createdAt: string;
  inviteUrl: string;
}

export interface OutboxEmailItem {
  id: string;
  type: "reset_password" | "verify_email" | "invitation";
  to: string;
  subject: string;
  url: string;
  createdAt: string;
}

export interface WorkspaceOverviewResponse {
  workspace: WorkspaceInfo;
  members: WorkspaceMember[];
  invitations: WorkspaceInvitation[];
  outbox: OutboxEmailItem[];
}

export interface PlatformAdminUser {
  id: string;
  name: string;
  email: string;
  emailVerified: boolean;
  image?: string | null;
  role: string;
  twoFactorEnabled: boolean;
  banned: boolean;
  banReason?: string | null;
  createdAt: string;
}

async function authFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers || {});
  if (!headers.has("Content-Type") && options.body && typeof options.body === "string") {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(`${AUTH_BASE_URL}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data?.error?.message || "Unable to complete workspace request.");
  }
  return data as T;
}

export const workspaceApi = {
  getOverview: () => authFetch<WorkspaceOverviewResponse>("/api/workspace"),

  updateWorkspace: (payload: {
    name?: string;
    slug?: string;
    plan?: "starter" | "growth" | "enterprise";
    billingProvider?: "midtrans" | "stripe";
  }) =>
    authFetch<{ workspace: WorkspaceInfo }>("/api/workspace", {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  createInvitation: (payload: {
    email: string;
    role: "owner" | "admin" | "viewer";
    scopeType: "workspace" | "project";
    projectId?: string | null;
  }) =>
    authFetch<{ invitation: WorkspaceInvitation }>("/api/workspace/invitations", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  acceptInvitation: (inviteId: string) =>
    authFetch<{ accepted: boolean; memberId: string }>(
      `/api/workspace/invitations/${inviteId}/accept`,
      { method: "POST" }
    ),

  revokeInvitation: (inviteId: string) =>
    authFetch<{ revoked: boolean; id: string }>(
      `/api/workspace/invitations/${inviteId}`,
      { method: "DELETE" }
    ),

  updateMemberRole: (memberId: string, role: "owner" | "admin" | "viewer") =>
    authFetch<{ updated: boolean; id: string; role: string }>(
      `/api/workspace/members/${memberId}`,
      {
        method: "PATCH",
        body: JSON.stringify({ role }),
      }
    ),

  removeMember: (memberId: string) =>
    authFetch<{ removed: boolean; id: string }>(
      `/api/workspace/members/${memberId}`,
      { method: "DELETE" }
    ),

  listAdminUsers: () =>
    authFetch<{ users: PlatformAdminUser[] }>("/api/workspace/admin/users"),

  updateAdminUser: (
    userId: string,
    payload: { role?: string; banned?: boolean; banReason?: string; emailVerified?: boolean }
  ) =>
    authFetch<{ updated: boolean; id: string }>(
      `/api/workspace/admin/users/${userId}`,
      {
        method: "PATCH",
        body: JSON.stringify(payload),
      }
    ),
};
