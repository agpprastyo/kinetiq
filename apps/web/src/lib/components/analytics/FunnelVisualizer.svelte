<script lang="ts">
  import Card from "$lib/components/ui/Card.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import { Filter, ArrowDownRight, Plus, X } from "lucide-svelte";
  import type { FunnelData, FunnelDropoffData } from "$lib/api";
  import { FUNNEL_WINDOWS, dropoffPage, type FunnelOrder } from "$lib/funnel";
  import { formatDuration } from "$lib/format-duration";

  const DROPOFF_PAGE = 20;

  interface Props {
    funnel: FunnelData | null;
    loading?: boolean;
    step1: string;
    step2: string;
    step3?: string;
    step4?: string;
    step5?: string;
    availableEvents?: string[];
    windowSize?: string;
    order?: FunnelOrder;
    onCalculate: (steps: string[]) => void;
    loadDropoff?: (step: number, offset: number, limit: number) => Promise<FunnelDropoffData | null>;
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
    windowSize = $bindable("14d"),
    order = $bindable<FunnelOrder>("ordered"),
    onCalculate,
    loadDropoff
  }: Props = $props();

  let dropoff = $state<FunnelDropoffData | null>(null);
  let dropoffLoading = $state(false);

  async function openDropoff(step: number, offset = 0) {
    if (!loadDropoff) return;
    dropoffLoading = true;
    try {
      dropoff = await loadDropoff(step, offset, DROPOFF_PAGE);
    } finally {
      dropoffLoading = false;
    }
  }

  $effect(() => {
    funnel;
    dropoff = null;
  });

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

    <div class="grid grid-cols-2 gap-2.5">
      <div class="space-y-1">
        <label for="funnel-window" class="text-[11px] font-medium text-[var(--text-muted)]">Conversion window</label>
        <select
          id="funnel-window"
          bind:value={windowSize}
          class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--primary-emerald)]"
        >
          {#each FUNNEL_WINDOWS as w}
            <option value={w}>{w}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-1">
        <label for="funnel-order" class="text-[11px] font-medium text-[var(--text-muted)]">Step order</label>
        <select
          id="funnel-order"
          bind:value={order}
          class="w-full px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--primary-emerald)]"
        >
          <option value="ordered">Ordered (events in between allowed)</option>
          <option value="strict">Strict (no events in between)</option>
        </select>
      </div>
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
            {#if loadDropoff && st.dropOff > 0}
              <button
                type="button"
                onclick={() => openDropoff(st.step - 1)}
                class="flex items-center gap-1 text-rose-500 hover:underline cursor-pointer"
                aria-label="Show users who dropped off after step {st.step - 1}"
              >
                <ArrowDownRight class="w-3.5 h-3.5" />
                <span>Drop-off: {st.dropOff.toLocaleString()} users</span>
              </button>
            {:else}
              <div class="flex items-center gap-1 text-rose-500">
                <ArrowDownRight class="w-3.5 h-3.5" />
                <span>Drop-off: {st.dropOff.toLocaleString()} users</span>
              </div>
            {/if}
            {#if st.median_time_to_convert_seconds != null}
              <span>Median time: <span class="font-bold text-[var(--text-primary)]">{formatDuration(st.median_time_to_convert_seconds)}</span></span>
            {/if}
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

    {#if dropoff || dropoffLoading}
      {@const pg = dropoff ? dropoffPage(dropoff.offset, dropoff.limit, dropoff.total) : null}
      <div class="p-3.5 rounded-2xl bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)] space-y-2" aria-live="polite">
        <div class="flex items-center justify-between text-xs">
          <span class="font-semibold text-[var(--text-primary)]">
            {#if dropoff}Dropped after {dropoff.event_name} (did not reach {dropoff.next_event_name}){:else}Loading…{/if}
          </span>
          <button type="button" onclick={() => (dropoff = null)} class="text-[var(--text-muted)] hover:text-[var(--text-primary)]" aria-label="Close drop-off list">
            <X class="w-3.5 h-3.5" />
          </button>
        </div>
        {#if dropoff && pg}
          {#if dropoff.items.length === 0}
            <p class="text-[11px] text-[var(--text-muted)]">No users.</p>
          {:else}
            <ul class="divide-y divide-[var(--border-subtle)] text-[11px]">
              {#each dropoff.items as u}
                <li class="flex items-center justify-between py-1.5">
                  <span class="font-mono text-[var(--text-primary)]">{u.distinct_id || u.person_key}</span>
                  <span class="text-[var(--text-muted)]">last step {new Date(u.reached_at).toLocaleString()}</span>
                </li>
              {/each}
            </ul>
          {/if}
          <div class="flex items-center justify-between text-[11px] text-[var(--text-muted)]">
            <span>{pg.from}–{pg.to} of {dropoff.total.toLocaleString()}</span>
            <div class="flex gap-2">
              <Button size="sm" variant="ghost" disabled={!pg.hasPrev || dropoffLoading} onclick={() => openDropoff(dropoff!.step, dropoff!.offset - dropoff!.limit)}>Prev</Button>
              <Button size="sm" variant="ghost" disabled={!pg.hasNext || dropoffLoading} onclick={() => openDropoff(dropoff!.step, dropoff!.offset + dropoff!.limit)}>Next</Button>
            </div>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</Card>
