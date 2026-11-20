<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import {
    api,
    type Project,
    type UserExplorerItem,
    type UserTimelineItem,
  } from "$lib/api";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Card from "$lib/components/ui/Card.svelte";
  import JsonViewer from "$lib/components/ui/JsonViewer.svelte";
  import {
    Search,
    Users,
    Clock,
    Activity,
    ChevronRight,
    RefreshCw,
    X,
    Calendar,
    Hash,
    Layers,
    UserCheck,
    ArrowUpDown
  } from "lucide-svelte";

  const projectId = $derived(page.params.projectId as string);

  let project = $state<Project | null>(null);
  let users = $state<UserExplorerItem[]>([]);
  let loading = $state(true);
  let refreshing = $state(false);
  let error = $state("");

  // Search & Pagination
  let searchQuery = $state("");
  let searchDebounceTimeout: any = null;

  // Selected User Slide-over Timeline
  let selectedUser = $state<UserExplorerItem | null>(null);
  let timeline = $state<UserTimelineItem[]>([]);
  let timelineLoading = $state(false);
  let timelineError = $state("");
  let expandedTimelineEvents = $state<Record<string, boolean>>({});

  onMount(async () => {
    try {
      project = await api.getProject(projectId);
    } catch {
      // Non-blocking
    }
    await loadUsers();
  });

  async function loadUsers(isRefresh = false) {
    if (isRefresh) refreshing = true;
    else loading = true;
    error = "";

    try {
      users = await api.listAnalyticsUsers(projectId, searchQuery.trim() || undefined, 100, 0);
    } catch (err: any) {
      error = err.message || "Failed to load users";
    } finally {
      loading = false;
      refreshing = false;
    }
  }

  function handleSearchInput(e: Event) {
    const target = e.target as HTMLInputElement;
    searchQuery = target.value;
    if (searchDebounceTimeout) clearTimeout(searchDebounceTimeout);
    searchDebounceTimeout = setTimeout(() => {
      loadUsers();
    }, 300);
  }

  async function selectUser(user: UserExplorerItem) {
    selectedUser = user;
    timeline = [];
    expandedTimelineEvents = {};
    timelineLoading = true;
    timelineError = "";

    try {
      timeline = await api.getUserTimeline(projectId, user.distinct_id, 100);
    } catch (err: any) {
      timelineError = err.message || "Failed to load user activity timeline";
    } finally {
      timelineLoading = false;
    }
  }

  function closeDrawer() {
    selectedUser = null;
    timeline = [];
    expandedTimelineEvents = {};
  }

  function toggleEventProperties(id: string) {
    expandedTimelineEvents[id] = !expandedTimelineEvents[id];
  }

  function formatDate(iso: string): string {
    if (!iso) return "—";
    try {
      const d = new Date(iso);
      return d.toLocaleString(undefined, {
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

<div class="space-y-6">
  <!-- Header Bar -->
  <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
    <div>
      <div class="flex items-center gap-2.5 flex-wrap">
        <h1 class="text-2xl sm:text-3xl font-extrabold tracking-tight text-[var(--text-primary)]">
          User Explorer
        </h1>
        {#if project}
          <Badge variant="neutral">
            {project.name}
          </Badge>
        {/if}
      </div>
      <p class="text-xs text-[var(--text-muted)] mt-1">
        Browse unique distinct users, analyze total activity volume, and inspect individual event journeys.
      </p>
    </div>

    <div class="flex items-center gap-2.5">
      <Button
        variant="secondary"
        size="md"
        onclick={() => loadUsers(true)}
        loading={refreshing}
        title="Refresh users directory"
      >
        <RefreshCw class="w-3.5 h-3.5 text-[var(--text-secondary)] {refreshing ? 'animate-spin' : ''}" />
        <span>Refresh</span>
      </Button>
    </div>
  </div>

  <!-- Search & Filter Controls -->
  <Card class="p-4 bg-[var(--bg-surface)] border-[var(--border-subtle)]">
    <div class="flex flex-col sm:flex-row items-center justify-between gap-3">
      <div class="relative w-full sm:max-w-md">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-muted)]" />
        <input
          type="text"
          value={searchQuery}
          oninput={handleSearchInput}
          placeholder="Search by distinct_id prefix..."
          class="w-full pl-9 pr-4 py-2 rounded-xl text-xs bg-[var(--bg-canvas)] border border-[var(--border-subtle)] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-primary)] transition-all font-mono"
        />
      </div>

      <div class="flex items-center gap-2 text-xs text-[var(--text-muted)] self-start sm:self-center font-medium">
        <Users class="w-3.5 h-3.5" />
        <span>Showing <strong>{users.length}</strong> identified users</span>
      </div>
    </div>
  </Card>

  {#if error}
    <div class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs font-medium text-rose-600 dark:text-rose-400 flex items-center justify-between">
      <span>{error}</span>
      <Button variant="danger" size="sm" onclick={() => loadUsers(true)}>Try Again</Button>
    </div>
  {/if}

  <!-- Users Table Card -->
  <Card class="overflow-hidden border-[var(--border-subtle)] shadow-[var(--shadow-bento)] p-0">
    <div class="overflow-x-auto">
      <table class="w-full text-left border-collapse text-xs">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--bg-surface-muted)]/50 text-[var(--text-muted)] font-semibold uppercase tracking-wider text-[10px]">
            <th class="py-3 px-4">Distinct ID</th>
            <th class="py-3 px-4">Total Events</th>
            <th class="py-3 px-4">First Seen</th>
            <th class="py-3 px-4">Last Active</th>
            <th class="py-3 px-4 text-right">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[var(--border-subtle)]">
          {#if loading}
            <tr>
              <td colspan="5" class="py-12 text-center text-[var(--text-muted)]">
                <div class="flex flex-col items-center justify-center gap-2">
                  <div class="w-6 h-6 rounded-full border-2 border-emerald-500/20 border-t-[var(--brand-primary)] animate-spin"></div>
                  <span>Loading user directory...</span>
                </div>
              </td>
            </tr>
          {:else if users.length === 0}
            <tr>
              <td colspan="5" class="py-12 text-center text-[var(--text-muted)]">
                <div class="flex flex-col items-center justify-center gap-2 max-w-sm mx-auto">
                  <div class="w-10 h-10 rounded-full bg-[var(--bg-surface-muted)] flex items-center justify-center text-[var(--text-muted)]">
                    <UserCheck class="w-5 h-5" />
                  </div>
                  <span class="font-semibold text-sm text-[var(--text-primary)]">No users found</span>
                  <span class="text-xs">
                    {searchQuery ? `No distinct user matches query "${searchQuery}".` : "No tracked users recorded for this project yet."}
                  </span>
                </div>
              </td>
            </tr>
          {:else}
            {#each users as u}
              <tr
                onclick={() => selectUser(u)}
                class="hover:bg-[var(--bg-surface-muted)]/60 cursor-pointer transition-colors group {selectedUser?.distinct_id === u.distinct_id ? 'bg-[var(--brand-soft)]/40' : ''}"
              >
                <td class="py-3 px-4 font-mono font-medium text-[var(--text-primary)]">
                  <div class="flex items-center gap-2">
                    <div class="w-7 h-7 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center font-bold text-xs uppercase">
                      {u.distinct_id.charAt(0)}
                    </div>
                    <span class="hover:underline">{u.distinct_id}</span>
                  </div>
                </td>
                <td class="py-3 px-4">
                  <Badge variant="neutral">
                    <Activity class="w-3 h-3 mr-1 text-[var(--brand-primary)]" />
                    {u.total_events.toLocaleString()} events
                  </Badge>
                </td>
                <td class="py-3 px-4 text-[var(--text-muted)]">
                  {formatDate(u.first_seen_at)}
                </td>
                <td class="py-3 px-4">
                  <span class="text-[var(--text-secondary)] font-medium">
                    {formatRelativeTime(u.last_seen_at)}
                  </span>
                </td>
                <td class="py-3 px-4 text-right">
                  <button
                    onclick={(e) => {
                      e.stopPropagation();
                      selectUser(u);
                    }}
                    class="inline-flex items-center gap-1 text-[11px] font-semibold text-[var(--brand-primary)] hover:underline"
                  >
                    <span>Timeline</span>
                    <ChevronRight class="w-3.5 h-3.5 transition-transform group-hover:translate-x-0.5" />
                  </button>
                </td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </Card>
</div>

<!-- Activity Timeline Slide-over / Modal Drawer -->
{#if selectedUser}
  <div class="fixed inset-0 z-50 overflow-hidden bg-black/40 backdrop-blur-xs flex justify-end animate-in fade-in duration-200">
    <div
      class="w-full max-w-xl bg-[var(--bg-surface)] h-full shadow-2xl border-l border-[var(--border-subtle)] flex flex-col animate-in slide-in-from-right duration-300"
    >
      <!-- Drawer Header -->
      <div class="p-5 border-b border-[var(--border-subtle)] flex items-center justify-between bg-[var(--bg-surface-muted)]/40">
        <div class="flex items-center gap-3 overflow-hidden">
          <div class="w-10 h-10 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center font-extrabold text-sm uppercase shrink-0">
            {selectedUser.distinct_id.charAt(0)}
          </div>
          <div class="overflow-hidden">
            <h2 class="text-sm font-bold text-[var(--text-primary)] truncate font-mono">
              {selectedUser.distinct_id}
            </h2>
            <div class="flex items-center gap-2 text-[11px] text-[var(--text-muted)]">
              <span>{selectedUser.total_events.toLocaleString()} total events</span>
              <span>•</span>
              <span>Active {formatRelativeTime(selectedUser.last_seen_at)}</span>
            </div>
          </div>
        </div>

        <button
          onclick={closeDrawer}
          class="p-2 rounded-xl text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)] transition-colors cursor-pointer"
          title="Close drawer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <!-- Drawer Content: Timeline Feed -->
      <div class="flex-1 overflow-y-auto p-5 space-y-4">
        {#if timelineLoading}
          <div class="py-16 flex flex-col items-center justify-center gap-3 text-[var(--text-muted)] text-xs">
            <div class="w-6 h-6 rounded-full border-2 border-emerald-500/20 border-t-[var(--brand-primary)] animate-spin"></div>
            <span>Fetching activity timeline...</span>
          </div>
        {:else if timelineError}
          <div class="p-4 rounded-xl border border-rose-500/30 bg-rose-500/10 text-xs text-rose-600 dark:text-rose-400">
            {timelineError}
          </div>
        {:else if timeline.length === 0}
          <div class="py-16 text-center text-[var(--text-muted)] text-xs">
            No events found for this user.
          </div>
        {:else}
          <div class="relative pl-6 space-y-6 before:absolute before:left-2.5 before:top-2 before:bottom-2 before:w-0.5 before:bg-[var(--border-subtle)]">
            {#each timeline as item}
              <div class="relative group">
                <!-- Dot indicator -->
                <div class="absolute -left-6 top-1 w-2.5 h-2.5 rounded-full bg-[var(--brand-primary)] ring-4 ring-[var(--bg-surface)]"></div>

                <div class="bg-[var(--bg-canvas)] border border-[var(--border-subtle)] rounded-xl p-3.5 space-y-2.5 hover:border-[var(--brand-primary)]/50 transition-colors">
                  <div class="flex items-start justify-between gap-2">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="font-bold text-xs text-[var(--text-primary)] font-mono">
                        {item.name}
                      </span>
                      <span class="text-[10px] text-[var(--text-muted)] font-mono">
                        {item.id.slice(0, 8)}
                      </span>
                    </div>

                    <span class="text-[10px] text-[var(--text-muted)] shrink-0 font-medium">
                      {formatRelativeTime(item.occurred_at)}
                    </span>
                  </div>

                  <div class="text-[11px] text-[var(--text-muted)] flex items-center gap-1.5">
                    <Clock class="w-3 h-3" />
                    <span>{formatDate(item.occurred_at)}</span>
                  </div>

                  {#if item.properties && Object.keys(item.properties).length > 0}
                    <div class="pt-2 border-t border-[var(--border-subtle)]">
                      <button
                        onclick={() => toggleEventProperties(item.id)}
                        class="text-[11px] font-medium text-[var(--brand-primary)] hover:underline flex items-center gap-1 cursor-pointer"
                      >
                        <span>{expandedTimelineEvents[item.id] ? "Hide Properties" : "View Properties"}</span>
                        <span>({Object.keys(item.properties).length})</span>
                      </button>

                      {#if expandedTimelineEvents[item.id]}
                        <div class="mt-2 animate-in fade-in duration-150">
                          <JsonViewer value={item.properties} />
                        </div>
                      {/if}
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Drawer Footer -->
      <div class="p-4 border-t border-[var(--border-subtle)] bg-[var(--bg-surface-muted)]/40 flex justify-between items-center text-xs text-[var(--text-muted)]">
        <span>Showing latest {timeline.length} events</span>
        <Button variant="secondary" size="sm" onclick={closeDrawer}>Done</Button>
      </div>
    </div>
  </div>
{/if}
