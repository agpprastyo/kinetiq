import { loginUrl } from "./redirect";
import { extrasToQuery, type QueryExtras } from "./filters";
import { funnelOptionsToQuery, type FunnelOptions } from "./funnel";
export const API_BASE_URL = import.meta.env.VITE_API_URL || "http://localhost:8080/api/v1";

export interface Project {
  id: string;
  name: string;
  role?: "owner" | "admin" | "viewer" | string;
  status: string;
  retention_days?: number;
  allowed_origins?: string | string[];
  rate_limit_per_sec?: number;
  anonymize_ip?: boolean;
  created_at: string;
  updated_at: string;
}

export interface AlertRule {
  id: string;
  project_id: string;
  name: string;
  event_name: string;
  condition: "spike_gt" | "drop_lt" | string;
  threshold_count: number;
  window_minutes: number;
  webhook_url: string;
  webhook_secret?: string;
  channel_type: "slack" | "discord" | "telegram" | "webhook" | string;
  telegram_chat_id?: string;
  is_active: boolean;
  cooldown_minutes: number;
  last_triggered_at?: string | null;
  snoozed_until?: string | null;
  status?: "healthy" | "firing" | "snoozed" | string;
  created_at: string;
  updated_at: string;
}

export interface CreateAlertRulePayload {
  name: string;
  event_name: string;
  condition: "spike_gt" | "drop_lt";
  threshold_count: number;
  window_minutes: number;
  webhook_url: string;
  webhook_secret?: string;
  channel_type: "slack" | "discord" | "telegram" | "webhook";
  telegram_chat_id?: string;
  is_active?: boolean;
  cooldown_minutes?: number;
}

export interface UpdateProjectPayload {
  name?: string;
  status?: "active" | "paused";
  retention_days?: number;
  allowed_origins?: string;
  rate_limit_per_sec?: number;
  anonymize_ip?: boolean;
}

export interface ProjectApiKey {
  id: string;
  project_id: string;
  name: string;
  key_prefix: string;
  environment: "live" | "test" | string;
  last_used_at?: string | null;
  revoked_at?: string | null;
  created_at: string;
}

export interface CreateApiKeyResponse {
  id?: string;
  name?: string;
  key?: ProjectApiKey;
  raw_key?: string;
  api_key: string;
}

export interface CreateProjectResponse {
  id?: string;
  name?: string;
  project: Project;
  api_key: string;
}

export interface SummaryData {
  total_events: number;
  unique_distinct_ids: number;
  breakdown?: { value: string; count: number }[];
}

export interface CatalogEvent {
  event_name: string;
  count: number;
  last_seen: string;
}

export interface CatalogProperty {
  name: string;
  scope: "event" | "person";
  type: string;
  count: number;
}

export interface CatalogValue {
  value: string;
  count: number;
}

export interface TopEventItem {
  event_name: string;
  count: number;
}

export interface TopEventsResponse {
  items: TopEventItem[];
}

export interface VolumePoint {
  timestamp: string;
  count: number;
}

export interface VolumeData {
  interval: string;
  points: VolumePoint[];
}

export interface FunnelStep {
  step: number;
  event_name: string;
  count: number;
  conversion_rate: number;
  median_time_to_convert_seconds?: number | null;
}

export interface FunnelData {
  steps: FunnelStep[];
  window_seconds?: number;
  order?: string;
  breakdown?: { value: string; steps: FunnelStep[] }[];
}

export interface FunnelDropoffUser {
  person_key: string;
  distinct_id: string;
  entered_at: string;
  reached_at: string;
}

export interface FunnelDropoffData {
  step: number;
  event_name: string;
  next_event_name: string;
  total: number;
  limit: number;
  offset: number;
  items: FunnelDropoffUser[];
}

export interface BreakdownItem {
  property_value: string;
  count: number;
}

export interface RetentionPeriod {
  period: number;
  count: number;
  percentage: number;
}

export interface CohortItem {
  cohort_date: string;
  total_users: number;
  periods: RetentionPeriod[];
}

export interface RetentionData {
  granularity: string;
  first_event: string;
  return_event: string;
  from: string;
  to: string;
  cohorts: CohortItem[];
}

export interface RotateKeyResponse {
  project: Project;
  api_key: string;
}

export class ApiError extends Error {
  status: number;
  code?: string;

  constructor(status: number, message: string, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

/**
 * Ensures raw server/SQL/runtime errors are never exposed directly in the UI.
 */
export function toSafeUserMessage(err: unknown, fallback = "Unable to complete your request. Please try again."): string {
  if (!err) return fallback;
  if (err instanceof ApiError) {
    if (err.status >= 500) {
      return "Service is temporarily unavailable. Please try again in a moment.";
    }
    return err.message || fallback;
  }
  if (err instanceof Error) {
    // Filter out low-level JS TypeError/ReferenceError/SyntaxError or fetch internals
    if (
      err.name === "TypeError" ||
      err.name === "SyntaxError" ||
      err.name === "ReferenceError" ||
      /is not a function|cannot read prop|undefined is not|failed to fetch|networkerror/i.test(err.message)
    ) {
      console.error("[Kinetiq Client Error]", err);
      return fallback;
    }
    return err.message || fallback;
  }
  return fallback;
}

export interface OffsetPagination {
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

export interface OffsetPage<T> {
  items: T[];
  pagination: OffsetPagination;
}

export interface CursorPagination {
  limit: number;
  next_cursor: string | null;
  has_more: boolean;
}

export interface CursorPage<T> {
  items: T[];
  pagination: CursorPagination;
}

/** Accepts the {items, pagination} envelope or a legacy bare array. */
export function itemsOf<T>(data: OffsetPage<T> | CursorPage<T> | T[] | null | undefined): T[] {
  if (Array.isArray(data)) return data;
  return data?.items ?? [];
}

// ponytail: CRUD lists fetch one max-size page (100) and expose items only; add page controls when a list can exceed 100.
const MAX_PAGE = 100;

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const url = `${API_BASE_URL}${path}`;
  const headers = new Headers(options.headers || {});
  if (!headers.has("Content-Type") && options.body && typeof options.body === "string") {
    headers.set("Content-Type", "application/json");
  }

  let res: Response;
  try {
    res = await fetch(url, {
      ...options,
      headers,
      credentials: "include",
    });
  } catch (networkErr: any) {
    if (networkErr?.name === "AbortError" || options.signal?.aborted) {
      throw networkErr;
    }
    console.error("[Kinetiq API Network Error]", networkErr);
    throw new ApiError(0, "Unable to reach Kinetiq server. Please check your connection.");
  }

  const contentType = res.headers.get("content-type") || "";
  let data: any = null;
  if (contentType.includes("application/json")) {
    data = await res.json().catch(() => null);
  }

  if (res.status === 401 && typeof window !== "undefined" && !location.pathname.startsWith("/login")) {
    location.assign(loginUrl(location.pathname, location.search));
  }

  if (!res.ok) {
    const errorCode = data?.error?.code;
    const rawMsg = data?.error?.message || data?.message;
    const safeMsg =
      res.status >= 500
        ? "Service is temporarily unavailable. Please try again shortly."
        : rawMsg || `Request could not be completed (${res.status}).`;
    throw new ApiError(res.status, safeMsg, errorCode);
  }

  return (data?.data !== undefined ? data.data : data) as T;
}

export const api = {
  getProjects: (signal?: AbortSignal) =>
    request<OffsetPage<Project> | Project[]>(`/projects?limit=${MAX_PAGE}`, { signal }).then(itemsOf),

  getProject: (id: string, signal?: AbortSignal) =>
    request<Project>(`/projects/${id}`, { signal }),

  createProject: (name: string) =>
    request<CreateProjectResponse>("/projects", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),

  updateProject: (projectId: string, payload: UpdateProjectPayload) =>
    request<Project>(`/projects/${projectId}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  deleteProject: (projectId: string) =>
    request<{ deleted: boolean; id: string }>(`/projects/${projectId}`, {
      method: "DELETE",
    }),

  purgeEvents: (projectId: string) =>
    request<{ purged_count?: number; purged_events?: number }>(`/projects/${projectId}/purge-events`, {
      method: "POST",
    }),

  rotateKey: (projectId: string) =>
    request<RotateKeyResponse>(`/projects/${projectId}/rotate-key`, {
      method: "POST",
    }),

  listApiKeys: (projectId: string, signal?: AbortSignal) =>
    request<OffsetPage<ProjectApiKey> | ProjectApiKey[]>(`/projects/${projectId}/keys?limit=${MAX_PAGE}`, { signal }).then(itemsOf),

  createApiKey: (projectId: string, name: string, environment: "live" | "test" = "live") =>
    request<CreateApiKeyResponse>(`/projects/${projectId}/keys`, {
      method: "POST",
      body: JSON.stringify({ name, environment }),
    }),

  revokeApiKey: (projectId: string, keyId: string) =>
    request<{ revoked: boolean; key_id?: string; id?: string }>(`/projects/${projectId}/keys/${keyId}`, {
      method: "DELETE",
    }),

  sendTestEvent: async (
    apiKey: string,
    eventName: string = "page_view",
    properties: Record<string, any> = { path: "/onboarding-test", source: "kinetiq-dashboard" }
  ) => {
    const res = await fetch(`${API_BASE_URL}/ingest`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${apiKey}`,
      },
      body: JSON.stringify({
        events: [
          {
            event_id: `evt_test_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
            event_name: eventName,
            distinct_id: "usr_onboarding_tester",
            occurred_at: new Date().toISOString(),
            properties,
          },
        ],
      }),
    });
    if (!res.ok) {
      throw new Error("Could not ingest test event. Verify the API key is active.");
    }
    return res.json();
  },

  getSummary: (projectId: string, from: string, to: string, signal?: AbortSignal, extra?: QueryExtras) =>
    request<SummaryData>(
      `/projects/${projectId}/analytics/summary?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}${extrasToQuery(extra)}`,
      { signal }
    ),

  getTopEvents: (projectId: string, from: string, to: string, limit: number = 10, signal?: AbortSignal, extra?: QueryExtras) =>
    request<TopEventsResponse | TopEventItem[]>(
      `/projects/${projectId}/analytics/top-events?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&limit=${limit}${extrasToQuery(extra)}`,
      { signal }
    ),

  getVolume: (projectId: string, from: string, to: string, interval: string = "hour", signal?: AbortSignal, extra?: QueryExtras) =>
    request<VolumeData>(
      `/projects/${projectId}/analytics/volume?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&interval=${interval}${extrasToQuery(extra)}`,
      { signal }
    ),

  getFunnel: (projectId: string, from: string, to: string, steps: string[], signal?: AbortSignal, extra?: QueryExtras, opts?: FunnelOptions) =>
    request<FunnelData>(
      `/projects/${projectId}/analytics/funnel?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&steps=${encodeURIComponent(steps.join(","))}${extrasToQuery(extra)}${funnelOptionsToQuery(opts)}`,
      { signal }
    ),

  getFunnelDropoff: (
    projectId: string,
    from: string,
    to: string,
    steps: string[],
    step: number,
    offset: number,
    limit: number,
    signal?: AbortSignal,
    extra?: QueryExtras,
    opts?: FunnelOptions
  ) =>
    request<FunnelDropoffData>(
      `/projects/${projectId}/analytics/funnel/dropoff?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&steps=${encodeURIComponent(steps.join(","))}&step=${step}&offset=${offset}&limit=${limit}${extrasToQuery({ filter: extra?.filter })}${funnelOptionsToQuery(opts)}`,
      { signal }
    ),

  getPropertyBreakdown: (
    projectId: string,
    eventName: string,
    property: string,
    from: string,
    to: string,
    limit: number = 10,
    signal?: AbortSignal,
    extra?: QueryExtras
  ) =>
    request<BreakdownItem[]>(
      `/projects/${projectId}/analytics/breakdown?event_name=${encodeURIComponent(eventName)}&property=${encodeURIComponent(property)}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&limit=${limit}${extrasToQuery(extra)}`,
      { signal }
    ),

  getRetention: (
    projectId: string,
    from: string,
    to: string,
    firstEvent: string = "signup",
    returnEvent: string = "app_opened",
    granularity: "day" | "week" = "day",
    signal?: AbortSignal,
    extra?: QueryExtras
  ) =>
    request<RetentionData>(
      `/projects/${projectId}/analytics/retention?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&first_event=${encodeURIComponent(firstEvent)}&return_event=${encodeURIComponent(returnEvent)}&granularity=${granularity}${extrasToQuery(extra)}`,
      { signal }
    ),

  exportEventsCsv: async (projectId: string, from: string, to: string, signal?: AbortSignal): Promise<Blob> => {
    const res = await fetch(
      `${API_BASE_URL}/projects/${projectId}/export?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
      {
        credentials: "include",
        signal,
      }
    );
    if (!res.ok) {
      throw new Error("Could not export CSV right now. Please try again.");
    }
    return res.blob();
  },

  listAlertRules: (projectId: string, signal?: AbortSignal) =>
    request<OffsetPage<AlertRule> | AlertRule[]>(`/projects/${projectId}/alerts?limit=${MAX_PAGE}`, { signal }).then(itemsOf),

  createAlertRule: (projectId: string, payload: CreateAlertRulePayload) =>
    request<AlertRule>(`/projects/${projectId}/alerts`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  updateAlertRule: (projectId: string, ruleId: string, payload: Partial<CreateAlertRulePayload>) =>
    request<AlertRule>(`/projects/${projectId}/alerts/${ruleId}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),

  toggleAlertRule: (projectId: string, ruleId: string, isActive: boolean) =>
    request<AlertRule>(`/projects/${projectId}/alerts/${ruleId}`, {
      method: "PUT",
      body: JSON.stringify({ is_active: isActive }),
    }),

  deleteAlertRule: (projectId: string, ruleId: string) =>
    request<{ deleted: boolean; id: string }>(`/projects/${projectId}/alerts/${ruleId}`, {
      method: "DELETE",
    }),

  testAlertRule: (projectId: string, ruleId: string) =>
    request<{ tested: boolean; id: string }>(`/projects/${projectId}/alerts/${ruleId}/test`, {
      method: "POST",
    }),

  snoozeAlertRule: (projectId: string, ruleId: string, durationHours: number) =>
    request<AlertRule>(`/projects/${projectId}/alerts/${ruleId}/snooze`, {
      method: "POST",
      body: JSON.stringify({ duration_hours: durationHours }),
    }),

  getProjectQuota: (projectId: string, signal?: AbortSignal) =>
    request<ProjectQuota>(`/projects/${projectId}/quota`, { signal }),

  // Phase 9: Custom Insight Widgets
  listInsights: (projectId: string, signal?: AbortSignal) =>
    request<OffsetPage<ProjectInsight> | ProjectInsight[]>(`/projects/${projectId}/insights?limit=${MAX_PAGE}`, { signal }).then(itemsOf),

  createInsight: (projectId: string, payload: CreateInsightPayload) =>
    request<ProjectInsight>(`/projects/${projectId}/insights`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  updateInsight: (projectId: string, insightId: string, payload: Partial<CreateInsightPayload>) =>
    request<ProjectInsight>(`/projects/${projectId}/insights/${insightId}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),

  deleteInsight: (projectId: string, insightId: string) =>
    request<{ message: string }>(`/projects/${projectId}/insights/${insightId}`, {
      method: "DELETE",
    }),

  // Phase 10: Automated Intelligence & Digest
  getWeeklyDigest: (projectId: string, signal?: AbortSignal) =>
    request<WeeklyDigestReport>(`/projects/${projectId}/intelligence/digest`, { signal }),

  checkAnomaly: (projectId: string, eventName: string, signal?: AbortSignal) =>
    request<AnomalyResult>(`/projects/${projectId}/intelligence/anomaly?event_name=${encodeURIComponent(eventName)}`, { signal }),

  // Phase 15: User Explorer & Activity Timeline
  listAnalyticsUsers: (
    projectId: string,
    search?: string,
    limit: number = 50,
    page: number = 1,
    signal?: AbortSignal
  ) => {
    const params = new URLSearchParams();
    if (search) params.set("search", search);
    params.set("limit", Math.min(limit, MAX_PAGE).toString());
    params.set("page", page.toString());
    return request<OffsetPage<UserExplorerItem> | UserExplorerItem[]>(
      `/projects/${projectId}/analytics/users?${params.toString()}`,
      { signal }
    ).then(itemsOf);
  },

  getUserTimeline: (
    projectId: string,
    distinctId: string,
    limit: number = 100,
    cursor?: string | null,
    signal?: AbortSignal
  ) => {
    const params = new URLSearchParams();
    params.set("limit", limit.toString());
    if (cursor) params.set("cursor", cursor);
    return request<CursorPage<UserTimelineItem>>(
      `/projects/${projectId}/analytics/users/${encodeURIComponent(distinctId)}/timeline?${params.toString()}`,
      { signal }
    );
  },

  // Phase 18: Workspace Audit Trail & Activity Logs
  listWorkspaceAuditLogs: (
    workspaceId?: string,
    action?: string,
    targetType?: string,
    search?: string,
    page: number = 1,
    limit: number = 50,
    signal?: AbortSignal
  ) => {
    const params = new URLSearchParams();
    if (workspaceId) params.set("workspace_id", workspaceId);
    if (action) params.set("action", action);
    if (targetType) params.set("target_type", targetType);
    if (search) params.set("search", search);
    params.set("page", page.toString());
    params.set("limit", limit.toString());
    return request<ListAuditLogsResponse>(`/workspace/audit-logs?${params.toString()}`, { signal });
  },

  // Phase 16: Device & Geo Breakdowns
  getDeviceBreakdown: (projectId: string, from: string, to: string, signal?: AbortSignal) =>
    request<DeviceBreakdownItem[]>(
      `/projects/${projectId}/analytics/breakdown/devices?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
      { signal }
    ),

  getGeoBreakdown: (projectId: string, from: string, to: string, limit: number = 10, signal?: AbortSignal) =>
    request<GeoBreakdownItem[]>(
      `/projects/${projectId}/analytics/breakdown/geo?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&limit=${limit}`,
      { signal }
    ),

  getSessions: (projectId: string, from: string, to: string, limit: number = 50, signal?: AbortSignal) =>
    request<SessionsResponse>(
      `/projects/${projectId}/analytics/sessions?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&limit=${limit}`,
      { signal }
    ),

  listCatalogEvents: (projectId: string, search = "", signal?: AbortSignal) =>
    request<CatalogEvent[]>(`/projects/${projectId}/catalog/events?search=${encodeURIComponent(search)}`, { signal }),

  listCatalogProperties: (projectId: string, scope: "event" | "person" = "event", search = "", signal?: AbortSignal) =>
    request<CatalogProperty[]>(
      `/projects/${projectId}/catalog/properties?scope=${scope}&search=${encodeURIComponent(search)}`,
      { signal }
    ),

  listCatalogValues: (projectId: string, property: string, scope: "event" | "person" = "event", search = "", signal?: AbortSignal) =>
    request<CatalogValue[]>(
      `/projects/${projectId}/catalog/property-values?property=${encodeURIComponent(property)}&scope=${scope}&search=${encodeURIComponent(search)}`,
      { signal }
    ),
};

export interface SessionCount {
  value: string;
  count: number;
}

export interface SessionsResponse {
  total_sessions: number;
  avg_duration_seconds: number;
  top_entry_pages: SessionCount[];
  top_referring_domains: SessionCount[];
}

export interface CheckoutRequest {
  plan: "growth" | "enterprise" | string;
  provider: "midtrans" | "stripe" | string;
  organization_id?: string;
  success_redirect?: string;
}

export interface CheckoutResponse {
  order_id: string;
  provider: string;
  plan: string;
  amount: number;
  currency: string;
  payment_url: string;
  snap_token?: string;
}

export interface PaymentTransaction {
  id: string;
  organization_id?: string | null;
  user_id: string;
  order_id: string;
  provider: string;
  plan: string;
  amount: number;
  currency: string;
  status: "pending" | "settled" | "paid" | "expired" | "failed" | string;
  payment_url: string;
  created_at: string;
  updated_at: string;
}

export interface ProjectQuota {
  project_id: string;
  plan: string;
  billing_provider: string;
  monthly_limit: number;
  used_this_month: number;
  remaining: number;
  usage_pct: number;
  reset_time: string;
}

export const billingApi = {
  createCheckout: (payload: CheckoutRequest) =>
    request<CheckoutResponse>("/billing/checkout", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  listTransactions: (organizationId?: string, signal?: AbortSignal) => {
    const query = organizationId ? `?organization_id=${encodeURIComponent(organizationId)}` : "";
    const sep = query ? "&" : "?";
    return request<OffsetPage<PaymentTransaction> | PaymentTransaction[]>(
      `/billing/transactions${query}${sep}limit=${MAX_PAGE}`,
      { signal }
    ).then(itemsOf);
  },

  getProjectQuota: (projectId: string, signal?: AbortSignal) =>
    request<ProjectQuota>(`/projects/${projectId}/quota`, { signal }),
};

export interface DeadLetterItem {
  id: number;
  project_id?: string | null;
  project_name?: string | null;
  event_id?: string | null;
  reason: string;
  payload: any;
  attempts: number;
  status: "failed" | "replayed";
  failed_at: string;
  replayed_at?: string | null;
}

export interface DLQSummary {
  total_items: number;
  pending_items: number;
  replayed_items: number;
}

export interface DLQResponse extends CursorPage<DeadLetterItem> {
  summary: DLQSummary;
}

export const dlqApi = {
  listDeadLetters: (limit = 50, cursor?: string | null, signal?: AbortSignal) =>
    request<DLQResponse>(
      `/ops/dlq?limit=${limit}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`,
      { signal }
    ),

  replayDeadLetter: (id: number) =>
    request<{ replayed: boolean; id: number }>(`/ops/dlq/replay/${id}`, {
      method: "POST",
    }),

  replayAllDeadLetters: () =>
    request<{ count: number; status: string }>("/ops/dlq/replay-all", {
      method: "POST",
    }),

  purgeDeadLetter: (id: number) =>
    request<{ purged: boolean; id: number }>(`/ops/dlq/${id}`, {
      method: "DELETE",
    }),

  purgeAllDeadLetters: () =>
    request<{ purged: boolean }>("/ops/dlq", {
      method: "DELETE",
    }),
};

export interface PlatformOverview {
  total_users: number;
  total_organizations: number;
  total_projects: number;
  total_events: number;
  total_dead_letters: number;
  live_stream_lag: number;
  active_workers_count: number;
  database_stats: {
    acquired_conns: number;
    idle_conns: number;
    total_conns: number;
    max_conns: number;
  };
  system_health: string;
  timestamp: string;
}

export interface TenantDetail {
  organization_id: string;
  organization_name: string;
  slug: string;
  plan: string;
  monthly_event_quota: number;
  current_month_usage: number;
  usage_percentage: number;
  total_projects: number;
  owner_user_id?: string;
  owner_email?: string;
  two_factor_enabled?: boolean;
  created_at: string;
}

export interface FinancialTransaction {
  id: string;
  organization_id?: string | null;
  organization_name?: string | null;
  user_id: string;
  order_id: string;
  provider: string;
  plan: string;
  amount: number;
  currency: string;
  status: string;
  created_at: string;
}

export interface FinancialSummary {
  total_revenue_idr: number;
  total_revenue_usd: number;
  paid_transactions_count: number;
  pending_transactions_count: number;
  recent_transactions: FinancialTransaction[];
}

export interface AuditLogEntry {
  id: string;
  actor_id: string;
  actor_email: string;
  action: string;
  target_type: string;
  target_id: string;
  details: Record<string, any>;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
}

export interface PlatformStaffMember {
  id: string;
  name: string;
  email: string;
  image?: string | null;
  isPlatformStaff: boolean;
  staffRole: "superadmin" | "ops" | "finance" | "support" | string;
  platformRoleId?: string | null;
  roleName?: string | null;
  roleDescription?: string | null;
  createdAt: string;
}

export interface GranularPermission {
  id?: string;
  resource: string;
  action: string;
}

export interface CustomRole {
  id: string;
  organizationId?: string;
  name: string;
  description?: string | null;
  isSystem: boolean;
  createdAt: string;
  permissions: GranularPermission[];
}

const AUTH_BASE_URL =
  (typeof import.meta !== "undefined" && import.meta.env?.VITE_AUTH_URL) ||
  "http://localhost:3001";

export const rbacApi = {
  // Ops Staff Roles & Granular Permissions
  listOpsRoles: async () => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/roles`, { credentials: "include" });
    if (!res.ok) throw new Error("Failed to list platform roles");
    return res.json() as Promise<{ roles: CustomRole[] }>;
  },
  createOpsRole: async (payload: { name: string; description?: string; permissions: GranularPermission[] }) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/roles`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to create platform role");
    }
    return res.json();
  },
  updateOpsRole: async (roleId: string, payload: { name?: string; description?: string; permissions?: GranularPermission[] }) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/roles/${roleId}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to update platform role");
    }
    return res.json();
  },
  deleteOpsRole: async (roleId: string) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/roles/${roleId}`, {
      method: "DELETE",
      credentials: "include",
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to delete platform role");
    }
    return res.json();
  },

  // Workspace / Tenant Custom Roles
  listWorkspaceRoles: async () => {
    const res = await fetch(`${AUTH_BASE_URL}/api/workspace/roles`, { credentials: "include" });
    if (!res.ok) throw new Error("Failed to list workspace roles");
    return res.json() as Promise<{ roles: CustomRole[] }>;
  },
  createWorkspaceRole: async (payload: { name: string; description?: string; permissions: GranularPermission[] }) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/workspace/roles`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to create workspace role");
    }
    return res.json();
  },
  updateWorkspaceRole: async (roleId: string, payload: { name?: string; description?: string; permissions?: GranularPermission[] }) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/workspace/roles/${roleId}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to update workspace role");
    }
    return res.json();
  },
  deleteWorkspaceRole: async (roleId: string) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/workspace/roles/${roleId}`, {
      method: "DELETE",
      credentials: "include",
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to delete workspace role");
    }
    return res.json();
  },
  updateMemberRole: async (memberId: string, payload: { role?: string; customRoleId?: string | null }) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/workspace/members/${memberId}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to update member role");
    }
    return res.json();
  },
};

export const superadminApi = {
  getOverview: (signal?: AbortSignal) =>
    request<PlatformOverview>("/ops/overview", { signal }),

  listTenants: (limit = 50, page = 1, signal?: AbortSignal) =>
    request<OffsetPage<TenantDetail>>(`/ops/tenants?limit=${Math.min(limit, MAX_PAGE)}&page=${page}`, { signal }),

  overrideTenantQuota: (orgId: string, payload: { monthly_event_quota: number; plan: string }) =>
    request<{ updated: boolean; organization_id: string }>(`/ops/tenants/${orgId}/quota`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }),

  setProjectStatus: (projectId: string, payload: { status: "active" | "paused"; reason: string }) =>
    request<{ updated: boolean; project_id: string; status: string }>(`/ops/projects/${projectId}/status`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }),

  getFinancials: (limit = 50, signal?: AbortSignal) =>
    request<FinancialSummary>(`/ops/financials?limit=${limit}`, { signal }),

  reconcileTransaction: (txId: string, status: string) =>
    request<{ reconciled: boolean; transaction_id: string; status: string }>(
      `/ops/financials/reconcile/${txId}`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status }),
      }
    ),

  listAuditLogs: (limit = 50, page = 1, signal?: AbortSignal) =>
    request<OffsetPage<AuditLogEntry>>(`/ops/audit-logs?limit=${Math.min(limit, MAX_PAGE)}&page=${page}`, { signal }),

  listStaff: async () => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/staff`, {
      credentials: "include",
    });
    if (!res.ok) throw new Error("Failed to list staff");
    return res.json() as Promise<{ staff: PlatformStaffMember[] }>;
  },

  grantStaff: async (email: string, staffRole: string) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/staff/grant`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, staffRole }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to grant staff role");
    }
    return res.json();
  },

  revokeStaff: async (userId: string) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/staff/revoke`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ userId }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to revoke staff role");
    }
    return res.json();
  },

  resetUser2FA: async (userId: string, reason: string) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/users/${userId}/reset-2fa`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ reason }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to reset 2FA");
    }
    return res.json() as Promise<{ status: string; userId: string; twoFactorReset: boolean; message: string }>;
  },

  impersonateUser: async (userId: string) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/impersonate`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ userId }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to initiate impersonation");
    }
    return res.json() as Promise<{
      status: string;
      impersonating: boolean;
      targetUser: { id: string; name: string; email: string };
      actor: { id: string; email: string };
    }>;
  },

  stopImpersonating: async (targetUserId?: string) => {
    const res = await fetch(`${AUTH_BASE_URL}/api/ops/stop-impersonating`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ targetUserId }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err?.error?.message || "Failed to conclude impersonation");
    }
    return res.json() as Promise<{ status: string; stopped: boolean }>;
  },
};

export interface ProjectInsight {
  id: string;
  project_id: string;
  title: string;
  chart_type: "metric" | "bar" | "line" | "donut";
  event_name: string;
  aggregation: "count" | "unique_users" | "sum_property" | "avg_property" | "min_property" | "max_property";
  property_key?: string;
  computed_value: number;
  created_at: string;
  updated_at: string;
}

export interface CreateInsightPayload {
  title: string;
  chart_type: "metric" | "bar" | "line" | "donut";
  event_name: string;
  aggregation: "count" | "unique_users" | "sum_property" | "avg_property" | "min_property" | "max_property";
  property_key?: string;
}

export interface WeeklyDigestTopEvent {
  event_name: string;
  count: number;
}

export interface WeeklyDigestReport {
  project_id: string;
  project_name: string;
  total_events: number;
  unique_users: number;
  top_events?: WeeklyDigestTopEvent[];
  conversion_rate: number;
  period_start: string;
  period_end: string;
}

export interface AnomalyResult {
  project_id: string;
  event_name: string;
  current_hourly: number;
  baseline_hourly: number;
  drop_percentage: number;
  is_anomaly: boolean;
  evaluated_at: string;
}

export interface UserExplorerItem {
  distinct_id: string;
  first_seen_at: string;
  last_seen_at: string;
  total_events: number;
}

export interface UserTimelineItem {
  id: string;
  name: string;
  distinct_id: string;
  properties: Record<string, any>;
  occurred_at: string;
}

export interface WorkspaceAuditLogItem {
  id: string;
  workspace_id: string;
  project_id?: string;
  actor_id: string;
  actor_name?: string;
  actor_email?: string;
  action: string;
  target_type: string;
  target_id: string;
  metadata: Record<string, any>;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
}

export type ListAuditLogsResponse = OffsetPage<WorkspaceAuditLogItem>;

export interface ProjectStorageSync {
  id?: string;
  project_id?: string;
  provider: "s3" | "r2" | "gcs" | "minio";
  bucket_name: string;
  endpoint_url?: string;
  region: string;
  access_key_id: string;
  secret_access_key?: string;
  prefix_path: string;
  frequency: "hourly" | "daily";
  is_enabled: boolean;
  last_sync_at?: string | null;
  last_sync_status?: "idle" | "syncing" | "success" | "failed";
  last_error_message?: string | null;
}

export const storageSyncApi = {
  get: (projectId: string) => request<ProjectStorageSync | null>(`/projects/${projectId}/storage-sync`),
  put: (projectId: string, payload: Partial<ProjectStorageSync>) =>
    request<ProjectStorageSync>(`/projects/${projectId}/storage-sync`, {
      method: "PUT",
      body: JSON.stringify(payload)
    }),
  test: (projectId: string, payload: Partial<ProjectStorageSync>) =>
    request<{ success: boolean; message: string }>(`/projects/${projectId}/storage-sync/test`, {
      method: "POST",
      body: JSON.stringify(payload)
    })
};

export interface DeviceBreakdownItem {
  device_type: string;
  count: number;
}

export interface GeoBreakdownItem {
  country_code: string;
  count: number;
}





