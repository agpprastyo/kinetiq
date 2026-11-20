<script lang="ts">
  import Card from "$lib/components/ui/Card.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import { Filter, ArrowDownRight, Plus, X } from "lucide-svelte";
  import type { FunnelData } from "$lib/api";

  interface Props {
    funnel: FunnelData | null;
    loading?: boolean;
    step1: string;
    step2: string;
    step3?: string;
    step4?: string;
    step5?: string;
    availableEvents?: string[];
    onCalculate: (steps: string[]) => void;
  }

  let {
    funnel,
    loading = false,
    step1 = $bindable(),
    step2 = $bindable(),
    step3 = $bindable(""),
    step4 = $bindable(""),
    step5 = $bindable(""),
    availableEvents = [],
    onCalculate
  }: Props = $props();

  let showStep4 = $state(false);
  let showStep5 = $state(false);

  function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    const steps = [step1.trim(), step2.trim()];
    if (step3 && step3.trim()) steps.push(step3.trim());
    if (showStep4 && step4 && step4.trim()) steps.push(step4.trim());
    if (showStep5 && step5 && step5.trim()) steps.push(step5.trim());
    onCalculate(steps);
  }

  const stepStats = $derived.by(() => {
    if (!funnel || !funnel.steps || funnel.steps.length === 0) return [];
    return funnel.steps.map((st, i, arr) => {
      if (i === 0) {
        return {
          ...st,
          stepToStepRate: 100,
          dropOff: 0
        };
      }
      const prev = arr[i - 1];
      const stepRate = prev.count > 0 ? (st.count / prev.count) * 100 : 0;
      const dropOffCount = Math.max(0, prev.count - st.count);
      return {
        ...st,
        stepToStepRate: stepRate,
        dropOff: dropOffCount
      };
    });
  });
</script>

<Card class="space-y-4">
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2.5">
      <div class="w-8 h-8 rounded-full bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)] flex items-center justify-center text-[var(--primary-emerald)]">
        <Filter class="w-4 h-4" />
      </div>
      <div>
        <h2 class="text-sm font-semibold text-[var(--text-primary)]">Sequential Conversion Funnel</h2>
        <p class="text-[11px] text-[var(--text-muted)]">Progressive steps & drop-offs per distinct ID</p>
      </div>
    </div>
  </div>

  <!-- Datalist for fast event selection -->
  <datalist id="funnel-events-list">
    {#each availableEvents as ev}
      <option value={ev}></option>
    {/each}
  </datalist>

  <!-- Funnel Configuration Form -->
  <form onsubmit={handleSubmit} class="p-3.5 rounded-2xl bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)] space-y-3">
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
      <div class="space-y-1">
        <label for="funnel-s1" class="text-[11px] font-medium text-[var(--text-muted)]">Step 1</label>
        <input
          id="funnel-s1"
          type="text"
          list="funnel-events-list"
          bind:value={step1}
          placeholder="e.g. page_view"
          class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--primary-emerald)] transition-colors"
          required
        />
      </div>
      <div class="space-y-1">
        <label for="funnel-s2" class="text-[11px] font-medium text-[var(--text-muted)]">Step 2</label>
        <input
          id="funnel-s2"
          type="text"
          list="funnel-events-list"
          bind:value={step2}
          placeholder="e.g. signup"
          class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--primary-emerald)] transition-colors"
          required
        />
      </div>
      <div class="space-y-1">
        <label for="funnel-s3" class="text-[11px] font-medium text-[var(--text-muted)]">Step 3 (Opsional)</label>
        <input
          id="funnel-s3"
          type="text"
          list="funnel-events-list"
          bind:value={step3}
          placeholder="e.g. checkout"
          class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--primary-emerald)] transition-colors"
        />
      </div>

      {#if showStep4}
        <div class="space-y-1">
          <div class="flex items-center justify-between">
            <label for="funnel-s4" class="text-[11px] font-medium text-[var(--text-muted)]">Step 4 (Opsional)</label>
            <button
              type="button"
              onclick={() => { showStep4 = false; step4 = ""; if (showStep5) { showStep5 = false; step5 = ""; } }}
              class="text-[10px] text-rose-500 hover:underline"
            >
              Hapus
            </button>
          </div>
          <input
            id="funnel-s4"
            type="text"
            list="funnel-events-list"
            bind:value={step4}
            placeholder="e.g. payment_success"
            class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--primary-emerald)] transition-colors"
          />
        </div>
      {/if}

      {#if showStep5}
        <div class="space-y-1">
          <div class="flex items-center justify-between">
            <label for="funnel-s5" class="text-[11px] font-medium text-[var(--text-muted)]">Step 5 (Opsional)</label>
            <button
              type="button"
              onclick={() => { showStep5 = false; step5 = ""; }}
              class="text-[10px] text-rose-500 hover:underline"
            >
              Hapus
            </button>
          </div>
          <input
            id="funnel-s5"
            type="text"
            list="funnel-events-list"
            bind:value={step5}
            placeholder="e.g. referral"
            class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--primary-emerald)] transition-colors"
          />
        </div>
      {/if}
    </div>

    <div class="flex items-center justify-between pt-1">
      <div class="flex items-center gap-2">
        {#if !showStep4}
          <button
            type="button"
            onclick={() => (showStep4 = true)}
            class="inline-flex items-center gap-1 text-[11px] font-medium text-[var(--primary-emerald)] hover:underline cursor-pointer"
          >
            <Plus class="w-3.5 h-3.5" />
            <span>Tambah Step 4</span>
          </button>
        {:else if !showStep5}
          <button
            type="button"
            onclick={() => (showStep5 = true)}
            class="inline-flex items-center gap-1 text-[11px] font-medium text-[var(--primary-emerald)] hover:underline cursor-pointer"
          >
            <Plus class="w-3.5 h-3.5" />
            <span>Tambah Step 5</span>
          </button>
        {/if}
      </div>

      <Button type="submit" variant="primary" size="sm" {loading}>
        Hitung Funnel
      </Button>
    </div>
  </form>

  {#if !funnel || funnel.steps.length === 0}
    <div class="py-8 text-center text-xs text-[var(--text-muted)]">
      Masukkan tahapan event di atas untuk menganalisis rasio konversi.
    </div>
  {:else}
    <div class="space-y-3 pt-1">
      {#each stepStats as st, i}
        {#if i > 0}
          <div class="flex items-center justify-between px-3 text-[11px] text-[var(--text-muted)]">
            <div class="flex items-center gap-1 text-rose-500">
              <ArrowDownRight class="w-3.5 h-3.5" />
              <span>Drop-off: {st.dropOff.toLocaleString()} users</span>
            </div>
            <div class="flex items-center gap-1 text-[var(--text-secondary)]">
              <span>Step conversion:</span>
              <span class="font-bold text-[var(--text-primary)]">{st.stepToStepRate.toFixed(1)}%</span>
            </div>
          </div>
        {/if}

        <div class="p-3.5 rounded-2xl bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)] space-y-2">
          <div class="flex items-center justify-between text-xs">
            <div class="flex items-center gap-2">
              <Badge variant="emerald">Step {st.step}</Badge>
              <span class="font-semibold text-[var(--text-primary)]">{st.event_name}</span>
            </div>
            <div class="flex items-center gap-2">
              <span class="font-bold text-[var(--text-primary)]">{st.count.toLocaleString()} users</span>
              <span class="px-2 py-0.5 rounded-full text-[11px] font-semibold bg-[var(--primary-tint)] text-[var(--primary-emerald)]">
                {st.conversion_rate.toFixed(1)}%
              </span>
            </div>
          </div>

          <div class="w-full h-2 rounded-full bg-[var(--bg-surface)] overflow-hidden">
            <div
              class="h-full rounded-full bg-[var(--primary-emerald)] transition-all duration-300"
              style="width: {st.conversion_rate}%;"
            ></div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</Card>
