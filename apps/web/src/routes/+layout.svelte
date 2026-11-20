<script lang="ts">
  import "../app.css";
  import favicon from "#lib/assets/favicon.svg";
  import type { LayoutProps } from "./$types";
  import { onMount } from "svelte";
  import { getTheme, setTheme } from "$lib/theme";
  import { WifiOff, AlertTriangle, X } from "lucide-svelte";

  let { children }: LayoutProps = $props();

  let isOffline = $state(false);
  let networkAlert = $state<string | null>(null);

  onMount(() => {
    setTheme(getTheme());

    const updateOnlineStatus = () => {
      isOffline = !navigator.onLine;
    };

    window.addEventListener("online", updateOnlineStatus);
    window.addEventListener("offline", updateOnlineStatus);
    updateOnlineStatus();

    // Listen to global unhandled promise rejections for network/fetch failures
    const handleUnhandledRejection = (e: PromiseRejectionEvent) => {
      const reason = e?.reason;
      const msg = typeof reason === "string" ? reason : reason?.message || "";
      if (/failed to fetch|networkerror|err_connection_refused/i.test(msg)) {
        networkAlert = "Unable to reach Kinetiq services. Backend or Auth service may be temporarily unavailable.";
        setTimeout(() => {
          networkAlert = null;
        }, 8000);
      }
    };

    window.addEventListener("unhandledrejection", handleUnhandledRejection);

    return () => {
      window.removeEventListener("online", updateOnlineStatus);
      window.removeEventListener("offline", updateOnlineStatus);
      window.removeEventListener("unhandledrejection", handleUnhandledRejection);
    };
  });
</script>

<svelte:head>
  <link rel="icon" href={favicon} />
  <title>Kinetiq — Real-Time Product Analytics</title>
  <script>
    try {
      const theme = localStorage.getItem('kinetiq_theme') || 'light';
      if (theme === 'dark') {
        document.documentElement.classList.add('dark');
        document.documentElement.classList.remove('light');
      } else {
        document.documentElement.classList.add('light');
        document.documentElement.classList.remove('dark');
      }
    } catch (_) {}
  </script>
</svelte:head>

<div class="app-root">
  {#if isOffline}
    <div
      class="bg-amber-500 text-zinc-950 px-4 py-2 text-xs font-bold flex items-center justify-center gap-2 sticky top-0 z-[100] shadow-md"
      role="status"
    >
      <WifiOff class="w-4 h-4 shrink-0" />
      <span>You are currently offline. Check your internet connection.</span>
    </div>
  {:else if networkAlert}
    <div
      class="bg-rose-500 text-white px-4 py-2.5 text-xs font-semibold flex items-center justify-between gap-3 sticky top-0 z-[100] shadow-md animate-fade-in"
      role="alert"
    >
      <div class="flex items-center gap-2">
        <AlertTriangle class="w-4 h-4 shrink-0" />
        <span>{networkAlert}</span>
      </div>
      <button
        type="button"
        onclick={() => (networkAlert = null)}
        class="p-1 hover:bg-black/20 rounded-md cursor-pointer transition"
        aria-label="Dismiss alert"
      >
        <X class="w-3.5 h-3.5" />
      </button>
    </div>
  {/if}

  {@render children()}
</div>

<style>
  .app-root {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }
</style>
