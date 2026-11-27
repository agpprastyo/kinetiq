<script lang="ts">
  import { onMount } from "svelte";
  import { systemStatusApi, type SystemStatusResponse } from "$lib/api";
  import { healthBadgeClasses, queueHealth } from "$lib/system-status";

  let status = $state<SystemStatusResponse | null>(null);
  let loading = $state(true);
  let error = $state("");
  let lastUpdated = $state<string | null>(null);
  let autoRefresh = $state(true);
  let refreshIntervalSec = $state<5 | 10>(5);

  async function fetchStatus() {
    loading = true;
    error = "";
    try {
      status = await systemStatusApi.get();
      lastUpdated = new Date().toLocaleTimeString();
    } catch (err: any) {
      error = err?.message || "Failed to load system status";
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    fetchStatus();
  });

  $effect(() => {
    if (!autoRefresh) return;
    const id = setInterval(() => {
      fetchStatus();
    }, refreshIntervalSec * 1000);
    return () => clearInterval(id);
  });
</script>

<div class="max-w-5xl mx-auto px-4 sm:px-6 py-8 space-y-6">
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
    <div>
      <h1 class="text-2xl font-bold text-[var(--text-primary)] tracking-tight">System Health & Queue Status</h1>
      <p class="text-sm text-[var(--text-secondary)] mt-1">
        Real-time infrastructure health for PostgreSQL, Redis, worker queue, and ingestion rate.
        {#if lastUpdated}
          <span class="ml-1 text-xs opacity-75">(Last updated: {lastUpdated})</span>
        {/if}
      </p>
    </div>

    <div class="flex items-center gap-3">
      <label class="flex items-center gap-2 text-xs text-[var(--text-secondary)] cursor-pointer select-none">
        <input type="checkbox" bind:checked={autoRefresh} class="rounded border-[var(--border-subtle)]" />
        Auto-refresh
      </label>

      <select
        bind:value={refreshIntervalSec}
        disabled={!autoRefresh}
        class="px-2.5 py-1.5 text-xs rounded-lg bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-[var(--text-primary)] disabled:opacity-50"
      >
        <option value={5}>5s</option>
        <option value={10}>10s</option>
      </select>

      <button
        type="button"
        onclick={() => fetchStatus()}
        disabled={loading}
        class="px-3.5 py-1.5 text-xs font-medium rounded-lg bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)] transition-colors disabled:opacity-50 cursor-pointer"
      >
        {loading ? "Refreshing..." : "Refresh"}
      </button>
    </div>
  </div>

  {#if error}
    <div class="p-4 rounded-xl border border-red-500/30 bg-red-500/10 text-red-500 text-sm">
      <p class="font-semibold">Unable to fetch system status</p>
      <p class="mt-1 opacity-90">{error}</p>
    </div>
  {/if}

  {#if status}
    {@const qHealth = queueHealth(status.queue.pending_pel_count, status.queue.consumer_lag)}
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <!-- PostgreSQL Card -->
      <div class="p-5 rounded-2xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] space-y-3">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-[var(--text-primary)]">PostgreSQL</h2>
          <span class="px-2.5 py-0.5 text-xs font-medium rounded-full border {healthBadgeClasses(status.postgres.status)}">
            {status.postgres.status}
          </span>
        </div>
        <div class="grid grid-cols-2 gap-3 pt-1">
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Latency</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.postgres.latency_ms} ms</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Open Connections</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.postgres.open_connections}</div>
          </div>
        </div>
      </div>

      <!-- Redis Card -->
      <div class="p-5 rounded-2xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] space-y-3">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-[var(--text-primary)]">Redis</h2>
          <span class="px-2.5 py-0.5 text-xs font-medium rounded-full border {healthBadgeClasses(status.redis.status)}">
            {status.redis.status}
          </span>
        </div>
        <div class="grid grid-cols-2 gap-3 pt-1">
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Latency</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.redis.latency_ms} ms</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Used Memory</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.redis.used_memory_human}</div>
          </div>
        </div>
      </div>

      <!-- Worker Queue / PEL Card -->
      <div class="p-5 rounded-2xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] space-y-3">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-[var(--text-primary)]">Worker Queue & PEL</h2>
          <span class="px-2.5 py-0.5 text-xs font-medium rounded-full border {healthBadgeClasses(qHealth)}">
            {qHealth}
          </span>
        </div>
        <div class="grid grid-cols-3 gap-3 pt-1">
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Stream Length</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.queue.stream_length}</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Pending (PEL)</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.queue.pending_pel_count}</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Consumer Lag</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.queue.consumer_lag}</div>
          </div>
        </div>
      </div>

      <!-- Ingestion Rate Card -->
      <div class="p-5 rounded-2xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] space-y-3">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-[var(--text-primary)]">Ingestion Rate</h2>
          <span class="px-2.5 py-0.5 text-xs font-medium rounded-full border {healthBadgeClasses('healthy')}">
            live
          </span>
        </div>
        <div class="grid grid-cols-2 gap-3 pt-1">
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Events / sec (1m)</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.ingestion.events_per_sec_1m}</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-secondary)]">Total Events (24h)</div>
            <div class="text-lg font-semibold text-[var(--text-primary)]">{status.ingestion.events_total_24h.toLocaleString()}</div>
          </div>
        </div>
      </div>
    </div>
  {:else if loading}
    <div class="p-8 text-center text-sm text-[var(--text-secondary)]">Loading system status...</div>
  {/if}
</div>
