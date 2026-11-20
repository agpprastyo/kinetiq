<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import {
    workspaceApi,
    type WorkspaceInfo,
    type WorkspaceMember,
    type WorkspaceInvitation,
    type OutboxEmailItem,
  } from "$lib/auth-client";
  import {
    api,
    rbacApi,
    toSafeUserMessage,
    type Project,
    type CustomRole,
    type GranularPermission
  } from "$lib/api";
  import { getUserAvatar, userStore } from "$lib/user.svelte";
  import { copyToClipboard } from "$lib/utils";
  import Card from "$lib/components/ui/Card.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import WorkspaceRolesSection from "./WorkspaceRolesSection.svelte";
  import {
    Building2,
    Users,
    UserPlus,
    CreditCard,
    Shield,
    Mail,
    Copy,
    Check,
    Trash2,
    AlertTriangle,
    FolderKanban,
    Globe,
    Sparkles,
    Save,
    ExternalLink,
    RefreshCw,
    Activity,
    Code
  } from "lucide-svelte";

  type SectionType = "team" | "roles" | "general";
  let activeSection = $state<SectionType>("team");

  let workspace = $state<WorkspaceInfo | null>(null);
  let members = $state<WorkspaceMember[]>([]);
  let invitations = $state<WorkspaceInvitation[]>([]);
  let outbox = $state<OutboxEmailItem[]>([]);
  let projects = $state<Project[]>([]);

  let loading = $state(true);
  let error = $state<string | null>(null);
  let feedback = $state<{ type: "success" | "error"; text: string } | null>(null);

  // Invite form states
  let inviteEmail = $state("");
  let inviteScope = $state<"workspace" | "project">("workspace");
  let inviteProjectId = $state("");
  let inviteRole = $state<"owner" | "admin" | "viewer">("viewer");
  let sendingInvite = $state(false);
  let copiedInviteId = $state<string | null>(null);

  // General & Billing form states
  let wsName = $state("");
  let wsSlug = $state("");
  let savingWorkspace = $state(false);

  // Workspace Custom Roles State
  let customRoles = $state<CustomRole[]>([]);
  let loadingRoles = $state(false);
  let isOwner = $derived((userStore.current as any)?.role === "owner");
  const wsAvailableResources = [
    { resource: "analytics", label: "Analytics & Funnels", actions: ["read", "export"] },
    { resource: "projects", label: "Projects & Streams", actions: ["create", "read", "update", "delete"] },
    { resource: "api_keys", label: "Project API Keys", actions: ["create", "read", "update", "delete"] },
    { resource: "webhooks", label: "Webhooks Dispatch", actions: ["create", "read", "update", "delete"] },
    { resource: "alerts", label: "Notification Rules", actions: ["create", "read", "update", "delete"] },
    { resource: "members", label: "Team Members", actions: ["create", "read", "update", "delete"] },
  ];

  async function loadRoles() {
    loadingRoles = true;
    try {
      const res = await rbacApi.listWorkspaceRoles();
      customRoles = res.roles || [];
    } catch {
      // ignore
    } finally {
      loadingRoles = false;
    }
  }

  async function handleSaveWorkspaceRole(role: { id?: string; name: string; description?: string; permissions: GranularPermission[] }) {
    try {
      if (role.id) {
        await rbacApi.updateWorkspaceRole(role.id, {
          description: role.description,
          permissions: role.permissions,
        });
        feedback = { type: "success", text: `Updated role ${role.name}.` };
      } else {
        await rbacApi.createWorkspaceRole({
          name: role.name,
          description: role.description,
          permissions: role.permissions,
        });
        feedback = { type: "success", text: `Created custom role ${role.name}.` };
      }
      loadRoles();
    } catch (err: any) {
      feedback = { type: "error", text: err.message || "Failed to save role" };
    }
  }

  async function handleDeleteWorkspaceRole(roleId: string, name: string) {
    if (!confirm(`Delete custom workspace role "${name}"?`)) return;
    try {
      await rbacApi.deleteWorkspaceRole(roleId);
      feedback = { type: "success", text: `Deleted custom role ${name}.` };
      loadRoles();
    } catch (err: any) {
      feedback = { type: "error", text: err.message || "Failed to delete role" };
    }
  }

  async function loadAll() {
    loading = true;
    error = null;
    try {
      const [overview, projList] = await Promise.all([
        workspaceApi.getOverview(),
        api.getProjects().catch(() => []),
      ]);

      workspace = overview.workspace;
      members = overview.members || [];
      invitations = overview.invitations || [];
      outbox = overview.outbox || [];
      projects = projList || [];

      wsName = workspace.name;
      wsSlug = workspace.slug;

      if (projects.length > 0 && !inviteProjectId) {
        inviteProjectId = projects[0].id;
      }

      // Check if URL has ?accept_invite=<id>
      const acceptId = page.url.searchParams.get("accept_invite") || page.url.searchParams.get("invite");
      if (acceptId) {
        const pending = invitations.find((i) => i.id === acceptId && i.status === "pending");
        if (pending) {
          await handleAcceptInvite(acceptId);
        }
      }
    } catch (err: any) {
      error = toSafeUserMessage(err, "Unable to load workspace settings.");
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadAll();
  });

  async function handleCreateInvite(e: SubmitEvent) {
    e.preventDefault();
    feedback = null;
    if (!inviteEmail.trim()) {
      feedback = { type: "error", text: "Please enter an email address to invite." };
      return;
    }
    if (inviteScope === "project" && !inviteProjectId) {
      feedback = { type: "error", text: "Please select a project for this project-scoped invitation." };
      return;
    }

    sendingInvite = true;
    try {
      await workspaceApi.createInvitation({
        email: inviteEmail.trim(),
        role: inviteRole,
        scopeType: inviteScope,
        projectId: inviteScope === "project" ? inviteProjectId : null,
      });
      inviteEmail = "";
      await loadAll();
      feedback = {
        type: "success",
        text: `Invitation created (${inviteScope === "project" ? "Project Scope" : "Workspace Scope"} · ${inviteRole.toUpperCase()}). You can copy the invite link or activate it below.`
      };
    } catch (err: any) {
      feedback = {
        type: "error",
        text: toSafeUserMessage(err, "Unable to create invitation.")
      };
    } finally {
      sendingInvite = false;
    }
  }

  async function handleAcceptInvite(inviteId: string) {
    feedback = null;
    try {
      await workspaceApi.acceptInvitation(inviteId);
      await loadAll();
      feedback = { type: "success", text: "Invitation accepted and member added to active RBAC registry." };
    } catch (err: any) {
      feedback = { type: "error", text: toSafeUserMessage(err, "Unable to accept invitation.") };
    }
  }

  async function handleRevokeInvite(inviteId: string) {
    feedback = null;
    try {
      await workspaceApi.revokeInvitation(inviteId);
      invitations = invitations.filter((i) => i.id !== inviteId);
      feedback = { type: "success", text: "Invitation revoked." };
    } catch (err: any) {
      feedback = { type: "error", text: toSafeUserMessage(err, "Unable to revoke invitation.") };
    }
  }

  async function handleCopyInviteUrl(inv: WorkspaceInvitation) {
    const ok = await copyToClipboard(inv.inviteUrl);
    if (ok) {
      copiedInviteId = inv.id;
      setTimeout(() => (copiedInviteId = null), 2000);
    }
  }

  async function handleChangeMemberRole(memberId: string, newRole: "owner" | "admin" | "viewer") {
    feedback = null;
    try {
      await workspaceApi.updateMemberRole(memberId, newRole);
      members = members.map((m) => (m.id === memberId ? { ...m, role: newRole } : m));
      feedback = { type: "success", text: "Team member role updated." };
    } catch (err: any) {
      feedback = { type: "error", text: toSafeUserMessage(err, "Unable to update member role.") };
    }
  }

  async function handleRemoveMember(memberId: string) {
    feedback = null;
    try {
      await workspaceApi.removeMember(memberId);
      members = members.filter((m) => m.id !== memberId);
      feedback = { type: "success", text: "Team member removed." };
    } catch (err: any) {
      feedback = { type: "error", text: toSafeUserMessage(err, "Unable to remove team member.") };
    }
  }

  async function handleSaveWorkspace(e: SubmitEvent) {
    e.preventDefault();
    feedback = null;
    if (!wsName.trim()) {
      feedback = { type: "error", text: "Workspace name cannot be empty." };
      return;
    }

    savingWorkspace = true;
    try {
      await workspaceApi.updateWorkspace({
        name: wsName.trim(),
        slug: wsSlug.trim(),
      });
      await loadAll();
      feedback = { type: "success", text: "Workspace profile updated successfully." };
    } catch (err: any) {
      feedback = { type: "error", text: toSafeUserMessage(err, "Unable to update workspace settings.") };
    } finally {
      savingWorkspace = false;
    }
  }


  const quotaPct = $derived.by(() => {
    if (!workspace || !workspace.monthlyEventQuota) return 0;
    return Math.min(100, (workspace.usedEventsThisMonth / workspace.monthlyEventQuota) * 100);
  });
</script>

<svelte:head>
  <title>Workspace Settings — Kinetiq</title>
</svelte:head>

<div class="space-y-6 w-full">
  <!-- Header & Sub-Navigation Pills -->
  <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
    <div>
      <div class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] text-[11px] font-semibold mb-2">
        <Building2 class="w-3.5 h-3.5" />
        Global Workspace Scope
      </div>
      <h1 class="text-2xl sm:text-3xl font-extrabold tracking-tight text-[var(--text-primary)]">
        Workspace Settings & Team RBAC
      </h1>
      <p class="text-xs sm:text-sm text-[var(--text-secondary)] mt-0.5">
        Manage organization identity, invite team members to your Workspace or specific Projects, and configure general settings.
      </p>
    </div>

    <!-- Section Switcher Pill -->
    <div class="inline-flex items-center gap-1 bg-[var(--bg-surface)] p-1.5 rounded-full border border-[var(--border-subtle)] shadow-[var(--shadow-pill)] self-start">
      <button
        type="button"
        onclick={() => (activeSection = "team")}
        class="px-4 py-1.5 rounded-full text-xs font-semibold transition-all cursor-pointer flex items-center gap-1.5 {activeSection === 'team' ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
      >
        <Users class="w-3.5 h-3.5" />
        Team & Invites ({members.length})
      </button>

      <button
        type="button"
        onclick={() => {
          activeSection = "roles";
          loadRoles();
        }}
        class="px-4 py-1.5 rounded-full text-xs font-semibold transition-all cursor-pointer flex items-center gap-1.5 {activeSection === 'roles' ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
      >
        <Shield class="w-3.5 h-3.5" />
        Custom Roles ({customRoles.length})
      </button>

      <button
        type="button"
        onclick={() => (activeSection = "general")}
        class="px-4 py-1.5 rounded-full text-xs font-semibold transition-all cursor-pointer flex items-center gap-1.5 {activeSection === 'general' ? 'bg-[var(--brand-primary)] text-white' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
      >
        <CreditCard class="w-3.5 h-3.5" />
        Organization & Billing
      </button>
    </div>
  </div>

  {#if feedback}
    <div
      class="p-4 rounded-2xl text-xs font-medium flex items-center justify-between gap-3 {feedback.type === 'success' ? 'bg-[var(--brand-soft)] text-[var(--brand-primary)] border border-emerald-500/20' : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/25'}"
    >
      <div class="flex items-center gap-2">
        {#if feedback.type === "success"}
          <Check class="w-4 h-4 shrink-0" />
        {:else}
          <AlertTriangle class="w-4 h-4 shrink-0" />
        {/if}
        <span>{feedback.text}</span>
      </div>
      <button type="button" onclick={() => (feedback = null)} class="text-xs opacity-70 hover:opacity-100 cursor-pointer">
        Dismiss
      </button>
    </div>
  {/if}

  {#if loading}
    <div class="p-16 text-center text-xs font-mono text-[var(--text-muted)]">
      LOADING WORKSPACE GOVERNANCE...
    </div>
  {:else if error}
    <div class="p-6 rounded-2xl border border-rose-500/25 bg-rose-500/10 text-rose-600 text-sm flex items-center justify-between">
      <span>{error}</span>
      <Button variant="secondary" size="sm" onclick={loadAll}>Retry</Button>
    </div>
  {:else if workspace}
    <!-- Balanced 12-Column Bento Split -->
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
      <!-- LEFT COLUMN (8 cols) -->
      <div class="lg:col-span-8 space-y-6">
        {#if activeSection === "team"}
          <!-- 1. INVITE MEMBER FORM (Supports Workspace OR Project Scope + Role Selection) -->
          <Card class="space-y-5">
            <div class="pb-4 border-b border-[var(--border-subtle)]">
              <h2 class="text-base font-bold text-[var(--text-primary)] flex items-center gap-2">
                <UserPlus class="w-4 h-4 text-[var(--brand-primary)]" />
                Invite Team Member (Workspace or Project Scope)
              </h2>
              <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                Grant access to your entire Workspace (all projects) or restrict a collaborator to a single specific Project with granular RBAC roles.
              </p>
            </div>

            <form onsubmit={handleCreateInvite} class="space-y-4">
              <div class="grid grid-cols-1 md:grid-cols-12 gap-4">
                <!-- Email -->
                <div class="md:col-span-5 space-y-1.5">
                  <label for="invEmail" class="text-xs font-semibold text-[var(--text-secondary)]">
                    Collaborator Email
                  </label>
                  <input
                    id="invEmail"
                    type="email"
                    bind:value={inviteEmail}
                    placeholder="teammate@company.id"
                    class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                    required
                  />
                </div>

                <!-- Target Scope (Workspace vs Project) -->
                <div class="md:col-span-4 space-y-1.5">
                  <label for="invScope" class="text-xs font-semibold text-[var(--text-secondary)]">
                    Access Scope
                  </label>
                  <select
                    id="invScope"
                    bind:value={inviteScope}
                    class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-medium text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                  >
                    <option value="workspace">Entire Workspace (All Projects)</option>
                    <option value="project">Specific Project Only</option>
                  </select>
                </div>

                <!-- Role -->
                <div class="md:col-span-3 space-y-1.5">
                  <label for="invRole" class="text-xs font-semibold text-[var(--text-secondary)]">
                    Role Permission
                  </label>
                  <select
                    id="invRole"
                    bind:value={inviteRole}
                    class="w-full px-3.5 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-semibold text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                  >
                    <option value="owner">Owner (Full)</option>
                    <option value="admin">Admin (Manage)</option>
                    <option value="viewer">Viewer (Read-Only)</option>
                  </select>
                </div>
              </div>

              {#if inviteScope === "project"}
                <div class="p-4 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] space-y-2">
                  <label for="invProjSelect" class="text-xs font-semibold text-[var(--text-secondary)] flex items-center gap-1.5">
                    <FolderKanban class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
                    Select Target Project
                  </label>
                  {#if projects.length === 0}
                    <p class="text-xs text-rose-600">
                      You don't have any projects yet. Create a project first or invite to the Entire Workspace.
                    </p>
                  {:else}
                    <select
                      id="invProjSelect"
                      bind:value={inviteProjectId}
                      class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs font-semibold text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                    >
                      {#each projects as p}
                        <option value={p.id}>{p.name} ({p.id.slice(0, 8)}...)</option>
                      {/each}
                    </select>
                  {/if}
                </div>
              {/if}

              <div class="flex items-center justify-between pt-1">
                <span class="text-[11px] text-[var(--text-muted)]">
                  Invitations are valid for 7 days and generate a shareable invite link.
                </span>
                <Button type="submit" variant="primary" size="md" loading={sendingInvite}>
                  <UserPlus class="w-4 h-4" />
                  Send Invitation
                </Button>
              </div>
            </form>
          </Card>

          <!-- 2. ACTIVE MEMBERS TABLE -->
          <Card class="space-y-4">
            <div class="pb-3 border-b border-[var(--border-subtle)] flex items-center justify-between">
              <div>
                <h2 class="text-base font-bold text-[var(--text-primary)]">
                  Active Workspace & Project Members
                </h2>
                <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                  Manage roles across global workspace members and project-specific collaborators.
                </p>
              </div>
              <Badge variant="emerald">{members.length} Active</Badge>
            </div>

            <div class="overflow-x-auto">
              <table class="w-full text-left text-xs">
                <thead class="border-b border-[var(--border-subtle)] text-[11px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                  <tr>
                    <th class="pb-3 pr-4">Member</th>
                    <th class="pb-3 px-3">Access Scope</th>
                    <th class="pb-3 px-3">Role</th>
                    <th class="pb-3 px-3">Joined</th>
                    <th class="pb-3 pl-3 text-right">Action</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--border-subtle)]">
                  {#each members as m (m.id)}
                    <tr class="hover:bg-[var(--bg-surface-muted)]/50 transition-colors">
                      <td class="py-3.5 pr-4">
                        <div class="flex items-center gap-2.5">
                          <img
                            src={getUserAvatar({ image: m.image })}
                            alt={m.name}
                            class="w-8 h-8 rounded-full object-cover border border-[var(--border-subtle)]"
                          />
                          <div>
                            <div class="font-bold text-[var(--text-primary)]">{m.name}</div>
                            <div class="text-[11px] font-mono text-[var(--text-muted)]">{m.email}</div>
                          </div>
                        </div>
                      </td>
                      <td class="py-3.5 px-3">
                        {#if m.scopeType === "project"}
                          <Badge variant="amber">
                            Project: {m.projectName || "Scoped"}
                          </Badge>
                        {:else}
                          <Badge variant="emerald">Entire Workspace</Badge>
                        {/if}
                      </td>
                      <td class="py-3.5 px-3">
                        <select
                          value={m.role === "member" ? "viewer" : m.role}
                          onchange={(e) =>
                            handleChangeMemberRole(
                              m.id,
                              (e.currentTarget as HTMLSelectElement).value as any
                            )}
                          class="px-3 py-1 rounded-full bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-xs font-semibold text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                        >
                          <option value="owner">Owner</option>
                          <option value="admin">Admin</option>
                          <option value="viewer">Viewer</option>
                        </select>
                      </td>
                      <td class="py-3.5 px-3 font-mono text-[11px] text-[var(--text-muted)]">
                        {new Date(m.createdAt).toLocaleDateString()}
                      </td>
                      <td class="py-3.5 pl-3 text-right">
                        {#if members.length > 1}
                          <button
                            type="button"
                            onclick={() => handleRemoveMember(m.id)}
                            class="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[11px] font-semibold text-rose-600 hover:bg-rose-500/10 cursor-pointer"
                          >
                            <Trash2 class="w-3 h-3" />
                            Remove
                          </button>
                        {:else}
                          <span class="text-[11px] text-[var(--text-muted)] font-mono">Primary Owner</span>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          </Card>

          <!-- 3. PENDING INVITATIONS TABLE -->
          <Card class="space-y-4">
            <div class="pb-3 border-b border-[var(--border-subtle)] flex items-center justify-between">
              <div>
                <h2 class="text-base font-bold text-[var(--text-primary)]">
                  Invitations Registry
                </h2>
                <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                  Share the invitation link with your teammate or click "Accept Now" to test role provisioning locally.
                </p>
              </div>
              <Badge variant="neutral">{invitations.length} total</Badge>
            </div>

            {#if invitations.length === 0}
              <div class="py-8 text-center text-xs text-[var(--text-muted)]">
                No invitations sent yet. Use the form above to invite a collaborator to your Workspace or a specific Project.
              </div>
            {:else}
              <div class="overflow-x-auto">
                <table class="w-full text-left text-xs">
                  <thead class="border-b border-[var(--border-subtle)] text-[11px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                    <tr>
                      <th class="pb-3 pr-4">Invited Email</th>
                      <th class="pb-3 px-3">Scope</th>
                      <th class="pb-3 px-3">Role</th>
                      <th class="pb-3 px-3">Status</th>
                      <th class="pb-3 pl-3 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-[var(--border-subtle)]">
                    {#each invitations as inv (inv.id)}
                      <tr class="hover:bg-[var(--bg-surface-muted)]/50 transition-colors">
                        <td class="py-3.5 pr-4 font-mono font-semibold text-[var(--text-primary)]">
                          {inv.email}
                        </td>
                        <td class="py-3.5 px-3">
                          {#if inv.scopeType === "project"}
                            <Badge variant="amber">
                              Project: {inv.projectName || "Specific"}
                            </Badge>
                          {:else}
                            <Badge variant="emerald">Workspace</Badge>
                          {/if}
                        </td>
                        <td class="py-3.5 px-3 uppercase font-mono text-[11px] font-bold text-[var(--text-secondary)]">
                          {inv.role}
                        </td>
                        <td class="py-3.5 px-3">
                          <Badge variant={inv.status === "accepted" ? "emerald" : "neutral"}>
                            {inv.status}
                          </Badge>
                        </td>
                        <td class="py-3.5 pl-3 text-right">
                          <div class="inline-flex items-center gap-1.5">
                            <button
                              type="button"
                              onclick={() => handleCopyInviteUrl(inv)}
                              class="px-2.5 py-1 rounded-full bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-[11px] font-semibold text-[var(--text-primary)] hover:border-[var(--brand-primary)] cursor-pointer inline-flex items-center gap-1"
                            >
                              {#if copiedInviteId === inv.id}
                                <Check class="w-3 h-3 text-[var(--brand-primary)]" />
                                Copied
                              {:else}
                                <Copy class="w-3 h-3" />
                                Copy Link
                              {/if}
                            </button>

                            {#if inv.status === "pending"}
                              <button
                                type="button"
                                onclick={() => handleAcceptInvite(inv.id)}
                                class="px-2.5 py-1 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] text-[11px] font-bold hover:opacity-90 cursor-pointer"
                              >
                                Accept Now
                              </button>
                            {/if}

                            <button
                              type="button"
                              onclick={() => handleRevokeInvite(inv.id)}
                              class="p-1.5 rounded-full text-rose-600 hover:bg-rose-500/10 cursor-pointer"
                              title="Revoke Invitation"
                            >
                              <Trash2 class="w-3.5 h-3.5" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {/if}
          </Card>
        {:else if activeSection === "roles"}
          <!-- WORKSPACE CUSTOM ROLES & PERMISSION MATRIX -->
          <WorkspaceRolesSection
            roles={customRoles}
            loading={loadingRoles}
            {isOwner}
            availableResources={wsAvailableResources}
            onSaveRole={handleSaveWorkspaceRole}
            onDeleteRole={handleDeleteWorkspaceRole}
          />
        {:else if activeSection === "general"}
          <!-- WORKSPACE GENERAL SETTINGS -->
          <Card>
            <form onsubmit={handleSaveWorkspace} class="space-y-6">
              <div class="pb-4 border-b border-[var(--border-subtle)]">
                <h2 class="text-base font-bold text-[var(--text-primary)]">
                  Organization Profile
                </h2>
                <p class="text-xs text-[var(--text-secondary)] mt-0.5">
                  Configure your global Workspace name and URL slug.
                </p>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
                <div class="space-y-1.5">
                  <label for="wsNameInput" class="text-xs font-semibold text-[var(--text-secondary)]">
                    Workspace / Organization Name
                  </label>
                  <input
                    id="wsNameInput"
                    type="text"
                    bind:value={wsName}
                    class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm font-semibold text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                    required
                  />
                </div>

                <div class="space-y-1.5">
                  <label for="wsSlugInput" class="text-xs font-semibold text-[var(--text-secondary)]">
                    Workspace Slug
                  </label>
                  <input
                    id="wsSlugInput"
                    type="text"
                    bind:value={wsSlug}
                    class="w-full px-4 py-2.5 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-sm font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--brand-primary)]"
                    required
                  />
                </div>
              </div>

              <!-- Monthly Event Quota Usage Bar -->
              <div class="p-4 rounded-2xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] space-y-2.5">
                <div class="flex items-center justify-between text-xs">
                  <span class="font-bold text-[var(--text-primary)]">Combined Workspace Monthly Event Quota</span>
                  <span class="font-mono font-bold text-[var(--brand-primary)]">
                    {workspace.usedEventsThisMonth.toLocaleString()} / {workspace.monthlyEventQuota.toLocaleString()} events ({quotaPct.toFixed(1)}%)
                  </span>
                </div>
                <div class="w-full h-2.5 rounded-full bg-[var(--border-subtle)] overflow-hidden">
                  <div
                    class="h-full rounded-full bg-[var(--brand-primary)] transition-all duration-300"
                    style="width: {Math.max(2, quotaPct)}%"
                  ></div>
                </div>
                <p class="text-[11px] text-[var(--text-muted)]">
                  Aggregates ingested events across all {projects.length} project(s) in this Workspace.
                </p>
              </div>

              <div class="flex justify-end">
                <Button type="submit" variant="primary" size="md" loading={savingWorkspace}>
                  <Save class="w-4 h-4" />
                  Save Workspace Profile
                </Button>
              </div>
            </form>
          </Card>
        {/if}
      </div>

      <!-- RIGHT COLUMN (4 cols): Workspace Overview Card & Scope Architecture Guide -->
      <div class="lg:col-span-4 space-y-6">
        <div class="rounded-[24px] bg-gradient-to-br from-[#127852] via-[#106B49] to-[#0C5439] text-white p-6 shadow-[var(--shadow-bento)] space-y-4">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-emerald-100">Active Organization</span>
            <span class="px-2.5 py-0.5 rounded-full text-[10px] font-mono uppercase bg-white/15 text-emerald-100">
              Active
            </span>
          </div>

          <div>
            <h3 class="text-xl font-extrabold tracking-tight">{workspace.name}</h3>
            <p class="text-xs font-mono text-emerald-100/80 mt-0.5">/{workspace.slug}</p>
          </div>

          <div class="grid grid-cols-2 gap-3 pt-2 border-t border-white/15 text-xs">
            <div class="p-3 rounded-2xl bg-white/10">
              <div class="text-[10px] text-emerald-100/75">Total Projects</div>
              <div class="text-lg font-extrabold mt-0.5">{projects.length}</div>
            </div>
            <div class="p-3 rounded-2xl bg-white/10">
              <div class="text-[10px] text-emerald-100/75">Team Members</div>
              <div class="text-lg font-extrabold mt-0.5">{members.length}</div>
            </div>
          </div>
        </div>

        <!-- Settings Hierarchy Guide Card -->
        <Card class="space-y-3.5">
          <div class="flex items-center gap-2">
            <Sparkles class="w-4 h-4 text-[var(--brand-primary)]" />
            <h3 class="text-sm font-bold text-[var(--text-primary)]">3-Tier Settings Architecture</h3>
          </div>
          <div class="space-y-3 text-xs text-[var(--text-secondary)]">
            <div class="p-3 rounded-2xl bg-[var(--brand-soft)]/50 border border-emerald-500/20">
              <div class="font-bold text-[var(--brand-primary)]">1. Workspace Settings (/app/settings)</div>
              <p class="text-[11px] mt-0.5">
                Organization identity and Team & Project RBAC invitations.
              </p>
            </div>
            <div class="p-3 rounded-2xl bg-[var(--bg-surface-muted)]">
              <div class="font-bold text-[var(--text-primary)]">2. Project Settings (/app/projects/[id]/settings)</div>
              <p class="text-[11px] mt-0.5">
                Per-project rename, Multi-API Keys (<code class="font-mono">tlk_live_...</code>), CORS origins, TTL, and event purge.
              </p>
            </div>
            <div class="p-3 rounded-2xl bg-[var(--bg-surface-muted)]">
              <div class="font-bold text-[var(--text-primary)]">3. Personal Profile (/app/profile)</div>
              <p class="text-[11px] mt-0.5">
                Your personal avatar, display name, password, 2FA authenticator, and active devices.
              </p>
            </div>
          </div>
        </Card>
      </div>
    </div>
  {/if}
</div>
