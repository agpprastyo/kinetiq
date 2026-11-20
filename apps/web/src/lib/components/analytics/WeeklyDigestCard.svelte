<script lang="ts">
  import {
    Sparkles,
    Calendar,
    Users,
    Zap,
    TrendingUp,
    Send,
    Loader2,
    CheckCircle2
  } from "lucide-svelte";
  import type { WeeklyDigestReport } from "$lib/api";
  import Button from "$lib/components/ui/Button.svelte";
  import Card from "$lib/components/ui/Card.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";

  interface Props {
    digest: WeeklyDigestReport | null;
    loading?: boolean;
    onTriggerDispatch?: () => Promise<void>;
  }

  let {
    digest = null,
    loading = false,
    onTriggerDispatch
  }: Props = $props();

  let sending = $state(false);
  let sentSuccess = $state(false);

  async function handleSendNow() {
    if (!onTriggerDispatch) return;
    sending = true;
    sentSuccess = false;
    try {
      await onTriggerDispatch();
      sentSuccess = true;
      setTimeout(() => { sentSuccess = false; }, 3000);
    } finally {
      sending = false;
    }
  }
</script>

<Card class="relative overflow-hidden p-6 sm:p-7 border border-emerald-500/20 bg-gradient-to-br from-emerald-500/[0.03] via-transparent to-[var(--bg-surface)]">
  <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-6">
    <div class="space-y-2 max-w-xl">
      <div class="flex items-center gap-2">
        <div class="w-8 h-8 rounded-full bg-emerald-500/10 text-emerald-500 flex items-center justify-center">
          <Sparkles class="w-4 h-4" />
        </div>
        <h3 class="text-base sm:text-lg font-bold tracking-tight text-[var(--text-primary)]">
          Weekly Automated Intelligence Digest
        </h3>
        <Badge variant="emerald">Auto-Cron</Badge>
      </div>
      <p class="text-xs sm:text-sm text-[var(--text-secondary)] leading-relaxed">
        Laporan performa berkala 7 hari terakhir dikompilasi dan dikirim otomatis setiap Senin pagi ke email seluruh Workspace Owner & Admin.
      </p>
    </div>

    <div class="flex items-center gap-3 shrink-0">
      {#if onTriggerDispatch}
        <Button
          variant="outline"
          size="sm"
          onclick={handleSendNow}
          loading={sending}
          disabled={sentSuccess}
        >
          {#if sentSuccess}
            <CheckCircle2 class="w-4 h-4 mr-1.5 text-emerald-500" />
            <span class="text-emerald-500">Terkirim ke Outbox!</span>
          {:else}
            <Send class="w-4 h-4 mr-1.5" />
            <span>Kirim Test Digest Sekarang</span>
          {/if}
        </Button>
      {/if}
    </div>
  </div>

  {#if loading}
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 mt-6">
      {#each [1, 2, 3] as _}
        <div class="h-20 rounded-2xl bg-[var(--bg-surface-muted)] animate-pulse"></div>
      {/each}
    </div>
  {:else if digest}
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 mt-6 pt-5 border-t border-[var(--border-subtle)]/60">
      <div class="p-4 rounded-2xl bg-[var(--bg-surface-muted)]/70 border border-[var(--border-subtle)]">
        <div class="flex items-center justify-between text-xs text-[var(--text-muted)] mb-1">
          <span>7-Day Volume</span>
          <Zap class="w-3.5 h-3.5 text-emerald-500" />
        </div>
        <div class="text-2xl font-extrabold tracking-tight text-[var(--text-primary)] font-mono">
          {digest.total_events.toLocaleString()}
        </div>
        <span class="text-[11px] text-[var(--text-muted)]">Total telemetry events</span>
      </div>

      <div class="p-4 rounded-2xl bg-[var(--bg-surface-muted)]/70 border border-[var(--border-subtle)]">
        <div class="flex items-center justify-between text-xs text-[var(--text-muted)] mb-1">
          <span>Active Distinct IDs</span>
          <Users class="w-3.5 h-3.5 text-teal-500" />
        </div>
        <div class="text-2xl font-extrabold tracking-tight text-[var(--text-primary)] font-mono">
          {digest.unique_users.toLocaleString()}
        </div>
        <span class="text-[11px] text-[var(--text-muted)]">Unique user identifiers</span>
      </div>

      <div class="p-4 rounded-2xl bg-[var(--bg-surface-muted)]/70 border border-[var(--border-subtle)]">
        <div class="flex items-center justify-between text-xs text-[var(--text-muted)] mb-1">
          <span>Top Conversion</span>
          <TrendingUp class="w-3.5 h-3.5 text-amber-500" />
        </div>
        <div class="text-2xl font-extrabold tracking-tight text-[var(--text-primary)] font-mono">
          {digest.conversion_rate > 0 ? `${digest.conversion_rate}%` : "N/A"}
        </div>
        <span class="text-[11px] text-[var(--text-muted)]">Step-to-step completion</span>
      </div>
    </div>
  {/if}
</Card>
