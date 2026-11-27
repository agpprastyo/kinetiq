<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import { authClient } from "$lib/auth-client";
  import { getSetupStatus } from "$lib/setup";
  import { api, superadminApi, type Project } from "$lib/api";
  import { userStore, getUserAvatar } from "$lib/user.svelte";
  import type { LayoutProps } from "./$types";
  import Modal from "$lib/components/ui/Modal.svelte";
  import { toggleTheme, subscribeTheme, type Theme } from "$lib/theme";
  import {
    Activity,
    FolderKanban,
    Plus,
    LogOut,
    Radio,
    Sun,
    Moon,
    Settings,
    ChevronDown,
    Check,
    User,
    LayoutGrid,
    Building2,
    ShieldAlert
  } from "lucide-svelte";

  let { children }: LayoutProps = $props();

  let loading = $state(true);
  let currentTheme = $state<Theme>("light");
  let unsubTheme: (() => void) | null = null;

  let projects = $state<Project[]>([]);
  let isProjectDropdownOpen = $state(false);
  let isSignOutModalOpen = $state(false);
  let signingOut = $state(false);

  // Phase 7: Impersonation banner state
  let impersonationData = $state<{ target: any; actor: any } | null>(null);

  onMount(() => {
    try {
      const targetStr = sessionStorage.getItem("kinetiq_impersonated_target");
      const actorStr = sessionStorage.getItem("kinetiq_impersonated_by");
      if (targetStr && actorStr) {
        impersonationData = {
          target: JSON.parse(targetStr),
          actor: JSON.parse(actorStr),
        };
      }
    } catch (e) {
      console.warn("Failed to parse impersonation session data", e);
    }
  });

  async function handleStopImpersonation() {
    try {
      await superadminApi.stopImpersonating();
      sessionStorage.removeItem("kinetiq_impersonated_target");
      sessionStorage.removeItem("kinetiq_impersonated_by");
      window.location.href = "/ops";
    } catch (e) {
      console.error("Failed to stop impersonation", e);
      sessionStorage.removeItem("kinetiq_impersonated_target");
      sessionStorage.removeItem("kinetiq_impersonated_by");
      window.location.href = "/ops";
    }
  }

  async function refreshProjects() {
    try {
      projects = await api.getProjects();
    } catch (e) {
      console.error("Failed to fetch projects list", e);
    }
  }

  onMount(() => {
    unsubTheme = subscribeTheme((t) => {
      currentTheme = t;
    });

    (async () => {
      try {
        const session = await authClient.getSession();
        if (!session?.data?.user) {
          goto((await getSetupStatus()) === false ? "/setup" : "/login");
        } else {
          userStore.set(session.data.user);
          await refreshProjects();
        }
      } catch {
        goto("/login");
      } finally {
        loading = false;
      }
    })();
  });

  $effect(() => {
    const _path = page.url.pathname;
    if (userStore.current && (_path === "/app" || _path.startsWith("/app/projects/"))) {
      refreshProjects();
    }
  });

  onDestroy(() => {
    if (unsubTheme) unsubTheme();
  });

  async function handleConfirmSignOut() {
    signingOut = true;
    try {
      await authClient.signOut();
      userStore.set(null);
      isSignOutModalOpen = false;
      goto("/login");
    } catch (e) {
      console.error("Sign out error", e);
      goto("/login");
    } finally {
      signingOut = false;
    }
  }

  const user = $derived(userStore.current);
  const avatarSrc = $derived(getUserAvatar(user));

  // Distinguish Workspace Scope vs Project Scope clearly
  const activeRouteProjectId = $derived(page.params.projectId);
  const isInsideProject = $derived(Boolean(activeRouteProjectId));
  const fallbackProjectId = $derived(activeRouteProjectId || (projects[0]?.id ?? ""));
  const currentProject = $derived(
    projects.find((p) => p.id === activeRouteProjectId) || null
  );

  const pathname = $derived(page.url.pathname);
  const isProjectsIndex = $derived(pathname === "/app");
  const isNewProjectRoute = $derived(pathname === "/app/projects/new");
  const isWorkspaceSettingsActive = $derived(pathname.startsWith("/app/settings"));
  const isDashboardActive = $derived(
    Boolean(activeRouteProjectId && pathname === `/app/projects/${activeRouteProjectId}`)
  );
  const isLiveActive = $derived(pathname.endsWith("/live"));
  const isUsersActive = $derived(
    Boolean(activeRouteProjectId && pathname === `/app/projects/${activeRouteProjectId}/users`)
  );
  const isSettingsActive = $derived(
    Boolean(activeRouteProjectId && pathname === `/app/projects/${activeRouteProjectId}/settings`)
  );
  const isProfileActive = $derived(pathname.startsWith("/app/profile"));
  let scrollY = $state(0);
  const isScrolled = $derived(scrollY > 16);
</script>

<svelte:window
  bind:scrollY
  onclick={(e) => {
    const target = e.target as HTMLElement;
    if (!target.closest("#project-switcher-container")) {
      isProjectDropdownOpen = false;
    }
  }}
/>

{#if loading}
  <div class="flex flex-col items-center justify-center min-h-screen gap-3 text-[var(--text-muted)] bg-[var(--bg-canvas)]">
    <div class="w-9 h-9 rounded-full border-2 border-emerald-500/20 border-t-[var(--brand-primary)] animate-spin"></div>
    <span class="text-xs font-medium text-[var(--text-muted)]">Opening your workspace...</span>
  </div>
{:else if user}
  <div class="min-h-screen flex flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)] font-sans">
    {#if impersonationData}
      <!-- Impersonation Warning Banner -->
      <div class="relative z-[60] bg-gradient-to-r from-amber-500 via-amber-600 to-amber-500 text-zinc-950 px-4 py-2 text-xs font-mono font-medium shadow-md flex items-center justify-between gap-3">
        <div class="flex items-center gap-2 overflow-hidden">
          <ShieldAlert class="w-4 h-4 shrink-0 text-zinc-950 animate-pulse" />
          <span class="truncate">
            <strong>[IMPERSONATION MODE]</strong> You are viewing this workspace as <span class="underline font-bold">{impersonationData.target.email}</span>. Operator actions are audited.
          </span>
        </div>
        <button
          onclick={handleStopImpersonation}
          class="shrink-0 px-3 py-1 rounded-lg bg-zinc-950 text-amber-400 hover:bg-black font-bold text-[11px] transition-all cursor-pointer shadow-sm"
        >
          Exit to /ops
        </button>
      </div>
    {/if}

    <!-- Top Bar: Fixed at top, full-width flush header -> Smoothly morphs into Floating Glassmorphism Pill on scroll -->
    <div
      class="fixed top-0 inset-x-0 z-50 pointer-events-none transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] {isScrolled
        ? 'px-4 sm:px-6 pt-3 pb-2'
        : 'px-0 pt-0 pb-0'}"
    >
      <header
        class="pointer-events-auto mx-auto flex items-center justify-between gap-4 transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] {isScrolled
          ? 'max-w-[1320px] rounded-full px-4 sm:px-6 py-2.5 bg-[var(--bg-surface)]/78 backdrop-blur-xl backdrop-saturate-150 border border-[var(--border-hover)] shadow-[var(--shadow-dropdown)]'
          : 'max-w-full rounded-none px-4 sm:px-8 py-3.5 bg-[var(--bg-surface)]/95 backdrop-blur-md border-b border-[var(--border-subtle)] shadow-none'}"
      >
        <!-- Left: Brand Identity & Workspace / Project Scope Switcher -->
        <div class="flex items-center gap-3 shrink-0">
          <a href="/app" class="flex items-center gap-2.5 group">
            <div class="w-9 h-9 rounded-full bg-[var(--brand-primary)] flex items-center justify-center text-white transition-transform group-hover:scale-105">
              <Activity class="w-4 h-4" />
            </div>
            <div class="flex items-baseline gap-1.5">
              <span class="font-extrabold text-base tracking-tight text-[var(--text-primary)]">Kinetiq</span>
              <span class="hidden sm:inline-block text-[10px] font-semibold uppercase tracking-widest px-2 py-0.5 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)]">
                Workspace
              </span>
            </div>
          </a>

          <!-- Project Scope Switcher Pill -->
          {#if projects.length > 0}
            <div class="hidden md:flex items-center gap-2 text-[var(--text-muted)]">
              <span>/</span>
              <div id="project-switcher-container" class="relative">
                <button
                  type="button"
                  onclick={() => (isProjectDropdownOpen = !isProjectDropdownOpen)}
                  class="flex items-center gap-2 px-3.5 py-1.5 rounded-full text-xs font-medium transition-all cursor-pointer whitespace-nowrap {isInsideProject ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] border border-emerald-500/20' : 'bg-[var(--bg-surface-muted)]/85 text-[var(--text-secondary)] hover:text-[var(--text-primary)] border border-[var(--border-subtle)]'}"
                >
                  <span class="w-2 h-2 rounded-full shrink-0 {isInsideProject ? 'bg-[var(--brand-primary)]' : 'bg-[var(--text-muted)]'}"></span>
                  <span class="font-semibold max-w-[150px] truncate">
                    {currentProject ? currentProject.name : "Select Project"}
                  </span>
                  <ChevronDown class="w-3.5 h-3.5 opacity-75 shrink-0 transition-transform {isProjectDropdownOpen ? 'rotate-180' : ''}" />
                </button>

                {#if isProjectDropdownOpen}
                  <div class="absolute left-0 top-full mt-2 w-64 rounded-2xl border border-[var(--border-subtle)] bg-[var(--bg-surface)]/95 backdrop-blur-xl p-2 shadow-[var(--shadow-dropdown)] z-50">
                    <div class="px-3 py-1.5 flex items-center justify-between text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                      <span>Switch Project</span>
                      <a
                        href="/app"
                        onclick={() => (isProjectDropdownOpen = false)}
                        class="text-[var(--brand-primary)] hover:underline font-sans font-semibold capitalize"
                      >
                        View All ({projects.length})
                      </a>
                    </div>
                    <div class="max-h-60 overflow-y-auto space-y-1">
                      {#each projects as p}
                        <a
                          href="/app/projects/{p.id}"
                          onclick={() => (isProjectDropdownOpen = false)}
                          class="flex items-center justify-between px-3 py-2 rounded-xl text-xs transition-colors {p.id === activeRouteProjectId ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] font-semibold' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)]'}"
                        >
                          <span class="truncate">{p.name}</span>
                          {#if p.id === activeRouteProjectId}
                            <Check class="w-3.5 h-3.5 text-[var(--brand-primary)] shrink-0" />
                          {/if}
                        </a>
                      {/each}
                    </div>
                    <div class="pt-1.5 mt-1.5 border-t border-[var(--border-subtle)]">
                      <a
                        href="/app/projects/new"
                        onclick={() => (isProjectDropdownOpen = false)}
                        class="flex items-center gap-1.5 px-3 py-2 rounded-xl text-xs font-medium text-[var(--brand-primary)] hover:bg-[var(--brand-soft)] transition-colors"
                      >
                        <Plus class="w-3.5 h-3.5" />
                        New Project
                      </a>
                    </div>
                  </div>
                {/if}
              </div>
            </div>
          {/if}
        </div>

        <!-- Center: Contextual Navigation Pill (Single-line whitespace-nowrap) -->
        <nav class="hidden lg:flex items-center gap-1 bg-[var(--bg-surface-muted)]/80 backdrop-blur-md p-1 rounded-full border border-[var(--border-subtle)] whitespace-nowrap">
          {#if isInsideProject && activeRouteProjectId}
            <!-- PER-PROJECT NAVIGATION TABS -->
            <a
              href="/app"
              class="px-3.5 py-1.5 rounded-full text-xs font-medium text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-all whitespace-nowrap"
              title="Back to All Workspace Projects"
            >
              ← Workspace
            </a>
            <div class="h-3.5 w-[1px] bg-[var(--border-subtle)] shrink-0"></div>
            <a
              href="/app/projects/{activeRouteProjectId}"
              class="px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap {isDashboardActive ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
            >
              Overview
            </a>
            <a
              href="/app/projects/{activeRouteProjectId}/live"
              class="flex items-center gap-1.5 px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap {isLiveActive ? 'bg-[var(--bg-surface)] text-[var(--brand-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
            >
              <span class="w-1.5 h-1.5 rounded-full bg-[var(--brand-primary)] shrink-0 {isLiveActive ? 'animate-ping' : ''}"></span>
              Live Feed
            </a>
            <a
              href="/app/projects/{activeRouteProjectId}/users"
              class="px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap {isUsersActive ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
            >
              Users
            </a>
            <a
              href="/app/projects/{activeRouteProjectId}/settings"
              class="px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap {isSettingsActive ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
            >
              Project Settings
            </a>
          {:else}
            <!-- WORKSPACE-LEVEL NAVIGATION TABS -->
            <a
              href="/app"
              class="px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap {isProjectsIndex ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
            >
              All Projects ({projects.length})
            </a>
            <a
              href="/app/projects/new"
              class="px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap {isNewProjectRoute ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
            >
              + New Project
            </a>
            <a
              href="/app/settings"
              class="px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap {isWorkspaceSettingsActive && !pathname.startsWith('/app/settings/status') ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
            >
              Workspace Settings
            </a>
            <a
              href="/app/settings/status"
              class="px-4 py-1.5 rounded-full text-xs font-medium transition-all whitespace-nowrap {pathname.startsWith('/app/settings/status') ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] font-semibold shadow-xs' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
            >
              System Status
            </a>
          {/if}
        </nav>

        <!-- Right: Theme & Personal Account Pill (Profile lives here, never mixed into Project tabs) -->
        <div class="flex items-center gap-2">
          <button
            type="button"
            onclick={() => toggleTheme()}
            title={currentTheme === "dark" ? "Switch to Soft Light mode" : "Switch to Dark mode"}
            class="w-9 h-9 rounded-full flex items-center justify-center bg-[var(--bg-surface-muted)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] border border-[var(--border-subtle)] transition-colors cursor-pointer"
          >
            {#if currentTheme === "dark"}
              <Sun class="w-4 h-4 text-amber-400" />
            {:else}
              <Moon class="w-4 h-4 text-[var(--text-secondary)]" />
            {/if}
          </button>

          <!-- User Account Pill (Always has Default Profile Pic if user.image is empty) -->
          <a
            href="/app/profile"
            class="flex items-center gap-2.5 pl-1.5 pr-3.5 py-1 rounded-full border transition-all {isProfileActive ? 'bg-[var(--brand-soft)] border-emerald-500/30' : 'bg-[var(--bg-surface-muted)] hover:bg-[var(--bg-surface-elevated)] border-[var(--border-subtle)]'}"
            title="Personal Profile & Security Settings"
          >
            <img
              src={avatarSrc}
              alt={user.name || "User Avatar"}
              class="w-7 h-7 rounded-full object-cover border border-[var(--border-subtle)] shrink-0"
            />
            <div class="hidden sm:flex flex-col text-left">
              <span class="text-xs font-semibold text-[var(--text-primary)] leading-tight max-w-[125px] truncate">
                {user.name || user.email.split("@")[0]}
              </span>
              <span class="text-[10px] text-[var(--text-muted)] leading-tight">
                {isProfileActive ? "Editing Profile" : "Account & Profile"}
              </span>
            </div>
          </a>

          <button
            type="button"
            onclick={() => (isSignOutModalOpen = true)}
            title="Sign Out"
            class="w-9 h-9 rounded-full flex items-center justify-center text-[var(--text-muted)] hover:text-rose-500 hover:bg-rose-500/10 transition-colors cursor-pointer"
          >
            <LogOut class="w-4 h-4" />
          </button>
        </div>
      </header>
    </div>

    <!-- Top Spacer for Fixed Navbar -->
    <div class="h-[68px] shrink-0"></div>

    <!-- Main Content Layout with Floating Left Icon Dock -->
    <div class="flex-1 max-w-[1440px] mx-auto w-full px-4 sm:px-6 pt-4 pb-12 flex gap-6 items-start">
      <!-- Floating Left Vertical Icon Dock (Grouped by Scope: Workspace -> Active Project -> Account) -->
      <aside class="hidden xl:flex flex-col items-center justify-between py-3 px-2 rounded-full bg-[var(--bg-surface)] border border-[var(--border-subtle)] shadow-[var(--shadow-pill)] sticky top-24 min-h-[440px]">
        <div class="flex flex-col items-center gap-2">
          <!-- Group 1: Workspace Level -->
          <a
            href="/app"
            title="Workspace: All Projects"
            class="w-10 h-10 rounded-full flex items-center justify-center transition-all {isProjectsIndex ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)]'}"
          >
            <FolderKanban class="w-4 h-4" />
          </a>

          <a
            href="/app/projects/new"
            title="Workspace: Create New Project"
            class="w-10 h-10 rounded-full flex items-center justify-center transition-all {isNewProjectRoute ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--brand-primary)] hover:bg-[var(--brand-soft)]'}"
          >
            <Plus class="w-4 h-4" />
          </a>

          <a
            href="/app/settings"
            title="Workspace: Settings & Team RBAC"
            class="w-10 h-10 rounded-full flex items-center justify-center transition-all {isWorkspaceSettingsActive ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)]'}"
          >
            <Building2 class="w-4 h-4" />
          </a>

          <!-- Group 2: Active Project Scope -->
          {#if fallbackProjectId}
            <div class="w-5 h-[1px] bg-[var(--border-subtle)] my-1"></div>

            <a
              href="/app/projects/{fallbackProjectId}"
              title="Project: Overview Dashboard"
              class="w-10 h-10 rounded-full flex items-center justify-center transition-all {isDashboardActive ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)]'}"
            >
              <LayoutGrid class="w-4 h-4" />
            </a>

            <a
              href="/app/projects/{fallbackProjectId}/live"
              title="Project: Live Event Stream"
              class="w-10 h-10 rounded-full flex items-center justify-center transition-all {isLiveActive ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)]'}"
            >
              <Radio class="w-4 h-4" />
            </a>

            <a
              href="/app/projects/{fallbackProjectId}/settings"
              title="Project: API Keys & Settings"
              class="w-10 h-10 rounded-full flex items-center justify-center transition-all {isSettingsActive ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)]'}"
            >
              <Settings class="w-4 h-4" />
            </a>
          {/if}
        </div>

        <!-- Group 3: Personal Account Scope -->
        <div class="flex flex-col items-center gap-2 pt-3 border-t border-[var(--border-subtle)]">
          <a
            href="/app/profile"
            title="Account: Profile & Security"
            class="w-10 h-10 rounded-full flex items-center justify-center transition-all {isProfileActive ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-muted)]'}"
          >
            <User class="w-4 h-4" />
          </a>

          <button
            type="button"
            onclick={() => (isSignOutModalOpen = true)}
            title="Sign Out"
            class="w-10 h-10 rounded-full flex items-center justify-center text-[var(--text-muted)] hover:text-rose-500 hover:bg-rose-500/10 transition-all cursor-pointer"
          >
            <LogOut class="w-4 h-4" />
          </button>
        </div>
      </aside>

      <!-- Main Route Container -->
      <main class="flex-1 min-w-0 w-full">
        {@render children()}
      </main>
    </div>
  </div>

  <!-- Sign Out Confirmation Modal -->
  <Modal
    open={isSignOutModalOpen}
    title="Sign Out"
    description="Are you sure you want to sign out of your Kinetiq workspace?"
    confirmText="Sign Out"
    confirmVariant="danger"
    confirmLoading={signingOut}
    onconfirm={handleConfirmSignOut}
    oncancel={() => (isSignOutModalOpen = false)}
  />
{/if}
