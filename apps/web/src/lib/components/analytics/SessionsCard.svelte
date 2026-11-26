<script lang="ts">
  import Card from "$lib/components/ui/Card.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import type { SessionsResponse, SessionCount } from "$lib/api";
  import { formatDuration } from "$lib/format-duration";
  import { Timer, LogIn, Link2 } from "lucide-svelte";

  interface Props {
    sessions: SessionsResponse | null;
    loading?: boolean;
  }

  let { sessions = null, loading = false }: Props = $props();
</script>

{#snippet list(items: SessionCount[], empty: string)}
  {#if items.length === 0}
    <div class="py-4 text-center text-xs text-[var(--text-muted)]">{empty}</div>
  {:else}
    {#each items.slice(0, 5) as item}
      <div class="flex items-center justify-between p-2 rounded-xl bg-[var(--bg-surface-muted)]/60 text-xs">
        <span class="font-mono text-[var(--text-primary)] truncate">{item.value}</span>
        <span class="font-mono font-bold text-[var(--text-primary)]">{item.count.toLocaleString()}</span>
      </div>
    {/each}
  {/if}
{/snippet}

<div class="grid grid-cols-1 lg:grid-cols-3 gap-5 pt-2" data-testid="sessions-cards">
  <Card class="space-y-3">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2.5">
        <div class="w-9 h-9 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
          <Timer class="w-4 h-4" />
        </div>
        <div>
          <h3 class="text-sm font-bold text-[var(--text-primary)]">Sessions</h3>
          <p class="text-[11px] text-[var(--text-muted)]">30-min inactivity window</p>
        </div>
      </div>
      <Badge variant="neutral">Autocapture</Badge>
    </div>
    {#if loading}
      <div class="py-6 text-center text-xs text-[var(--text-muted)]">Loading sessions...</div>
    {:else}
      <div class="grid grid-cols-2 gap-3">
        <div>
          <div class="text-2xl font-bold text-[var(--text-primary)]">{(sessions?.total_sessions ?? 0).toLocaleString()}</div>
          <div class="text-[11px] text-[var(--text-muted)]">Total sessions</div>
        </div>
        <div>
          <div class="text-2xl font-bold text-[var(--text-primary)]">{formatDuration(sessions?.avg_duration_seconds ?? 0)}</div>
          <div class="text-[11px] text-[var(--text-muted)]">Avg. duration</div>
        </div>
      </div>
    {/if}
  </Card>

  <Card class="space-y-3">
    <div class="flex items-center gap-2.5">
      <LogIn class="w-4 h-4 text-[var(--brand-primary)]" />
      <h3 class="text-sm font-bold text-[var(--text-primary)]">Top Entry Pages</h3>
    </div>
    {@render list(sessions?.top_entry_pages ?? [], "No sessions in this period.")}
  </Card>

  <Card class="space-y-3">
    <div class="flex items-center gap-2.5">
      <Link2 class="w-4 h-4 text-[var(--brand-primary)]" />
      <h3 class="text-sm font-bold text-[var(--text-primary)]">Top Referrers</h3>
    </div>
    {@render list(sessions?.top_referring_domains ?? [], "No referrer data.")}
  </Card>
</div>
