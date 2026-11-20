<script lang="ts">
  import type { Snippet } from "svelte";
  import { X } from "lucide-svelte";
  import Button from "./Button.svelte";

  interface Props {
    open: boolean;
    title: string;
    description?: string;
    confirmText?: string;
    confirmVariant?: "primary" | "secondary" | "danger";
    confirmLoading?: boolean;
    onconfirm?: () => void;
    oncancel?: () => void;
    children?: Snippet;
  }

  let {
    open = false,
    title,
    description,
    confirmText = "Confirm",
    confirmVariant = "primary",
    confirmLoading = false,
    onconfirm,
    oncancel,
    children
  }: Props = $props();

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && open && oncancel) {
      oncancel();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/35 backdrop-blur-xs animate-in fade-in duration-150">
    <div
      class="w-full max-w-md rounded-3xl border border-[var(--border-subtle)] bg-[var(--bg-surface)] p-6 animate-in zoom-in-95 duration-150 text-[var(--text-primary)]"
      style="box-shadow: var(--shadow-dropdown);"
      role="dialog"
      aria-modal="true"
    >
      <div class="flex items-center justify-between pb-3 border-b border-[var(--border-subtle)]">
        <h3 class="text-base font-semibold text-[var(--text-primary)]">{title}</h3>
        {#if oncancel}
          <button
            type="button"
            class="text-[var(--text-muted)] hover:text-[var(--text-primary)] p-1.5 rounded-full hover:bg-[var(--bg-surface-elevated)] transition-colors cursor-pointer"
            onclick={oncancel}
            aria-label="Close"
          >
            <X class="w-4 h-4" />
          </button>
        {/if}
      </div>

      <div class="py-4 text-sm text-[var(--text-secondary)]">
        {#if description}
          <p class="leading-relaxed mb-3">{description}</p>
        {/if}
        {#if children}
          {@render children()}
        {/if}
      </div>

      <div class="flex items-center justify-end gap-2.5 pt-3 border-t border-[var(--border-subtle)]">
        {#if oncancel}
          <Button variant="secondary" size="md" onclick={oncancel} disabled={confirmLoading}>
            Batal
          </Button>
        {/if}
        {#if onconfirm}
          <Button variant={confirmVariant} size="md" onclick={onconfirm} loading={confirmLoading}>
            {confirmText}
          </Button>
        {/if}
      </div>
    </div>
  </div>
{/if}
