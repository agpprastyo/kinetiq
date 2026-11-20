<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { authClient } from "$lib/auth-client";
  import AuthShell from "$lib/components/ui/AuthShell.svelte";
  import { ArrowLeft, ArrowRight, Check } from "lucide-svelte";

  const tokenFromUrl = $derived(page.url.searchParams.get("token") || "");

  let email = $state("");
  let newPassword = $state("");
  let confirmPassword = $state("");
  let loading = $state(false);
  let message = $state<{ type: "success" | "error"; text: string } | null>(null);

  async function handleRequestReset(e: SubmitEvent) {
    e.preventDefault();
    message = null;
    if (!email.trim()) {
      message = { type: "error", text: "Please enter your account email address." };
      return;
    }

    loading = true;
    try {
      const res = await (authClient as any).forgetPassword({
        email: email.trim(),
        redirectTo: `${window.location.origin}/forgot-password`,
      });
      if (res?.error) {
        message = { type: "error", text: res.error.message || "Unable to send reset link." };
      } else {
        message = {
          type: "success",
          text: "If an account exists for that email, a password reset link has been dispatched (also available in the Workspace Email Outbox in local dev)."
        };
      }
    } catch {
      message = {
        type: "success",
        text: "Password reset link dispatched. Check your email or local dev outbox."
      };
    } finally {
      loading = false;
    }
  }

  async function handleConfirmReset(e: SubmitEvent) {
    e.preventDefault();
    message = null;
    if (newPassword.length < 8) {
      message = { type: "error", text: "New password must be at least 8 characters." };
      return;
    }
    if (newPassword !== confirmPassword) {
      message = { type: "error", text: "Passwords do not match." };
      return;
    }

    loading = true;
    try {
      const res = await authClient.resetPassword({
        newPassword,
        token: tokenFromUrl,
      });
      if (res?.error) {
        message = { type: "error", text: "Reset token is invalid or has expired." };
      } else {
        message = { type: "success", text: "Password updated. Redirecting to sign in..." };
        setTimeout(() => goto("/login"), 1500);
      }
    } catch {
      message = { type: "error", text: "Unable to reset password with this token." };
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head>
  <title>Reset Password — Kinetiq</title>
</svelte:head>

<AuthShell
  title={tokenFromUrl ? "Set a new password" : "Reset your password"}
  subtitle={tokenFromUrl
    ? "Enter a new password below to restore access to your workspace."
    : "Enter the email associated with your account and we will send a recovery link."}
>
  {#if message}
    <div
      class="rounded-xl px-3.5 py-2.5 text-xs font-medium flex items-start gap-2 {message.type === 'success'
        ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] border border-emerald-500/20'
        : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/25'}"
    >
      {#if message.type === "success"}
        <Check class="w-4 h-4 shrink-0 mt-0.5" />
      {/if}
      <span>{message.text}</span>
    </div>
  {/if}

  {#if !tokenFromUrl}
    <form onsubmit={handleRequestReset} class="space-y-4">
      <div class="space-y-1.5">
        <label for="resetEmail" class="block text-xs font-medium text-[var(--text-secondary)]">
          Work email
        </label>
        <input
          id="resetEmail"
          type="email"
          bind:value={email}
          placeholder="name@company.com"
          class="w-full h-10 px-3.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:bg-[var(--bg-surface)] focus:border-[var(--brand-primary)] transition-all"
          required
          disabled={loading}
        />
      </div>

      <button
        type="submit"
        disabled={loading}
        class="w-full h-10 px-4 rounded-xl bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold flex items-center justify-center gap-2 transition-colors cursor-pointer disabled:opacity-50"
      >
        <span>{loading ? "Sending recovery link..." : "Send recovery link"}</span>
        <ArrowRight class="w-3.5 h-3.5" />
      </button>
    </form>
  {:else}
    <form onsubmit={handleConfirmReset} class="space-y-4">
      <div class="space-y-1.5">
        <div class="flex items-center justify-between">
          <label for="newPwd" class="block text-xs font-medium text-[var(--text-secondary)]">
            New password
          </label>
          <span class="text-[11px] font-mono text-[var(--text-muted)]">min. 8 chars</span>
        </div>
        <input
          id="newPwd"
          type="password"
          bind:value={newPassword}
          minlength={8}
          placeholder="Enter new password"
          class="w-full h-10 px-3.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] focus:outline-none focus:bg-[var(--bg-surface)] focus:border-[var(--brand-primary)] transition-all"
          required
        />
      </div>

      <div class="space-y-1.5">
        <label for="confirmPwd" class="block text-xs font-medium text-[var(--text-secondary)]">
          Confirm new password
        </label>
        <input
          id="confirmPwd"
          type="password"
          bind:value={confirmPassword}
          minlength={8}
          placeholder="Repeat new password"
          class="w-full h-10 px-3.5 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm text-[var(--text-primary)] focus:outline-none focus:bg-[var(--bg-surface)] focus:border-[var(--brand-primary)] transition-all"
          required
        />
      </div>

      <button
        type="submit"
        disabled={loading}
        class="w-full h-10 px-4 rounded-xl bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold flex items-center justify-center gap-2 transition-colors cursor-pointer disabled:opacity-50"
      >
        <span>{loading ? "Updating password..." : "Update password"}</span>
        <ArrowRight class="w-3.5 h-3.5" />
      </button>
    </form>
  {/if}

  {#snippet footer()}
    <div class="flex items-center justify-between text-xs text-[var(--text-secondary)]">
      <a href="/login" class="inline-flex items-center gap-1.5 font-semibold text-[var(--brand-primary)] hover:underline">
        <ArrowLeft class="w-3.5 h-3.5" />
        Back to sign in
      </a>
      <a href="/signup" class="text-[var(--text-muted)] hover:text-[var(--text-primary)]">
        Create workspace
      </a>
    </div>
  {/snippet}
</AuthShell>
