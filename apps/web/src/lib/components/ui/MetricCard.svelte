<script lang="ts">
  import type { Snippet } from "svelte";
  import Card from "./Card.svelte";
  import { cn } from "$lib/utils";

  interface Props {
    title: string;
    value: string | number;
    subtext?: string;
    trend?: "up" | "down" | "neutral";
    trendValue?: string;
    icon?: Snippet;
    class?: string;
  }

  let {
    title,
    value,
    subtext,
    trend,
    trendValue,
    icon,
    class: className = ""
  }: Props = $props();
</script>

<Card class={cn("relative overflow-hidden group", className)}>
  <div class="flex items-start justify-between">
    <div>
      <span class="text-xs font-semibold text-[var(--text-primary)] block">{title}</span>
      {#if subtext}
        <span class="text-[11px] text-[var(--text-muted)] mt-0.5 block">{subtext}</span>
      {/if}
    </div>
    {#if icon}
      <div class="w-8 h-8 rounded-full bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)] flex items-center justify-center text-[var(--text-secondary)] group-hover:text-[var(--primary-emerald)] transition-colors">
        {@render icon()}
      </div>
    {/if}
  </div>

  <div class="mt-4 flex items-center justify-between gap-2">
    <div class="text-2xl font-bold tracking-tight text-[var(--text-primary)]">
      {typeof value === "number" ? value.toLocaleString() : value}
    </div>
    {#if trendValue}
      <span class={cn(
        "px-2.5 py-0.5 rounded-full text-[11px] font-semibold",
        trend === "up" && "bg-[var(--primary-emerald)] text-white",
        trend === "down" && "bg-rose-500/15 text-rose-600 dark:text-rose-400",
        (!trend || trend === "neutral") && "bg-[var(--primary-tint)] text-[var(--primary-emerald)]"
      )}>
        {trendValue}
      </span>
    {/if}
  </div>
</Card>
