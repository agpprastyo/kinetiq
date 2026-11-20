<script lang="ts">
  import type { Snippet } from "svelte";
  import { cn } from "$lib/utils";

  interface Props {
    hoverable?: boolean;
    class?: string;
    children?: Snippet;
  }

  let {
    hoverable = false,
    class: className = "",
    children
  }: Props = $props();
</script>

<div
  class={cn(
    "bento-card p-6 text-[var(--text-primary)] transition-all duration-150",
    hoverable && "hover:border-[var(--border-hover)] hover:bg-[var(--bg-surface-elevated)]",
    className
  )}
>
  {#if children}
    {@render children()}
  {/if}
</div>
