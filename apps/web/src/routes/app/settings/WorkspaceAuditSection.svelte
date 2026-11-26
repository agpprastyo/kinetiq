<script lang="ts">
  import { onMount } from "svelte";
  import { api, type WorkspaceAuditLogItem } from "$lib/api";
  import Card from "$lib/components/ui/Card.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import JsonViewer from "$lib/components/ui/JsonViewer.svelte";
  import {
    ShieldCheck,
    Search,
    RefreshCw,
    Filter,
    Clock,
    User,
    Key,
    UserPlus,
    UserMinus,
    Trash2,
    Settings,
    X,
    ChevronRight,
    Globe,
    Layers
  } from "lucide-svelte";

  interface Props {
    workspaceId?: string;
  }

  let { workspaceId }: Props = $props();

  let auditLogs = $state<WorkspaceAuditLogItem[]>([]);
  let total = $state(0);
  let page = $state(1);
  let limit = 50;
  let loading = $state(true);
  let refreshing = $state(false);
  let error = $state("");

  // Filters
  let selectedAction = $state("");
  let selectedTargetType = $state("");
  let searchQuery = $state("");
  let searchTimeout: any = null;

  // Selected Log Drawer Detail
  let selectedLog = $state<WorkspaceAuditLogItem | null>(null);

  onMount(() => {
    loadAuditLogs();
  });

  async function loadAuditLogs(isRefresh = false) {
    if (isRefresh) refreshing = true;
    else loading = true;
    error = "";

    try {
      const res = await api.listWorkspaceAuditLogs(
        workspaceId,
        selectedAction || undefined,
        selectedTargetType || undefined,
        searchQuery.trim() || undefined,
        page,
        limit
      );
      auditLogs = res.items || [];
      total = res.pagination?.total ?? 0;
    } catch (err: any) {
      error = err.message || "Unable to fetch workspace audit logs.";
    } finally {
      loading = false;
      refreshing = false;
    }
  }

  function handleSearchInput(e: Event) {
    const target = e.target as HTMLInputElement;
    searchQuery = target.value;
    if (searchTimeout) clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => {
      page = 1;
      loadAuditLogs();
    }, 300);
  }

  function handleFilterChange() {
    page = 1;
    loadAuditLogs();
  }

  function getActionBadgeVariant(action: string): "emerald" | "amber" | "rose" | "indigo" | "neutral" {
    if (action.includes("create") || action.includes("invite")) return "emerald";
    if (action.includes("delete") || action.includes("revoke") || action.includes("remove")) return "rose";
    if (action.includes("update")) return "amber";
    return "indigo";
  }

  function getActionIcon(action: string) {
    if (action.startsWith("api_key")) return Key;
    if (action.startsWith("member.invite")) return UserPlus;
    if (action.startsWith("member.remove")) return UserMinus;
    if (action.includes("delete")) return Trash2;
    return Settings;
  }

  function formatDate(iso: string): string {
    if (!iso) return "—";
    try {
      return new Date(iso).toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
    } catch {
      return iso;
    }
  }

  function formatRelativeTime(iso: string): string {
    if (!iso) return "—";
    try {
      const diff = Date.now() - new Date(iso).getTime();
      const mins = Math.floor(diff / 60000);
      if (mins < 1) return "Just now";
      if (mins < 60) return `${mins}m ago`;
      const hours = Math.floor(mins / 60);
      if (hours < 24) return `${hours}h ago`;
      const days = Math.floor(hours / 24);
      return `${days}d ago`;
    } catch {
      return iso;
    }
  }
</script>

<div class="space-y-5">
  <!-- Top Summary & Control Card -->
  <Card class="space-y-4">
    <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
      <div>
        <h2 class="text-base font-bold text-[var(--text-primary)] flex items-center gap-2">
          <ShieldCheck class="w-4 h-4 text-[var(--brand-primary)]" />
          Enterprise Audit Trail & Compliance Log
        </h2>
        <p class="text-xs text-[var(--text-secondary)] mt-0.5">
          Immutable append-only record of all security-sensitive actions across API keys, memberships, and project settings.
        </p>
      </div>

      <div class="flex items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          onclick={() => loadAuditLogs(true)}
          loading={refreshing}
          title="Refresh audit logs"
        >
          <RefreshCw class="w-3 h-3 text-[var(--text-secondary)] {refreshing ? 'animate-spin' : ''}" />
          <span>Refresh</span>
        </Button>
      </div>
    </div>

    <!-- Filters Bar -->
    <div class="grid grid-cols-1 sm:grid-cols-12 gap-3 pt-1">
      <div class="sm:col-span-5 relative">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-[var(--text-muted)]" />
        <input
          type="text"
          value={searchQuery}
          oninput={handleSearchInput}
          placeholder="Search by actor email, name, or target ID..."
          class="w-full pl-9 pr-4 py-2 rounded-xl text-xs bg-[var(--bg-canvas)] border border-[var(--border-subtle)] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-primary)]"
        />
      </div>

      <div class="sm:col-span-4">
        <select
          bind:value={selectedAction}
          onchange={handleFilterChange}
          class="w-full px-3 py-2 rounded-xl text-xs bg-[var(--bg-canvas)] border border-[var(--border-subtle)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        >
          <option value="">All Actions</option>
          <option value="project.create">project.create</option>
          <option value="project.delete">project.delete</option>
          <option value="api_key.create">api_key.create</option>
          <option value="api_key.revoke">api_key.revoke</option>
          <option value="member.invite">member.invite</option>
          <option value="member.remove">member.remove</option>
          <option value="member.role_update">member.role_update</option>
          <option value="storage_sync.update">storage_sync.update</option>
        </select>
      </div>

      <div class="sm:col-span-3">
        <select
          bind:value={selectedTargetType}
          onchange={handleFilterChange}
          class="w-full px-3 py-2 rounded-xl text-xs bg-[var(--bg-canvas)] border border-[var(--border-subtle)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        >
          <option value="">All Target Types</option>
          <option value="project">Project</option>
          <option value="api_key">API Key</option>
          <option value="member">Member</option>
          <option value="storage_sync">Storage Sync</option>
        </select>
      </div>
    </div>
  </Card>

  {#if error}
    <div class="p-4 rounded-xl border border-rose-500/30 bg-rose-500/10 text-xs text-rose-600 dark:text-rose-400 flex items-center justify-between">
      <span>{error}</span>
      <Button variant="danger" size="sm" onclick={() => loadAuditLogs(true)}>Try Again</Button>
    </div>
  {/if}

  <!-- Audit Logs Table -->
  <Card class="overflow-hidden p-0 border-[var(--border-subtle)]">
    <div class="overflow-x-auto">
      <table class="w-full text-left border-collapse text-xs">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--bg-surface-muted)]/50 text-[var(--text-muted)] font-semibold uppercase tracking-wider text-[10px]">
            <th class="py-3 px-4">Action</th>
            <th class="py-3 px-4">Actor</th>
            <th class="py-3 px-4">Target</th>
            <th class="py-3 px-4">IP / Client</th>
            <th class="py-3 px-4">Timestamp</th>
            <th class="py-3 px-4 text-right">Details</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[var(--border-subtle)]">
          {#if loading}
            <tr>
              <td colspan="6" class="py-12 text-center text-[var(--text-muted)]">
                <div class="flex flex-col items-center justify-center gap-2">
                  <div class="w-6 h-6 rounded-full border-2 border-emerald-500/20 border-t-[var(--brand-primary)] animate-spin"></div>
                  <span>Loading audit trail events...</span>
                </div>
              </td>
            </tr>
          {:else if auditLogs.length === 0}
            <tr>
              <td colspan="6" class="py-12 text-center text-[var(--text-muted)]">
                <div class="flex flex-col items-center justify-center gap-2 max-w-sm mx-auto">
                  <ShieldCheck class="w-8 h-8 text-[var(--text-muted)] opacity-60" />
                  <span class="font-semibold text-sm text-[var(--text-primary)]">No audit entries found</span>
                  <span class="text-xs">No sensitive operational events match your filter criteria.</span>
                </div>
              </td>
            </tr>
          {:else}
            {#each auditLogs as log}
              {@const Icon = getActionIcon(log.action)}
              <tr
                onclick={() => (selectedLog = log)}
                class="hover:bg-[var(--bg-surface-muted)]/60 cursor-pointer transition-colors group {selectedLog?.id === log.id ? 'bg-[var(--brand-soft)]/40' : ''}"
              >
                <td class="py-3 px-4">
                  <div class="flex items-center gap-2">
                    <Badge variant={getActionBadgeVariant(log.action)}>
                      <Icon class="w-3 h-3 mr-1" />
                      <span class="font-mono">{log.action}</span>
                    </Badge>
                  </div>
                </td>
                <td class="py-3 px-4">
                  <div>
                    <div class="font-semibold text-[var(--text-primary)]">{log.actor_email || log.actor_name || log.actor_id}</div>
                    {#if log.actor_name && log.actor_email}
                      <div class="text-[10px] text-[var(--text-muted)]">{log.actor_name}</div>
                    {/if}
                  </div>
                </td>
                <td class="py-3 px-4">
                  <div class="font-mono text-[11px] text-[var(--text-secondary)]">
                    <span class="font-semibold uppercase text-[10px] bg-[var(--bg-canvas)] px-1.5 py-0.5 rounded border border-[var(--border-subtle)] mr-1">
                      {log.target_type}
                    </span>
                    <span>{log.target_id.slice(0, 12)}...</span>
                  </div>
                </td>
                <td class="py-3 px-4 text-[var(--text-muted)] font-mono text-[11px]">
                  <div>{log.ip_address || "—"}</div>
                </td>
                <td class="py-3 px-4">
                  <div class="text-[var(--text-secondary)]">{formatRelativeTime(log.created_at)}</div>
                  <div class="text-[10px] text-[var(--text-muted)]">{formatDate(log.created_at)}</div>
                </td>
                <td class="py-3 px-4 text-right">
                  <button
                    onclick={(e) => {
                      e.stopPropagation();
                      selectedLog = log;
                    }}
                    class="inline-flex items-center gap-1 text-[11px] font-semibold text-[var(--brand-primary)] hover:underline"
                  >
                    <span>Inspect</span>
                    <ChevronRight class="w-3 h-3 transition-transform group-hover:translate-x-0.5" />
                  </button>
                </td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>

    <!-- Pagination Footer -->
    {#if total > limit}
      <div class="p-4 border-t border-[var(--border-subtle)] flex items-center justify-between text-xs text-[var(--text-muted)]">
        <span>Showing {auditLogs.length} of {total} total entries</span>
        <div class="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            disabled={page <= 1}
            onclick={() => {
              page--;
              loadAuditLogs();
            }}
          >
            Previous
          </Button>
          <span class="px-2 font-semibold text-[var(--text-primary)]">Page {page}</span>
          <Button
            variant="secondary"
            size="sm"
            disabled={page * limit >= total}
            onclick={() => {
              page++;
              loadAuditLogs();
            }}
          >
            Next
          </Button>
        </div>
      </div>
    {/if}
  </Card>
</div>

<!-- Slide-over Audit Detail Drawer -->
{#if selectedLog}
  <div class="fixed inset-0 z-50 overflow-hidden bg-black/40 backdrop-blur-xs flex justify-end animate-in fade-in duration-200">
    <div class="w-full max-w-lg bg-[var(--bg-surface)] h-full shadow-2xl border-l border-[var(--border-subtle)] flex flex-col animate-in slide-in-from-right duration-300">
      <div class="p-5 border-b border-[var(--border-subtle)] flex items-center justify-between bg-[var(--bg-surface-muted)]/40">
        <div class="flex items-center gap-2.5">
          <Badge variant={getActionBadgeVariant(selectedLog.action)}>
            <span class="font-mono">{selectedLog.action}</span>
          </Badge>
          <span class="text-xs font-mono text-[var(--text-muted)]">
            ID: {selectedLog.id.slice(0, 8)}
          </span>
        </div>
        <button
          onclick={() => (selectedLog = null)}
          class="p-2 rounded-xl text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)] transition-colors cursor-pointer"
          title="Close drawer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <div class="flex-1 overflow-y-auto p-5 space-y-4 text-xs">
        <div class="space-y-3 bg-[var(--bg-canvas)] p-4 rounded-xl border border-[var(--border-subtle)]">
          <div class="flex justify-between items-center">
            <span class="text-[var(--text-muted)]">Timestamp</span>
            <span class="font-mono text-[var(--text-primary)]">{formatDate(selectedLog.created_at)}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-[var(--text-muted)]">Actor Email</span>
            <span class="font-mono font-semibold text-[var(--text-primary)]">{selectedLog.actor_email || "—"}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-[var(--text-muted)]">Actor User ID</span>
            <span class="font-mono text-[var(--text-secondary)]">{selectedLog.actor_id}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-[var(--text-muted)]">Target Type</span>
            <span class="font-mono uppercase font-bold text-[var(--brand-primary)]">{selectedLog.target_type}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-[var(--text-muted)]">Target ID</span>
            <span class="font-mono text-[var(--text-secondary)]">{selectedLog.target_id}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-[var(--text-muted)]">Client IP</span>
            <span class="font-mono text-[var(--text-primary)]">{selectedLog.ip_address || "—"}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-[var(--text-muted)]">User-Agent</span>
            <span class="truncate max-w-[240px] text-[var(--text-muted)]" title={selectedLog.user_agent}>
              {selectedLog.user_agent || "—"}
            </span>
          </div>
        </div>

        <div>
          <span class="font-bold text-xs text-[var(--text-primary)] block mb-1.5">Action Metadata & Diff Payload</span>
          <JsonViewer value={selectedLog.metadata} />
        </div>
      </div>

      <div class="p-4 border-t border-[var(--border-subtle)] bg-[var(--bg-surface-muted)]/40 flex justify-end">
        <Button variant="secondary" size="sm" onclick={() => (selectedLog = null)}>Done</Button>
      </div>
    </div>
  </div>
{/if}
