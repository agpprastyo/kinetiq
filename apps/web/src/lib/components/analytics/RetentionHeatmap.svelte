<script lang="ts">
  import type { RetentionData } from "$lib/api";
  import { Users, Calendar, ArrowRight, RefreshCw } from "lucide-svelte";

  interface Props {
    data: RetentionData | null;
    loading: boolean;
    onRefresh: () => void;
  }

  let { data, loading, onRefresh }: Props = $props();

  function getRetentionColor(pct: number): string {
    if (pct >= 80) return "bg-emerald-600 text-white font-bold";
    if (pct >= 60) return "bg-emerald-500/80 text-white font-semibold";
    if (pct >= 40) return "bg-emerald-500/50 text-white";
    if (pct >= 20) return "bg-emerald-500/25 text-emerald-300";
    if (pct > 0) return "bg-emerald-500/10 text-emerald-400";
    return "bg-zinc-900/60 text-zinc-600";
  }
</script>

<div class="rounded-3xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-6 space-y-5 shadow-sm font-sans">
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-[var(--border-subtle)] pb-4">
    <div class="space-y-1">
      <div class="flex items-center gap-2">
        <div class="w-8 h-8 rounded-xl bg-emerald-500/10 flex items-center justify-center text-emerald-500">
          <Users class="w-4 h-4" />
        </div>
        <h3 class="text-sm font-bold text-[var(--text-primary)]">User Retention & Cohort Heatmap</h3>
      </div>
      <p class="text-xs text-[var(--text-muted)]">
        Tracks user return rate from <span class="font-medium text-emerald-500">"{data?.first_event || 'signup'}"</span> to <span class="font-medium text-emerald-500">"{data?.return_event || 'app_opened'}"</span> over time.
      </p>
    </div>

    <button
      onclick={onRefresh}
      disabled={loading}
      class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium border border-[var(--border-subtle)] bg-[var(--bg-canvas)] hover:border-emerald-500/40 text-[var(--text-secondary)] hover:text-emerald-500 transition-all cursor-pointer disabled:opacity-50 self-start sm:self-auto"
    >
      <RefreshCw class="w-3.5 h-3.5 {loading ? 'animate-spin' : ''}" />
      Refresh Cohorts
    </button>
  </div>

  {#if loading}
    <div class="py-12 flex flex-col items-center justify-center gap-2 text-zinc-500">
      <RefreshCw class="w-5 h-5 animate-spin text-emerald-500" />
      <span class="text-xs">Computing cohort retention matrix...</span>
    </div>
  {:else if !data || data.cohorts.length === 0}
    <div class="py-12 text-center text-xs text-[var(--text-muted)]">
      No cohort data recorded in the selected range. Send events with consistent <code class="px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-300">distinct_id</code> to generate retention matrices.
    </div>
  {:else}
    <div class="overflow-x-auto">
      <table class="w-full text-xs border-collapse font-mono">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] text-[10px] uppercase text-[var(--text-muted)]">
            <th class="py-2.5 px-3 text-left">Cohort</th>
            <th class="py-2.5 px-3 text-right">Users</th>
            {#each data.cohorts[0]?.periods || [] as p}
              <th class="py-2.5 px-2 text-center w-14">
                {data.granularity === 'week' ? `W${p.period}` : `D${p.period}`}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody class="divide-y divide-[var(--border-subtle)]">
          {#each data.cohorts as c}
            <tr class="hover:bg-zinc-500/5 transition-colors">
              <td class="py-2.5 px-3 text-[var(--text-primary)] font-medium whitespace-nowrap">
                {c.cohort_date}
              </td>
              <td class="py-2.5 px-3 text-right text-[var(--text-secondary)] font-bold">
                {c.total_users.toLocaleString()}
              </td>
              {#each c.periods as p}
                <td class="py-1 px-1">
                  <div
                    class="h-8 rounded-lg flex flex-col items-center justify-center text-[10px] transition-transform hover:scale-105 {getRetentionColor(p.percentage)}"
                    title={`Day ${p.period}: ${p.count} users (${p.percentage}%)`}
                  >
                    <span>{p.percentage > 0 ? `${p.percentage.toFixed(0)}%` : '-'}</span>
                  </div>
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
