<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Project } from "$lib/api";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import Card from "$lib/components/ui/Card.svelte";
  import {
    Plus,
    FolderKanban,
    ArrowUpRight,
    Calendar,
    Activity,
    RefreshCw,
    Settings,
    Radio,
    Search,
    ShieldCheck
  } from "lucide-svelte";

  let projects = $state<Project[]>([]);
  let loading = $state(true);
  let error = $state("");
  let searchQuery = $state("");

  async function loadProjects() {
    loading = true;
    error = "";
    try {
      const res = await api.getProjects();
      projects = res || [];
    } catch (err: any) {
      error = err.message || "Failed to load projects.";
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadProjects();
  });

  const filteredProjects = $derived(
    searchQuery.trim()
      ? projects.filter((p) =>
          p.name.toLowerCase().includes(searchQuery.trim().toLowerCase()) ||
          p.id.toLowerCase().includes(searchQuery.trim().toLowerCase())
        )
      : projects
  );
</script>

<div class="space-y-6">
  <!-- Bento Header Row -->
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
    <div>
      <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] text-[11px] font-semibold mb-2">
        <ShieldCheck class="w-3.5 h-3.5" />
        All Projects
      </div>
      <h1 class="text-2xl sm:text-3xl font-extrabold tracking-tight text-[var(--text-primary)]">
        Your Projects
      </h1>
      <p class="text-xs sm:text-sm text-[var(--text-secondary)] mt-1">
        Select a project to view visitor activity, live events, conversion funnels, or manage project keys.
      </p>
    </div>

    <div class="flex items-center gap-2.5">
      <div class="relative">
        <Search class="w-3.5 h-3.5 text-[var(--text-muted)] absolute left-3.5 top-1/2 -translate-y-1/2 pointer-events-none" />
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Search project..."
          class="pl-9 pr-4 py-2 rounded-full bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-primary)] shadow-[var(--shadow-pill)] w-44 sm:w-56"
        />
      </div>

      <Button variant="secondary" size="md" onclick={loadProjects} loading={loading} title="Refresh projects">
        <RefreshCw class="w-3.5 h-3.5 text-[var(--text-secondary)] {loading ? 'animate-spin' : ''}" />
      </Button>

      <a href="/app/projects/new">
        <Button variant="primary" size="md">
          <Plus class="w-4 h-4" />
          New Project
        </Button>
      </a>
    </div>
  </div>

  {#if loading}
    <div class="flex flex-col items-center justify-center py-24 gap-3 text-[var(--text-muted)] font-mono text-xs">
      <div class="w-8 h-8 rounded-full border-2 border-emerald-500/20 border-t-[var(--brand-primary)] animate-spin"></div>
      <span>LOADING PROJECTS...</span>
    </div>
  {:else if error}
    <div class="rounded-2xl border border-rose-500/30 bg-rose-500/10 p-4 text-xs font-mono text-rose-600 dark:text-rose-400 flex items-center justify-between">
      <span>{error}</span>
      <Button variant="danger" size="sm" onclick={loadProjects}>Retry</Button>
    </div>
  {:else if projects.length === 0}
    <!-- Empty State -->
    <Card class="flex flex-col items-center justify-center py-16 text-center max-w-lg mx-auto">
      <div class="w-12 h-12 rounded-2xl bg-[var(--brand-soft)] flex items-center justify-center text-[var(--brand-primary)] mb-3">
        <FolderKanban class="w-6 h-6" />
      </div>
      <h3 class="text-base font-bold text-[var(--text-primary)]">No projects created yet</h3>
      <p class="text-xs text-[var(--text-secondary)] mt-1 max-w-xs leading-relaxed">
        Create your first project to get an API key and start tracking user activity on your app or website.
      </p>
      <div class="mt-5">
        <a href="/app/projects/new">
          <Button variant="primary" size="md">
            <Plus class="w-4 h-4" />
            Create Your First Project
          </Button>
        </a>
      </div>
    </Card>
  {:else}
    <!-- Quixotic Bento Projects Grid -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
      {#each filteredProjects as project, idx}
        <div
          class="group rounded-[24px] border border-[var(--border-subtle)] {idx === 0 ? 'bg-gradient-to-br from-[#127852] to-[#0E5F40] text-white border-transparent' : 'bg-[var(--bg-surface)] text-[var(--text-primary)]'} p-6 shadow-[var(--shadow-bento)] hover:shadow-[var(--shadow-bento-hover)] transition-all duration-200 hover:-translate-y-0.5 flex flex-col justify-between"
        >
          <div class="space-y-4">
            <div class="flex items-start justify-between gap-3">
              <div class="flex items-center gap-3">
                <div
                  class="w-10 h-10 rounded-2xl flex items-center justify-center {idx === 0 ? 'bg-white/15 text-white' : 'bg-[var(--brand-soft)] text-[var(--brand-primary)]'}"
                >
                  <Activity class="w-5 h-5" />
                </div>
                <div>
                  <a
                    href="/app/projects/{project.id}"
                    class="text-base font-bold tracking-tight hover:underline line-clamp-1 {idx === 0 ? 'text-white' : 'text-[var(--text-primary)]'}"
                  >
                    {project.name}
                  </a>
                  <div class="flex items-center gap-2 mt-1">
                    {#if idx === 0}
                      <span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[10px] font-mono uppercase tracking-wider bg-white/15 text-white">
                        <span class="w-1.5 h-1.5 rounded-full bg-emerald-300 animate-pulse"></span>
                        {project.status}
                      </span>
                    {:else}
                      <Badge
                        variant={project.status === "active" ? "emerald" : "amber"}
                        pulse={project.status === "active"}
                      >
                        {project.status}
                      </Badge>
                    {/if}
                    {#if project.retention_days}
                      <span class="text-[11px] font-mono {idx === 0 ? 'text-emerald-100/80' : 'text-[var(--text-muted)]'}">
                        {project.retention_days}d history
                      </span>
                    {/if}
                  </div>
                </div>
              </div>

              <a
                href="/app/projects/{project.id}"
                class="w-8 h-8 rounded-full flex items-center justify-center transition-colors {idx === 0 ? 'bg-white/10 text-white hover:bg-white/20' : 'bg-[var(--bg-surface-muted)] text-[var(--text-secondary)] hover:text-[var(--brand-primary)]'}"
                title="Open Dashboard"
              >
                <ArrowUpRight class="w-4 h-4" />
              </a>
            </div>

            <!-- Project ID preview -->
            <div
              class="px-3.5 py-2.5 rounded-2xl flex items-center justify-between text-[11px] font-mono {idx === 0 ? 'bg-black/20 text-emerald-100' : 'bg-[var(--bg-surface-muted)] text-[var(--text-secondary)]'}"
            >
              <span class="opacity-75">PROJECT ID</span>
              <span class="truncate ml-2 font-semibold">{project.id.slice(0, 8)}...{project.id.slice(-4)}</span>
            </div>
          </div>

          <!-- Footer Quick Actions -->
          <div
            class="mt-6 pt-4 border-t {idx === 0 ? 'border-white/15 text-emerald-100' : 'border-[var(--border-subtle)] text-[var(--text-muted)]'} flex items-center justify-between text-xs"
          >
            <span class="flex items-center gap-1.5 font-mono text-[11px]">
              <Calendar class="w-3.5 h-3.5" />
              {new Date(project.created_at).toLocaleDateString([], { month: "short", day: "numeric", year: "numeric" })}
            </span>

            <div class="flex items-center gap-1.5">
              <a
                href="/app/projects/{project.id}/live"
                class="px-2.5 py-1 rounded-full text-[11px] font-medium transition-colors {idx === 0 ? 'bg-white/15 text-white hover:bg-white/25' : 'bg-[var(--bg-surface-muted)] text-[var(--text-secondary)] hover:text-[var(--brand-primary)]'}"
                title="Live Stream"
              >
                <Radio class="w-3 h-3 inline mr-1" />
                Live
              </a>
              <a
                href="/app/projects/{project.id}/settings"
                class="px-2.5 py-1 rounded-full text-[11px] font-medium transition-colors {idx === 0 ? 'bg-white/15 text-white hover:bg-white/25' : 'bg-[var(--bg-surface-muted)] text-[var(--text-secondary)] hover:text-[var(--brand-primary)]'}"
                title="Project Settings & API Keys"
              >
                <Settings class="w-3 h-3 inline mr-1" />
                Settings
              </a>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
