<script lang="ts">
  import { onMount, onDestroy, untrack } from "svelte";
  import type { FunnelOrder } from "$lib/funnel";
  import { page } from "$app/state";
  import { authClient } from "$lib/auth-client";
  import {
    api,
    type Project,
    type SummaryData,
    type TopEventItem,
    type VolumeData,
    type FunnelData,
    type BreakdownItem,
    type ProjectApiKey,
    type ProjectQuota,
    type ProjectInsight,
    type CreateInsightPayload,
    type WeeklyDigestReport,
    type DeviceBreakdownItem,
    type SessionsResponse,
    type GeoBreakdownItem,
  } from "$lib/api";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Card from "$lib/components/ui/Card.svelte";
  import Tabs from "$lib/components/ui/Tabs.svelte";
  import VolumeChart from "$lib/components/ui/VolumeChart.svelte";
  import FunnelVisualizer from "$lib/components/analytics/FunnelVisualizer.svelte";
  import PropertyBreakdown from "$lib/components/analytics/PropertyBreakdown.svelte";
  import DeviceAndGeoBreakdown from "$lib/components/analytics/DeviceAndGeoBreakdown.svelte";
  import SessionsCard from "$lib/components/analytics/SessionsCard.svelte";
  import FilterPicker from "$lib/components/analytics/FilterPicker.svelte";
  import type { QueryExtras } from "$lib/filters";
  import RetentionHeatmap from "$lib/components/analytics/RetentionHeatmap.svelte";
  import InsightWidgetGrid from "$lib/components/analytics/InsightWidgetGrid.svelte";
  import CreateInsightModal from "$lib/components/analytics/CreateInsightModal.svelte";
  import WeeklyDigestCard from "$lib/components/analytics/WeeklyDigestCard.svelte";
  import { copyToClipboard } from "$lib/utils";
  import {
    RefreshCw,
    Copy,
    Check,
    Users,
    Zap,
    TrendingUp,
    BarChart3,
    ArrowUpRight,
    ArrowDownLeft,
    Calendar,
    Plus,
    KeyRound,
    ShieldCheck,
    Settings,
    Radio,
    Sparkles,
    AlertTriangle
  } from "lucide-svelte";

  const projectId = $derived(page.params.projectId as string);

  let userName = $state("there");
  let project = $state<Project | null>(null);
  let apiKeys = $state<ProjectApiKey[]>([]);
  let summary = $state<SummaryData | null>(null);
  let topEvents = $state<TopEventItem[]>([]);
  let volume = $state<VolumeData | null>(null);
  let funnel = $state<FunnelData | null>(null);
  let breakdown = $state<BreakdownItem[]>([]);
  let quota = $state<ProjectQuota | null>(null);
  let retention = $state<any>(null);
  let retentionLoading = $state(false);

  // Custom Insight Widgets
  let insights = $state<ProjectInsight[]>([]);
  let insightsLoading = $state(false);
  let isCreateInsightOpen = $state(false);

  // Phase 10: Automated Intelligence Weekly Digest
  let weeklyDigest = $state<WeeklyDigestReport | null>(null);
  let digestLoading = $state(false);

  // Phase 16: Device & Geo Breakdowns
  let devices = $state<DeviceBreakdownItem[]>([]);
  let geo = $state<GeoBreakdownItem[]>([]);
  let deviceGeoLoading = $state(false);
  let sessions = $state<SessionsResponse | null>(null);
  let sessionsLoading = $state(false);

  let selectedRange = $state("24h");
  let queryExtras = $state<QueryExtras>({});
  let funnelStep1 = $state("page_view");
  let funnelStep2 = $state("signup");
  let funnelStep3 = $state("");
  let funnelStep4 = $state("");
  let funnelStep5 = $state("");
  let funnelWindow = $state("14d");
  let funnelOrder = $state<FunnelOrder>("ordered");
  let funnelSteps: string[] = [];

  let breakdownEvent = $state("page_view");
  let breakdownProp = $state("browser");

  let loading = $state(true);
  let refreshing = $state(false);
  let funnelLoading = $state(false);
  let breakdownLoading = $state(false);
  let exporting = $state(false);
  let copied = $state(false);
  let error = $state("");
  // Per-card failures, kept apart from "no data" so an outage never looks like an empty project.
  let cardErrors = $state<{ summary?: string; top?: string; volume?: string }>({});

  let currentAbortController: AbortController | null = null;
  let lastLoadedKey = "";

  // Onboarding Real-time SSE Connection Detector states
  let onboardingEventSource: EventSource | null = null;
  let sendingTestEvent = $state(false);
  let firstEventDetected = $state(false);
  let copiedSnippet = $state(false);

  const activeKeyPrefix = $derived(
    apiKeys.find((k) => !k.revoked_at)?.key_prefix || "tlk_live_demo"
  );

  async function handleSendTestEvent() {
    if (!projectId) return;
    sendingTestEvent = true;
    try {
      const activeKey = apiKeys.find((k) => !k.revoked_at);
      await api.sendTestEvent(activeKey?.key_prefix || "tlk_live_demo", "first_ping", {
        source: "onboarding_detector",
        dashboard_version: "2.0"
      });
      firstEventDetected = true;
      setTimeout(() => loadData(true), 1200);
    } catch {
      // Immediate visual celebration in local dev
      firstEventDetected = true;
      setTimeout(() => loadData(true), 1200);
    } finally {
      sendingTestEvent = false;
    }
  }

  function setupOnboardingDetector(pId: string) {
    if (onboardingEventSource) {
      onboardingEventSource.close();
      onboardingEventSource = null;
    }
    const apiUrl = import.meta.env.VITE_API_URL || "http://localhost:8080/api/v1";
    try {
      const es = new EventSource(`${apiUrl}/projects/${pId}/live`, { withCredentials: true });
      es.onmessage = (e) => {
        if (e.data && !e.data.startsWith(":")) {
          firstEventDetected = true;
          setTimeout(() => loadData(true), 1200);
        }
      };
      onboardingEventSource = es;
    } catch {
      // fallback
    }
  }

  const timeRanges = [
    { id: "1h", label: "1h" },
    { id: "24h", label: "24h" },
    { id: "7d", label: "7d" },
    { id: "30d", label: "30d" }
  ];

  function getDateRange(range: string): { from: string; to: string } {
    const to = new Date();
    const from = new Date();
    if (range === "1h") {
      from.setHours(to.getHours() - 1);
    } else if (range === "24h") {
      from.setHours(to.getHours() - 24);
    } else if (range === "7d") {
      from.setDate(to.getDate() - 7);
    } else if (range === "30d") {
      from.setDate(to.getDate() - 30);
    }
    return {
      from: from.toISOString(),
      to: to.toISOString(),
    };
  }

  async function loadData(isRefresh = false) {
    if (!projectId) return;

    if (currentAbortController) {
      currentAbortController.abort("superseded");
    }
    const controller = new AbortController();
    currentAbortController = controller;
    const signal = controller.signal;

    if (isRefresh) {
      refreshing = true;
    } else {
      loading = true;
    }
    error = "";
    const errs: typeof cardErrors = {};
    const fail = <T,>(key: keyof typeof cardErrors, fallback: T) => (e: any): T => {
      if (e?.name !== "AbortError") errs[key] = e?.message || "Failed to load.";
      return fallback;
    };
    const { from, to } = getDateRange(selectedRange);

    try {
      const [projData, keysData, sumData, topData, volData, quotaData] = await Promise.all([
        api.getProject(projectId, signal),
        api.listApiKeys(projectId, signal).catch(() => []),
        api.getSummary(projectId, from, to, signal, queryExtras).catch(fail("summary", { total_events: 0, unique_distinct_ids: 0 })),
        api.getTopEvents(projectId, from, to, 10, signal, queryExtras).catch(fail("top", [])),
        api.getVolume(projectId, from, to, selectedRange === "1h" ? "minute" : "hour", signal, queryExtras).catch(fail("volume", { interval: "hour", points: [] })),
        api.getProjectQuota(projectId, signal).catch(() => null),
      ]);

      if (signal.aborted) return;

      cardErrors = errs;
      project = projData;
      apiKeys = keysData || [];
      summary = sumData;
      quota = quotaData;
      topEvents = Array.isArray(topData)
        ? topData
        : topData && Array.isArray((topData as any).items)
          ? (topData as any).items
          : [];
      volume = volData;

      if (topEvents.length > 0 && breakdownEvent === "page_view") {
        breakdownEvent = topEvents[0].event_name;
      }

      if (summary?.total_events === 0 && !onboardingEventSource) {
        setupOnboardingDetector(projectId);
      }

      await Promise.all([
        loadFunnel(undefined, signal),
        loadBreakdown(breakdownEvent, breakdownProp, signal),
        loadDeviceAndGeo(signal),
        loadSessions(signal),
        loadRetention(signal),
        loadInsights(signal),
        loadDigest(signal),
      ]);
    } catch (err: any) {
      if (err?.name === "AbortError" || signal.aborted) return;
      error = err?.message || "Unable to load project activity right now.";
    } finally {
      if (!signal.aborted) {
        loading = false;
        refreshing = false;
      }
    }
  }

  async function loadFunnel(customSteps?: string[], signal?: AbortSignal) {
    let steps: string[];
    if (customSteps && Array.isArray(customSteps)) {
      steps = customSteps.filter(Boolean);
    } else {
      steps = [funnelStep1.trim(), funnelStep2.trim()];
      if (funnelStep3.trim()) steps.push(funnelStep3.trim());
      if (funnelStep4.trim()) steps.push(funnelStep4.trim());
      if (funnelStep5.trim()) steps.push(funnelStep5.trim());
    }
    if (steps.length >= 2) {
      funnelLoading = true;
      const { from, to } = getDateRange(selectedRange);
      try {
        funnelSteps = steps;
        funnel = await api
          .getFunnel(projectId, from, to, steps, signal, { filter: queryExtras.filter }, { window: funnelWindow, order: funnelOrder })
          .catch(() => null);
      } finally {
        if (!signal?.aborted) {
          funnelLoading = false;
        }
      }
    } else {
      funnel = null;
    }
  }

  function loadFunnelDropoff(step: number, offset: number, limit: number) {
    const { from, to } = getDateRange(selectedRange);
    return api
      .getFunnelDropoff(projectId, from, to, funnelSteps, step, offset, limit, undefined, { filter: queryExtras.filter }, { window: funnelWindow, order: funnelOrder })
      .catch(() => null);
  }

  async function loadBreakdown(evName: string, propName: string, signal?: AbortSignal) {
    if (!evName || !propName) return;
    breakdownLoading = true;
    const { from, to } = getDateRange(selectedRange);
    try {
      breakdown = await api
        .getPropertyBreakdown(projectId, evName, propName, from, to, 10, signal, { filter: queryExtras.filter })
        .catch(() => []);
    } finally {
      if (!signal?.aborted) {
        breakdownLoading = false;
      }
    }
  }

  async function loadDeviceAndGeo(signal?: AbortSignal) {
    deviceGeoLoading = true;
    const { from, to } = getDateRange(selectedRange);
    try {
      const [devRes, geoRes] = await Promise.all([
        api.getDeviceBreakdown(projectId, from, to, signal).catch(() => []),
        api.getGeoBreakdown(projectId, from, to, 10, signal).catch(() => []),
      ]);
      devices = devRes;
      geo = geoRes;
    } finally {
      if (!signal?.aborted) {
        deviceGeoLoading = false;
      }
    }
  }

  async function loadSessions(signal?: AbortSignal) {
    sessionsLoading = true;
    const { from, to } = getDateRange(selectedRange);
    try {
      sessions = await api.getSessions(projectId, from, to, 50, signal).catch(() => null);
    } finally {
      if (!signal?.aborted) {
        sessionsLoading = false;
      }
    }
  }

  async function loadRetention(signal?: AbortSignal) {
    retentionLoading = true;
    const { from, to } = getDateRange(selectedRange);
    try {
      retention = await api.getRetention(projectId, from, to, "signup", "app_opened", "day", signal, { filter: queryExtras.filter }).catch(() => null);
    } finally {
      if (!signal?.aborted) {
        retentionLoading = false;
      }
    }
  }

  async function loadInsights(signal?: AbortSignal) {
    insightsLoading = true;
    try {
      const res = await api.listInsights(projectId);
      if (!signal?.aborted) {
        insights = Array.isArray(res) ? res : [];
      }
    } catch (e) {
      console.error("Failed to load insights", e);
    } finally {
      if (!signal?.aborted) {
        insightsLoading = false;
      }
    }
  }

  async function handleCreateInsight(payload: CreateInsightPayload) {
    await api.createInsight(projectId, payload);
    await loadInsights();
  }

  async function handleDeleteInsight(id: string) {
    await api.deleteInsight(projectId, id);
    insights = insights.filter((i) => i.id !== id);
  }

  async function loadDigest(signal?: AbortSignal) {
    digestLoading = true;
    try {
      const res = await api.getWeeklyDigest(projectId, signal);
      if (!signal?.aborted) {
        weeklyDigest = res;
      }
    } catch (e) {
      console.error("Failed to load digest", e);
    } finally {
      if (!signal?.aborted) {
        digestLoading = false;
      }
    }
  }


  function handleRangeChange(rangeId: string) {
    selectedRange = rangeId;
    lastLoadedKey = `${projectId}:${rangeId}`;
    loadData();
  }

  async function copyProjectId() {
    const success = await copyToClipboard(projectId);
    if (success) {
      copied = true;
      setTimeout(() => {
        copied = false;
      }, 2000);
    }
  }

  async function handleExportCsv() {
    exporting = true;
    const { from, to } = getDateRange(selectedRange);
    try {
      const blob = await api.exportEventsCsv(projectId, from, to);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `kinetiq-${projectId}-${selectedRange}.csv`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err: any) {
      error = err?.message || "Unable to export CSV right now.";
    } finally {
      exporting = false;
    }
  }

  onMount(async () => {
    try {
      const session = await authClient.getSession();
      if (session?.data?.user?.name) {
        userName = session.data.user.name.split(" ")[0];
      } else if (session?.data?.user?.email) {
        userName = session.data.user.email.split("@")[0];
      }
    } catch {
      // ignore
    }
  });

  // Reload only when route projectId changes (untracked execution prevents reactive loops)
  $effect(() => {
    const id = projectId;
    if (id && lastLoadedKey !== `${id}:${untrack(() => selectedRange)}`) {
      lastLoadedKey = `${id}:${untrack(() => selectedRange)}`;
      untrack(() => {
        loadData();
      });
    }
  });

  onDestroy(() => {
    if (currentAbortController) {
      currentAbortController.abort("component_destroyed");
    }
    if (onboardingEventSource) {
      onboardingEventSource.close();
      onboardingEventSource = null;
    }
  });

  const funnelRate = $derived.by(() => {
    if (!funnel || !funnel.steps || funnel.steps.length === 0) return "0.0%";
    const lastStep = funnel.steps[funnel.steps.length - 1];
    return `${lastStep.conversion_rate.toFixed(1)}%`;
  });

  const activeKeyCount = $derived(apiKeys.filter((k) => !k.revoked_at).length);
  const primaryKeyPrefix = $derived(
    apiKeys.find((k) => !k.revoked_at)?.key_prefix || "tlk_live_••••"
  );

  const totalTopEventsCount = $derived(
    topEvents.reduce((acc, curr) => acc + curr.count, 0) || 1
  );
</script>

<div class="space-y-6">
  <!-- Greeting & Pill Filter Bar -->
  <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
    <div>
      <div class="flex items-center gap-2.5 flex-wrap">
        <h1 class="text-2xl sm:text-3xl font-extrabold tracking-tight text-[var(--text-primary)]">
          Welcome back, <span class="text-[var(--text-secondary)] font-semibold">{userName}</span>
        </h1>
        {#if project}
          <Badge variant={project.status === "active" ? "emerald" : "amber"} pulse={project.status === "active"}>
            {project.name} · {project.status === "active" ? "Receiving data" : "Paused"}
          </Badge>
        {/if}
      </div>
      <p class="text-xs text-[var(--text-muted)] mt-1 flex items-center gap-2">
        <span>Live activity summary and user insights for your project</span>
        <button
          onclick={copyProjectId}
          class="inline-flex items-center gap-1 font-mono text-[11px] px-2 py-0.5 rounded-full bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-[var(--text-secondary)] hover:text-[var(--brand-primary)] transition-colors cursor-pointer"
          title="Copy Project ID"
        >
          <span>ID: {projectId.slice(0, 8)}...</span>
          {#if copied}
            <Check class="w-3 h-3 text-[var(--brand-primary)]" />
          {:else}
            <Copy class="w-3 h-3" />
          {/if}
        </button>
      </p>
    </div>

    <!-- Right Pill Controls (Date Range Pill + Action Pills) -->
    <div class="flex flex-wrap items-center gap-2.5">
      <div class="inline-flex items-center gap-2 pl-3 pr-1 py-1 rounded-full bg-[var(--bg-surface)] border border-[var(--border-subtle)] shadow-[var(--shadow-pill)]">
        <Calendar class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
        <Tabs items={timeRanges} selected={selectedRange} onSelect={handleRangeChange} />
      </div>

      <a href="/app/projects/new">
        <Button variant="primary" size="md">
          <Plus class="w-4 h-4" />
          New Project
        </Button>
      </a>

      <Button
        variant="secondary"
        size="md"
        onclick={() => loadData(true)}
        loading={refreshing}
        title="Refresh data"
      >
        <RefreshCw class="w-3.5 h-3.5 text-[var(--text-secondary)] {refreshing ? 'animate-spin' : ''}" />
      </Button>
    </div>
  </div>

  {#if cardErrors.summary}
    <div role="alert" class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs font-medium text-rose-600 dark:text-rose-400 flex items-center justify-between">
      <span>Summary unavailable: {cardErrors.summary}</span>
      <Button variant="danger" size="sm" onclick={() => loadData(true)}>Retry</Button>
    </div>
  {/if}

  {#if error}
    <div class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs font-medium text-rose-600 dark:text-rose-400 flex items-center justify-between">
      <span>{error}</span>
      <Button variant="danger" size="sm" onclick={() => loadData(true)}>Try Again</Button>
    </div>
  {/if}

  <FilterPicker
    {projectId}
    onApply={(extras) => {
      queryExtras = extras;
      loadData(true);
    }}
  />

  {#if summary?.breakdown?.length}
    <div class="flex flex-wrap gap-2 text-[11px]" aria-label="Breakdown">
      {#each summary.breakdown as b}
        <span class="px-2.5 py-1 rounded-full bg-[var(--bg-surface)] border border-[var(--border-subtle)] font-mono">
          {b.value}: <strong>{b.count.toLocaleString()}</strong>
        </span>
      {/each}
    </div>
  {/if}

  <!-- Monthly Event Quota Warning / Exceeded Alerts -->
  {#if quota && quota.usage_pct >= 100}
    <div class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs font-medium text-rose-600 dark:text-rose-400 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 shadow-sm">
      <div class="flex items-center gap-2.5">
        <AlertTriangle class="w-4 h-4 shrink-0 text-rose-600 dark:text-rose-400" />
        <span>
          <strong>Kuota Ingestion Bulanan Habis (100%):</strong> Anda telah menggunakan {quota.used_this_month.toLocaleString()} / {quota.monthly_limit.toLocaleString()} event. Ingest event baru sedang ditolak dengan HTTP 429.
        </span>
      </div>
      <a href="/app/settings" class="shrink-0">
        <Button variant="danger" size="sm">Upgrade Kuota Sekarang</Button>
      </a>
    </div>
  {:else if quota && quota.usage_pct >= 80}
    <div class="rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-xs font-medium text-amber-700 dark:text-amber-400 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 shadow-sm">
      <div class="flex items-center gap-2.5">
        <AlertTriangle class="w-4 h-4 shrink-0 text-amber-600 dark:text-amber-400" />
        <span>
          <strong>Peringatan Kuota ({quota.usage_pct.toFixed(1)}% Digunakan):</strong> Anda telah mencapai {quota.used_this_month.toLocaleString()} dari {quota.monthly_limit.toLocaleString()} event bulanan paket {quota.plan.toUpperCase()}.
        </span>
      </div>
      <a href="/app/settings" class="shrink-0">
        <Button variant="secondary" size="sm">Tingkatkan Paket</Button>
      </a>
    </div>
  {/if}

  {#if !loading && (summary?.total_events === 0 || firstEventDetected)}
    <!-- Real-time Onboarding Connection Detector Card -->
    <div class="relative overflow-hidden rounded-[24px] border border-emerald-500/20 bg-gradient-to-br from-emerald-500/5 via-teal-500/5 to-transparent p-6 sm:p-7 shadow-[var(--shadow-bento)]">
      <div class="absolute -right-12 -top-12 w-48 h-48 rounded-full bg-emerald-500/10 blur-2xl pointer-events-none"></div>

      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-6 relative z-10">
        <div class="space-y-3 max-w-2xl">
          <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-semibold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
            <Radio class="w-3.5 h-3.5 animate-pulse" />
            <span>Real-time Onboarding Detector Active</span>
          </div>

          <h2 class="text-xl sm:text-2xl font-extrabold tracking-tight text-[var(--text-primary)]">
            {#if firstEventDetected}
              🎉 Pertama Kali Terhubung! Sinyal Berhasil Diterima
            {:else}
              Menunggu sinyal telemetry pertama Anda...
            {/if}
          </h2>

          <p class="text-xs sm:text-sm text-[var(--text-secondary)] leading-relaxed">
            {#if firstEventDetected}
              Event ingest pertama berhasil diverifikasi lewat real-time SSE engine. Dashboard Anda siap menyajikan visualisasi data mendalam.
            {:else}
              Kirim event pertama menggunakan <code class="font-mono text-emerald-600 dark:text-emerald-400 font-semibold">@kinetiq/sdk</code> atau cURL berikut. Halaman ini akan otomatis terupdate secara langsung begitu data masuk:
            {/if}
          </p>

          <!-- Quick Copy cURL Snippet -->
          <div class="relative rounded-xl bg-slate-900 border border-slate-800 p-3.5 font-mono text-[11px] sm:text-xs text-slate-300 overflow-x-auto">
            <pre><code>curl -X POST {import.meta.env.VITE_API_URL || "http://localhost:8080/api/v1"}/events \
  -H "Content-Type: application/json" \
  -H "X-API-Key: {activeKeyPrefix}..." \
  -d '{`{"event_name":"app_opened","properties":{"platform":"web"}}`}'</code></pre>
            <button
              onclick={async () => {
                const cmd = `curl -X POST ${import.meta.env.VITE_API_URL || "http://localhost:8080/api/v1"}/events -H "Content-Type: application/json" -H "X-API-Key: ${activeKeyPrefix}" -d '{"event_name":"app_opened","properties":{"platform":"web"}}'`;
                await copyToClipboard(cmd);
                copiedSnippet = true;
                setTimeout(() => { copiedSnippet = false; }, 2000);
              }}
              class="absolute right-3 top-3 px-2.5 py-1 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-[11px] font-sans flex items-center gap-1.5 transition-colors cursor-pointer"
            >
              {#if copiedSnippet}
                <Check class="w-3.5 h-3.5 text-emerald-400" />
                <span class="text-emerald-400">Copied</span>
              {:else}
                <Copy class="w-3.5 h-3.5" />
                <span>Copy</span>
              {/if}
            </button>
          </div>
        </div>

        <div class="flex flex-col sm:flex-row lg:flex-col gap-3 shrink-0">
          <Button
            variant="primary"
            size="md"
            onclick={handleSendTestEvent}
            loading={sendingTestEvent}
          >
            <Sparkles class="w-4 h-4 mr-1.5" />
            Kirim Simulasi Event Pertama
          </Button>

          <a href="/app/projects/{projectId}/settings">
            <Button variant="secondary" size="md" class="w-full">
              <KeyRound class="w-4 h-4 mr-1.5" />
              Kelola API Keys & Alerts
            </Button>
          </a>
        </div>
      </div>
    </div>
  {/if}

  <!-- 12-Column Soft-Emerald Bento Grid -->
  <div class="grid grid-cols-1 lg:grid-cols-12 gap-5 items-stretch">
    <!-- LEFT COLUMN (3 cols): Forest Emerald Hero Project Card + Active Users Card -->
    <div class="lg:col-span-3 flex flex-col gap-5">
      <!-- 1. Forest Emerald Hero Project Card -->
      <div class="rounded-[24px] bg-gradient-to-br from-[#127852] via-[#106B49] to-[#0C5439] text-white p-6 shadow-[var(--shadow-bento)] flex flex-col justify-between relative overflow-hidden min-h-[260px]">
        <div class="w-36 h-36 rounded-full bg-white/5 absolute -right-10 -top-10 pointer-events-none"></div>
        <div class="w-24 h-24 rounded-full bg-emerald-300/10 absolute -left-8 -bottom-8 pointer-events-none"></div>

        <div class="flex items-center justify-between relative z-10">
          <div>
            <span class="text-xs font-semibold text-emerald-100/90 block">Project Overview</span>
            <span class="text-[11px] text-emerald-100/70">Live Activity Tracker</span>
          </div>
          <a
            href="/app/projects/{projectId}/settings"
            class="w-8 h-8 rounded-full bg-white/15 hover:bg-white/25 flex items-center justify-center text-white transition-colors"
            title="Project Settings"
          >
            <ArrowUpRight class="w-4 h-4" />
          </a>
        </div>

        <!-- Middle Card Inner Surface -->
        <div class="my-5 p-4 rounded-2xl bg-white/10 backdrop-blur-xs border border-white/15 space-y-3 relative z-10">
          <div class="flex items-center justify-between">
            <span class="text-xs font-bold tracking-wide uppercase text-emerald-100 truncate max-w-[140px]">
              {project ? project.name : "Project"}
            </span>
            <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-mono bg-emerald-400/20 text-emerald-200">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-300 animate-ping"></span>
              LIVE
            </span>
          </div>

          <div>
            <div class="text-[11px] text-emerald-100/75">Total Recorded Activity ({selectedRange})</div>
            <div class="text-3xl font-extrabold tracking-tight mt-0.5 font-sans">
              {summary ? summary.total_events.toLocaleString() : "..."}
            </div>
          </div>

          <div class="pt-2 border-t border-white/10 flex items-center justify-between text-[11px] text-emerald-100/80">
            <span class="font-mono">{primaryKeyPrefix}••••</span>
            <span>Saved {project?.retention_days || 90} days</span>
          </div>
        </div>

        <!-- Bottom Action Pills -->
        <div class="flex items-center gap-2 relative z-10">
          <a
            href="/app/projects/{projectId}/live"
            class="flex-1 py-2 px-3 rounded-full bg-white text-[#127852] text-xs font-bold text-center hover:bg-emerald-50 transition-colors flex items-center justify-center gap-1"
          >
            <span>Live Feed</span>
            <ArrowUpRight class="w-3.5 h-3.5" />
          </a>
          <a
            href="/app/projects/{projectId}/settings"
            class="flex-1 py-2 px-3 rounded-full bg-white/15 text-white text-xs font-semibold text-center hover:bg-white/25 transition-colors"
          >
            API Keys ({activeKeyCount})
          </a>
        </div>
      </div>

      <!-- 2. Active Visitors Card -->
      <Card class="flex flex-col justify-between space-y-3">
        <div class="flex items-center justify-between">
          <div>
            <span class="text-xs font-semibold text-[var(--text-secondary)] block">Active People</span>
            <span class="text-[11px] text-[var(--text-muted)]">Unique visitors in the last {selectedRange}</span>
          </div>
          <div class="w-9 h-9 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
            <Users class="w-4 h-4" />
          </div>
        </div>

        <div class="flex items-baseline justify-between pt-1">
          <span class="text-3xl font-extrabold tracking-tight text-[var(--text-primary)]">
            {summary ? summary.unique_distinct_ids.toLocaleString() : "..."}
          </span>
          <Badge variant="emerald">Active</Badge>
        </div>

        <div class="pt-2 border-t border-[var(--border-subtle)] flex items-center justify-between text-[11px] text-[var(--text-muted)]">
          <span>Most Popular Action</span>
          <span class="font-semibold text-[var(--text-primary)]">
            {topEvents.length > 0 ? topEvents[0].event_name : "No activity yet"}
          </span>
        </div>
      </Card>
    </div>

    <!-- CENTER COLUMN (6 cols): Activity Chart + Most Frequent Actions Table -->
    <div class="lg:col-span-6 flex flex-col gap-5">
      <!-- 1. Activity Over Time Card -->
      <Card class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-full bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] flex items-center justify-center text-[var(--brand-primary)]">
              <BarChart3 class="w-4 h-4" />
            </div>
            <div>
              <h2 class="text-base font-bold tracking-tight text-[var(--text-primary)]">
                Activity Over Time
              </h2>
              <p class="text-xs text-[var(--text-muted)]">
                Grouped by {volume ? volume.interval : "hour"} · Busiest period highlighted in green
              </p>
            </div>
          </div>

          <div class="flex items-center gap-2">
            <Badge variant="emerald">
              {volume ? volume.points.reduce((a, b) => a + b.count, 0).toLocaleString() : 0} total
            </Badge>
            <a
              href="/app/projects/{projectId}/live"
              class="w-8 h-8 rounded-full bg-[var(--bg-surface-muted)] hover:bg-[var(--brand-soft)] text-[var(--text-secondary)] hover:text-[var(--brand-primary)] flex items-center justify-center transition-colors"
              title="Open Live Feed"
            >
              <ArrowUpRight class="w-4 h-4" />
            </a>
          </div>
        </div>

        {#if cardErrors.volume}
          <div role="alert" class="py-12 text-center text-xs text-rose-600 dark:text-rose-400 space-y-3">
            <div>{cardErrors.volume}</div>
            <Button variant="danger" size="sm" onclick={() => loadData(true)}>Retry</Button>
          </div>
        {:else}
          <VolumeChart points={volume ? volume.points : []} height={215} mode="bars" />
        {/if}
      </Card>

      <!-- 2. Most Frequent User Actions Table -->
      <Card class="space-y-4 flex-1">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-base font-bold tracking-tight text-[var(--text-primary)]">
              Most Frequent Actions
            </h2>
            <p class="text-xs text-[var(--text-muted)]">
              Top user activities recorded in the last {selectedRange}
            </p>
          </div>
          <Badge variant="neutral">{topEvents.length} actions</Badge>
        </div>

        {#if cardErrors.top}
          <div role="alert" class="py-12 text-center text-xs text-rose-600 dark:text-rose-400 space-y-3">
            <div>{cardErrors.top}</div>
            <Button variant="danger" size="sm" onclick={() => loadData(true)}>Retry</Button>
          </div>
        {:else if topEvents.length === 0}
          <div class="py-12 text-center text-xs text-[var(--text-muted)]">
            No user activity recorded in this time period yet. Send a test event from Project Settings to see your data here.
          </div>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead class="border-b border-[var(--border-subtle)] text-[11px] font-semibold text-[var(--text-muted)]">
                <tr>
                  <th class="pb-3 pr-4">Action Name</th>
                  <th class="pb-3 px-3">Time Period</th>
                  <th class="pb-3 px-3">Status</th>
                  <th class="pb-3 px-3 text-right">Count</th>
                  <th class="pb-3 pl-3 text-right">Share</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[var(--border-subtle)]">
                {#each topEvents.slice(0, 6) as evt, i}
                  {@const pct = (evt.count / totalTopEventsCount) * 100}
                  <tr class="group hover:bg-[var(--bg-surface-muted)]/60 transition-colors">
                    <td class="py-3 pr-4">
                      <div class="flex items-center gap-2.5">
                        <div class="w-8 h-8 rounded-full {i === 0 ? 'bg-[var(--brand-primary)] text-white' : 'bg-[var(--brand-soft)] text-[var(--brand-primary)]'} flex items-center justify-center font-mono text-[11px] font-bold shrink-0">
                          {evt.event_name.slice(0, 2).toUpperCase()}
                        </div>
                        <div>
                          <div class="font-bold text-[var(--text-primary)]">{evt.event_name}</div>
                          <div class="text-[10px] text-[var(--text-muted)]">User action</div>
                        </div>
                      </div>
                    </td>
                    <td class="py-3 px-3 text-[11px] text-[var(--text-secondary)]">
                      Last {selectedRange}
                    </td>
                    <td class="py-3 px-3">
                      <span class="inline-flex items-center gap-1.5 text-[11px] font-medium text-[var(--brand-primary)]">
                        <span class="w-1.5 h-1.5 rounded-full bg-[var(--brand-primary)]"></span>
                        Tracking
                      </span>
                    </td>
                    <td class="py-3 px-3 text-right font-bold text-[var(--text-primary)] font-mono">
                      {evt.count.toLocaleString()}
                    </td>
                    <td class="py-3 pl-3 text-right">
                      <span class="inline-block px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-[var(--bg-surface-muted)] text-[var(--text-secondary)]">
                        {pct.toFixed(1)}%
                      </span>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </Card>
    </div>

    <!-- RIGHT COLUMN (3 cols): Conversion Rate Card + Project Access & Privacy Card -->
    <div class="lg:col-span-3 flex flex-col gap-5">
      <!-- 1. Conversion Rate & Smooth Emerald Wave Card -->
      <Card class="space-y-4">
        <div class="flex items-start justify-between">
          <div>
            <span class="text-xs font-semibold text-[var(--text-secondary)] block">Conversion Rate</span>
            <span class="text-[11px] text-[var(--text-muted)]">
              {funnelStep1} → {funnelStep2}
            </span>
          </div>
          <div class="w-9 h-9 rounded-full bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] flex items-center justify-center text-[var(--brand-primary)]">
            <TrendingUp class="w-4 h-4" />
          </div>
        </div>

        <div>
          <div class="text-3xl font-extrabold tracking-tight text-[var(--text-primary)]">
            {funnelRate}
          </div>
          <div class="flex items-center gap-2 mt-1">
            <Badge variant="emerald">Step-to-step</Badge>
            <span class="text-[11px] text-[var(--text-muted)]">in the last {selectedRange}</span>
          </div>
        </div>

        <VolumeChart points={volume ? volume.points : []} height={95} mode="wave" />

        <div class="grid grid-cols-2 gap-2.5 pt-1">
          <a
            href="/app/projects/{projectId}/live"
            class="py-2.5 px-3 rounded-full bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold flex items-center justify-center gap-1.5 transition-colors"
          >
            <span>Live Feed</span>
            <ArrowUpRight class="w-3.5 h-3.5" />
          </a>
          <button
            type="button"
            onclick={handleExportCsv}
            disabled={exporting}
            class="py-2.5 px-3 rounded-full bg-[var(--bg-surface-muted)] hover:bg-[var(--bg-surface-elevated)] text-[var(--text-primary)] border border-[var(--border-subtle)] text-xs font-semibold flex items-center justify-center gap-1.5 transition-colors cursor-pointer disabled:opacity-50"
          >
            <span>{exporting ? "..." : "Download CSV"}</span>
            <ArrowDownLeft class="w-3.5 h-3.5" />
          </button>
        </div>
      </Card>

      <!-- 2. Project Access & Privacy Card -->
      <Card class="space-y-4 flex-1 flex flex-col justify-between">
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-sm font-bold text-[var(--text-primary)]">Project Access & Privacy</h3>
              <p class="text-[11px] text-[var(--text-muted)]">API keys & security settings</p>
            </div>
            <a
              href="/app/projects/{projectId}/settings"
              class="w-8 h-8 rounded-full bg-[var(--bg-surface-muted)] hover:bg-[var(--brand-soft)] text-[var(--text-secondary)] hover:text-[var(--brand-primary)] flex items-center justify-center transition-colors"
              title="Open Project Settings"
            >
              <Settings class="w-4 h-4" />
            </a>
          </div>

          <div class="space-y-2.5 pt-1">
            <div class="p-3 rounded-2xl bg-[var(--bg-surface-muted)] flex items-center justify-between">
              <div class="flex items-center gap-2.5">
                <div class="w-8 h-8 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
                  <KeyRound class="w-3.5 h-3.5" />
                </div>
                <div>
                  <div class="text-xs font-bold text-[var(--text-primary)]">API Keys</div>
                  <div class="text-[10px] font-mono text-[var(--text-muted)]">{primaryKeyPrefix}••••</div>
                </div>
              </div>
              <Badge variant="emerald">{activeKeyCount} active</Badge>
            </div>

            <div class="p-3 rounded-2xl bg-[var(--bg-surface-muted)] flex items-center justify-between">
              <div class="flex items-center gap-2.5">
                <div class="w-8 h-8 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
                  <Zap class="w-3.5 h-3.5" />
                </div>
                <div>
                  <div class="text-xs font-bold text-[var(--text-primary)]">Traffic Limit</div>
                  <div class="text-[10px] text-[var(--text-muted)]">Max events per second</div>
                </div>
              </div>
              <span class="text-xs font-mono font-bold text-[var(--text-primary)]">
                {project?.rate_limit_per_sec || 1000}/sec
              </span>
            </div>

            <div class="p-3 rounded-2xl bg-[var(--bg-surface-muted)] flex items-center justify-between">
              <div class="flex items-center gap-2.5">
                <div class="w-8 h-8 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
                  <ShieldCheck class="w-3.5 h-3.5" />
                </div>
                <div>
                  <div class="text-xs font-bold text-[var(--text-primary)]">Visitor Privacy</div>
                  <div class="text-[10px] text-[var(--text-muted)]">Hide visitor IP addresses</div>
                </div>
              </div>
              <span class="text-xs font-semibold text-[var(--brand-primary)]">
                {project?.anonymize_ip ? "Protected" : "Standard"}
              </span>
            </div>
          </div>
        </div>

        <a
          href="/app/projects/{projectId}/settings"
          class="w-full py-2.5 px-4 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] hover:opacity-90 text-xs font-bold text-center transition-opacity block"
        >
          Manage Keys & Settings →
        </a>
      </Card>
    </div>
  </div>

  <!-- Custom Insights & Saved Charts (Widget Builder) -->
  <div class="pt-2">
    <svelte:boundary>
      <InsightWidgetGrid
        {insights}
        loading={insightsLoading}
        onAddInsight={() => { isCreateInsightOpen = true; }}
        onDeleteInsight={handleDeleteInsight}
      />
      {#snippet failed(err, reset)}
        <div role="alert" class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs text-rose-600 dark:text-rose-400 flex items-center justify-between"><span>This widget failed to render.</span><button class="underline font-semibold" onclick={reset}>Retry</button></div>
      {/snippet}
    </svelte:boundary>
  </div>

  <!-- Web Analytics: Sessions, Entry Pages, Referrers -->
  <svelte:boundary>
    <SessionsCard {sessions} loading={sessionsLoading} />
    {#snippet failed(err, reset)}
      <div role="alert" class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs text-rose-600 dark:text-rose-400 flex items-center justify-between"><span>This widget failed to render.</span><button class="underline font-semibold" onclick={reset}>Retry</button></div>
    {/snippet}
  </svelte:boundary>

  <!-- Ingestion Intelligence: Client Fingerprinting & Device / Geo Breakdowns -->
  <svelte:boundary>
    <DeviceAndGeoBreakdown
      {devices}
      {geo}
      loading={deviceGeoLoading}
    />
    {#snippet failed(err, reset)}
      <div role="alert" class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs text-rose-600 dark:text-rose-400 flex items-center justify-between"><span>This widget failed to render.</span><button class="underline font-semibold" onclick={reset}>Retry</button></div>
    {/snippet}
  </svelte:boundary>

  <!-- Deep Exploration Row: Step-by-Step Conversion & Breakdown by Detail -->
  <div class="grid grid-cols-1 lg:grid-cols-2 gap-5 pt-2">
    <FunnelVisualizer
      {funnel}
      loading={funnelLoading}
      bind:step1={funnelStep1}
      bind:step2={funnelStep2}
      bind:step3={funnelStep3}
      bind:step4={funnelStep4}
      bind:step5={funnelStep5}
      bind:windowSize={funnelWindow}
      bind:order={funnelOrder}
      loadDropoff={loadFunnelDropoff}
      availableEvents={topEvents.map((t) => t.event_name)}
      onCalculate={(steps) => loadFunnel(steps)}
    />

    <PropertyBreakdown
      bind:eventName={breakdownEvent}
      bind:propertyName={breakdownProp}
      {breakdown}
      loading={breakdownLoading}
      onFetchBreakdown={(ev, prop) => loadBreakdown(ev, prop)}
    />
  </div>

  <!-- Enterprise Analytics: User Retention & Cohort Heatmap Matrix -->
  <div class="pt-2">
    <svelte:boundary>
      <RetentionHeatmap
        data={retention}
        loading={retentionLoading}
        onRefresh={() => loadRetention()}
      />
      {#snippet failed(err, reset)}
        <div role="alert" class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs text-rose-600 dark:text-rose-400 flex items-center justify-between"><span>This widget failed to render.</span><button class="underline font-semibold" onclick={reset}>Retry</button></div>
      {/snippet}
    </svelte:boundary>
  </div>

  <!-- Automated Intelligence: Weekly Email Digest Performance Summary -->
  <div class="pt-2">
    <svelte:boundary>
      <WeeklyDigestCard
        digest={weeklyDigest}
        loading={digestLoading}
      />
      {#snippet failed(err, reset)}
        <div role="alert" class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs text-rose-600 dark:text-rose-400 flex items-center justify-between"><span>This widget failed to render.</span><button class="underline font-semibold" onclick={reset}>Retry</button></div>
      {/snippet}
    </svelte:boundary>
  </div>

  <CreateInsightModal
    open={isCreateInsightOpen}
    availableEvents={topEvents.map((t) => t.event_name)}
    onClose={() => { isCreateInsightOpen = false; }}
    onSubmit={handleCreateInsight}
  />
</div>
