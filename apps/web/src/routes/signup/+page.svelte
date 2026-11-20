<script lang="ts">
  import { goto } from "$app/navigation";
  import { authClient } from "$lib/auth-client";
  import AuthShell from "$lib/components/ui/AuthShell.svelte";
  import { ArrowRight, Eye, EyeOff } from "lucide-svelte";

  let name = $state("");
  let email = $state("");
  let password = $state("");
  let showPassword = $state(false);
  let error = $state("");
  let loading = $state(false);

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    if (!name.trim() || !email.trim() || !password) {
      error = "Please complete all required fields.";
      return;
    }

    if (password.length < 8) {
      error = "Password must be at least 8 characters long.";
      return;
    }

    loading = true;
    try {
      const res = await authClient.signUp.email({
        name: name.trim(),
        email: email.trim(),
        password,
      });

      if (res.error) {
        error = res.error.message || "Failed to create account. Please try again.";
      } else {
        goto("/app");
      }
    } catch (err: any) {
      if (/failed to fetch|networkerror|err_connection_refused/i.test(err?.message || "")) {
        error = "Auth service is currently unreachable (port 3001). Please check if backend is running.";
      } else {
        error = "Unable to create account right now. Please try again.";
      }
    } finally {
      loading = false;
    }
  }

  async function handleSocialSignIn(provider: "google" | "github") {
    error = "";
    loading = true;
    try {
      const res = await authClient.signIn.social({
        provider,
        callbackURL: `${window.location.origin}/app`,
      });
      if (res?.error) {
        error = `${provider === "google" ? "Google" : "GitHub"} OAuth requires client credentials in .env.`;
      }
    } catch {
      error = `${provider === "google" ? "Google" : "GitHub"} OAuth is not configured in this local environment yet.`;
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head>
  <title>Create Workspace — Kinetiq</title>
</svelte:head>

<AuthShell
  title="Create your workspace"
  subtitle="Provision a new telemetry organization and generate your first project API key."
>
  {#if error}
    <div
      class="rounded-xl border border-rose-500/25 bg-rose-500/10 px-3.5 py-2.5 text-xs font-medium text-rose-600 dark:text-rose-400"
      role="alert"
    >
      {error}
    </div>
  {/if}

  <!-- Authentic SVG Social OAuth Buttons -->
  <div class="grid grid-cols-2 gap-2.5">
    <button
      type="button"
      onclick={() => handleSocialSignIn("google")}
      disabled={loading}
      class="h-10 px-3.5 rounded-xl bg-[var(--bg-surface)] hover:bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-medium text-[var(--text-primary)] flex items-center justify-center gap-2.5 transition-colors cursor-pointer disabled:opacity-50"
    >
      <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" aria-hidden="true">
        <path
          fill="#4285F4"
          d="M23.49 12.27c0-.79-.07-1.54-.19-2.27H12v4.51h6.47c-.29 1.48-1.14 2.73-2.4 3.58v3h3.86c2.26-2.09 3.56-5.17 3.56-8.82Z"
        />
        <path
          fill="#34A853"
          d="M12 24c3.24 0 5.95-1.08 7.93-2.91l-3.86-3c-1.08.72-2.45 1.16-4.07 1.16-3.13 0-5.78-2.11-6.73-4.96H1.29v3.09C3.26 21.3 7.31 24 12 24Z"
        />
        <path
          fill="#FBBC05"
          d="M5.27 14.29c-.25-.72-.38-1.49-.38-2.29s.14-1.57.38-2.29V6.62H1.29C.47 8.24 0 10.06 0 12s.47 3.76 1.29 5.38l3.98-3.09Z"
        />
        <path
          fill="#EA4335"
          d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.31 0 3.26 2.7 1.29 6.62l3.98 3.09c.95-2.85 3.6-4.96 6.73-4.96Z"
        />
      </svg>
      <span>Sign up with Google</span>
    </button>

    <button
      type="button"
      onclick={() => handleSocialSignIn("github")}
      disabled={loading}
      class="h-10 px-3.5 rounded-xl bg-[var(--bg-surface)] hover:bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-medium text-[var(--text-primary)] flex items-center justify-center gap-2.5 transition-colors cursor-pointer disabled:opacity-50"
    >
      <svg class="w-4 h-4 shrink-0 fill-current" viewBox="0 0 24 24" aria-hidden="true">
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12Z"
        />
      </svg>
      <span>Sign up with GitHub</span>
    </button>
  </div>

  <div class="relative flex items-center">
    <div class="grow border-t border-[var(--border-subtle)]"></div>
    <span class="shrink-0 px-3 text-[11px] font-mono text-[var(--text-muted)]">
      or register with email
    </span>
    <div class="grow border-t border-[var(--border-subtle)]"></div>
  </div>

  <form onsubmit={handleSubmit} class="space-y-3.5">
    <div class="space-y-1.5">
      <label for="name" class="block text-xs font-medium text-[var(--text-secondary)]">
        Full name
      </label>
      <input
        id="name"
        type="text"
        autocomplete="name"
        bind:value={name}
        placeholder="Budi Santoso"
        class="w-full h-10 px-3.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:bg-[var(--bg-surface)] focus:border-[var(--brand-primary)] transition-all"
        required
        disabled={loading}
      />
    </div>

    <div class="space-y-1.5">
      <label for="email" class="block text-xs font-medium text-[var(--text-secondary)]">
        Work email
      </label>
      <input
        id="email"
        type="email"
        autocomplete="email"
        bind:value={email}
        placeholder="budi@company.id"
        class="w-full h-10 px-3.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:bg-[var(--bg-surface)] focus:border-[var(--brand-primary)] transition-all"
        required
        disabled={loading}
      />
    </div>

    <div class="space-y-1.5">
      <div class="flex items-center justify-between">
        <label for="password" class="block text-xs font-medium text-[var(--text-secondary)]">
          Password
        </label>
        <span class="text-[11px] font-mono text-[var(--text-muted)]">min. 8 chars</span>
      </div>
      <div class="relative">
        <input
          id="password"
          type={showPassword ? "text" : "password"}
          autocomplete="new-password"
          bind:value={password}
          placeholder="Create a strong password"
          class="w-full h-10 pl-3.5 pr-10 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:bg-[var(--bg-surface)] focus:border-[var(--brand-primary)] transition-all"
          required
          minlength={8}
          disabled={loading}
        />
        <button
          type="button"
          onclick={() => (showPassword = !showPassword)}
          class="absolute right-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)] hover:text-[var(--text-primary)] cursor-pointer"
          tabindex={-1}
          aria-label={showPassword ? "Hide password" : "Show password"}
        >
          {#if showPassword}
            <EyeOff class="w-4 h-4" />
          {:else}
            <Eye class="w-4 h-4" />
          {/if}
        </button>
      </div>
    </div>

    <div class="pt-1">
      <button
        type="submit"
        disabled={loading}
        class="w-full h-10 px-4 rounded-xl bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold flex items-center justify-center gap-2 transition-colors cursor-pointer disabled:opacity-50"
      >
        {#if loading}
          <span>Provisioning workspace...</span>
        {:else}
          <span>Create workspace</span>
          <ArrowRight class="w-3.5 h-3.5" />
        {/if}
      </button>
    </div>
  </form>

  {#snippet footer()}
    <div class="flex items-center justify-between text-xs text-[var(--text-secondary)]">
      <span>Already have an account?</span>
      <a href="/login" class="font-semibold text-[var(--brand-primary)] hover:underline">
        Sign in →
      </a>
    </div>
  {/snippet}
</AuthShell>
