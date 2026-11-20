<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLButtonAttributes } from "svelte/elements";
  import { cn } from "$lib/utils";

  interface Props extends HTMLButtonAttributes {
    variant?: "primary" | "secondary" | "outline" | "ghost" | "danger";
    size?: "sm" | "md" | "lg";
    loading?: boolean;
    children?: Snippet;
  }

  let {
    variant = "secondary",
    size = "md",
    loading = false,
    disabled = false,
    class: className = "",
    children,
    ...rest
  }: Props = $props();

  const variantStyles = {
    primary: "bg-[var(--primary-emerald)] text-white hover:bg-[var(--primary-emerald-hover)] border-transparent bento-pill active:scale-[0.98]",
    secondary: "bg-[var(--btn-secondary-bg)] text-[var(--btn-secondary-text)] border-[var(--btn-secondary-border)] hover:bg-[var(--btn-secondary-hover-bg)] hover:border-[var(--btn-secondary-hover-border)] bento-pill active:scale-[0.98]",
    outline: "bg-[var(--bg-surface)] text-[var(--text-secondary)] border-[var(--border-subtle)] hover:bg-[var(--bg-surface-elevated)] hover:text-[var(--text-primary)] hover:border-[var(--border-hover)] bento-pill",
    ghost: "bg-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-elevated)] border-transparent",
    danger: "bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/20 hover:bg-rose-500/15 active:scale-[0.98]"
  };

  const sizeStyles = {
    sm: "px-3 py-1.5 text-xs gap-1.5 h-8",
    md: "px-4 py-2 text-xs font-medium gap-2 h-9",
    lg: "px-5 py-2.5 text-sm font-medium gap-2.5 h-11"
  };
</script>

<button
  class={cn(
    "inline-flex items-center justify-center rounded-full border font-medium transition-all duration-150 disabled:opacity-50 disabled:pointer-events-none select-none cursor-pointer",
    variantStyles[variant],
    sizeStyles[size],
    className
  )}
  disabled={disabled || loading}
  {...rest}
>
  {#if loading}
    <svg class="animate-spin -ml-0.5 mr-1.5 h-3.5 w-3.5 text-current" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
    </svg>
  {/if}
  {#if children}
    {@render children()}
  {/if}
</button>
