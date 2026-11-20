<script lang="ts">
  import Card from "$lib/components/ui/Card.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import { PieChart, RefreshCw } from "lucide-svelte";
  import type { BreakdownItem } from "$lib/api";

  interface Props {
    eventName: string;
    propertyName: string;
    breakdown: BreakdownItem[];
    loading?: boolean;
    onFetchBreakdown: (eventName: string, propertyName: string) => void;
  }

  let {
    eventName = $bindable("page_view"),
    propertyName = $bindable("browser"),
    breakdown = [],
    loading = false,
    onFetchBreakdown
  }: Props = $props();

  function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    onFetchBreakdown(eventName.trim(), propertyName.trim());
  }

  const totalCount = $derived(
    breakdown.reduce((acc, curr) => acc + curr.count, 0) || 1
  );

  const commonProperties = ["browser", "os", "device", "plan", "country", "referrer"];
</script>

<Card class="space-y-4">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
    <div class="flex items-center gap-2.5">
      <div class="w-8 h-8 rounded-full bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)] flex items-center justify-center text-[var(--primary-emerald)]">
        <PieChart class="w-4 h-4" />
      </div>
      <div>
        <h2 class="text-sm font-semibold text-[var(--text-primary)]">Property Breakdown</h2>
        <p class="text-[11px] text-[var(--text-muted)]">Distribution by JSON event payload properties</p>
      </div>
    </div>
    <Badge variant="neutral">{breakdown.length} values</Badge>
  </div>

  <form onsubmit={handleSubmit} class="p-3.5 rounded-2xl bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)] space-y-3">
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <div class="space-y-1">
        <label for="breakdown-event" class="text-[11px] font-medium text-[var(--text-muted)]">Event Name</label>
        <input
          id="breakdown-event"
          type="text"
          bind:value={eventName}
          placeholder="e.g. page_view"
          class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--primary-emerald)] transition-colors"
          required
        />
      </div>

      <div class="space-y-1">
        <label for="breakdown-prop" class="text-[11px] font-medium text-[var(--text-muted)]">Property Key</label>
        <input
          id="breakdown-prop"
          type="text"
          bind:value={propertyName}
          placeholder="e.g. browser"
          class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--primary-emerald)] transition-colors"
          required
        />
      </div>
    </div>

    <div class="flex items-center justify-between flex-wrap gap-2 pt-1">
      <div class="flex items-center gap-1.5 flex-wrap">
        <span class="text-[11px] text-[var(--text-muted)]">Presets:</span>
        {#each commonProperties as prop}
          <button
            type="button"
            onclick={() => {
              propertyName = prop;
              onFetchBreakdown(eventName.trim(), prop);
            }}
            class="px-2.5 py-0.5 rounded-full text-[11px] font-medium transition-colors cursor-pointer {propertyName === prop ? 'bg-[var(--primary-emerald)] text-white' : 'bg-[var(--bg-surface)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] border border-[var(--border-subtle)]'}"
          >
            {prop}
          </button>
        {/each}
      </div>

      <Button type="submit" variant="secondary" size="sm" {loading}>
        <RefreshCw class="w-3.5 h-3.5 {loading ? 'animate-spin' : ''}" />
        Run Breakdown
      </Button>
    </div>
  </form>

  {#if loading}
    <div class="py-8 text-center text-xs text-[var(--text-muted)] animate-pulse">
      Menghitung distribusi properti...
    </div>
  {:else if breakdown.length === 0}
    <div class="py-8 text-center text-xs text-[var(--text-muted)]">
      Belum ada data untuk event "{eventName}" pada properti "{propertyName}".
    </div>
  {:else}
    <div class="space-y-3 pt-1">
      {#each breakdown as item}
        {@const pct = (item.count / totalCount) * 100}
        <div class="space-y-1.5 group">
          <div class="flex items-center justify-between text-xs">
            <span class="font-medium text-[var(--text-primary)] group-hover:text-[var(--primary-emerald)] transition-colors truncate max-w-[240px]">
              {item.property_value || "(empty / null)"}
            </span>
            <div class="flex items-center gap-2">
              <span class="text-[var(--text-primary)] font-bold">{item.count.toLocaleString()}</span>
              <span class="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-[var(--primary-tint)] text-[var(--primary-emerald)]">
                {pct.toFixed(1)}%
              </span>
            </div>
          </div>
          <div class="w-full h-2 rounded-full bg-[var(--bg-surface-elevated)] overflow-hidden">
            <div
              class="h-full rounded-full bg-[var(--primary-emerald)] transition-all duration-300"
              style="width: {pct}%;"
            ></div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</Card>
