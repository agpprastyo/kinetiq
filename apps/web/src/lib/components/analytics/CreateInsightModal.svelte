<script lang="ts">
  import { X, Sparkles, Loader2 } from "lucide-svelte";
  import type { CreateInsightPayload } from "$lib/api";
  import Button from "$lib/components/ui/Button.svelte";

  interface Props {
    open: boolean;
    availableEvents: string[];
    onClose: () => void;
    onSubmit: (payload: CreateInsightPayload) => Promise<void>;
  }

  let {
    open = false,
    availableEvents = [],
    onClose,
    onSubmit
  }: Props = $props();

  let title = $state("");
  let chartType = $state<"metric" | "bar" | "line" | "donut">("metric");
  let eventName = $state("");
  let aggregation = $state<"count" | "unique_users" | "sum_property" | "avg_property" | "min_property" | "max_property">("count");
  let propertyKey = $state("");
  let submitting = $state(false);
  let error = $state("");

  $effect(() => {
    if (open) {
      title = "";
      chartType = "metric";
      eventName = availableEvents[0] || "page_view";
      aggregation = "count";
      propertyKey = "";
      error = "";
    }
  });

  const isNumericPropertyAggregation = (agg: string) =>
    ["sum_property", "avg_property", "min_property", "max_property"].includes(agg);

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (!title.trim()) {
      error = "Judul insight harus diisi";
      return;
    }
    if (!eventName.trim()) {
      error = "Event name harus dipilih atau diisi";
      return;
    }

    submitting = true;
    error = "";
    try {
      await onSubmit({
        title: title.trim(),
        chart_type: chartType,
        event_name: eventName.trim(),
        aggregation,
        property_key: isNumericPropertyAggregation(aggregation) ? (propertyKey.trim() || "amount") : undefined,
      });
      onClose();
    } catch (err: any) {
      error = err?.message || "Gagal menyimpan insight widget";
    } finally {
      submitting = false;
    }
  }
</script>

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-200">
    <div class="relative w-full max-w-lg rounded-3xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] shadow-2xl p-6 sm:p-8 space-y-6">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-full bg-emerald-500/10 text-emerald-500 flex items-center justify-center">
            <Sparkles class="w-4 h-4" />
          </div>
          <h3 class="text-lg font-bold text-[var(--text-primary)]">Custom Insight Widget Builder</h3>
        </div>
        <button
          type="button"
          aria-label="Tutup modal"
          onclick={onClose}
          class="p-1.5 rounded-xl text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)] transition-colors cursor-pointer"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      {#if error}
        <div class="p-3 text-xs rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-500">
          {error}
        </div>
      {/if}

      <form onsubmit={handleSubmit} class="space-y-4">
        <div>
          <label for="insight-title" class="block text-xs font-semibold text-[var(--text-secondary)] mb-1">
            Judul Widget
          </label>
          <input
            id="insight-title"
            type="text"
            bind:value={title}
            placeholder="Contoh: Total Checkouts, Active Buyers"
            required
            class="w-full px-3.5 py-2.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
          />
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label for="insight-chart-type" class="block text-xs font-semibold text-[var(--text-secondary)] mb-1">
              Visual Style
            </label>
            <select
              id="insight-chart-type"
              bind:value={chartType}
              class="w-full px-3 py-2.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
            >
              <option value="metric">Single Metric KPI</option>
              <option value="bar">Bar Chart</option>
              <option value="line">Trend Line</option>
              <option value="donut">Donut Distribution</option>
            </select>
          </div>

          <div>
            <label for="insight-aggregation" class="block text-xs font-semibold text-[var(--text-secondary)] mb-1">
              Agregasi Perhitungan
            </label>
            <select
              id="insight-aggregation"
              bind:value={aggregation}
              class="w-full px-3 py-2.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
            >
              <option value="count">Total Occurrence (Count)</option>
              <option value="unique_users">Unique Users (Distinct ID)</option>
              <option value="sum_property">Sum of Property Value</option>
              <option value="avg_property">Average of Property Value</option>
              <option value="min_property">Min Property Value</option>
              <option value="max_property">Max Property Value</option>
            </select>
          </div>
        </div>

        <div>
          <label for="insight-event-name" class="block text-xs font-semibold text-[var(--text-secondary)] mb-1">
            Target Event Name
          </label>
          {#if availableEvents.length > 0}
            <div class="flex gap-2">
              <select
                id="insight-event-name"
                bind:value={eventName}
                class="w-full px-3.5 py-2.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              >
                {#each availableEvents as ev}
                  <option value={ev}>{ev}</option>
                {/each}
              </select>
            </div>
          {:else}
            <input
              id="insight-event-name"
              type="text"
              bind:value={eventName}
              placeholder="e.g. checkout, login, button_click"
              required
              class="w-full px-3.5 py-2.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
            />
          {/if}
        </div>

        {#if isNumericPropertyAggregation(aggregation)}
          <div>
            <label for="insight-prop-key" class="block text-xs font-semibold text-[var(--text-secondary)] mb-1">
              Property Key (Numeric JSON field)
            </label>
            <input
              id="insight-prop-key"
              type="text"
              bind:value={propertyKey}
              placeholder="amount, price, duration (default: amount)"
              class="w-full px-3.5 py-2.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
            />
          </div>
        {/if}

        <div class="flex items-center justify-end gap-3 pt-4 border-t border-[var(--border-subtle)]">
          <Button variant="ghost" size="sm" type="button" onclick={onClose}>
            Batal
          </Button>
          <Button variant="primary" size="sm" type="submit" loading={submitting}>
            Simpan Widget
          </Button>
        </div>
      </form>
    </div>
  </div>
{/if}
