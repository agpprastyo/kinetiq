<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { page } from "$app/state";
  import { api, type Project } from "$lib/api";
  import { connectLiveStream, type LiveEvent } from "$lib/live-stream";
  import JsonViewer from "$lib/components/ui/JsonViewer.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Card from "$lib/components/ui/Card.svelte";
  import {
    Radio,
    Pause,
    Play,
    Trash2,
    Search,
    ChevronDown,
    ChevronRight,
    ArrowLeft,
    Copy,
    Check,
    Terminal,
    Activity
  } from "lucide-svelte";

  const projectId = $derived(page.params.projectId as string);

  let project = $state<Project | null>(null);
  let events = $state<LiveEvent[]>([]);
  let isPaused = $state(false);
  let filterText = $state("");
  let disconnectFn: (() => void) | null = null;
  let expandedRows = $state<Record<string, boolean>>({});

  // Developer Tooling: Copied visual feedback states
  let copiedId = $state<string | null>(null);
  let copiedCurlId = $state<string | null>(null);

  // Rate Metric: Sliding window count for events received in the last 60 seconds
  let eventTimestamps = $state<number[]>([]);
  let eventsPerMinute = $state(0);
  let rateInterval: any = null;

  let filteredEvents = $derived(
    events.filter((e) => {
      const query = filterText.trim().toLowerCase();
      if (!query) return true;
      const matchName = e.name.toLowerCase().includes(query);
      const matchDistinct = e.distinct_id ? e.distinct_id.toLowerCase().includes(query) : false;
      const matchProps = e.properties ? JSON.stringify(e.properties).toLowerCase().includes(query) : false;
      return matchName || matchDistinct || matchProps;
    })
  );

  onMount(async () => {
    try {
      project = await api.getProject(projectId);
    } catch {
      // Non-blocking project info load
    }

    startStream();

    rateInterval = setInterval(() => {
      const now = Date.now();
      eventTimestamps = eventTimestamps.filter((t) => now - t <= 60000);
      eventsPerMinute = eventTimestamps.length;
    }, 1000);
  });

  onDestroy(() => {
    if (disconnectFn) {
      disconnectFn();
    }
    if (rateInterval) {
      clearInterval(rateInterval);
    }
  });

  function startStream() {
    if (disconnectFn) {
      disconnectFn();
    }
    disconnectFn = connectLiveStream(projectId, (evt) => {
      if (!isPaused) {
        events = [evt, ...events.slice(0, 199)];
        eventTimestamps = [...eventTimestamps, Date.now()];
      }
    });
  }

  function togglePause() {
    isPaused = !isPaused;
  }

  function clearEvents() {
    events = [];
    expandedRows = {};
    eventTimestamps = [];
    eventsPerMinute = 0;
  }

  function toggleRow(id: string) {
    expandedRows[id] = !expandedRows[id];
  }

  async function copyRawJson(evt: LiveEvent, id: string, e: MouseEvent) {
    e.stopPropagation();
    const jsonStr = JSON.stringify(
      {
        event_name: evt.name,
        distinct_id: evt.distinct_id,
        properties: evt.properties || {},
        timestamp: evt.timestamp,
      },
      null,
      2
    );
    await navigator.clipboard.writeText(jsonStr);
    copiedId = id;
    setTimeout(() => {
      if (copiedId === id) copiedId = null;
    }, 2000);
  }

  async function copyAsCurl(evt: LiveEvent, id: string, e: MouseEvent) {
    e.stopPropagation();
    const payload = JSON.stringify({
      event_name: evt.name,
      distinct_id: evt.distinct_id || undefined,
      properties: evt.properties || {},
    });
    const curl = `curl -X POST http://localhost:8080/api/v1/projects/${projectId}/events \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer <YOUR_API_KEY>" \\\n  -d '${payload}'`;
    await navigator.clipboard.writeText(curl);
    copiedCurlId = id;
    setTimeout(() => {
      if (copiedCurlId === id) copiedCurlId = null;
    }, 2000);
  }
</script>

<div class="space-y-6">
  <!-- Top Navigation & Title Bar -->
  <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
    <div>
      <a
        href="/app/projects/{projectId}"
        class="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--brand-primary)] transition-colors"
      >
        <ArrowLeft class="w-3.5 h-3.5" />
        Back to Dashboard
      </a>
      <h1 class="text-2xl font-extrabold tracking-tight text-[var(--text-primary)] flex items-center gap-2 mt-1">
        Live Activity Feed
        {#if project}
          <span class="text-[var(--text-muted)] font-normal text-base">— {project.name}</span>
        {/if}
      </h1>
      <p class="text-xs text-[var(--text-secondary)] mt-0.5">
        Watch visitor actions arrive in real time as people use your website or app.
      </p>
    </div>

    <!-- Live Status Pill & Action Buttons -->
    <div class="flex items-center gap-2.5">
      <!-- Live Throughput Rate Gauge -->
      <div class="flex items-center gap-1.5 px-3 py-1 rounded-full bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-secondary)]">
        <Activity class="w-3.5 h-3.5 text-emerald-500 animate-pulse" />
        <span>Rate: <strong class="text-[var(--text-primary)]">{eventsPerMinute}</strong>/min</span>
      </div>

      <Badge variant={isPaused ? "amber" : "emerald"} pulse={!isPaused}>
        {isPaused ? "PAUSED" : "LIVE"}
      </Badge>

      <Button
        variant="secondary"
        size="md"
        onclick={togglePause}
        title={isPaused ? "Resume live feed" : "Pause live feed"}
      >
        {#if isPaused}
          <Play class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
          Resume
        {:else}
          <Pause class="w-3.5 h-3.5 text-amber-500" />
          Pause
        {/if}
      </Button>

      <Button
        variant="secondary"
        size="md"
        onclick={clearEvents}
        title="Clear activity list"
      >
        <Trash2 class="w-3.5 h-3.5 text-[var(--text-muted)] hover:text-rose-500" />
        Clear
      </Button>
    </div>
  </div>

  <!-- Search Filter Toolbar -->
  <div class="flex items-center justify-between gap-4">
    <div class="relative flex-1 max-w-md">
      <Search class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-muted)]" />
      <input
        type="text"
        bind:value={filterText}
        placeholder="Search by activity name or user..."
        class="w-full pl-10 pr-4 py-2 rounded-full bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-primary)] shadow-[var(--shadow-pill)] transition-colors"
      />
    </div>

    <div class="text-xs font-mono text-[var(--text-muted)]">
      Showing: <span class="text-[var(--text-primary)] font-bold">{filteredEvents.length}</span> / {events.length}
    </div>
  </div>

  <!-- Live Feed Stream Table -->
  <Card class="p-0 overflow-hidden">
    {#if filteredEvents.length === 0}
      <div class="flex flex-col items-center justify-center py-20 text-center space-y-3">
        <div class="w-11 h-11 rounded-full bg-[var(--brand-soft)] flex items-center justify-center text-[var(--brand-primary)]">
          <Radio class="w-5 h-5 animate-pulse" />
        </div>
        <div>
          <h3 class="text-sm font-bold text-[var(--text-primary)]">Waiting for live activity...</h3>
          <p class="text-xs text-[var(--text-secondary)] mt-1 max-w-sm">
            As soon as someone interacts with your app—or when you click "Send Test Event" in Project Settings—you will see it appear here live.
          </p>
        </div>
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs font-mono">
          <thead class="border-b border-[var(--border-subtle)] bg-[var(--bg-surface-muted)] text-[11px] uppercase tracking-wider text-[var(--text-muted)]">
            <tr>
              <th class="w-8 px-4 py-3"></th>
              <th class="px-4 py-3">Time</th>
              <th class="px-4 py-3">Activity</th>
              <th class="px-4 py-3">User / Visitor</th>
              <th class="px-4 py-3">Details</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[var(--border-subtle)]">
            {#each filteredEvents as evt, i (evt.event_id || evt.timestamp + evt.name + i)}
              {@const rowId = evt.event_id || `${evt.timestamp}-${i}`}
              {@const isExpanded = !!expandedRows[rowId]}
              <tr
                class="hover:bg-[var(--bg-surface-muted)] transition-colors cursor-pointer select-none group"
                onclick={() => toggleRow(rowId)}
              >
                <td class="px-4 py-2.5 text-[var(--text-muted)]">
                  {#if isExpanded}
                    <ChevronDown class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                  {:else}
                    <ChevronRight class="w-3.5 h-3.5" />
                  {/if}
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-[var(--text-muted)]">
                  {new Date(evt.timestamp).toLocaleTimeString([], { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" })}
                </td>
                <td class="px-4 py-2.5">
                  <Badge variant="emerald">{evt.name}</Badge>
                </td>
                <td class="px-4 py-2.5 text-[var(--text-primary)]">
                  {evt.distinct_id || evt.user_id || "-"}
                </td>
                <td class="px-4 py-2.5 text-[var(--text-muted)] truncate max-w-md">
                  {JSON.stringify(evt.properties || {})}
                </td>
                <td class="px-4 py-2.5 text-right whitespace-nowrap">
                  <div class="inline-flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                    <button
                      type="button"
                      title="Copy Raw JSON"
                      onclick={(e) => copyRawJson(evt, rowId, e)}
                      class="p-1 rounded-md text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface)] border border-transparent hover:border-[var(--border-subtle)] transition-all cursor-pointer"
                    >
                      {#if copiedId === rowId}
                        <Check class="w-3.5 h-3.5 text-emerald-500" />
                      {:else}
                        <Copy class="w-3.5 h-3.5" />
                      {/if}
                    </button>
                    <button
                      type="button"
                      title="Copy as cURL"
                      onclick={(e) => copyAsCurl(evt, rowId, e)}
                      class="p-1 rounded-md text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface)] border border-transparent hover:border-[var(--border-subtle)] transition-all cursor-pointer"
                    >
                      {#if copiedCurlId === rowId}
                        <Check class="w-3.5 h-3.5 text-emerald-500" />
                      {:else}
                        <Terminal class="w-3.5 h-3.5" />
                      {/if}
                    </button>
                  </div>
                </td>
              </tr>

              {#if isExpanded}
                <tr class="bg-[var(--bg-surface-muted)]/60 border-t border-[var(--border-subtle)]">
                  <td colspan="6" class="px-6 py-4">
                    <div class="p-3.5 rounded-2xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] space-y-2">
                      <div class="flex items-center justify-between text-[11px] text-[var(--text-muted)] pb-1 border-b border-[var(--border-subtle)]">
                        <span>Activity Details</span>
                        <div class="flex items-center gap-3">
                          <button
                            type="button"
                            onclick={(e) => copyRawJson(evt, rowId, e)}
                            class="inline-flex items-center gap-1 text-[var(--brand-primary)] hover:underline cursor-pointer"
                          >
                            {#if copiedId === rowId}
                              <Check class="w-3 h-3 text-emerald-500" />
                              <span>Copied JSON</span>
                            {:else}
                              <Copy class="w-3 h-3" />
                              <span>Copy JSON</span>
                            {/if}
                          </button>
                          <span>•</span>
                          <button
                            type="button"
                            onclick={(e) => copyAsCurl(evt, rowId, e)}
                            class="inline-flex items-center gap-1 text-[var(--brand-primary)] hover:underline cursor-pointer"
                          >
                            {#if copiedCurlId === rowId}
                              <Check class="w-3 h-3 text-emerald-500" />
                              <span>Copied cURL</span>
                            {:else}
                              <Terminal class="w-3 h-3" />
                              <span>Copy cURL</span>
                            {/if}
                          </button>
                          <span>•</span>
                          <span>ID: {evt.event_id || "-"}</span>
                        </div>
                      </div>
                      <JsonViewer
                        value={{
                          event_id: evt.event_id,
                          name: evt.name,
                          distinct_id: evt.distinct_id,
                          timestamp: evt.timestamp,
                          properties: evt.properties
                        }}
                      />
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </Card>
</div>
