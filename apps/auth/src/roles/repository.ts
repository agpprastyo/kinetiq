import type { Pool, PoolClient } from "pg";

export interface PermissionItem {
  resource: string;
  action: string;
}

export interface CustomRoleRow {
  id: string;
  organizationId?: string;
  name: string;
  description: string | null;
  isSystem: boolean;
  createdAt: string;
  permissions: PermissionItem[];
}

interface RoleTableConfig {
  roleTable: "platform_roles" | "organization_roles";
  permTable: "platform_role_permissions" | "organization_role_permissions";
  scopeColumn?: "organization_id";
}

const PLATFORM_CONFIG: RoleTableConfig = {
  roleTable: "platform_roles",
  permTable: "platform_role_permissions",
};

const WORKSPACE_CONFIG: RoleTableConfig = {
  roleTable: "organization_roles",
  permTable: "organization_role_permissions",
  scopeColumn: "organization_id",
};

export class RoleRepository {
  constructor(private pool: Pool) {}

  private async fetchRoles(cfg: RoleTableConfig, scopeId?: string): Promise<CustomRoleRow[]> {
    const orgSelect = cfg.scopeColumn ? `r.organization_id as "organizationId",` : "";
    const whereClause = cfg.scopeColumn ? `WHERE r.${cfg.scopeColumn} = $1` : "";
    const orderBy = cfg.scopeColumn ? `ORDER BY r.created_at ASC` : `ORDER BY r.is_system DESC, r.name ASC`;
    const params = scopeId ? [scopeId] : [];

    const query = `
      SELECT r.id, ${orgSelect} r.name, r.description, r.is_system as "isSystem", r.created_at as "createdAt",
             COALESCE(
               json_agg(json_build_object('id', p.id, 'resource', p.resource, 'action', p.action))
               FILTER (WHERE p.id IS NOT NULL), '[]'
             ) as permissions
      FROM ${cfg.roleTable} r
      LEFT JOIN ${cfg.permTable} p ON r.id = p.role_id
      ${whereClause}
      GROUP BY r.id
      ${orderBy}
    `;

    const res = await this.pool.query(query, params);
    return res.rows;
  }

  private async insertRoleWithPermissions(
    cfg: RoleTableConfig,
    name: string,
    description: string,
    permissions: PermissionItem[],
    scopeId?: string
  ): Promise<CustomRoleRow> {
    const client = await this.pool.connect();
    try {
      await client.query("BEGIN");
      let insertRes;
      if (cfg.scopeColumn && scopeId) {
        insertRes = await client.query(
          `INSERT INTO ${cfg.roleTable} (${cfg.scopeColumn}, name, description, is_system)
           VALUES ($1, $2, $3, FALSE)
           RETURNING id, ${cfg.scopeColumn} as "organizationId", name, description, is_system as "isSystem", created_at as "createdAt"`,
          [scopeId, name, description]
        );
      } else {
        insertRes = await client.query(
          `INSERT INTO ${cfg.roleTable} (name, description, is_system)
           VALUES ($1, $2, FALSE)
           RETURNING id, name, description, is_system as "isSystem", created_at as "createdAt"`,
          [name, description]
        );
      }
      const newRole = insertRes.rows[0];

      await this.savePermissions(client, cfg.permTable, newRole.id, permissions);
      await client.query("COMMIT");
      return { ...newRole, permissions };
    } catch (err) {
      await client.query("ROLLBACK");
      throw err;
    } finally {
      client.release();
    }
  }

  private async modifyRole(
    cfg: RoleTableConfig,
    roleId: string,
    name?: string,
    description?: string,
    permissions?: PermissionItem[],
    scopeId?: string
  ): Promise<boolean> {
    const checkQuery = cfg.scopeColumn
      ? `SELECT is_system FROM ${cfg.roleTable} WHERE id = $1 AND ${cfg.scopeColumn} = $2`
      : `SELECT is_system FROM ${cfg.roleTable} WHERE id = $1`;
    const checkParams = cfg.scopeColumn ? [roleId, scopeId] : [roleId];

    const checkRes = await this.pool.query(checkQuery, checkParams);
    if (checkRes.rows.length === 0) {
      throw new Error(cfg.scopeColumn ? "Role not found in this workspace" : "Role not found");
    }
    if (checkRes.rows[0].is_system) {
      throw new Error("System roles cannot be modified");
    }

    const client = await this.pool.connect();
    try {
      await client.query("BEGIN");
      if (name !== undefined || description !== undefined) {
        if (cfg.scopeColumn) {
          await client.query(
            `UPDATE ${cfg.roleTable}
             SET name = COALESCE($1, name), description = COALESCE($2, description), updated_at = NOW()
             WHERE id = $3 AND ${cfg.scopeColumn} = $4`,
            [name, description, roleId, scopeId]
          );
        } else {
          await client.query(
            `UPDATE ${cfg.roleTable}
             SET name = COALESCE($1, name), description = COALESCE($2, description), updated_at = NOW()
             WHERE id = $3`,
            [name, description, roleId]
          );
        }
      }

      if (permissions !== undefined) {
        await client.query(`DELETE FROM ${cfg.permTable} WHERE role_id = $1`, [roleId]);
        await this.savePermissions(client, cfg.permTable, roleId, permissions);
      }

      await client.query("COMMIT");
      return true;
    } catch (err) {
      await client.query("ROLLBACK");
      throw err;
    } finally {
      client.release();
    }
  }

  private async removeRole(cfg: RoleTableConfig, roleId: string, scopeId?: string): Promise<boolean> {
    const checkQuery = cfg.scopeColumn
      ? `SELECT is_system FROM ${cfg.roleTable} WHERE id = $1 AND ${cfg.scopeColumn} = $2`
      : `SELECT is_system FROM ${cfg.roleTable} WHERE id = $1`;
    const checkParams = cfg.scopeColumn ? [roleId, scopeId] : [roleId];

    const checkRes = await this.pool.query(checkQuery, checkParams);
    if (checkRes.rows.length === 0) {
      throw new Error(cfg.scopeColumn ? "Role not found in this workspace" : "Role not found");
    }
    if (checkRes.rows[0].is_system) {
      throw new Error("System roles cannot be deleted");
    }

    if (cfg.scopeColumn) {
      await this.pool.query(`DELETE FROM ${cfg.roleTable} WHERE id = $1 AND ${cfg.scopeColumn} = $2`, [roleId, scopeId]);
    } else {
      await this.pool.query(`DELETE FROM ${cfg.roleTable} WHERE id = $1`, [roleId]);
    }
    return true;
  }

  private async savePermissions(client: PoolClient, permTable: string, roleId: string, permissions: PermissionItem[]): Promise<void> {
    for (const p of permissions) {
      if (p.resource && p.action) {
        await client.query(
          `INSERT INTO ${permTable} (role_id, resource, action)
           VALUES ($1, $2, $3)
           ON CONFLICT DO NOTHING`,
          [roleId, p.resource.trim().toLowerCase(), p.action.trim().toLowerCase()]
        );
      }
    }
  }

  // --- Public Platform Role Methods ---
  async listPlatformRoles(): Promise<CustomRoleRow[]> {
    return this.fetchRoles(PLATFORM_CONFIG);
  }

  async createPlatformRole(name: string, description: string, permissions: PermissionItem[]): Promise<CustomRoleRow> {
    return this.insertRoleWithPermissions(PLATFORM_CONFIG, name, description, permissions);
  }

  async updatePlatformRole(roleId: string, name?: string, description?: string, permissions?: PermissionItem[]): Promise<boolean> {
    return this.modifyRole(PLATFORM_CONFIG, roleId, name, description, permissions);
  }

  async deletePlatformRole(roleId: string): Promise<boolean> {
    return this.removeRole(PLATFORM_CONFIG, roleId);
  }

  // --- Public Workspace Role Methods ---
  async listWorkspaceRoles(orgId: string): Promise<CustomRoleRow[]> {
    return this.fetchRoles(WORKSPACE_CONFIG, orgId);
  }

  async createWorkspaceRole(orgId: string, name: string, description: string, permissions: PermissionItem[]): Promise<CustomRoleRow> {
    return this.insertRoleWithPermissions(WORKSPACE_CONFIG, name, description, permissions, orgId);
  }

  async updateWorkspaceRole(roleId: string, orgId: string, name?: string, description?: string, permissions?: PermissionItem[]): Promise<boolean> {
    return this.modifyRole(WORKSPACE_CONFIG, roleId, name, description, permissions, orgId);
  }

  async deleteWorkspaceRole(roleId: string, orgId: string): Promise<boolean> {
    return this.removeRole(WORKSPACE_CONFIG, roleId, orgId);
  }
}
