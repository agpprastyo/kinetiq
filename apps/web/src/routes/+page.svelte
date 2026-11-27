<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { authClient } from "$lib/auth-client";
  import { toggleTheme, subscribeTheme, type Theme } from "$lib/theme";
  import {
    Activity,
    ArrowRight,
    Check,
    Copy,
    Eye,
    EyeOff,
    KeyRound,
    Moon,
    Pause,
    Play,
    Radio,
    Server,
    ShieldCheck,
    Sun,
    Terminal
  } from "lucide-svelte";

  type Lang = "id" | "en";
  type Currency = "IDR" | "USD";
  type FunnelPreset = "ecommerce" | "saas" | "mobile";

  let lang = $state<Lang>("id");
  let currency = $state<Currency>("IDR");
  let currentTheme = $state<Theme>("light");
  let unsubTheme: (() => void) | null = null;
  let scrollY = $state(0);
  const isScrolled = $derived(scrollY > 16);

  let isAuthenticated = $state(false);

  // Interactive Telemetry Instrument state
  interface DemoEvent {
    id: string;
    name: string;
    user: string;
    region: string;
    detail: string;
    time: string;
  }

  let autoStream = $state(true);
  let streamTimer: ReturnType<typeof setInterval> | null = null;
  let demoTotalEvents = $state(14820);
  let demoActiveVisitors = $state(64);
  let demoConversionRate = $state(18.4);
  let demoBars = $state<number[]>([
    42, 58, 49, 64, 52, 71, 63, 78, 69, 84, 74, 88, 79, 92, 85, 95
  ]);
  let activeTrigger = $state("checkout_completed");

  let demoFeed = $state<DemoEvent[]>([
    {
      id: "evt_04",
      name: "checkout_completed",
      user: "rina@tokokopi.id",
      region: "ID-JK",
      detail: "Kopi Arabika Gayo · Rp 249.000",
      time: "00:01s"
    },
    {
      id: "evt_03",
      name: "add_to_cart",
      user: "budi.santoso",
      region: "ID-JI",
      detail: "Mesin Espresso Mini · 1 unit",
      time: "00:04s"
    },
    {
      id: "evt_02",
      name: "page_view",
      user: "visitor_8921",
      region: "SG-01",
      detail: "/katalog/promo-spesial",
      time: "00:11s"
    },
    {
      id: "evt_01",
      name: "signup_completed",
      user: "nadia@studio.id",
      region: "ID-JB",
      detail: "Google OAuth Workspace",
      time: "00:19s"
    }
  ]);

  let activeFunnel = $state<FunnelPreset>("ecommerce");
  let maskPrivacy = $state(true);
  let codeTab = $state<"sdk" | "docker" | "curl">("sdk");
  let copiedCode = $state(false);
  let copiedDockerHero = $state(false);

  const dockerOneLiner = "docker compose up -d";

  const sampleActions = [
    {
      name: "page_view",
      detailId: "/produk/sepatu-lari-aero",
      detailEn: "/products/aero-running-shoes",
      region: "ID-JK",
      user: "andi@karya.id"
    },
    {
      name: "add_to_cart",
      detailId: "Sepatu Lari Aero · Uk 42",
      detailEn: "Aero Running Shoes · Size 42",
      region: "ID-JB",
      user: "siti.rahma"
    },
    {
      name: "checkout_completed",
      detailId: "Order #INV-908 · Rp 450.000",
      detailEn: "Order #INV-908 · $29.00",
      region: "ID-JI",
      user: "dimas@tokokita.id"
    },
    {
      name: "signup_completed",
      detailId: "Pendaftaran Akun Baru",
      detailEn: "New Workspace Account",
      region: "SG-01",
      user: "reza@stasion.co"
    }
  ];

  function triggerDemoAction(
    actionName: string,
    detailText: string,
    customRegion?: string,
    customUser?: string
  ) {
    activeTrigger = actionName;
    demoTotalEvents += 1;
    if (Math.random() > 0.4) {
      demoActiveVisitors = Math.min(
        98,
        Math.max(48, demoActiveVisitors + (Math.random() > 0.45 ? 1 : -1))
      );
    }
    if (actionName === "checkout_completed") {
      demoConversionRate = Number((demoConversionRate + 0.1).toFixed(1));
    }

    const lastBar = demoBars[demoBars.length - 1] ?? 80;
    const nextVal = Math.min(100, Math.max(40, lastBar + Math.floor(Math.random() * 20) - 8));
    demoBars = [...demoBars.slice(1), nextVal];

    const regions = ["ID-JK", "ID-JI", "ID-JB", "ID-YO", "SG-01"];
    const users = [
      "andi@startup.id",
      "siti.rahma",
      "dimas@karya.co",
      "visitor_4019",
      "reza@tokokita.id"
    ];

    demoFeed = [
      {
        id: `evt_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
        name: actionName,
        user: customUser || users[Math.floor(Math.random() * users.length)],
        region: customRegion || regions[Math.floor(Math.random() * regions.length)],
        detail: detailText,
        time: "00:00s"
      },
      ...demoFeed.slice(0, 3)
    ];
  }

  function copyHeroDocker() {
    navigator.clipboard.writeText(dockerOneLiner);
    copiedDockerHero = true;
    setTimeout(() => (copiedDockerHero = false), 1800);
  }

  const exampleAuthHeader = ["Authorization", "Bearer <your_project_key>"].join(": ");

  const rawSnippets = {
    sdk: `import { KinetiqClient } from '@kinetiq/sdk';

const kinetiq = new KinetiqClient({
  apiKey: process.env.KINETIQ_API_KEY,
  endpoint: 'https://ingest.kinetiq.io'
});

await kinetiq.track('checkout_completed', {
  plan: 'growth_monthly',
  currency: 'IDR',
  value: 249000
});`,
    docker: `git clone https://github.com/agpprastyo/kinetiq.git kinetiq
cd kinetiq

docker compose up -d --build`,
    curl: `curl -X POST http://localhost:8080/api/v1/ingest \\
  -H "Content-Type: application/json" \\
  -H "${exampleAuthHeader}" \\
  -d '{
    "events": [{
      "event_name": "checkout_completed",
      "distinct_id": "usr_01J9X2K8M",
      "properties": { "plan": "growth_monthly" }
    }]
  }'`
  };

  function copyActiveSnippet() {
    navigator.clipboard.writeText(rawSnippets[codeTab]);
    copiedCode = true;
    setTimeout(() => (copiedCode = false), 1800);
  }

  const funnelSteps = $derived.by(() => {
    if (activeFunnel === "ecommerce") {
      return lang === "id"
        ? [
            { step: "01. Lihat Halaman Produk", users: 12400, pct: 100, note: "100% pengunjung masuk" },
            { step: "02. Tambah ke Keranjang", users: 4960, pct: 40, note: "-60% berhenti di estimasi ongkir" },
            { step: "03. Pembayaran Selesai", users: 2280, pct: 18.4, note: "18.4% konversi transaksi" }
          ]
        : [
            { step: "01. View Product Page", users: 12400, pct: 100, note: "100% entry traffic" },
            { step: "02. Add to Shopping Cart", users: 4960, pct: 40, note: "-60% drop at shipping estimate" },
            { step: "03. Payment Completed", users: 2280, pct: 18.4, note: "18.4% final conversion" }
          ];
    }
    if (activeFunnel === "saas") {
      return lang === "id"
        ? [
            { step: "01. Buka Halaman Harga", users: 6800, pct: 100, note: "100% pengunjung masuk" },
            { step: "02. Buat Akun Gratis", users: 2856, pct: 42, note: "42% mendaftar akun" },
            { step: "03. Buat Proyek Pertama", users: 2140, pct: 31.5, note: "75% pengguna baru langsung aktif" }
          ]
        : [
            { step: "01. Visit Pricing Page", users: 6800, pct: 100, note: "100% entry traffic" },
            { step: "02. Create Free Account", users: 2856, pct: 42, note: "42% signup rate" },
            { step: "03. Create First Project", users: 2140, pct: 31.5, note: "75% activation rate" }
          ];
    }
    return lang === "id"
      ? [
          { step: "01. Buka Aplikasi Mobile", users: 24500, pct: 100, note: "100% unduhan aktif" },
          { step: "02. Selesaikan Onboarding", users: 18130, pct: 74, note: "-26% melewati pengenalan" },
          { step: "03. Aktifkan Langganan", users: 3920, pct: 16.0, note: "3.920 pelanggan berbayar" }
        ]
      : [
          { step: "01. Launch Mobile App", users: 24500, pct: 100, note: "100% active installs" },
          { step: "02. Complete Onboarding", users: 18130, pct: 74, note: "-26% skipped intro" },
          { step: "03. Start Subscription", users: 3920, pct: 16.0, note: "3,920 paid subscribers" }
        ];
  });

  onMount(async () => {
    unsubTheme = subscribeTheme((t) => {
      currentTheme = t;
    });

    streamTimer = setInterval(() => {
      if (!autoStream) return;
      const pick = sampleActions[Math.floor(Math.random() * sampleActions.length)];
      triggerDemoAction(
        pick.name,
        lang === "id" ? pick.detailId : pick.detailEn,
        pick.region,
        pick.user
      );
    }, 2800);

    try {
      const session = await authClient.getSession();
      if (session?.data?.user) {
        isAuthenticated = true;
      }
    } catch {
      // Guest visitor
    }
  });

  onDestroy(() => {
    if (unsubTheme) unsubTheme();
    if (streamTimer) clearInterval(streamTimer);
  });
</script>

<svelte:window bind:scrollY />

<svelte:head>
  <title>Kinetiq — {lang === "id" ? "Analitik Aktivitas Produk Real-Time" : "Real-Time Product Activity Analytics"}</title>
</svelte:head>

<div class="min-h-screen bg-[var(--bg-canvas)] text-[var(--text-primary)] flex flex-col justify-between selection:bg-emerald-500/20">
  <!-- FIXED TOP NAVBAR: Full-width flush bar at top -> Smoothly morphs into Floating Glassmorphism Pill on scroll -->
  <div
    class="fixed top-0 inset-x-0 z-50 pointer-events-none transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] {isScrolled
      ? 'px-4 sm:px-8 pt-3.5 pb-2'
      : 'px-0 pt-0 pb-0'}"
  >
    <header
      class="pointer-events-auto mx-auto transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] {isScrolled
        ? 'max-w-[1120px] rounded-full px-5 py-2.5 bg-[var(--bg-surface)]/78 backdrop-blur-xl backdrop-saturate-150 border border-[var(--border-hover)] shadow-[var(--shadow-dropdown)]'
        : 'max-w-full rounded-none px-6 sm:px-10 py-3.5 bg-[var(--bg-surface)]/95 backdrop-blur-md border-b border-[var(--border-subtle)] shadow-none'}"
    >
      <div class="mx-auto w-full max-w-[1220px] flex items-center justify-between gap-4">
        <!-- Brand Identity -->
        <a href="/" class="flex items-center gap-2.5 group shrink-0">
          <div class="w-8 h-8 rounded-lg bg-[var(--brand-primary)] text-white flex items-center justify-center transition-transform group-hover:scale-[1.02]">
            <Activity class="w-4 h-4" />
          </div>
          <div class="flex items-baseline gap-2">
            <span class="text-base font-bold tracking-tight text-[var(--text-primary)]">Kinetiq</span>
            <span class="hidden sm:inline-block text-[11px] font-mono text-[var(--text-muted)] border border-[var(--border-subtle)] bg-[var(--bg-surface-muted)] px-1.5 py-0.5 rounded-md">
              analytics
            </span>
          </div>
        </a>

        <!-- Center Navigation Links -->
        <nav class="hidden md:flex items-center gap-1 text-xs font-medium">
          <a
            href="#overview"
            class="px-3 py-1.5 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)] transition-colors whitespace-nowrap"
          >
            {lang === "id" ? "Simulasi" : "Live Demo"}
          </a>
          <a
            href="#inspection"
            class="px-3 py-1.5 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)] transition-colors whitespace-nowrap"
          >
            {lang === "id" ? "Konversi & Privasi" : "Funnel & Privacy"}
          </a>
          <a
            href="#integration"
            class="px-3 py-1.5 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)] transition-colors whitespace-nowrap"
          >
            {lang === "id" ? "Integrasi" : "Integration"}
          </a>
        </nav>

        <!-- Right Controls: Language, Theme, Auth CTA -->
        <div class="flex items-center gap-2 shrink-0">
          <div class="inline-flex items-center p-0.5 rounded-lg bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-[11px] font-mono font-semibold">
            <button
              type="button"
              onclick={() => (lang = "id")}
              class="px-2 py-1 rounded-md transition-colors cursor-pointer {lang === 'id'
                ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] shadow-2xs border border-[var(--border-subtle)]'
                : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
            >
              ID
            </button>
            <button
              type="button"
              onclick={() => (lang = "en")}
              class="px-2 py-1 rounded-md transition-colors cursor-pointer {lang === 'en'
                ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] shadow-2xs border border-[var(--border-subtle)]'
                : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
            >
              EN
            </button>
          </div>

          <button
            type="button"
            onclick={() => toggleTheme()}
            title={currentTheme === "dark" ? "Light mode" : "Dark mode"}
            class="w-8 h-8 rounded-lg flex items-center justify-center bg-[var(--bg-surface)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] border border-[var(--border-subtle)] transition-colors cursor-pointer"
          >
            {#if currentTheme === "dark"}
              <Sun class="w-3.5 h-3.5 text-amber-400" />
            {:else}
              <Moon class="w-3.5 h-3.5" />
            {/if}
          </button>

          {#if isAuthenticated}
            <a
              href="/app"
              class="h-8 px-3.5 rounded-lg bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold inline-flex items-center gap-1.5 transition-colors whitespace-nowrap"
            >
              <span>{lang === "id" ? "Buka Dashboard" : "Dashboard"}</span>
              <ArrowRight class="w-3.5 h-3.5" />
            </a>
          {:else}
            <a
              href="/login"
              class="hidden sm:inline-flex h-8 px-3 rounded-lg text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)] items-center transition-colors whitespace-nowrap"
            >
              {lang === "id" ? "Masuk" : "Sign in"}
            </a>
            <a
              href="/signup"
              class="h-8 px-3.5 rounded-lg bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold inline-flex items-center gap-1.5 transition-colors whitespace-nowrap"
            >
              <span>{lang === "id" ? "Mulai Gratis" : "Start free"}</span>
              <ArrowRight class="w-3.5 h-3.5" />
            </a>
          {/if}
        </div>
      </div>
    </header>
  </div>

  <!-- Top Spacer so content is never hidden behind the Fixed Navbar -->
  <div class="h-[61px] shrink-0"></div>

  <!-- ARCHITECTURAL SWISS-GRID FRAME (Exposed 1px Structural Hairlines) -->
  <main class="flex-1 w-full max-w-[1220px] mx-auto border-x border-[var(--border-subtle)] bg-[var(--bg-surface)]">
    <!-- SECTION 01: SPLIT ARCHITECTURAL HERO & LIVE TELEMETRY INSTRUMENT -->
    <section
      id="overview"
      class="grid grid-cols-1 lg:grid-cols-12 divide-y lg:divide-y-0 lg:divide-x divide-[var(--border-subtle)] border-b border-[var(--border-subtle)] relative"
    >
      <!-- Left 6 Cols: Editorial Copy & Instant Self-Host Command -->
      <div class="lg:col-span-6 p-7 sm:p-12 flex flex-col justify-between space-y-8">
        <div class="space-y-6">
          <div class="flex items-center justify-between text-[11px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
            <span>01 / {lang === "id" ? "ANALITIK PRODUK REAL-TIME" : "REAL-TIME PRODUCT ANALYTICS"}</span>
            <span class="inline-flex items-center gap-1.5 text-[var(--brand-primary)] font-semibold">
              <span class="w-1.5 h-1.5 rounded-full bg-[var(--brand-primary)]"></span>
              v1.0 Open-Core
            </span>
          </div>

          <h1 class="text-3xl sm:text-[42px] font-bold tracking-[-0.03em] text-[var(--text-primary)] leading-[1.12]">
            {lang === "id"
              ? "Pahami aktivitas pengunjung aplikasi Anda detik ini juga, tanpa kerumitan."
              : "Understand what users do inside your product right now, without the bloat."}
          </h1>

          <p class="text-sm sm:text-[15px] text-[var(--text-secondary)] leading-relaxed">
            {lang === "id"
              ? "Kinetiq merekam kunjungan halaman, pendaftaran akun, dan konversi pembayaran secara langsung. Pasang gratis di server Anda sendiri dengan satu perintah."
              : "Kinetiq captures page views, signups, and checkout conversions in real time. Self-host on your own server with one command."}
          </p>

          <!-- Action Buttons -->
          <div class="flex flex-wrap items-center gap-3 pt-1">
            {#if isAuthenticated}
              <a
                href="/app"
                class="h-10 px-5 rounded-xl bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold inline-flex items-center gap-2 transition-colors"
              >
                <span>{lang === "id" ? "Buka Dashboard" : "Open Dashboard"}</span>
                <ArrowRight class="w-3.5 h-3.5" />
              </a>
            {:else}
              <a
                href="/signup"
                class="h-10 px-5 rounded-xl bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white text-xs font-semibold inline-flex items-center gap-2 transition-colors"
              >
                <span>{lang === "id" ? "Buat Workspace Gratis" : "Create Free Workspace"}</span>
                <ArrowRight class="w-3.5 h-3.5" />
              </a>
              <a
                href="/login"
                class="h-10 px-4 rounded-xl bg-[var(--bg-surface-muted)] hover:bg-[var(--border-subtle)] text-[var(--text-primary)] border border-[var(--border-subtle)] text-xs font-semibold inline-flex items-center transition-colors"
              >
                {lang === "id" ? "Masuk ke Akun" : "Sign in"}
              </a>
            {/if}
          </div>
        </div>

        <!-- Bottom Self-Host Command Bar -->
        <div class="pt-6 border-t border-[var(--border-subtle)] space-y-2">
          <div class="flex items-center justify-between text-[11px] font-mono text-[var(--text-muted)]">
            <span>{lang === "id" ? "SELF-HOST DI VPS ANDA (DOCKER)" : "SELF-HOST ON YOUR VPS (DOCKER)"}</span>
            <span>MIT Licensed</span>
          </div>
          <div class="h-10 px-3.5 rounded-xl bg-[var(--code-bg)] border border-[var(--code-border)] flex items-center justify-between gap-2 text-xs font-mono text-[var(--code-plain)]">
            <div class="flex items-center gap-2 truncate">
              <span class="text-[var(--brand-primary)] select-none">$</span>
              <code class="truncate">{dockerOneLiner}</code>
            </div>
            <button
              type="button"
              onclick={copyHeroDocker}
              class="inline-flex items-center gap-1 px-2 py-1 rounded text-[10px] text-[var(--text-muted)] hover:text-[var(--text-primary)] cursor-pointer shrink-0"
            >
              {#if copiedDockerHero}
                <Check class="w-3 h-3 text-[var(--brand-primary)]" />
                <span class="text-[var(--brand-primary)]">Copied</span>
              {:else}
                <Copy class="w-3 h-3" />
                <span>Copy</span>
              {/if}
            </button>
          </div>
        </div>
      </div>

      <!-- Right 6 Cols: Live Interactive Telemetry Instrument -->
      <div class="lg:col-span-6 bg-[var(--bg-surface-muted)]/60 p-6 sm:p-8 flex flex-col justify-between space-y-5">
        <!-- Instrument Header & Trigger Controls -->
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <button
              type="button"
              onclick={() => (autoStream = !autoStream)}
              class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[10px] font-mono uppercase tracking-wider border transition-colors cursor-pointer {autoStream
                ? 'bg-[var(--bg-surface)] text-[var(--brand-primary)] border-[var(--border-subtle)] font-semibold'
                : 'bg-amber-500/10 text-amber-600 border-amber-500/25'}"
            >
              {#if autoStream}
                <span class="w-1.5 h-1.5 rounded-full bg-[var(--brand-primary)] animate-pulse"></span>
                <span>STREAMING</span>
                <Pause class="w-2.5 h-2.5 ml-0.5" />
              {:else}
                <Play class="w-2.5 h-2.5" />
                <span>PAUSED</span>
              {/if}
            </button>
            <span class="text-[11px] font-mono text-[var(--text-muted)]">
              {lang === "id" ? "Uji kirim aktivitas:" : "Simulate event:"}
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-1">
            <button
              type="button"
              onclick={() =>
                triggerDemoAction(
                  "page_view",
                  lang === "id" ? "/katalog/sepatu-lari" : "/catalog/running-shoes"
                )}
              class="px-2.5 py-1 rounded-md text-[11px] font-mono bg-[var(--bg-surface)] hover:border-[var(--brand-primary)] text-[var(--text-primary)] border border-[var(--border-subtle)] transition-colors cursor-pointer"
            >
              + page_view
            </button>
            <button
              type="button"
              onclick={() =>
                triggerDemoAction(
                  "add_to_cart",
                  lang === "id" ? "Kopi Gayo 250g" : "Gayo Coffee 250g"
                )}
              class="px-2.5 py-1 rounded-md text-[11px] font-mono bg-[var(--bg-surface)] hover:border-[var(--brand-primary)] text-[var(--text-primary)] border border-[var(--border-subtle)] transition-colors cursor-pointer"
            >
              + add_to_cart
            </button>
            <button
              type="button"
              onclick={() =>
                triggerDemoAction(
                  "checkout_completed",
                  currency === "IDR" ? "Order #INV-914 · Rp 380.000" : "Order #INV-914 · $25.00"
                )}
              class="px-2.5 py-1 rounded-md text-[11px] font-mono font-semibold bg-[var(--brand-primary)] hover:bg-[var(--brand-primary-hover)] text-white transition-colors cursor-pointer"
            >
              + checkout
            </button>
          </div>
        </div>

        <!-- Instrument Window Frame -->
        <div class="rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] shadow-[var(--shadow-bento)] overflow-hidden divide-y divide-[var(--border-subtle)]">
          <!-- 3-Column Tabular KPI Strip -->
          <div class="grid grid-cols-3 divide-x divide-[var(--border-subtle)]">
            <div class="p-4">
              <div class="text-[10px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
                {lang === "id" ? "TOTAL AKTIVITAS" : "TOTAL ACTIVITY"}
              </div>
              <div class="text-xl sm:text-2xl font-bold font-mono tabular-nums text-[var(--text-primary)] mt-1">
                {demoTotalEvents.toLocaleString()}
              </div>
            </div>

            <div class="p-4">
              <div class="text-[10px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
                {lang === "id" ? "PENGUNJUNG AKTIF" : "ACTIVE VISITORS"}
              </div>
              <div class="text-xl sm:text-2xl font-bold font-mono tabular-nums text-[var(--brand-primary)] mt-1 flex items-center gap-2">
                <span>{demoActiveVisitors}</span>
                <span class="w-2 h-2 rounded-full bg-[var(--brand-primary)] animate-pulse"></span>
              </div>
            </div>

            <div class="p-4">
              <div class="text-[10px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
                {lang === "id" ? "KONVERSI BAYAR" : "CHECKOUT RATE"}
              </div>
              <div class="text-xl sm:text-2xl font-bold font-mono tabular-nums text-[var(--text-primary)] mt-1">
                {demoConversionRate}%
              </div>
            </div>
          </div>

          <!-- 16-Bar Precision Histogram -->
          <div class="p-4 space-y-2">
            <div class="flex items-center justify-between text-[11px] font-mono text-[var(--text-muted)]">
              <span>{lang === "id" ? "VOLUME AKTIVITAS (INTERVAL LANGSUNG)" : "ACTIVITY VOLUME (LIVE INTERVALS)"}</span>
              <span class="text-[var(--brand-primary)] font-semibold">{activeTrigger}</span>
            </div>
            <div class="h-24 flex items-end gap-1.5 pt-2">
              {#each demoBars as barPct, idx}
                {@const isLatest = idx === demoBars.length - 1}
                <div class="flex-1 h-full flex items-end">
                  <div
                    class="w-full rounded-t-sm transition-all duration-300 {isLatest
                      ? 'bg-[var(--brand-primary)]'
                      : idx % 2 === 0
                        ? 'bg-[var(--brand-primary)]/45'
                        : 'bg-[var(--brand-primary)]/20'}"
                    style="height: {barPct}%"
                  ></div>
                </div>
              {/each}
            </div>
          </div>

          <!-- Live Tabular Event Stream -->
          <div class="divide-y divide-[var(--border-subtle)] text-xs font-mono">
            <div class="px-4 py-2 bg-[var(--bg-surface-muted)]/60 flex items-center justify-between text-[10px] uppercase tracking-wider text-[var(--text-muted)]">
              <span>{lang === "id" ? "AKTIVITAS TERBARU" : "LIVE ACTIVITY LOG"}</span>
              <span>REGION / TIME</span>
            </div>
            {#each demoFeed as item (item.id)}
              <div class="px-4 py-2.5 flex items-center justify-between gap-3">
                <div class="flex items-center gap-2.5 min-w-0">
                  <span class="px-1.5 py-0.5 rounded bg-[var(--brand-soft)] text-[var(--brand-primary)] text-[10px] font-semibold shrink-0">
                    {item.name}
                  </span>
                  <span class="text-[var(--text-primary)] font-medium truncate text-[11px]">
                    {item.user}
                  </span>
                  <span class="text-[var(--text-muted)] truncate text-[11px] hidden sm:inline">
                    · {item.detail}
                  </span>
                </div>
                <div class="flex items-center gap-2 text-[10px] text-[var(--text-muted)] shrink-0">
                  <span>{item.region}</span>
                  <span>{item.time}</span>
                </div>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </section>

    <!-- SECTION 02: INTERACTIVE FUNNEL & PRIVACY CONTROLS (2-COLUMN ARCHITECTURAL SPLIT) -->
    <section
      id="inspection"
      class="grid grid-cols-1 lg:grid-cols-12 divide-y lg:divide-y-0 lg:divide-x divide-[var(--border-subtle)] border-b border-[var(--border-subtle)]"
    >
      <!-- Left 7 Cols: Interactive Conversion Funnel -->
      <div class="lg:col-span-7 p-7 sm:p-10 space-y-6 flex flex-col justify-between">
        <div class="space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <span class="text-[11px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
              02 / {lang === "id" ? "ANALISIS TAHAPAN KONVERSI" : "CONVERSION FUNNEL INSPECTION"}
            </span>

            <!-- Segmented Scenario Switcher -->
            <div class="inline-flex p-0.5 rounded-lg bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-[11px] font-mono">
              <button
                type="button"
                onclick={() => (activeFunnel = "ecommerce")}
                class="px-2.5 py-1 rounded-md transition-colors cursor-pointer {activeFunnel === 'ecommerce'
                  ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-2xs border border-[var(--border-subtle)]'
                  : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
              >
                {lang === "id" ? "Toko Online" : "E-Commerce"}
              </button>
              <button
                type="button"
                onclick={() => (activeFunnel = "saas")}
                class="px-2.5 py-1 rounded-md transition-colors cursor-pointer {activeFunnel === 'saas'
                  ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-2xs border border-[var(--border-subtle)]'
                  : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
              >
                SaaS
              </button>
              <button
                type="button"
                onclick={() => (activeFunnel = "mobile")}
                class="px-2.5 py-1 rounded-md transition-colors cursor-pointer {activeFunnel === 'mobile'
                  ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-2xs border border-[var(--border-subtle)]'
                  : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
              >
                Mobile
              </button>
            </div>
          </div>

          <div class="space-y-1">
            <h2 class="text-xl sm:text-2xl font-bold tracking-tight text-[var(--text-primary)]">
              {lang === "id"
                ? "Temukan di langkah mana pengunjung berhenti."
                : "Pinpoint the exact step where visitors drop off."}
            </h2>
            <p class="text-xs sm:text-sm text-[var(--text-secondary)]">
              {lang === "id"
                ? "Bandingkan jumlah pengunjung dari langkah pertama hingga pembayaran selesai."
                : "Measure visitor progression from initial landing to completed checkout."}
            </p>
          </div>
        </div>

        <div class="space-y-3.5 pt-2">
          {#each funnelSteps as f, i}
            <div class="p-3.5 rounded-xl bg-[var(--bg-surface-muted)]/60 border border-[var(--border-subtle)] space-y-2">
              <div class="flex items-center justify-between text-xs">
                <span class="font-semibold text-[var(--text-primary)] font-mono">{f.step}</span>
                <div class="flex items-center gap-3">
                  <span class="text-[11px] text-[var(--text-muted)] hidden sm:inline">{f.note}</span>
                  <span class="font-mono font-bold text-[var(--brand-primary)]">
                    {f.users.toLocaleString()} ({f.pct}%)
                  </span>
                </div>
              </div>
              <div class="w-full h-2.5 rounded-full bg-[var(--bg-canvas)] overflow-hidden">
                <div
                  class="h-full rounded-full transition-all duration-500 {i === funnelSteps.length - 1
                    ? 'bg-[var(--brand-primary)]'
                    : 'bg-[var(--brand-primary)]/65'}"
                  style="width: {f.pct}%"
                ></div>
              </div>
            </div>
          {/each}
        </div>
      </div>

      <!-- Right 5 Cols: Interactive UU PDP & GDPR Privacy Redactor -->
      <div class="lg:col-span-5 p-7 sm:p-10 space-y-6 flex flex-col justify-between bg-[var(--bg-surface-muted)]/30">
        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <span class="text-[11px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
              {lang === "id" ? " KONTROL PRIVASI PENGUNJUNG" : "VISITOR PRIVACY GUARD"}
            </span>

            <button
              type="button"
              onclick={() => (maskPrivacy = !maskPrivacy)}
              class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[11px] font-mono border transition-colors cursor-pointer {maskPrivacy
                ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] border-emerald-500/25 font-semibold'
                : 'bg-[var(--bg-surface)] text-[var(--text-secondary)] border-[var(--border-subtle)]'}"
            >
              {#if maskPrivacy}
                <EyeOff class="w-3 h-3" />
                <span>IP MASKING: ON</span>
              {:else}
                <Eye class="w-3 h-3" />
                <span>IP MASKING: OFF</span>
              {/if}
            </button>
          </div>

          <div class="space-y-1">
            <h3 class="text-xl font-bold tracking-tight text-[var(--text-primary)]">
              {lang === "id" ? "Kepatuhan UU PDP & Privasi Otomatis" : "Built-in IP & Identity Masking"}
            </h3>
            <p class="text-xs sm:text-sm text-[var(--text-secondary)] leading-relaxed">
              {lang === "id"
                ? "Aktifkan penyamaran IP agar alamat jaringan pribadi pengunjung tidak pernah disimpan mentah di database."
                : "Enable automatic IP masking so personal network addresses are anonymized before storage."}
            </p>
          </div>
        </div>

        <div class="rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] divide-y divide-[var(--border-subtle)] text-xs font-mono">
          <div class="px-3.5 py-2 bg-[var(--bg-surface-muted)]/60 flex items-center justify-between text-[10px] uppercase tracking-wider text-[var(--text-muted)]">
            <span>VISITOR</span>
            <span>STORED IP</span>
          </div>
          <div class="px-3.5 py-2.5 flex items-center justify-between">
            <span class="text-[var(--text-primary)]">{maskPrivacy ? "ri•••@tokokopi.id" : "rina@tokokopi.id"}</span>
            <span class="px-2 py-0.5 rounded {maskPrivacy ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] font-semibold' : 'bg-rose-500/10 text-rose-600'}">
              {maskPrivacy ? "103.144.18.0/24" : "103.144.18.92"}
            </span>
          </div>
          <div class="px-3.5 py-2.5 flex items-center justify-between">
            <span class="text-[var(--text-primary)]">{maskPrivacy ? "bu•••@studio.id" : "budi@studio.id"}</span>
            <span class="px-2 py-0.5 rounded {maskPrivacy ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] font-semibold' : 'bg-rose-500/10 text-rose-600'}">
              {maskPrivacy ? "114.122.40.0/24" : "114.122.40.188"}
            </span>
          </div>
          <div class="px-3.5 py-2.5 flex items-center justify-between">
            <span class="text-[var(--text-primary)]">{maskPrivacy ? "na•••@karya.co" : "nadia@karya.co"}</span>
            <span class="px-2 py-0.5 rounded {maskPrivacy ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] font-semibold' : 'bg-rose-500/10 text-rose-600'}">
              {maskPrivacy ? "180.252.91.0/24" : "180.252.91.14"}
            </span>
          </div>
        </div>
      </div>
    </section>

    <!-- SECTION 03: INTEGRATION & CARBON.SH THEME-ADAPTIVE CODE WINDOW -->
    <section
      id="integration"
      class="grid grid-cols-1 lg:grid-cols-12 divide-y lg:divide-y-0 lg:divide-x divide-[var(--border-subtle)] border-b border-[var(--border-subtle)]"
    >
      <!-- Left 5 Cols: Integration Spec & Multi-Key Table -->
      <div class="lg:col-span-5 p-7 sm:p-10 flex flex-col justify-between space-y-6">
        <div class="space-y-3">
          <span class="text-[11px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
            03 / {lang === "id" ? "INTEGRASI & MULTI-API KEY" : "INTEGRATION & MULTI-KEY SPEC"}
          </span>
          <h2 class="text-xl sm:text-2xl font-bold tracking-tight text-[var(--text-primary)]">
            {lang === "id"
              ? "Satu baris kode untuk mulai merekam aktivitas."
              : "One lightweight SDK for web, mobile, and backend."}
          </h2>
          <p class="text-xs sm:text-sm text-[var(--text-secondary)] leading-relaxed">
            {lang === "id"
              ? "Buat API key terpisah untuk Website, aplikasi Android/iOS, dan lingkungan pengujian di dalam satu proyek."
              : "Issue separate API keys for your Website, mobile apps, and staging environments within a single project."}
          </p>
        </div>

        <div class="rounded-xl bg-[var(--bg-surface-muted)]/60 border border-[var(--border-subtle)] divide-y divide-[var(--border-subtle)] text-xs">
          <div class="px-3.5 py-2 flex items-center justify-between text-[10px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
            <span>KEY LABEL</span>
            <span>PREFIX</span>
          </div>
          <div class="px-3.5 py-2.5 flex items-center justify-between">
            <span class="font-medium text-[var(--text-primary)]">Production Website</span>
            <code class="font-mono text-[11px] text-[var(--brand-primary)]">tlk_live_web_••••</code>
          </div>
          <div class="px-3.5 py-2.5 flex items-center justify-between">
            <span class="font-medium text-[var(--text-primary)]">Android & iOS App</span>
            <code class="font-mono text-[11px] text-[var(--brand-primary)]">tlk_live_app_••••</code>
          </div>
          <div class="px-3.5 py-2.5 flex items-center justify-between">
            <span class="font-medium text-[var(--text-primary)]">Staging Environment</span>
            <code class="font-mono text-[11px] text-[var(--text-muted)]">tlk_test_stg_••••</code>
          </div>
        </div>
      </div>

      <!-- Right 7 Cols: Theme-Adaptive Carbon.sh Code Window -->
      <div class="lg:col-span-7 p-7 sm:p-10 bg-[var(--bg-surface-muted)]/40 flex flex-col justify-center">
        <div class="rounded-xl bg-[var(--code-bg)] border border-[var(--code-border)] shadow-[var(--shadow-bento)] overflow-hidden text-[var(--code-plain)]">
          <div class="px-3.5 py-2.5 border-b border-[var(--code-border)] bg-[var(--code-header-bg)] flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="flex items-center gap-1.5">
                <span class="w-2.5 h-2.5 rounded-full bg-[#ff5f56]/90"></span>
                <span class="w-2.5 h-2.5 rounded-full bg-[#ffbd2e]/90"></span>
                <span class="w-2.5 h-2.5 rounded-full bg-[#27c93f]/90"></span>
              </div>
              <div class="flex items-center gap-1">
                <button
                  type="button"
                  onclick={() => (codeTab = "sdk")}
                  class="px-2.5 py-0.5 rounded-md text-[11px] font-mono cursor-pointer transition-colors {codeTab === 'sdk'
                    ? 'bg-[var(--code-bg)] text-[var(--brand-primary)] font-semibold border border-[var(--code-border)]'
                    : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
                >
                  tracker.ts
                </button>
                <button
                  type="button"
                  onclick={() => (codeTab = "docker")}
                  class="px-2.5 py-0.5 rounded-md text-[11px] font-mono cursor-pointer transition-colors {codeTab === 'docker'
                    ? 'bg-[var(--code-bg)] text-[var(--brand-primary)] font-semibold border border-[var(--code-border)]'
                    : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
                >
                  docker-compose.yml
                </button>
                <button
                  type="button"
                  onclick={() => (codeTab = "curl")}
                  class="px-2.5 py-0.5 rounded-md text-[11px] font-mono cursor-pointer transition-colors {codeTab === 'curl'
                    ? 'bg-[var(--code-bg)] text-[var(--brand-primary)] font-semibold border border-[var(--code-border)]'
                    : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
                >
                  curl.sh
                </button>
              </div>
            </div>

            <button
              type="button"
              onclick={copyActiveSnippet}
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-mono text-[var(--text-muted)] hover:text-[var(--text-primary)] cursor-pointer"
            >
              {#if copiedCode}
                <Check class="w-3 h-3 text-[var(--brand-primary)]" />
                <span class="text-[var(--brand-primary)]">Copied</span>
              {:else}
                <Copy class="w-3 h-3" />
                <span>Copy</span>
              {/if}
            </button>
          </div>

          <div class="p-4 text-[11px] font-mono leading-[1.7] overflow-x-auto select-text">
            {#if codeTab === "sdk"}
              <div class="grid grid-cols-[18px_1fr] gap-x-3">
                <span class="text-[var(--code-lineno)] select-none text-right">1</span>
                <div><span class="text-[var(--code-keyword)]">import</span> &#123; <span class="text-[var(--code-entity)]">Kinetiq</span> &#125; <span class="text-[var(--code-keyword)]">from</span> <span class="text-[var(--code-string)]">'@kinetiq/sdk'</span>;</div>
                <span class="text-[var(--code-lineno)] select-none text-right">2</span>
                <div></div>
                <span class="text-[var(--code-lineno)] select-none text-right">3</span>
                <div><span class="text-[var(--code-keyword)]">const</span> <span class="text-[var(--code-prop)]">kinetiq</span> = <span class="text-[var(--code-keyword)]">new</span> <span class="text-[var(--code-entity)]">Kinetiq</span>(&#123;</div>
                <span class="text-[var(--code-lineno)] select-none text-right">4</span>
                <div class="pl-3"><span class="text-[var(--code-prop)]">apiKey</span>: process.env.<span class="text-[var(--code-accent-str)]">KINETIQ_API_KEY</span>,</div>
                <span class="text-[var(--code-lineno)] select-none text-right">5</span>
                <div class="pl-3"><span class="text-[var(--code-prop)]">endpoint</span>: <span class="text-[var(--code-string)]">'https://ingest.kinetiq.io'</span></div>
                <span class="text-[var(--code-lineno)] select-none text-right">6</span>
                <div>&#125;);</div>
                <span class="text-[var(--code-lineno)] select-none text-right">7</span>
                <div></div>
                <span class="text-[var(--code-lineno)] select-none text-right">8</span>
                <div><span class="text-[var(--code-keyword)]">await</span> <span class="text-[var(--code-prop)]">kinetiq</span>.<span class="text-[var(--code-fn)]">track</span>(<span class="text-[var(--code-accent-str)]">'checkout_completed'</span>, &#123;</div>
                <span class="text-[var(--code-lineno)] select-none text-right">9</span>
                <div class="pl-3"><span class="text-[var(--code-prop)]">plan</span>: <span class="text-[var(--code-string)]">'growth_monthly'</span>,</div>
                <span class="text-[var(--code-lineno)] select-none text-right">10</span>
                <div class="pl-3"><span class="text-[var(--code-prop)]">currency</span>: <span class="text-[var(--code-string)]">'IDR'</span>,</div>
                <span class="text-[var(--code-lineno)] select-none text-right">11</span>
                <div class="pl-3"><span class="text-[var(--code-prop)]">value</span>: <span class="text-[var(--code-entity)]">249000</span></div>
                <span class="text-[var(--code-lineno)] select-none text-right">12</span>
                <div>&#125;);</div>
              </div>
            {:else}
              <pre class="text-[var(--code-plain)]">{rawSnippets[codeTab]}</pre>
            {/if}
          </div>

          <div class="px-3.5 py-2 border-t border-[var(--code-border)] bg-[var(--code-header-bg)] flex items-center justify-between text-[10px] font-mono">
            <span class="text-[var(--brand-primary)] font-semibold flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-[var(--brand-primary)]"></span>
              202 Accepted
            </span>
            <span class="text-[var(--text-muted)]">@kinetiq/sdk v1.0.0</span>
          </div>
        </div>
      </div>
    </section>
  </main>

  <!-- Minimal Architectural Footer -->
  <footer class="w-full max-w-[1220px] mx-auto border-x border-t border-[var(--border-subtle)] bg-[var(--bg-surface)] px-7 py-6 flex flex-col sm:flex-row items-center justify-between gap-4 text-xs text-[var(--text-muted)]">
    <div class="flex items-center gap-2">
      <span class="font-bold text-[var(--text-primary)]">Kinetiq Analytics</span>
      <span>·</span>
      <span class="font-mono">Self-Hosted Product Analytics</span>
    </div>

    <div class="flex items-center gap-5">
      <a href="/login" class="hover:text-[var(--text-primary)] transition-colors">
        {lang === "id" ? "Masuk" : "Sign in"}
      </a>
      <a href="/signup" class="hover:text-[var(--text-primary)] transition-colors">
        {lang === "id" ? "Daftar" : "Sign up"}
      </a>
      <a href="/app" class="hover:text-[var(--text-primary)] transition-colors">
        Dashboard
      </a>
    </div>
  </footer>
</div>
