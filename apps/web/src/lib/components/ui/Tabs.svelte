<script lang="ts">
  import { cn } from "$lib/utils";

  interface TabItem {
    id: string;
    label: string;
  }

  interface Props {
    items: TabItem[];
    selected: string;
    onSelect: (id: string) => void;
    class?: string;
  }

  let { items, selected, onSelect, class: className = "" }: Props = $props();
</script>

<div class={cn("inline-flex items-center gap-1 p-1 rounded-full bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)]", className)}>
  {#each items as item}
    <button
      type="button"
      onclick={() => onSelect(item.id)}
      class={cn(
        "px-3.5 py-1.5 rounded-full text-xs font-medium transition-all duration-150 select-none cursor-pointer",
        selected === item.id
          ? "bg-[var(--primary-emerald)] text-white font-semibold bento-pill"
          : "text-[var(--text-muted)] hover:text-[var(--text-primary)]"
      )}
    >
      {item.label}
    </button>
  {/each}
</div>
