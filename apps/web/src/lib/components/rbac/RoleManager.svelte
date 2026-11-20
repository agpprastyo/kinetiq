<script lang="ts">
  import type { CustomRole, GranularPermission } from "$lib/api";
  import { Shield, Plus, Trash2, Edit3, CheckSquare, Square } from "lucide-svelte";

  interface Props {
    title: string;
    description: string;
    createButtonText: string;
    roles: CustomRole[];
    loading: boolean;
    canManage: boolean;
    availableResources: Array<{ resource: string; label: string; actions: string[] }>;
    isDarkTheme?: boolean;
    onSaveRole: (role: { id?: string; name: string; description?: string; permissions: GranularPermission[] }) => Promise<void>;
    onDeleteRole: (roleId: string, name: string) => Promise<void>;
  }

  let {
    title,
    description,
    createButtonText,
    roles,
    loading,
    canManage,
    availableResources,
    isDarkTheme = false,
    onSaveRole,
    onDeleteRole
  }: Props = $props();

  let modalOpen = $state(false);
  let editingRoleId = $state<string | null>(null);
  let formName = $state("");
  let formDescription = $state("");
  let selectedPermissions = $state<Set<string>>(new Set());
  let saving = $state(false);

  function openCreateModal() {
    editingRoleId = null;
    formName = "";
    formDescription = "";
    selectedPermissions = new Set([
      availableResources[0]?.resource ? `${availableResources[0].resource}:${availableResources[0].actions[0] || 'read'}` : ""
    ]);
    modalOpen = true;
  }

  function openEditModal(role: CustomRole) {
    editingRoleId = role.id;
    formName = role.name;
    formDescription = role.description || "";
    selectedPermissions = new Set((role.permissions || []).map((p) => `${p.resource}:${p.action}`));
    modalOpen = true;
  }

  function togglePermission(key: string) {
    const updated = new Set(selectedPermissions);
    if (updated.has(key)) {
      updated.delete(key);
    } else {
      updated.add(key);
    }
    selectedPermissions = updated;
  }

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (!formName.trim()) return;
    saving = true;
    try {
      const perms: GranularPermission[] = Array.from(selectedPermissions).map((key) => {
        const [resource, action] = key.split(":");
        return { resource, action };
      });
      await onSaveRole({
        id: editingRoleId || undefined,
        name: formName.trim(),
        description: formDescription.trim(),
        permissions: perms,
      });
      modalOpen = false;
    } finally {
      saving = false;
    }
  }
</script>

<div class="space-y-4 font-mono">
  <div class="flex items-center justify-between">
    <div>
      <h2 class="text-sm font-bold {isDarkTheme ? 'text-white' : 'text-[var(--text-primary)]'}">{title}</h2>
      <p class="text-xs {isDarkTheme ? 'text-zinc-500' : 'text-[var(--text-secondary)]'}">{description}</p>
    </div>
    {#if canManage}
      <button
        onclick={openCreateModal}
        class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-bold bg-emerald-500 hover:bg-emerald-400 text-black cursor-pointer shadow-md"
      >
        <Plus class="w-3.5 h-3.5" />
        {createButtonText}
      </button>
    {/if}
  </div>

  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    {#if loading}
      <div class="p-8 rounded-2xl border {isDarkTheme ? 'border-zinc-800 bg-zinc-950 text-zinc-500' : 'border-[var(--border-subtle)] bg-[var(--bg-surface-muted)] text-[var(--text-muted)]'} text-center text-xs col-span-2">
        Loading role policies...
      </div>
    {:else if roles.length === 0}
      <div class="p-8 text-center rounded-2xl border border-dashed {isDarkTheme ? 'border-zinc-800 bg-zinc-950' : 'border-[var(--border-subtle)] bg-[var(--bg-surface-muted)]'} space-y-2 col-span-2">
        <Shield class="w-8 h-8 {isDarkTheme ? 'text-zinc-600' : 'text-[var(--text-muted)]'} mx-auto opacity-50" />
        <div class="text-xs font-semibold {isDarkTheme ? 'text-white' : 'text-[var(--text-primary)]'}">No Roles Configured</div>
      </div>
    {:else}
      {#each roles as r}
        <div class="p-4 rounded-2xl border {isDarkTheme ? 'border-zinc-800 bg-zinc-950' : 'border-[var(--border-subtle)] bg-[var(--bg-surface-muted)]'} space-y-3">
          <div class="flex items-start justify-between gap-2">
            <div>
              <div class="flex items-center gap-2">
                <span class="text-sm font-bold {isDarkTheme ? 'text-white' : 'text-[var(--text-primary)]'}">{r.name}</span>
                {#if r.isSystem}
                  <span class="px-1.5 py-0.5 rounded text-[9px] font-bold uppercase {isDarkTheme ? 'bg-zinc-800 text-zinc-400 border border-zinc-700' : 'bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400'}">
                    System
                  </span>
                {:else}
                  <span class="px-1.5 py-0.5 rounded text-[9px] font-bold uppercase bg-emerald-500/10 text-emerald-400 border border-emerald-500/30">
                    Custom
                  </span>
                {/if}
              </div>
              <p class="text-xs {isDarkTheme ? 'text-zinc-500' : 'text-[var(--text-secondary)]'} mt-0.5">{r.description || "No description provided."}</p>
            </div>
            {#if canManage && !r.isSystem}
              <div class="flex items-center gap-1">
                <button
                  type="button"
                  onclick={() => openEditModal(r)}
                  class="p-1.5 rounded-lg {isDarkTheme ? 'text-zinc-400 hover:text-white hover:bg-zinc-900' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface)]'} cursor-pointer"
                  title="Edit permissions"
                >
                  <Edit3 class="w-3.5 h-3.5" />
                </button>
                <button
                  type="button"
                  onclick={() => onDeleteRole(r.id, r.name)}
                  class="p-1.5 rounded-lg text-rose-500 hover:bg-rose-500/20 cursor-pointer"
                  title="Delete role"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </button>
              </div>
            {/if}
          </div>

          <div class="pt-2 border-t {isDarkTheme ? 'border-zinc-900' : 'border-[var(--border-subtle)]'}">
            <div class="text-[10px] {isDarkTheme ? 'text-zinc-400' : 'text-[var(--text-secondary)]'} uppercase tracking-wider mb-1.5">
              Permissions ({r.permissions?.length || 0})
            </div>
            <div class="flex flex-wrap gap-1.5 max-h-24 overflow-y-auto">
              {#if r.name === 'superadmin'}
                <span class="px-2 py-0.5 rounded text-[10px] bg-rose-500/20 text-rose-300 border border-rose-500/40">
                  Full Root (*:*)
                </span>
              {:else if (r.permissions?.length || 0) === 0}
                <span class="text-[10px] {isDarkTheme ? 'text-zinc-600' : 'text-[var(--text-muted)]'} italic">No permissions assigned</span>
              {:else}
                {#each r.permissions as p}
                  <span class="px-2 py-0.5 rounded text-[10px] {isDarkTheme ? 'bg-zinc-900 text-zinc-300 border border-zinc-800' : 'bg-[var(--bg-surface)] text-[var(--text-secondary)] border border-[var(--border-subtle)]'}">
                    <span class="text-emerald-400 font-semibold">{p.resource}</span>:{p.action}
                  </span>
                {/each}
              {/if}
            </div>
          </div>
        </div>
      {/each}
    {/if}
  </div>
</div>

<!-- Modal Create / Edit Role -->
{#if modalOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm font-mono">
    <div class="max-w-2xl w-full rounded-3xl {isDarkTheme ? 'bg-zinc-950 border border-zinc-800' : 'bg-[var(--bg-surface)] border border-[var(--border-subtle)]'} p-6 space-y-5 max-h-[90vh] overflow-y-auto shadow-2xl">
      <div class="flex items-center justify-between pb-3 border-b {isDarkTheme ? 'border-zinc-900' : 'border-[var(--border-subtle)]'}">
        <h3 class="text-sm font-bold {isDarkTheme ? 'text-white' : 'text-[var(--text-primary)]'}">
          {editingRoleId ? "Edit Role Permissions" : "Create New Role"}
        </h3>
        <button onclick={() => (modalOpen = false)} class="text-xs {isDarkTheme ? 'text-zinc-500 hover:text-white' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'} cursor-pointer">
          ✕
        </button>
      </div>

      <form onsubmit={handleSubmit} class="space-y-4">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div class="space-y-1">
            <label for="role-mgr-name" class="text-xs {isDarkTheme ? 'text-zinc-400' : 'text-[var(--text-secondary)]'} uppercase font-semibold">Role Name</label>
            <input
              id="role-mgr-name"
              type="text"
              required
              disabled={Boolean(editingRoleId)}
              placeholder="e.g. support-lead"
              bind:value={formName}
              class="w-full px-3.5 py-2 rounded-xl text-xs {isDarkTheme ? 'bg-zinc-900 border border-zinc-800 text-white focus:border-emerald-500' : 'bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-[var(--text-primary)] focus:border-[var(--brand-primary)]'} focus:outline-none"
            />
          </div>
          <div class="space-y-1">
            <label for="role-mgr-desc" class="text-xs {isDarkTheme ? 'text-zinc-400' : 'text-[var(--text-secondary)]'} uppercase font-semibold">Description</label>
            <input
              id="role-mgr-desc"
              type="text"
              placeholder="Policy scope description..."
              bind:value={formDescription}
              class="w-full px-3.5 py-2 rounded-xl text-xs {isDarkTheme ? 'bg-zinc-900 border border-zinc-800 text-white focus:border-emerald-500' : 'bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)] text-[var(--text-primary)] focus:border-[var(--brand-primary)]'} focus:outline-none"
            />
          </div>
        </div>

        <div class="space-y-2 pt-2">
          <div class="text-xs {isDarkTheme ? 'text-zinc-400' : 'text-[var(--text-secondary)]'} uppercase tracking-wider font-semibold">Granular CRUD Matrix</div>
          <div class="space-y-2.5 rounded-2xl border {isDarkTheme ? 'border-zinc-800 bg-zinc-900/50' : 'border-[var(--border-subtle)] bg-[var(--bg-surface-muted)]/50'} p-3">
            {#each availableResources as res}
              <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2 p-2.5 rounded-xl {isDarkTheme ? 'bg-zinc-900/80 border border-zinc-800/80' : 'bg-[var(--bg-surface)] border border-[var(--border-subtle)]'}">
                <div>
                  <div class="text-xs font-bold {isDarkTheme ? 'text-white' : 'text-[var(--text-primary)]'}">{res.label}</div>
                  <div class="text-[10px] {isDarkTheme ? 'text-zinc-500' : 'text-[var(--text-secondary)]'}">{res.resource}</div>
                </div>
                <div class="flex flex-wrap items-center gap-1.5">
                  {#each res.actions as act}
                    {@const key = `${res.resource}:${act}`}
                    {@const checked = selectedPermissions.has(key)}
                    <button
                      type="button"
                      onclick={() => togglePermission(key)}
                      class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[10px] font-bold uppercase transition-all cursor-pointer {checked ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/40' : isDarkTheme ? 'bg-zinc-800 text-zinc-500 border border-zinc-700/60' : 'bg-[var(--bg-surface-muted)] text-[var(--text-secondary)] border border-[var(--border-subtle)]'}"
                    >
                      {#if checked}
                        <CheckSquare class="w-3 h-3 text-emerald-400" />
                      {:else}
                        <Square class="w-3 h-3 {isDarkTheme ? 'text-zinc-500' : 'text-[var(--text-muted)]'}" />
                      {/if}
                      {act}
                    </button>
                  {/each}
                </div>
              </div>
            {/each}
          </div>
        </div>

        <div class="flex items-center justify-end gap-2 pt-3 border-t {isDarkTheme ? 'border-zinc-900' : 'border-[var(--border-subtle)]'}">
          <button
            type="button"
            onclick={() => (modalOpen = false)}
            class="px-4 py-2 rounded-xl text-xs {isDarkTheme ? 'text-zinc-400 hover:text-white bg-zinc-900' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] bg-[var(--bg-surface-muted)]'} cursor-pointer"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={saving}
            class="px-4 py-2 rounded-xl text-xs font-bold bg-emerald-500 hover:bg-emerald-400 text-black cursor-pointer disabled:opacity-50"
          >
            {saving ? "Saving Policy..." : "Save Role Policy"}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
