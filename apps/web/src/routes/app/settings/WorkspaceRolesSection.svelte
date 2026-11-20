<script lang="ts">
  import type { CustomRole, GranularPermission } from "$lib/api";
  import RoleManager from "$lib/components/rbac/RoleManager.svelte";
  import Card from "$lib/components/ui/Card.svelte";

  interface Props {
    roles: CustomRole[];
    loading: boolean;
    isOwner: boolean;
    availableResources: Array<{ resource: string; label: string; actions: string[] }>;
    onSaveRole: (role: { id?: string; name: string; description?: string; permissions: GranularPermission[] }) => Promise<void>;
    onDeleteRole: (roleId: string, name: string) => Promise<void>;
  }

  let { roles, loading, isOwner, availableResources, onSaveRole, onDeleteRole }: Props = $props();
</script>

<div class="space-y-6">
  <Card class="space-y-4">
    <RoleManager
      title="Custom Workspace Roles & Permissions"
      description="Define custom security roles with granular Create, Read, Update, and Delete permissions across workspace assets."
      createButtonText="New Custom Role"
      {roles}
      {loading}
      canManage={isOwner}
      {availableResources}
      isDarkTheme={false}
      {onSaveRole}
      {onDeleteRole}
    />
  </Card>
</div>
