<script lang="ts">
  import { onMount } from "svelte";
  import { api, toSafeUserMessage, type CreateProjectResponse, type Project } from "$lib/api";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Card from "$lib/components/ui/Card.svelte";
  import {
    ArrowLeft,
    Check,
    Copy,
    KeyRound,
    AlertTriangle,
    Terminal,
    ArrowRight,
    Send,
    Sparkles,
    Code2,
    Globe,
    ShieldCheck,
    Zap
  } from "lucide-svelte";

  let name = $state("");
  let existingProjects = $state<Project[]>([]);
  let loading = $state(false);
  let error = $state("");
  let result = $state<CreateProjectResponse | null>(null);
  let copied = $state(false);
  let snippetCopied = $state(false);
  let activeTab = $state<"sdk" | "curl" | "fetch">("sdk");

  // Interactive test event state
  let sendingTest = $state(false);
  let testEventSuccess = $state(false);
  let testEventError = $state("");

  onMount(async () => {
    try {
      existingProjects = await api.getProjects();
    } catch {
      // ignore initial check failure
    }
  });

  const duplicateNameConflict = $derived(
    name.trim().length > 0 &&
      existingProjects.some(
        (p) => p.name.trim().toLowerCase() === name.trim().toLowerCase()
      )
  );

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    const trimmed = name.trim();
    if (!trimmed) {
      error = "Project name cannot be empty.";
      return;
    }
    if (duplicateNameConflict) {
      error = `A project named "${trimmed}" already exists in your workspace. Please choose a unique name.`;
      return;
    }

    loading = true;
    try {
      const res = await api.createProject(trimmed);
      result = res;
    } catch (err: any) {
      error = toSafeUserMessage(err, "Unable to create project right now.");
    } finally {
      loading = false;
    }
  }

  async function copyKey() {
    if (!result?.api_key) return;
    try {
      await navigator.clipboard.writeText(result.api_key);
      copied = true;
      setTimeout(() => {
        copied = false;
      }, 2500);
    } catch (e) {
      console.error("Clipboard copy failed", e);
    }
  }

  const currentSnippet = $derived.by(() => {
    const key = result?.api_key || "tlk_live_your_api_key";
    if (activeTab === "sdk") {
      return `import { KinetiqClient } from "@kinetiq/sdk";

const kinetiq = new KinetiqClient({
  apiKey: "${key}",
  endpoint: "http://localhost:8080/api/v1/ingest"
});

// Track user event
kinetiq.track("page_view", {
  distinct_id: "usr_100",
  path: "/pricing",
  plan: "growth"
});`;
    }
    if (activeTab === "fetch") {
      return `await fetch("http://localhost:8080/api/v1/ingest", {
  method: "POST",
  headers: {
    "Authorization": "Bearer ${key}",
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    events: [
      {
        event_id: crypto.randomUUID(),
        event_name: "page_view",
        distinct_id: "usr_100",
        occurred_at: new Date().toISOString(),
        properties: { path: "/pricing" }
      }
    ]
  })
});`;
    }
    return `curl -X POST http://localhost:8080/api/v1/ingest \\
  -H "Authorization: Bearer ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "events": [
      {
        "event_id": "evt_sample_01",
        "event_name": "page_view",
        "distinct_id": "usr_100",
        "occurred_at": "${new Date().toISOString()}",
        "properties": { "path": "/pricing" }
      }
    ]
  }'`;
  });

  async function copySnippet() {
    try {
      await navigator.clipboard.writeText(currentSnippet);
      snippetCopied = true;
      setTimeout(() => {
        snippetCopied = false;
      }, 2000);
    } catch (e) {
      console.error("Copy snippet failed", e);
    }
  }

  async function handleSendTestEvent() {
    if (!result?.api_key) return;
    sendingTest = true;
    testEventError = "";
    try {
      await api.sendTestEvent(result.api_key, "onboarding_verified", {
        project_name: result.project?.name || name,
        source: "kinetiq_onboarding_wizard"
      });
      testEventSuccess = true;
    } catch (err: any) {
      testEventError = toSafeUserMessage(err, "Unable to send test event.");
    } finally {
      sendingTest = false;
    }
  }
</script>

<div class="space-y-6 w-full">
  <!-- Back navigation -->
  <div>
    <a
      href="/app"
      class="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--brand-primary)] transition-colors"
    >
      <ArrowLeft class="w-3.5 h-3.5" />
      Back to All Projects
    </a>
    <h1 class="text-2xl sm:text-3xl font-extrabold tracking-tight text-[var(--text-primary)] mt-1">
      {result ? "Project Ready!" : "Create New Project"}
    </h1>
    <p class="text-xs sm:text-sm text-[var(--text-secondary)] mt-0.5">
      Set up a dedicated space for your website or app with its own API keys and activity dashboard.
    </p>
  </div>

  <!-- Balanced 12-Column Bento Split -->
  <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
    <!-- LEFT COLUMN (8 cols): Form or API Key Setup -->
    <div class="lg:col-span-8">
      {#if !result}
        <Card class="space-y-6">
          <div>
            <div class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] text-[11px] font-semibold mb-2">
              <Sparkles class="w-3.5 h-3.5" />
              New Project Setup
            </div>
            <h2 class="text-lg font-bold tracking-tight text-[var(--text-primary)]">
              Project Details
            </h2>
            <p class="text-xs text-[var(--text-secondary)] mt-0.5">
              Each project keeps its visitor activity, conversion funnels, live feed, and API keys neatly separated.
            </p>
          </div>

          {#if error}
            <div class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs font-medium text-rose-600 dark:text-rose-400 flex items-center gap-2">
              <AlertTriangle class="w-4 h-4 shrink-0" />
              <span>{error}</span>
            </div>
          {/if}

          <form onsubmit={handleSubmit} class="space-y-5">
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <label for="projectName" class="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
                  Project Name
                </label>
                <span class="text-[11px] text-[var(--text-muted)]">Unique name in your workspace</span>
              </div>
              <input
                id="projectName"
                type="text"
                bind:value={name}
                placeholder="e.g. My Online Store or Mobile App"
                class="w-full px-4 py-3 rounded-2xl bg-[var(--bg-surface-muted)] border {duplicateNameConflict ? 'border-rose-500 focus:border-rose-500' : 'border-[var(--border-subtle)] focus:border-[var(--brand-primary)]'} text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none transition-colors"
                required
                disabled={loading}
              />
              {#if duplicateNameConflict}
                <p class="text-xs text-rose-600 dark:text-rose-400 font-medium flex items-center gap-1.5 mt-1">
                  <AlertTriangle class="w-3.5 h-3.5 shrink-0" />
                  A project named "{name.trim()}" already exists. Please pick a different name.
                </p>
              {/if}
            </div>

            <div class="flex items-center justify-end gap-3 pt-2">
              <a href="/app">
                <Button variant="ghost" size="md">Cancel</Button>
              </a>
              <Button
                type="submit"
                variant="primary"
                size="md"
                loading={loading}
                disabled={duplicateNameConflict || !name.trim()}
              >
                Create Project
              </Button>
            </div>
          </form>
        </Card>
      {:else}
        <Card class="space-y-6">
          <div class="flex items-start justify-between gap-4">
            <div class="flex items-center gap-3.5">
              <div class="w-11 h-11 rounded-2xl bg-[var(--brand-soft)] flex items-center justify-center text-[var(--brand-primary)]">
                <Check class="w-6 h-6" />
              </div>
              <div>
                <Badge variant="emerald" pulse={true}>Project Created</Badge>
                <h3 class="text-xl font-extrabold text-[var(--text-primary)] mt-1">
                  {result.project?.name || result.name || name}
                </h3>
              </div>
            </div>
          </div>

          <div class="flex items-start gap-3 p-4 rounded-2xl bg-amber-500/10 border border-amber-500/25 text-xs text-amber-800 dark:text-amber-300">
            <AlertTriangle class="w-4 h-4 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" />
            <div>
              <strong class="font-bold">Copy your API key now:</strong>
              For your security, this full key is shown only once. You can always create new keys later in Project Settings.
            </div>
          </div>

          <div class="space-y-2">
            <label for="rawKey" class="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)] flex items-center gap-1.5">
              <KeyRound class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
              Your Project API Key
            </label>
            <div class="flex items-center gap-2">
              <input
                id="rawKey"
                type="text"
                readonly
                value={result.api_key}
                class="flex-1 px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono font-semibold text-[var(--brand-primary)] select-all focus:outline-none"
              />
              <Button variant="secondary" size="md" onclick={copyKey}>
                {#if copied}
                  <Check class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                  Copied!
                {:else}
                  <Copy class="w-3.5 h-3.5" />
                  Copy Key
                {/if}
              </Button>
            </div>
          </div>

          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-1 bg-[var(--bg-surface-muted)] p-1 rounded-full border border-[var(--border-subtle)]">
                <button
                  type="button"
                  onclick={() => (activeTab = "sdk")}
                  class="px-3 py-1 rounded-full text-xs font-medium transition-all cursor-pointer {activeTab === 'sdk' ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)]'}"
                >
                  <Code2 class="w-3 h-3 inline mr-1" />
                  @kinetiq/sdk
                </button>
                <button
                  type="button"
                  onclick={() => (activeTab = "curl")}
                  class="px-3 py-1 rounded-full text-xs font-medium transition-all cursor-pointer {activeTab === 'curl' ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)]'}"
                >
                  <Terminal class="w-3 h-3 inline mr-1" />
                  cURL
                </button>
                <button
                  type="button"
                  onclick={() => (activeTab = "fetch")}
                  class="px-3 py-1 rounded-full text-xs font-medium transition-all cursor-pointer {activeTab === 'fetch' ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)]'}"
                >
                  <Globe class="w-3 h-3 inline mr-1" />
                  Fetch API
                </button>
              </div>

              <button
                type="button"
                onclick={copySnippet}
                class="text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--brand-primary)] flex items-center gap-1 cursor-pointer"
              >
                {#if snippetCopied}
                  <Check class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                  Copied snippet
                {:else}
                  <Copy class="w-3.5 h-3.5" />
                  Copy code
                {/if}
              </button>
            </div>

            <pre class="p-4 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] overflow-x-auto leading-relaxed">{currentSnippet}</pre>
          </div>

          <div class="p-4 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            <div>
              <h4 class="text-xs font-bold text-[var(--text-primary)]">Send a Quick Test Event</h4>
              <p class="text-[11px] text-[var(--text-secondary)] mt-0.5">
                Click the button to send a sample <code class="font-mono text-[var(--brand-primary)]">onboarding_verified</code> activity to your dashboard right now.
              </p>
              {#if testEventError}
                <p class="text-[11px] text-rose-600 mt-1">{testEventError}</p>
              {/if}
            </div>

            {#if testEventSuccess}
              <Badge variant="emerald" pulse={true}>
                <Check class="w-3 h-3 mr-1" />
                Test Event Received!
              </Badge>
            {:else}
              <Button variant="secondary" size="sm" onclick={handleSendTestEvent} loading={sendingTest}>
                <Send class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                Send Test Event
              </Button>
            {/if}
          </div>

          <div class="pt-2 flex items-center justify-between">
            <a href="/app/projects/{result.project?.id || result.id}/settings">
              <Button variant="ghost" size="md">
                Manage API Keys & Settings
              </Button>
            </a>
            <a href="/app/projects/{result.project?.id || result.id}">
              <Button variant="primary" size="md">
                Open Project Dashboard
                <ArrowRight class="w-4 h-4 ml-1" />
              </Button>
            </a>
          </div>
        </Card>
      {/if}
    </div>

    <!-- RIGHT COLUMN (4 cols): Friendly Guide Card -->
    <div class="lg:col-span-4 space-y-5">
      <div class="rounded-[24px] bg-gradient-to-br from-[#127852] via-[#106B49] to-[#0C5439] text-white p-6 shadow-[var(--shadow-bento)] space-y-4">
        <span class="inline-block text-[10px] font-mono uppercase tracking-widest px-2.5 py-0.5 rounded-full bg-white/15 text-emerald-100">
          How Projects Work
        </span>
        <h3 class="text-lg font-extrabold tracking-tight">
          Dedicated Space for Each App
        </h3>
        <p class="text-xs text-emerald-100/85 leading-relaxed">
          Every project gets its own API keys, activity history settings, and live activity feed.
        </p>

        <div class="space-y-3 pt-2 border-t border-white/15 text-xs">
          <div class="flex items-start gap-2.5">
            <KeyRound class="w-4 h-4 text-emerald-300 shrink-0 mt-0.5" />
            <div>
              <div class="font-bold">Multiple API Keys</div>
              <div class="text-[11px] text-emerald-100/75">Create separate keys for your website, mobile app, or server.</div>
            </div>
          </div>
          <div class="flex items-start gap-2.5">
            <Zap class="w-4 h-4 text-emerald-300 shrink-0 mt-0.5" />
            <div>
              <div class="font-bold">Instant Live Updates</div>
              <div class="text-[11px] text-emerald-100/75">Watch user actions show up on your dashboard in real time.</div>
            </div>
          </div>
          <div class="flex items-start gap-2.5">
            <ShieldCheck class="w-4 h-4 text-emerald-300 shrink-0 mt-0.5" />
            <div>
              <div class="font-bold">Privacy Controls Built In</div>
              <div class="text-[11px] text-emerald-100/75">Choose how long to keep activity history and protect visitor privacy.</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</div>
