<script lang="ts">
  import { goto } from "$app/navigation";
  import { onMount } from "svelte";
  import AuthShell from "$lib/components/ui/AuthShell.svelte";
  import { getSetupStatus, submitSetup } from "$lib/setup";
  import { ArrowRight, ArrowLeft, Eye, EyeOff } from "lucide-svelte";

  const MIN_PASSWORD = 12;
  const inputClass =
    "w-full h-10 px-3.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:bg-[var(--bg-surface)] focus:border-[var(--brand-primary)] transition-all";
  const btnClass =
    "h-10 px-4 rounded-xl bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold flex items-center justify-center gap-2 transition-colors cursor-pointer disabled:opacity-50";

  const backClass =
    "h-10 px-4 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-[var(--text-primary)] text-xs font-semibold flex items-center gap-2 cursor-pointer disabled:opacity-50";

  let step = $state<"checking" | "welcome" | "admin" | "config">("checking");
  let preflightError = $state("");

  let name = $state("");
  let email = $state("");
  let password = $state("");
  let showPassword = $state(false);
  let workspaceName = $state("");
  let appUrl = $state("");
  let timezone = $state("UTC");
  let error = $state("");
  let loading = $state(false);

  const stepNumber = $derived(step === "admin" ? 2 : step === "config" ? 3 : 1);

  async function preflight() {
    preflightError = "";
    step = "checking";
    const initialized = await getSetupStatus();
    if (initialized === true) {
      goto("/login");
    } else if (initialized === null) {
      preflightError = "Auth service is unreachable. Check that it is running, then retry.";
      step = "welcome";
    } else {
      step = "welcome";
    }
  }

  onMount(() => {
    appUrl = window.location.origin;
    timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
    preflight();
  });

  function nextFromAdmin(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    if (!name.trim() || !email.trim()) {
      error = "Name and email are required.";
    } else if (password.length < MIN_PASSWORD) {
      error = `Password must be at least ${MIN_PASSWORD} characters.`;
    } else {
      step = "config";
    }
  }

  async function finish(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    if (!workspaceName.trim() || !appUrl.trim()) {
      error = "Workspace name and App URL are required.";
      return;
    }
    loading = true;
    try {
      const res = await submitSetup({
        name: name.trim(),
        email: email.trim(),
        password,
        workspaceName: workspaceName.trim(),
        appUrl: appUrl.trim(),
        timezone: timezone.trim() || "UTC",
      });
      if (res.ok) {
        goto("/app");
      } else if (res.status === 403) {
        goto("/login");
      } else {
        error = res.message;
      }
    } catch {
      error = "Auth service is unreachable. Please try again.";
    } finally {
      loading = false;
    }
  }
</script>

<AuthShell title="Set up your instance" subtitle={`Step ${stepNumber} of 3`}>
  {#if error}
    <div
      class="rounded-xl border border-rose-500/25 bg-rose-500/10 px-3.5 py-2.5 text-xs font-medium text-rose-600 dark:text-rose-400"
      role="alert"
    >
      {error}
    </div>
  {/if}

  {#if step === "checking"}
    <p class="text-xs text-[var(--text-muted)]" data-testid="setup-checking">Checking instance status...</p>
  {:else if step === "welcome"}
    <div class="space-y-4" data-testid="setup-welcome">
      <p class="text-sm text-[var(--text-secondary)]">
        Welcome. This fresh instance has no administrator yet. Create the superadmin account and a default workspace to get
        started. This page locks itself once setup completes.
      </p>
      {#if preflightError}
        <div class="rounded-xl border border-rose-500/25 bg-rose-500/10 px-3.5 py-2.5 text-xs text-rose-600 dark:text-rose-400" role="alert">
          {preflightError}
        </div>
        <button type="button" class={btnClass + " w-full"} onclick={preflight}>Retry</button>
      {:else}
        <button type="button" class={btnClass + " w-full"} onclick={() => (step = "admin")}>
          Get started <ArrowRight class="w-3.5 h-3.5" />
        </button>
      {/if}
    </div>
  {:else if step === "admin"}
    <form onsubmit={nextFromAdmin} class="space-y-4" data-testid="setup-admin">
      <div class="space-y-1.5">
        <label for="name" class="block text-xs font-medium text-[var(--text-secondary)]">Name</label>
        <input id="name" type="text" autocomplete="name" bind:value={name} class={inputClass} required />
      </div>
      <div class="space-y-1.5">
        <label for="email" class="block text-xs font-medium text-[var(--text-secondary)]">Email</label>
        <input id="email" type="email" autocomplete="email" bind:value={email} class={inputClass} required />
      </div>
      <div class="space-y-1.5">
        <label for="password" class="block text-xs font-medium text-[var(--text-secondary)]">
          Master password (min {MIN_PASSWORD} characters)
        </label>
        <div class="relative">
          <input
            id="password"
            type={showPassword ? "text" : "password"}
            autocomplete="new-password"
            bind:value={password}
            class={inputClass + " pr-10"}
            minlength={MIN_PASSWORD}
            required
          />
          <button
            type="button"
            class="absolute right-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)] cursor-pointer"
            aria-label={showPassword ? "Hide password" : "Show password"}
            onclick={() => (showPassword = !showPassword)}
          >
            {#if showPassword}<EyeOff class="w-4 h-4" />{:else}<Eye class="w-4 h-4" />{/if}
          </button>
        </div>
      </div>
      <div class="flex gap-2">
        <button type="button" class={backClass} onclick={() => (step = "welcome")}>
          <ArrowLeft class="w-3.5 h-3.5" /> Back
        </button>
        <button type="submit" class={btnClass + " flex-1"}>Next <ArrowRight class="w-3.5 h-3.5" /></button>
      </div>
    </form>
  {:else}
    <form onsubmit={finish} class="space-y-4" data-testid="setup-config">
      <div class="space-y-1.5">
        <label for="workspaceName" class="block text-xs font-medium text-[var(--text-secondary)]">Workspace name</label>
        <input id="workspaceName" type="text" bind:value={workspaceName} placeholder="Acme Inc." class={inputClass} required disabled={loading} />
      </div>
      <div class="space-y-1.5">
        <label for="appUrl" class="block text-xs font-medium text-[var(--text-secondary)]">App URL</label>
        <input id="appUrl" type="url" bind:value={appUrl} class={inputClass} required disabled={loading} />
      </div>
      <div class="space-y-1.5">
        <label for="timezone" class="block text-xs font-medium text-[var(--text-secondary)]">Timezone (IANA)</label>
        <input id="timezone" type="text" bind:value={timezone} class={inputClass} required disabled={loading} />
      </div>
      <div class="flex gap-2">
        <button type="button" class={backClass} onclick={() => (step = "admin")} disabled={loading}>
          <ArrowLeft class="w-3.5 h-3.5" /> Back
        </button>
        <button type="submit" class={btnClass + " flex-1"} disabled={loading}>
          {loading ? "Initializing..." : "Create instance"}
        </button>
      </div>
    </form>
  {/if}
</AuthShell>
