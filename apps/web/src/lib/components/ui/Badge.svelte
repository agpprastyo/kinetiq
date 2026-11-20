<script lang="ts">
  import type { Snippet } from "svelte";
  import { cn } from "$lib/utils";

  interface Props {
    variant?: "emerald" | "indigo" | "amber" | "rose" | "neutral" | "solid-emerald";
    pulse?: boolean;
    class?: string;
    children?: Snippet;
  }

  let {
    variant = "neutral",
    pulse = false,
    class: className = "",
    children
  }: Props = $props();

  const variantStyles = {
    emerald: "bg-[var(--primary-tint)] text-[var(--primary-emerald)] border-emerald-500/20",
    "solid-emerald": "bg-[var(--primary-emerald)] text-white border-transparent",
    indigo: "bg-[var(--primary-tint)] text-[var(--primary-emerald)] border-emerald-500/20",
    amber: "bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20",
    rose: "bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20",
    neutral: "bg-[var(--bg-surface-elevated)] text-[var(--text-secondary)] border-[var(--border-subtle)]"
  };

  const dotStyles = {
    emerald: "bg-[var(--primary-emerald)]",
    "solid-emerald": "bg-white",
    indigo: "bg-[var(--primary-emerald)]",
    amber: "bg-amber-500",
    rose: "bg-rose-500",
    neutral: "bg-slate-400"
  };
</script>

<span
  class={cn(
    "inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-medium border tracking-wide select-none",
    variantStyles[variant],
    className
  )}
>
  {#if pulse}
    <span class="relative flex h-2 w-2">
      <span class={cn("animate-ping absolute inline-flex h-full w-full rounded-full opacity-75", dotStyles[variant])}></span>
      <span class={cn("relative inline-flex rounded-full h-2 w-2", dotStyles[variant])}></span>
    </span>
  {/if}
  {#if children}
    {@render children()}
  {/if}
</span>
