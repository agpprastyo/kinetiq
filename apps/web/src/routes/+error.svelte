<script lang="ts">
  import { page } from "$app/state";
  import { AlertTriangle, RefreshCw, Home, ArrowLeft } from "lucide-svelte";

  const status = $derived(page.status || 500);
  const errorMsg = $derived(page.error?.message || "An unexpected error occurred.");
  const isChunkOrNetworkError = $derived(
    /failed to fetch dynamically imported module|dynamically imported|chunk|connection lost|net::err/i.test(
      errorMsg
    )
  );

  function handleReload() {
    window.location.reload();
  }
</script>

<svelte:head>
  <title>{status} Error — Kinetiq</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center p-6 bg-[var(--bg-app)] text-[var(--text-primary)]">
  <div class="max-w-md w-full bg-[var(--bg-surface)] border border-[var(--border-subtle)] rounded-3xl p-8 shadow-xl text-center space-y-6">
    <div class="w-14 h-14 rounded-2xl bg-rose-500/10 border border-rose-500/20 text-rose-500 flex items-center justify-center mx-auto">
      <AlertTriangle class="w-7 h-7" />
    </div>

    <div class="space-y-2">
      <div class="text-xs font-mono font-bold uppercase tracking-wider text-rose-500">
        HTTP Status {status}
      </div>
      <h1 class="text-2xl font-black tracking-tight text-[var(--text-primary)]">
        {#if isChunkOrNetworkError}
          Connection Interrupted
        {:else if status === 404}
          Page Not Found
        {:else if status === 403}
          Access Denied
        {:else}
          Something Went Wrong
        {/if}
      </h1>
      <p class="text-xs text-[var(--text-secondary)] leading-relaxed">
        {#if isChunkOrNetworkError}
          The application lost connection to the server or a new client version was deployed. Reloading the page usually restores normal operation.
        {:else}
          {errorMsg}
        {/if}
      </p>
    </div>

    <div class="flex items-center justify-center gap-3 pt-2">
      <button
        type="button"
        onclick={handleReload}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-xl bg-[var(--brand-primary)] text-white text-xs font-semibold hover:opacity-90 transition cursor-pointer"
      >
        <RefreshCw class="w-3.5 h-3.5" />
        Reload Application
      </button>

      <a
        href="/"
        class="inline-flex items-center gap-2 px-4 py-2 rounded-xl bg-[var(--bg-surface-muted)] text-[var(--text-primary)] border border-[var(--border-subtle)] text-xs font-semibold hover:bg-[var(--border-subtle)] transition"
      >
        <Home class="w-3.5 h-3.5" />
        Home
      </a>
    </div>
  </div>
</div>
