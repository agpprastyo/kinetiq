<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import {
    api,
    storageSyncApi,
    toSafeUserMessage,
    type Project,
    type ProjectApiKey,
    type AlertRule,
    type CreateAlertRulePayload,
    type ProjectStorageSync
  } from "$lib/api";
  import { copyToClipboard } from "$lib/utils";
  import Card from "$lib/components/ui/Card.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Modal from "$lib/components/ui/Modal.svelte";
  import {
    KeyRound,
    RefreshCw,
    Copy,
    Check,
    CheckCircle,
    AlertTriangle,
    ShieldAlert,
    Plus,
    Trash2,
    Save,
    Globe,
    Clock,
    ShieldCheck,
    Gauge,
    ArrowLeft,
    Send,
    Terminal,
    Bell,
    Radio,
    Cloud
  } from "lucide-svelte";

  const projectId = $derived(page.params.projectId as string);

  let project = $state<Project | null>(null);
  let allProjects = $state<Project[]>([]);
  let apiKeys = $state<ProjectApiKey[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Project General & Governance form states
  let projectName = $state("");
  let projectStatus = $state<"active" | "paused">("active");
  let retentionDays = $state(90);
  let allowedOriginsText = $state("*");
  let rateLimitPerSec = $state(1000);
  let anonymizeIp = $state(false);

  let savingSettings = $state(false);
  let settingsMessage = $state<{ type: "success" | "error"; text: string } | null>(null);

  // Multi-API Key states
  let isCreateKeyModalOpen = $state(false);
  let newKeyName = $state("");
  let newKeyEnv = $state<"live" | "test">("live");
  let creatingKey = $state(false);
  let newlyCreatedPlainKey = $state<string | null>(null);
  let newlyCreatedKeyLabel = $state("");
  let copiedKey = $state(false);
  let copiedProjectId = $state(false);
  let testingKey = $state(false);
  let testKeySuccess = $state(false);

  // Multi-Channel Alert Rules states
  let alertRules = $state<AlertRule[]>([]);
  let isCreateAlertModalOpen = $state(false);
  let newAlertName = $state("");
  let newAlertEvent = $state("*");
  let newAlertCondition = $state<"spike_gt" | "drop_lt">("spike_gt");
  let newAlertThreshold = $state(100);
  let newAlertWindow = $state(5);
  let newAlertChannel = $state<"slack" | "discord" | "telegram" | "webhook">("slack");
  let newAlertWebhookUrl = $state("");
  let newAlertWebhookSecret = $state("");
  let newAlertTelegramChatId = $state("");
  let newAlertCooldown = $state(15);
  let creatingAlert = $state(false);
  let testingAlertId = $state<string | null>(null);
  let alertTestSuccessId = $state<string | null>(null);

  // Revoke & Rotate Modal states
  let keyToRevoke = $state<ProjectApiKey | null>(null);
  let revokingKey = $state(false);
  let isRotateModalOpen = $state(false);
  let rotating = $state(false);

  // Cloud Storage Sync states
  let storageSync = $state<ProjectStorageSync | null>(null);
  let storageProvider = $state<"s3" | "r2" | "gcs" | "minio">("s3");
  let storageBucketName = $state("");
  let storageEndpointUrl = $state("");
  let storageRegion = $state("us-east-1");
  let storageAccessKeyId = $state("");
  let storageSecretAccessKey = $state("");
  let storagePrefixPath = $state("kinetiq-events");
  let storageFrequency = $state<"hourly" | "daily">("daily");
  let storageIsEnabled = $state(true);
  let savingStorage = $state(false);
  let testingStorage = $state(false);
  let storageStatusMessage = $state<{ type: "success" | "error"; text: string } | null>(null);

  // Danger Zone states
  let isPurgeModalOpen = $state(false);
  let purging = $state(false);
  let purgeResultMessage = $state<string | null>(null);

  let isDeleteModalOpen = $state(false);
  let deleteConfirmInput = $state("");
  let deletingProject = $state(false);

  function normalizeAllowedOrigins(raw: unknown): string {
    if (Array.isArray(raw)) {
      return raw.length > 0 ? raw.join(", ") : "*";
    }
    if (typeof raw === "string" && raw.trim().length > 0) {
      return raw.trim();
    }
    return "*";
  }

  async function loadProjectAndKeys() {
    if (!projectId) return;
    loading = true;
    error = null;
    try {
      const [proj, keys, projectsList, rules, sync] = await Promise.all([
        api.getProject(projectId),
        api.listApiKeys(projectId).catch(() => []),
        api.getProjects().catch(() => []),
        api.listAlertRules(projectId).catch(() => []),
        storageSyncApi.get(projectId).catch(() => null),
      ]);
      project = proj;
      apiKeys = Array.isArray(keys) ? keys : [];
      allProjects = Array.isArray(projectsList) ? projectsList : [];
      alertRules = Array.isArray(rules) ? rules : [];

      if (sync) {
        storageSync = sync;
        storageProvider = sync.provider || "s3";
        storageBucketName = sync.bucket_name || "";
        storageEndpointUrl = sync.endpoint_url || "";
        storageRegion = sync.region || "us-east-1";
        storageAccessKeyId = sync.access_key_id || "";
        storageSecretAccessKey = sync.secret_access_key || "";
        storagePrefixPath = sync.prefix_path || "kinetiq-events";
        storageFrequency = sync.frequency || "daily";
        storageIsEnabled = sync.is_enabled ?? true;
      }

      projectName = proj.name || "";
      projectStatus = proj.status === "paused" ? "paused" : "active";
      retentionDays = proj.retention_days ?? 90;
      allowedOriginsText = normalizeAllowedOrigins(proj.allowed_origins);
      rateLimitPerSec = proj.rate_limit_per_sec ?? 1000;
      anonymizeIp = Boolean(proj.anonymize_ip);
    } catch (e: any) {
      error = toSafeUserMessage(e, "Unable to load project settings right now.");
    } finally {
      loading = false;
    }
  }

  async function handleCreateAlertRule() {
    if (!projectId) return;
    const name = newAlertName.trim();
    if (!name || !newAlertWebhookUrl.trim()) return;

    creatingAlert = true;
    try {
      const payload: CreateAlertRulePayload = {
        name,
        event_name: newAlertEvent.trim() || "*",
        condition: newAlertCondition,
        threshold_count: Number(newAlertThreshold),
        window_minutes: Number(newAlertWindow),
        channel_type: newAlertChannel,
        webhook_url: newAlertWebhookUrl.trim(),
        webhook_secret: newAlertWebhookSecret.trim() || undefined,
        telegram_chat_id: newAlertChannel === "telegram" ? newAlertTelegramChatId.trim() : undefined,
        cooldown_minutes: Number(newAlertCooldown) || 15,
        is_active: true,
      };

      await api.createAlertRule(projectId, payload);
      alertRules = await api.listAlertRules(projectId);
      isCreateAlertModalOpen = false;
      newAlertName = "";
      newAlertWebhookUrl = "";
      newAlertWebhookSecret = "";
      newAlertTelegramChatId = "";
    } catch (err: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(err, "Failed to create alert rule.")
      };
      isCreateAlertModalOpen = false;
    } finally {
      creatingAlert = false;
    }
  }

  async function handleSaveStorageSync() {
    if (!projectId) return;
    if (!storageBucketName.trim() || !storageAccessKeyId.trim()) {
      storageStatusMessage = { type: "error", text: "Bucket Name and Access Key ID are required." };
      return;
    }
    savingStorage = true;
    storageStatusMessage = null;
    try {
      const updated = await storageSyncApi.put(projectId, {
        provider: storageProvider,
        bucket_name: storageBucketName.trim(),
        endpoint_url: storageEndpointUrl.trim(),
        region: storageRegion.trim() || "us-east-1",
        access_key_id: storageAccessKeyId.trim(),
        secret_access_key: storageSecretAccessKey.trim(),
        prefix_path: storagePrefixPath.trim() || "kinetiq-events",
        frequency: storageFrequency,
        is_enabled: storageIsEnabled,
      });
      storageSync = updated;
      storageSecretAccessKey = updated.secret_access_key || "********";
      storageStatusMessage = { type: "success", text: "Cloud Storage Sync configuration saved successfully." };
    } catch (err: any) {
      storageStatusMessage = { type: "error", text: toSafeUserMessage(err, "Failed to save storage sync settings.") };
    } finally {
      savingStorage = false;
    }
  }

  async function handleTestStorageSync() {
    if (!projectId) return;
    testingStorage = true;
    storageStatusMessage = null;
    try {
      const res = await storageSyncApi.test(projectId, {
        provider: storageProvider,
        bucket_name: storageBucketName.trim(),
        endpoint_url: storageEndpointUrl.trim(),
        region: storageRegion.trim() || "us-east-1",
        access_key_id: storageAccessKeyId.trim(),
        secret_access_key: storageSecretAccessKey.trim(),
        prefix_path: storagePrefixPath.trim(),
      });
      storageStatusMessage = { type: "success", text: res.message || "Cloud storage connection test succeeded!" };
    } catch (err: any) {
      storageStatusMessage = { type: "error", text: toSafeUserMessage(err, "Failed to connect to storage bucket.") };
    } finally {
      testingStorage = false;
    }
  }

  async function handleToggleAlertRule(rule: AlertRule) {
    try {
      await api.toggleAlertRule(projectId, rule.id, !rule.is_active);
      alertRules = await api.listAlertRules(projectId);
    } catch (err: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(err, "Failed to toggle alert rule status.")
      };
    }
  }

  async function handleDeleteAlertRule(ruleId: string) {
    try {
      await api.deleteAlertRule(projectId, ruleId);
      alertRules = alertRules.filter((r) => r.id !== ruleId);
    } catch (err: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(err, "Failed to delete alert rule.")
      };
    }
  }

  async function handleTestAlertRule(ruleId: string) {
    testingAlertId = ruleId;
    alertTestSuccessId = null;
    try {
      await api.testAlertRule(projectId, ruleId);
      alertTestSuccessId = ruleId;
      setTimeout(() => {
        if (alertTestSuccessId === ruleId) alertTestSuccessId = null;
      }, 3000);
    } catch (err: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(err, "Failed to dispatch test notification.")
      };
    } finally {
      testingAlertId = null;
    }
  }

  async function handleSnoozeAlertRule(ruleId: string, hours: number) {
    try {
      await api.snoozeAlertRule(projectId, ruleId, hours);
      alertRules = await api.listAlertRules(projectId);
      settingsMessage = {
        type: "success",
        text: hours > 0 ? `Alert rule snoozed for ${hours}h.` : "Alert rule snooze cancelled."
      };
    } catch (err: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(err, "Failed to snooze alert rule.")
      };
    }
  }

  const duplicateNameConflict = $derived(
    projectName.trim().length > 0 &&
      allProjects.some(
        (p) =>
          p.id !== projectId &&
          p.name.trim().toLowerCase() === projectName.trim().toLowerCase()
      )
  );

  async function handleSaveSettings(e: SubmitEvent) {
    e.preventDefault();
    if (!projectId) return;
    settingsMessage = null;

    const trimmedName = projectName.trim();
    if (!trimmedName) {
      settingsMessage = { type: "error", text: "Project name cannot be empty." };
      return;
    }
    if (duplicateNameConflict) {
      settingsMessage = {
        type: "error",
        text: `Another project named "${trimmedName}" already exists. Project names must be unique.`
      };
      return;
    }

    savingSettings = true;
    try {
      const originsString = allowedOriginsText
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean)
        .join(", ");

      const updated = await api.updateProject(projectId, {
        name: trimmedName,
        status: projectStatus,
        retention_days: Number(retentionDays),
        allowed_origins: originsString || "*",
        rate_limit_per_sec: Number(rateLimitPerSec),
        anonymize_ip: Boolean(anonymizeIp),
      });
      project = updated;
      settingsMessage = {
        type: "success",
        text: "Project settings and name updated successfully."
      };
    } catch (err: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(err, "Unable to save project settings.")
      };
    } finally {
      savingSettings = false;
    }
  }

  async function handleCreateApiKey() {
    if (!projectId) return;
    const label = newKeyName.trim() || "API Key";
    creatingKey = true;
    try {
      const res = await api.createApiKey(projectId, label, newKeyEnv);
      newlyCreatedPlainKey = res.api_key || res.raw_key || null;
      newlyCreatedKeyLabel = res.key?.name || res.name || label;
      testKeySuccess = false;
      isCreateKeyModalOpen = false;
      newKeyName = "";
      apiKeys = await api.listApiKeys(projectId);
    } catch (err: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(err, "Unable to create new API key.")
      };
      isCreateKeyModalOpen = false;
    } finally {
      creatingKey = false;
    }
  }

  async function handleConfirmRevokeKey() {
    if (!projectId || !keyToRevoke) return;
    revokingKey = true;
    try {
      await api.revokeApiKey(projectId, keyToRevoke.id);
      keyToRevoke = null;
      apiKeys = await api.listApiKeys(projectId);
    } catch (err: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(err, "Unable to revoke API key.")
      };
      keyToRevoke = null;
    } finally {
      revokingKey = false;
    }
  }

  async function confirmRotateAllKeys() {
    if (!projectId) return;
    rotating = true;
    try {
      const res = await api.rotateKey(projectId);
      project = res.project;
      newlyCreatedPlainKey = res.api_key;
      newlyCreatedKeyLabel = "Rotated Default Key";
      testKeySuccess = false;
      isRotateModalOpen = false;
      apiKeys = await api.listApiKeys(projectId);
    } catch (e: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(e, "Unable to rotate API keys right now.")
      };
      isRotateModalOpen = false;
    } finally {
      rotating = false;
    }
  }

  async function handleCopyPlainKey() {
    if (!newlyCreatedPlainKey) return;
    const ok = await copyToClipboard(newlyCreatedPlainKey);
    if (ok) {
      copiedKey = true;
      setTimeout(() => (copiedKey = false), 2000);
    }
  }

  async function handleCopyProjectId() {
    if (!project) return;
    const ok = await copyToClipboard(project.id);
    if (ok) {
      copiedProjectId = true;
      setTimeout(() => (copiedProjectId = false), 2000);
    }
  }

  async function handleSendTestWithNewKey() {
    if (!newlyCreatedPlainKey) return;
    testingKey = true;
    try {
      await api.sendTestEvent(newlyCreatedPlainKey, "api_key_verified", {
        key_label: newlyCreatedKeyLabel,
        source: "project_settings"
      });
      testKeySuccess = true;
      apiKeys = await api.listApiKeys(projectId);
    } catch (e: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(e, "Unable to send test event.")
      };
    } finally {
      testingKey = false;
    }
  }

  async function handleConfirmPurge() {
    if (!projectId) return;
    purging = true;
    purgeResultMessage = null;
    try {
      const res = await api.purgeEvents(projectId);
      const count = res.purged_count ?? res.purged_events ?? 0;
      isPurgeModalOpen = false;
      purgeResultMessage = `Successfully purged ${count.toLocaleString()} events from this project.`;
    } catch (e: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(e, "Unable to purge events right now.")
      };
      isPurgeModalOpen = false;
    } finally {
      purging = false;
    }
  }

  async function handleConfirmDeleteProject() {
    if (!projectId || !project) return;
    if (deleteConfirmInput.trim() !== project.name) return;
    deletingProject = true;
    try {
      await api.deleteProject(projectId);
      isDeleteModalOpen = false;
      goto("/app");
    } catch (e: any) {
      settingsMessage = {
        type: "error",
        text: toSafeUserMessage(e, "Unable to delete project right now.")
      };
      isDeleteModalOpen = false;
    } finally {
      deletingProject = false;
    }
  }

  $effect(() => {
    if (projectId) {
      loadProjectAndKeys();
    }
  });

  const activeKeysCount = $derived(apiKeys.filter((k) => !k.revoked_at).length);
  const primaryActivePrefix = $derived(
    apiKeys.find((k) => !k.revoked_at)?.key_prefix || "tlk_live_••••"
  );
</script>

<svelte:head>
  <title>{project ? `${project.name} Settings` : "Project Settings"} — Kinetiq</title>
</svelte:head>

<div class="space-y-6 w-full">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
    <div>
      <a
        href="/app/projects/{projectId}"
        class="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--brand-primary)] transition-colors"
      >
        <ArrowLeft class="w-3.5 h-3.5" />
        Back to Project Overview
      </a>
      <h1 class="text-2xl sm:text-3xl font-extrabold tracking-tight text-[var(--text-primary)] mt-1">
        Project Settings & API Keys
      </h1>
      <p class="text-xs sm:text-sm text-[var(--text-secondary)] mt-0.5">
        Manage settings for <strong>{project?.name || "this project"}</strong>: rename your project, create API keys, and control how long activity history is saved.
      </p>
    </div>
  </div>

  {#if loading}
    <div class="p-16 text-center text-xs font-mono text-[var(--text-muted)]">
      LOADING PROJECT SETTINGS...
    </div>
  {:else if error}
    <div class="p-6 rounded-2xl border border-rose-500/25 bg-rose-500/10 text-rose-600 dark:text-rose-400 text-sm flex items-center justify-between">
      <span>{error}</span>
      <Button variant="secondary" size="sm" onclick={loadProjectAndKeys}>Try Again</Button>
    </div>
  {:else if project}
    <!-- Balanced 12-Column Bento Split (Eliminates right-side empty hole cleanly) -->
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
      <!-- LEFT COLUMN (8 cols): Multi-API Keys Table + General & Privacy Form -->
      <div class="lg:col-span-8 space-y-6">
        <!-- 1. MULTI-API KEY MANAGEMENT CARD -->
        <Card class="space-y-5">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-[var(--border-subtle)]">
            <div>
              <div class="flex items-center gap-2">
                <KeyRound class="w-4 h-4 text-[var(--brand-primary)]" />
                <h2 class="text-base font-bold text-[var(--text-primary)]">
                  Project API Keys
                </h2>
                <Badge variant="emerald">{activeKeysCount} Active</Badge>
              </div>
              <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                Create separate keys for your website, mobile app, or testing environment and disable any key anytime.
              </p>
            </div>

            <div class="flex items-center gap-2 shrink-0">
              <Button
                variant="secondary"
                size="sm"
                onclick={() => (isRotateModalOpen = true)}
                title="Replace all active keys with a brand-new default key"
              >
                <RefreshCw class="w-3.5 h-3.5" />
                Rotate All
              </Button>
              <Button
                variant="primary"
                size="sm"
                onclick={() => {
                  newKeyName = "";
                  newKeyEnv = "live";
                  isCreateKeyModalOpen = true;
                }}
              >
                <Plus class="w-3.5 h-3.5" />
                Create API Key
              </Button>
            </div>
          </div>

          {#if newlyCreatedPlainKey}
            <!-- One-time Secret Key Banner -->
            <div class="p-4 rounded-2xl border border-emerald-500/30 bg-[var(--brand-soft)]/50 space-y-3">
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-start gap-2.5">
                  <AlertTriangle class="w-4 h-4 shrink-0 mt-0.5 text-[var(--brand-primary)]" />
                  <div class="text-xs text-[var(--text-primary)] leading-relaxed">
                    <strong>Copy your new API key ({newlyCreatedKeyLabel}) now!</strong> For your security, this full key is shown only once.
                  </div>
                </div>
                <button
                  type="button"
                  onclick={() => (newlyCreatedPlainKey = null)}
                  class="text-xs text-[var(--text-muted)] hover:text-[var(--text-primary)] cursor-pointer"
                >
                  Dismiss
                </button>
              </div>

              <div class="flex flex-wrap items-center gap-2 bg-[var(--bg-surface)] p-2.5 rounded-2xl border border-[var(--border-subtle)]">
                <code class="font-mono text-xs font-bold text-[var(--brand-primary)] break-all select-all flex-1 px-2">
                  {newlyCreatedPlainKey}
                </code>
                <Button variant="secondary" size="sm" onclick={handleCopyPlainKey}>
                  {#if copiedKey}
                    <Check class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                    Copied!
                  {:else}
                    <Copy class="w-3.5 h-3.5" />
                    Copy Key
                  {/if}
                </Button>
                {#if testKeySuccess}
                  <Badge variant="emerald" pulse={true}>
                    <Check class="w-3 h-3 mr-1" />
                    Verified!
                  </Badge>
                {:else}
                  <Button variant="primary" size="sm" onclick={handleSendTestWithNewKey} loading={testingKey}>
                    <Send class="w-3.5 h-3.5" />
                    Send Test Event
                  </Button>
                {/if}
              </div>
            </div>
          {/if}

          <!-- Keys Table -->
          <div class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead class="border-b border-[var(--border-subtle)] text-[11px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                <tr>
                  <th class="pb-3 pr-4">Key Name</th>
                  <th class="pb-3 px-3">Type</th>
                  <th class="pb-3 px-3">Key Preview</th>
                  <th class="pb-3 px-3">Created</th>
                  <th class="pb-3 px-3">Last Used</th>
                  <th class="pb-3 px-3">Status</th>
                  <th class="pb-3 pl-3 text-right">Action</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[var(--border-subtle)]">
                {#if apiKeys.length === 0}
                  <tr>
                    <td colspan="7" class="py-8 text-center text-[var(--text-muted)]">
                      No API keys found. Click "Create API Key" to create one.
                    </td>
                  </tr>
                {:else}
                  {#each apiKeys as k (k.id)}
                    {@const isRevoked = Boolean(k.revoked_at)}
                    <tr class="hover:bg-[var(--bg-surface-muted)]/50 transition-colors {isRevoked ? 'opacity-55' : ''}">
                      <td class="py-3.5 pr-4 font-bold text-[var(--text-primary)]">
                        {k.name}
                      </td>
                      <td class="py-3.5 px-3">
                        <Badge variant={k.environment === "test" ? "amber" : "emerald"}>
                          {k.environment}
                        </Badge>
                      </td>
                      <td class="py-3.5 px-3 font-mono text-[11px] text-[var(--text-secondary)]">
                        <code class="px-2 py-0.5 rounded-md bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)]">
                          {k.key_prefix}••••
                        </code>
                      </td>
                      <td class="py-3.5 px-3 font-mono text-[11px] text-[var(--text-muted)]">
                        {new Date(k.created_at).toLocaleDateString([], { month: "short", day: "numeric", year: "numeric" })}
                      </td>
                      <td class="py-3.5 px-3 font-mono text-[11px] text-[var(--text-muted)]">
                        {#if k.last_used_at}
                          {new Date(k.last_used_at).toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}
                        {:else}
                          Never used
                        {/if}
                      </td>
                      <td class="py-3.5 px-3">
                        {#if isRevoked}
                          <Badge variant="rose">Revoked</Badge>
                        {:else}
                          <Badge variant="emerald" pulse={true}>Active</Badge>
                        {/if}
                      </td>
                      <td class="py-3.5 pl-3 text-right">
                        {#if !isRevoked}
                          <button
                            type="button"
                            onclick={() => (keyToRevoke = k)}
                            class="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[11px] font-semibold text-rose-600 hover:bg-rose-500/10 transition-colors cursor-pointer"
                          >
                            <Trash2 class="w-3 h-3" />
                            Revoke
                          </button>
                        {:else}
                          <span class="text-[11px] font-mono text-[var(--text-muted)]">
                            Revoked
                          </span>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                {/if}
              </tbody>
            </table>
          </div>
        </Card>

        <!-- 2. GENERAL & PRIVACY SETTINGS FORM -->
        <Card>
          <form onsubmit={handleSaveSettings} class="space-y-6">
            <div class="pb-4 border-b border-[var(--border-subtle)] flex items-center justify-between">
              <div>
                <h2 class="text-base font-bold text-[var(--text-primary)]">
                  General & Privacy Settings
                </h2>
                <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                  Update your project name, tracking status, history duration, allowed domains, and visitor privacy options.
                </p>
              </div>
            </div>

            {#if settingsMessage}
              <div
                class="p-3.5 rounded-2xl text-xs font-medium flex items-center gap-2 {settingsMessage.type === 'success' ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] border border-emerald-500/20' : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/25'}"
              >
                {#if settingsMessage.type === "success"}
                  <Check class="w-4 h-4 shrink-0" />
                {:else}
                  <AlertTriangle class="w-4 h-4 shrink-0" />
                {/if}
                <span>{settingsMessage.text}</span>
              </div>
            {/if}

            <div class="grid grid-cols-1 md:grid-cols-2 gap-5">
              <!-- Project Name (Unique) -->
              <div class="space-y-1.5">
                <label for="projNameInput" class="text-xs font-semibold text-[var(--text-secondary)] flex items-center justify-between">
                  <span>Project Name</span>
                  <span class="text-[10px] text-[var(--text-muted)]">Unique in your workspace</span>
                </label>
                <input
                  id="projNameInput"
                  type="text"
                  bind:value={projectName}
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border {duplicateNameConflict ? 'border-rose-500' : 'border-[var(--border-subtle)] focus:border-[var(--brand-primary)]'} text-sm font-semibold text-[var(--text-primary)] focus:outline-none transition-colors"
                  required
                />
                {#if duplicateNameConflict}
                  <p class="text-[11px] text-rose-600 font-medium">
                    A project with this name already exists in your workspace.
                  </p>
                {/if}
              </div>

              <!-- Activity Tracking Status -->
              <div class="space-y-1.5">
                <label for="projStatusSelect" class="text-xs font-semibold text-[var(--text-secondary)]">
                  Activity Tracking Status
                </label>
                <select
                  id="projStatusSelect"
                  bind:value={projectStatus}
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm font-medium text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                >
                  <option value="active">Active — Recording visitor activity</option>
                  <option value="paused">Paused — Temporarily ignore new events</option>
                </select>
              </div>

              <!-- Activity History Duration -->
              <div class="space-y-1.5">
                <label for="retentionSelect" class="text-xs font-semibold text-[var(--text-secondary)] flex items-center gap-1.5">
                  <Clock class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                  Keep Activity History For
                </label>
                <select
                  id="retentionSelect"
                  bind:value={retentionDays}
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm font-medium text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                >
                  <option value={30}>30 Days (Starter)</option>
                  <option value={90}>90 Days (Standard Default)</option>
                  <option value={180}>180 Days (Growth)</option>
                  <option value={365}>365 Days (1 Full Year)</option>
                </select>
              </div>

              <!-- Max Events Per Second -->
              <div class="space-y-1.5">
                <label for="rateLimitInput" class="text-xs font-semibold text-[var(--text-secondary)] flex items-center gap-1.5">
                  <Gauge class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                  Maximum Events per Second
                </label>
                <input
                  id="rateLimitInput"
                  type="number"
                  min="10"
                  max="100000"
                  step="50"
                  bind:value={rateLimitPerSec}
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                />
              </div>

              <!-- Allowed Website Domains -->
              <div class="space-y-1.5 md:col-span-2">
                <label for="corsInput" class="text-xs font-semibold text-[var(--text-secondary)] flex items-center gap-1.5">
                  <Globe class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                  Allowed Website Domains (comma-separated)
                </label>
                <input
                  id="corsInput"
                  type="text"
                  bind:value={allowedOriginsText}
                  placeholder="*, https://app.yourdomain.id, http://localhost:5173"
                  class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                />
                <p class="text-[11px] text-[var(--text-muted)]">
                  Leave as <code class="font-mono">*</code> to allow any website, or list your own website domains so only your sites can send events.
                </p>
              </div>

              <!-- Privacy / IP Protection Toggle -->
              <div class="md:col-span-2 p-4 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] flex items-center justify-between gap-4">
                <div class="flex items-start gap-3">
                  <div class="w-9 h-9 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center shrink-0 mt-0.5">
                    <ShieldCheck class="w-4 h-4" />
                  </div>
                  <div>
                    <div class="text-sm font-bold text-[var(--text-primary)]">
                      Hide Visitor IP Addresses (UU PDP & GDPR Privacy)
                    </div>
                    <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                      Automatically mask visitor IP addresses before saving activity so personal network addresses are never stored.
                    </p>
                  </div>
                </div>
                <input
                  type="checkbox"
                  bind:checked={anonymizeIp}
                  class="w-5 h-5 accent-[var(--brand-primary)] rounded cursor-pointer shrink-0"
                />
              </div>
            </div>

            <div class="flex justify-end pt-2">
              <Button
                type="submit"
                variant="primary"
                size="md"
                loading={savingSettings}
                disabled={duplicateNameConflict || !projectName.trim()}
              >
                <Save class="w-4 h-4" />
                Save Project Settings
              </Button>
            </div>
          </form>
        </Card>

        <!-- 3. Multi-Channel Alert Rules Section -->
        <Card class="p-6 sm:p-7 space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            <div class="space-y-1">
              <div class="flex items-center gap-2">
                <div class="w-8 h-8 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
                  <Bell class="w-4 h-4" />
                </div>
                <h2 class="text-base sm:text-lg font-bold text-[var(--text-primary)]">
                  Multi-Channel Alert Rules
                </h2>
              </div>
              <p class="text-xs text-[var(--text-muted)]">
                Dapatkan notifikasi instan via Slack, Discord, Telegram, atau custom Webhook saat terjadi lonjakan traffic atau penurunan event drastis.
              </p>
            </div>
            <Button
              variant="primary"
              size="sm"
              onclick={() => { isCreateAlertModalOpen = true; }}
            >
              <Plus class="w-3.5 h-3.5 mr-1" />
              New Alert Rule
            </Button>
          </div>

          {#if alertRules.length === 0}
            <div class="p-6 rounded-2xl border border-dashed border-[var(--border-subtle)] text-center space-y-2">
              <p class="text-xs font-semibold text-[var(--text-secondary)]">Belum ada rule alert yang dibuat</p>
              <p class="text-[11px] text-[var(--text-muted)] max-w-md mx-auto">
                Konfigurasikan webhook monitoring agar tim Anda segera tahu saat volume event mencapai batas threshold.
              </p>
            </div>
          {:else}
            <div class="divide-y divide-[var(--border-subtle)] border border-[var(--border-subtle)] rounded-2xl overflow-hidden">
              {#each alertRules as rule}
                <div class="p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-[var(--bg-surface)] hover:bg-[var(--bg-surface-hover)] transition-colors">
                  <div class="space-y-1">
                    <div class="flex items-center gap-2">
                      <span class="font-bold text-xs text-[var(--text-primary)]">{rule.name}</span>
                      <span class="px-2 py-0.5 rounded-full text-[10px] font-mono uppercase bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 font-semibold">
                        {rule.channel_type}
                      </span>
                      <!-- Status Badge -->
                      {#if rule.status === "firing"}
                        <span class="px-2 py-0.5 rounded-full text-[10px] font-bold bg-rose-500/15 text-rose-600 dark:text-rose-400 animate-pulse">
                          Firing
                        </span>
                      {:else if rule.status === "snoozed" || (rule.snoozed_until && new Date(rule.snoozed_until) > new Date())}
                        <span class="px-2 py-0.5 rounded-full text-[10px] font-bold bg-amber-500/15 text-amber-600 dark:text-amber-400">
                          Snoozed
                        </span>
                      {:else if !rule.is_active}
                        <span class="px-2 py-0.5 rounded-full text-[10px] bg-slate-500/10 text-slate-400">
                          Paused
                        </span>
                      {:else}
                        <span class="px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                          Healthy
                        </span>
                      {/if}
                    </div>
                    <div class="text-[11px] text-[var(--text-muted)] flex items-center gap-2 flex-wrap">
                      <span>Event: <code class="font-mono text-[var(--text-secondary)]">{rule.event_name}</code></span>
                      <span>·</span>
                      <span>Kondisi: <code class="font-mono">{rule.condition} {rule.threshold_count}</code> dlm {rule.window_minutes}m</span>
                      <span>·</span>
                      <span>Cooldown: {rule.cooldown_minutes}m</span>
                      {#if rule.snoozed_until && new Date(rule.snoozed_until) > new Date()}
                        <span>·</span>
                        <span class="text-amber-600 dark:text-amber-400 font-medium">
                          Snooze sampai: {new Date(rule.snoozed_until).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                        </span>
                      {/if}
                    </div>
                  </div>

                  <div class="flex items-center gap-2 self-end sm:self-center flex-wrap">
                    <!-- Snooze Selector Menu -->
                    <select
                      class="px-2 py-1 text-[11px] rounded-lg bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-[var(--text-secondary)] focus:outline-none focus:border-[var(--brand-primary)]"
                      onchange={(e) => {
                        const val = Number((e.target as HTMLSelectElement).value);
                        handleSnoozeAlertRule(rule.id, val);
                      }}
                    >
                      <option value="" disabled selected>Snooze...</option>
                      <option value="1">Snooze 1h</option>
                      <option value="6">Snooze 6h</option>
                      <option value="24">Snooze 24h</option>
                      <option value="0">Cancel Snooze</option>
                    </select>

                    <Button
                      variant="secondary"
                      size="sm"
                      onclick={() => handleTestAlertRule(rule.id)}
                      loading={testingAlertId === rule.id}
                      title="Test send payload to webhook"
                    >
                      {#if alertTestSuccessId === rule.id}
                        <CheckCircle class="w-3.5 h-3.5 text-emerald-500 mr-1" />
                        <span class="text-emerald-500">Sent!</span>
                      {:else}
                        <Send class="w-3.5 h-3.5 mr-1" />
                        Test
                      {/if}
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      onclick={() => handleToggleAlertRule(rule)}
                    >
                      {rule.is_active ? "Pause" : "Enable"}
                    </Button>
                    <button
                      onclick={() => handleDeleteAlertRule(rule.id)}
                      class="p-2 rounded-xl text-rose-500 hover:bg-rose-500/10 transition-colors cursor-pointer"
                      title="Delete rule"
                    >
                      <Trash2 class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </Card>

        <!-- CLOUD STORAGE SYNC CARD (Fase 17 Enterprise Data Portability) -->
        <Card class="p-6 sm:p-7 space-y-6">
          <div class="flex items-center justify-between flex-wrap gap-4">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-2xl bg-cyan-500/10 flex items-center justify-center text-cyan-600 dark:text-cyan-400">
                <Cloud class="w-5 h-5" />
              </div>
              <div>
                <h3 class="text-base font-semibold text-[var(--text-primary)]">Automated Cloud Storage Sync</h3>
                <p class="text-xs text-[var(--text-muted)]">Export raw compressed jsonl.gz events to your S3/R2/GCS bucket.</p>
              </div>
            </div>
            {#if storageSync?.last_sync_status}
              <div class="flex items-center gap-2">
                <span class="text-[11px] text-[var(--text-muted)]">Status:</span>
                <span class="px-2.5 py-0.5 rounded-full text-[10px] font-mono uppercase {storageSync.last_sync_status === 'success' ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30' : storageSync.last_sync_status === 'failed' ? 'bg-rose-500/15 text-rose-600 dark:text-rose-400 border border-rose-500/30' : storageSync.last_sync_status === 'syncing' ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30' : 'bg-[var(--bg-surface-muted)] text-[var(--text-secondary)]'}">
                  {storageSync.last_sync_status}
                </span>
              </div>
            {/if}
          </div>

          {#if storageStatusMessage}
            <div class="p-3.5 rounded-2xl text-xs flex items-center gap-2 {storageStatusMessage.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20' : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20'}">
              {#if storageStatusMessage.type === 'success'}
                <CheckCircle class="w-4 h-4 shrink-0" />
              {:else}
                <AlertTriangle class="w-4 h-4 shrink-0" />
              {/if}
              <span>{storageStatusMessage.text}</span>
            </div>
          {/if}

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div class="space-y-1.5">
              <label for="storageProvider" class="text-xs font-semibold text-[var(--text-secondary)]">Storage Provider</label>
              <select
                id="storageProvider"
                bind:value={storageProvider}
                class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              >
                <option value="s3">Amazon S3</option>
                <option value="r2">Cloudflare R2</option>
                <option value="gcs">Google Cloud Storage (S3 API)</option>
                <option value="minio">MinIO / Self-Hosted</option>
              </select>
            </div>

            <div class="space-y-1.5">
              <label for="storageBucketName" class="text-xs font-semibold text-[var(--text-secondary)]">Bucket Name</label>
              <input
                id="storageBucketName"
                type="text"
                bind:value={storageBucketName}
                placeholder="my-enterprise-data-lake"
                class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              />
            </div>

            <div class="space-y-1.5">
              <label for="storageRegion" class="text-xs font-semibold text-[var(--text-secondary)]">Region</label>
              <input
                id="storageRegion"
                type="text"
                bind:value={storageRegion}
                placeholder="us-east-1"
                class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              />
            </div>

            <div class="space-y-1.5">
              <label for="storageEndpointUrl" class="text-xs font-semibold text-[var(--text-secondary)]">
                Custom Endpoint URL {storageProvider === 'r2' || storageProvider === 'minio' ? '(Required)' : '(Optional)'}
              </label>
              <input
                id="storageEndpointUrl"
                type="text"
                bind:value={storageEndpointUrl}
                placeholder="https://<account-id>.r2.cloudflarestorage.com"
                class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              />
            </div>

            <div class="space-y-1.5">
              <label for="storageAccessKeyId" class="text-xs font-semibold text-[var(--text-secondary)]">Access Key ID</label>
              <input
                id="storageAccessKeyId"
                type="text"
                bind:value={storageAccessKeyId}
                placeholder="AKIAIOSFODNN7EXAMPLE"
                class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              />
            </div>

            <div class="space-y-1.5">
              <label for="storageSecretAccessKey" class="text-xs font-semibold text-[var(--text-secondary)]">Secret Access Key</label>
              <input
                id="storageSecretAccessKey"
                type="password"
                bind:value={storageSecretAccessKey}
                placeholder="••••••••••••••••••••••••"
                class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              />
            </div>

            <div class="space-y-1.5">
              <label for="storagePrefixPath" class="text-xs font-semibold text-[var(--text-secondary)]">Prefix Path</label>
              <input
                id="storagePrefixPath"
                type="text"
                bind:value={storagePrefixPath}
                placeholder="kinetiq-events"
                class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              />
            </div>

            <div class="space-y-1.5">
              <label for="storageFrequency" class="text-xs font-semibold text-[var(--text-secondary)]">Sync Frequency</label>
              <select
                id="storageFrequency"
                bind:value={storageFrequency}
                class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
              >
                <option value="hourly">Hourly (Every 1 Hour)</option>
                <option value="daily">Daily (Every 24 Hours)</option>
              </select>
            </div>
          </div>

          <div class="flex items-center justify-between pt-2 border-t border-[var(--border-subtle)] flex-wrap gap-3">
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                bind:checked={storageIsEnabled}
                class="w-4 h-4 rounded text-[var(--brand-primary)] focus:ring-0"
              />
              <span class="text-xs font-medium text-[var(--text-secondary)]">Enable Automated Sync Worker</span>
            </label>

            <div class="flex items-center gap-2">
              <Button
                variant="secondary"
                size="sm"
                onclick={handleTestStorageSync}
                loading={testingStorage}
              >
                <Radio class="w-3.5 h-3.5 mr-1" />
                Test Bucket Connection
              </Button>
              <Button
                variant="primary"
                size="sm"
                onclick={handleSaveStorageSync}
                loading={savingStorage}
              >
                <Save class="w-3.5 h-3.5 mr-1" />
                Save Sync Settings
              </Button>
            </div>
          </div>
        </Card>
      </div>

      <!-- RIGHT COLUMN (4 cols): Project Summary, Quick Reference & Danger Zone -->
      <div class="lg:col-span-4 space-y-6">
        <!-- Project Summary & Quick Endpoint Card -->
        <div class="rounded-[24px] bg-gradient-to-br from-[#127852] via-[#106B49] to-[#0C5439] text-white p-6 shadow-[var(--shadow-bento)] space-y-4">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-emerald-100">Project Summary</span>
            <span class="px-2.5 py-0.5 rounded-full text-[10px] font-mono uppercase bg-white/15 text-emerald-100">
              {project.status}
            </span>
          </div>

          <div>
            <h3 class="text-xl font-extrabold tracking-tight">{project.name}</h3>
            <div class="mt-2 p-2.5 rounded-xl bg-black/20 flex items-center justify-between text-[11px] font-mono text-emerald-100">
              <span class="truncate">{project.id}</span>
              <button
                type="button"
                onclick={handleCopyProjectId}
                class="ml-2 p-1 rounded hover:bg-white/10 text-white shrink-0 cursor-pointer"
                title="Copy Project ID"
              >
                {#if copiedProjectId}
                  <Check class="w-3.5 h-3.5 text-emerald-300" />
                {:else}
                  <Copy class="w-3.5 h-3.5" />
                {/if}
              </button>
            </div>
          </div>

          <div class="space-y-2 pt-1">
            <div class="text-[11px] font-semibold text-emerald-100 flex items-center gap-1.5">
              <Terminal class="w-3.5 h-3.5" />
              Event API Endpoint
            </div>
            <pre class="p-3 rounded-xl bg-black/25 text-[11px] font-mono text-emerald-100 overflow-x-auto leading-relaxed">POST /api/v1/ingest
Authorization: Bearer {primaryActivePrefix}••••</pre>
          </div>
        </div>

        <!-- Danger Zone Card -->
        <Card class="border-rose-500/25 space-y-4">
          <div>
            <h2 class="text-sm font-bold text-rose-600 dark:text-rose-400 flex items-center gap-2">
              <ShieldAlert class="w-4 h-4" />
              Danger Zone
            </h2>
            <p class="text-xs text-[var(--text-secondary)] mt-0.5">
              Permanent actions for <strong>{project.name}</strong>.
            </p>
          </div>

          {#if purgeResultMessage}
            <div class="p-3 rounded-2xl bg-amber-500/10 border border-amber-500/25 text-xs font-medium text-amber-700 dark:text-amber-300">
              {purgeResultMessage}
            </div>
          {/if}

          <div class="divide-y divide-[var(--border-subtle)]">
            <!-- Clear Events -->
            <div class="py-3.5 first:pt-0 space-y-2.5">
              <div>
                <div class="text-xs font-bold text-[var(--text-primary)]">Clear All Recorded Activity</div>
                <p class="text-[11px] text-[var(--text-secondary)] mt-0.5">
                  Delete all saved events while keeping your project settings and API keys active.
                </p>
              </div>
              <Button variant="secondary" size="sm" onclick={() => (isPurgeModalOpen = true)} class="w-full">
                Clear Project Activity
              </Button>
            </div>

            <!-- Delete Project -->
            <div class="py-3.5 last:pb-0 space-y-2.5">
              <div>
                <div class="text-xs font-bold text-rose-600 dark:text-rose-400">Delete Project Permanently</div>
                <p class="text-[11px] text-[var(--text-secondary)] mt-0.5">
                  Deletes this project, disables all its API keys, and erases all saved activity.
                </p>
              </div>
              <Button
                variant="danger"
                size="sm"
                onclick={() => {
                  deleteConfirmInput = "";
                  isDeleteModalOpen = true;
                }}
                class="w-full"
              >
                <Trash2 class="w-3.5 h-3.5" />
                Delete Project
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </div>
  {/if}
</div>

<!-- Create New Multi-API Key Modal -->
<Modal
  open={isCreateKeyModalOpen}
  title="Create New API Key"
  description="Create a dedicated API key for your website, mobile app, or testing environment."
  confirmText="Generate API Key"
  confirmVariant="primary"
  confirmLoading={creatingKey}
  onconfirm={handleCreateApiKey}
  oncancel={() => (isCreateKeyModalOpen = false)}
>
  <div class="space-y-4">
    <div class="space-y-1.5">
      <label for="keyLabelInput" class="text-xs font-semibold text-[var(--text-secondary)]">
        Key Name / Description
      </label>
      <input
        id="keyLabelInput"
        type="text"
        bind:value={newKeyName}
        placeholder="e.g. Android App, Main Website"
        class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
      />
    </div>

    <div class="space-y-1.5">
      <label for="keyEnvSelect" class="text-xs font-semibold text-[var(--text-secondary)]">
        Environment Type
      </label>
      <select
        id="keyEnvSelect"
        bind:value={newKeyEnv}
        class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
      >
        <option value="live">Production Live (tlk_live_...)</option>
        <option value="test">Staging / Test (tlk_test_...)</option>
      </select>
    </div>
  </div>
</Modal>

<!-- Revoke Single API Key Modal -->
<Modal
  open={Boolean(keyToRevoke)}
  title="Revoke API Key?"
  confirmText="Revoke Key"
  confirmVariant="danger"
  confirmLoading={revokingKey}
  onconfirm={handleConfirmRevokeKey}
  oncancel={() => (keyToRevoke = null)}
>
  <p class="text-xs text-[var(--text-secondary)] leading-relaxed">
    Are you sure you want to disable <strong>{keyToRevoke?.name}</strong> (<code class="font-mono">{keyToRevoke?.key_prefix}••••</code>)? Any app using this key will stop sending events immediately, while your other active keys will keep working normally.
  </p>
</Modal>

<!-- Rotate All Keys Modal -->
<Modal
  open={isRotateModalOpen}
  title="Rotate All API Keys?"
  confirmText="Rotate All Keys"
  confirmVariant="danger"
  confirmLoading={rotating}
  onconfirm={confirmRotateAllKeys}
  oncancel={() => (isRotateModalOpen = false)}
>
  <div class="space-y-3">
    <div class="flex items-start gap-3 p-3 rounded-2xl bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400">
      <ShieldAlert class="w-5 h-5 shrink-0 mt-0.5" />
      <div class="text-xs leading-relaxed">
        <strong>Warning:</strong> All existing active keys for <strong>{project?.name}</strong> will be immediately disabled and replaced by a single new default key.
      </div>
    </div>
  </div>
</Modal>

<!-- Clear All Events Confirmation Modal -->
<Modal
  open={isPurgeModalOpen}
  title="Clear All Recorded Activity?"
  confirmText="Yes, Clear All Activity"
  confirmVariant="danger"
  confirmLoading={purging}
  onconfirm={handleConfirmPurge}
  oncancel={() => (isPurgeModalOpen = false)}
>
  <p class="text-xs text-[var(--text-secondary)] leading-relaxed">
    This will permanently delete all recorded activity for <strong>{project?.name}</strong>. Your project settings and API keys will remain untouched.
  </p>
</Modal>

<!-- Delete Project Permanently Modal -->
<Modal
  open={isDeleteModalOpen}
  title="Delete Project Permanently?"
  confirmText="Delete Project"
  confirmVariant="danger"
  confirmLoading={deletingProject}
  onconfirm={handleConfirmDeleteProject}
  oncancel={() => (isDeleteModalOpen = false)}
>
  <div class="space-y-3">
    <p class="text-xs text-[var(--text-secondary)] leading-relaxed">
      This action cannot be undone. Type <strong class="text-[var(--text-primary)] font-mono">{project?.name}</strong> below to confirm permanent deletion:
    </p>
    <input
      type="text"
      bind:value={deleteConfirmInput}
      placeholder={project?.name}
      class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-rose-500"
    />
  </div>
</Modal>

<!-- Create Alert Rule Modal -->
<Modal
  open={isCreateAlertModalOpen}
  title="Create Alert Rule"
  confirmText="Save Alert Rule"
  confirmVariant="primary"
  confirmLoading={creatingAlert}
  onconfirm={handleCreateAlertRule}
  oncancel={() => (isCreateAlertModalOpen = false)}
>
  <div class="space-y-4">
    <div class="space-y-1.5">
      <label for="alertNameInput" class="text-xs font-semibold text-[var(--text-secondary)]">Rule Name</label>
      <input
        id="alertNameInput"
        type="text"
        bind:value={newAlertName}
        placeholder="e.g. Traffic Spike Alert"
        class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
      />
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div class="space-y-1.5">
        <label for="alertEventInput" class="text-xs font-semibold text-[var(--text-secondary)]">Event Name</label>
        <input
          id="alertEventInput"
          type="text"
          bind:value={newAlertEvent}
          placeholder="* or page_view"
          class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        />
      </div>

      <div class="space-y-1.5">
        <label for="alertConditionSelect" class="text-xs font-semibold text-[var(--text-secondary)]">Condition</label>
        <select
          id="alertConditionSelect"
          bind:value={newAlertCondition}
          class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        >
          <option value="spike_gt">Spike Greater Than (&gt;)</option>
          <option value="drop_lt">Drop Less Than (&lt;)</option>
        </select>
      </div>
    </div>

    <div class="grid grid-cols-3 gap-3">
      <div class="space-y-1.5">
        <label for="alertThresholdInput" class="text-xs font-semibold text-[var(--text-secondary)]">Threshold Count</label>
        <input
          id="alertThresholdInput"
          type="number"
          min="1"
          bind:value={newAlertThreshold}
          class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        />
      </div>

      <div class="space-y-1.5">
        <label for="alertWindowInput" class="text-xs font-semibold text-[var(--text-secondary)]">Window (Mins)</label>
        <input
          id="alertWindowInput"
          type="number"
          min="1"
          max="60"
          bind:value={newAlertWindow}
          class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        />
      </div>

      <div class="space-y-1.5">
        <label for="alertCooldownInput" class="text-xs font-semibold text-[var(--text-secondary)]">Cooldown (Mins)</label>
        <input
          id="alertCooldownInput"
          type="number"
          min="1"
          max="1440"
          bind:value={newAlertCooldown}
          class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        />
      </div>
    </div>

    <div class="space-y-1.5">
      <label for="alertChannelSelect" class="text-xs font-semibold text-[var(--text-secondary)]">Destination Channel</label>
      <select
        id="alertChannelSelect"
        bind:value={newAlertChannel}
        class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
      >
        <option value="slack">Slack Webhook</option>
        <option value="discord">Discord Webhook</option>
        <option value="telegram">Telegram Bot</option>
        <option value="webhook">Custom HMAC Webhook</option>
      </select>
    </div>

    <div class="space-y-1.5">
      <label for="alertWebhookUrlInput" class="text-xs font-semibold text-[var(--text-secondary)]">
        {newAlertChannel === "telegram" ? "Telegram Bot Token / API URL" : "Webhook URL"}
      </label>
      <input
        id="alertWebhookUrlInput"
        type="text"
        bind:value={newAlertWebhookUrl}
        placeholder={newAlertChannel === "slack" ? "https://hooks.slack.com/services/..." : newAlertChannel === "discord" ? "https://discord.com/api/webhooks/..." : newAlertChannel === "telegram" ? "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11" : "https://api.yourdomain.com/webhooks/alerts"}
        class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
      />
    </div>

    {#if newAlertChannel === "telegram"}
      <div class="space-y-1.5">
        <label for="alertChatIdInput" class="text-xs font-semibold text-[var(--text-secondary)]">Telegram Chat ID</label>
        <input
          id="alertChatIdInput"
          type="text"
          bind:value={newAlertTelegramChatId}
          placeholder="-1001234567890"
          class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        />
      </div>
    {/if}

    {#if newAlertChannel === "webhook"}
      <div class="space-y-1.5">
        <label for="alertSecretInput" class="text-xs font-semibold text-[var(--text-secondary)]">Webhook HMAC Secret (Optional)</label>
        <input
          id="alertSecretInput"
          type="password"
          bind:value={newAlertWebhookSecret}
          placeholder="whsec_..."
          class="w-full px-3.5 py-2 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
        />
      </div>
    {/if}
  </div>
</Modal>
