<script lang="ts">
  import {
    Plus,
    Trash2,
    BarChart2,
    PieChart,
    Activity,
    Layers,
    Sparkles,
    Loader2
  } from "lucide-svelte";
  import type { ProjectInsight, CreateInsightPayload } from "$lib/api";
  import Button from "$lib/components/ui/Button.svelte";
  import Card from "$lib/components/ui/Card.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";

  interface Props {
    insights: ProjectInsight[];
    loading?: boolean;
    onAddInsight: () => void;
    onDeleteInsight: (id: string) => Promise<void>;
  }

  let {
    insights = [],
    loading = false,
    onAddInsight,
    onDeleteInsight
  }: Props = $props();

  let deletingId = $state<string | null>(null);

  async function handleDelete(id: string) {
    if (confirm("Hapus custom insight widget ini?")) {
      deletingId = id;
      try {
        await onDeleteInsight(id);
      } finally {
        deletingId = null;
      }
    }
  }

  function getChartIcon(type: string) {
    switch (type) {
      case "bar":
        return BarChart2;
      case "donut":
        return PieChart;
      case "line":
        return Activity;
      default:
        return Layers;
    }
  }

  function formatAggLabel(agg: string, prop?: string) {
    switch (agg) {
      case "unique_users":
        return "Unique Users";
      case "sum_property":
        return `Sum (${prop || "amount"})`;
      case "avg_property":
        return `Avg (${prop || "amount"})`;
      case "min_property":
        return `Min (${prop || "amount"})`;
      case "max_property":
        return `Max (${prop || "amount"})`;
      default:
        return "Total Events (Count)";
    }
  }
</script>

<div class="space-y-4">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
    <div>
      <div class="flex items-center gap-2">
        <h3 class="text-base sm:text-lg font-bold tracking-tight text-[var(--text-primary)]">
          Custom Insights & Saved Charts
        </h3>
        <Badge variant="emerald">Live Widgets</Badge>
      </div>
      <p class="text-xs text-[var(--text-muted)] mt-0.5">
        Widget metrik terkustomisasi yang dihitung otomatis dari event stream
      </p>
    </div>

    <Button variant="primary" size="sm" onclick={onAddInsight}>
      <Plus class="w-4 h-4 mr-1.5" />
      <span>Add Custom Insight</span>
    </Button>
  </div>

  {#if loading}
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      {#each [1, 2, 3, 4] as _}
        <div class="h-32 rounded-2xl bg-[var(--bg-surface-muted)] animate-pulse border border-[var(--border-subtle)]"></div>
      {/each}
    </div>
  {:else if insights.length === 0}
    <div class="rounded-2xl border border-dashed border-[var(--border-subtle)] p-8 text-center bg-[var(--bg-surface-elevated)]/40">
      <div class="w-12 h-12 rounded-full bg-emerald-500/10 text-emerald-500 mx-auto flex items-center justify-center mb-3">
        <Sparkles class="w-6 h-6" />
      </div>
      <h4 class="text-sm font-bold text-[var(--text-primary)]">Belum ada custom insight tersimpan</h4>
      <p class="text-xs text-[var(--text-muted)] max-w-sm mx-auto mt-1 mb-4">
        Buat visualisasi metrik khusus seperti total revenue, checkout count, atau active users per event.
      </p>
      <Button variant="outline" size="sm" onclick={onAddInsight}>
        <Plus class="w-4 h-4 mr-1.5" />
        Buat Widget Pertama
      </Button>
    </div>
  {:else}
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      {#each insights as item (item.id)}
        {@const Icon = getChartIcon(item.chart_type)}
        <Card class="relative overflow-hidden group hover:border-[var(--brand-primary)]/40 transition-all p-4 flex flex-col justify-between">
          <div class="space-y-2">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
                <Icon class="w-3.5 h-3.5 text-emerald-500" />
                <span class="capitalize font-medium">{item.chart_type}</span>
              </div>
              <button
                type="button"
                aria-label="Delete insight widget"
                onclick={() => handleDelete(item.id)}
                disabled={deletingId === item.id}
                class="opacity-0 group-hover:opacity-100 p-1 text-[var(--text-muted)] hover:text-rose-500 hover:bg-rose-500/10 rounded-lg transition-all cursor-pointer"
              >
                {#if deletingId === item.id}
                  <Loader2 class="w-3.5 h-3.5 animate-spin" />
                {:else}
                  <Trash2 class="w-3.5 h-3.5" />
                {/if}
              </button>
            </div>

            <div>
              <h4 class="text-sm font-bold text-[var(--text-primary)] truncate" title={item.title}>
                {item.title}
              </h4>
              <p class="text-[11px] text-[var(--text-muted)] flex items-center gap-1 mt-0.5">
                <span class="font-mono text-emerald-600 dark:text-emerald-400 font-semibold">{item.event_name}</span>
                <span>•</span>
                <span>{formatAggLabel(item.aggregation, item.property_key)}</span>
              </p>
            </div>
          </div>

          <div class="mt-4 pt-3 border-t border-[var(--border-subtle)]/60 flex items-baseline justify-between">
            <span class="text-2xl font-extrabold tracking-tight text-[var(--text-primary)] font-mono">
              {item.computed_value ? item.computed_value.toLocaleString() : "0"}
            </span>
            <span class="text-[10px] text-[var(--text-muted)]">Real-time aggregate</span>
          </div>
        </Card>
      {/each}
    </div>
  {/if}
</div>
