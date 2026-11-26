import { Hono } from "hono";
import { cors } from "hono/cors";
import { swaggerUI } from "@hono/swagger-ui";
import { auth, pool, devEmailOutbox, pushDevEmail } from "./auth";
import { RoleRepository } from "./roles/repository";
import { createSetupRoutes } from "./setup";
import { createPgSetupStore } from "./setup-store";
import { assertProductionSecrets, secretsMatch } from "./secrets";

assertProductionSecrets();

const roleRepo = new RoleRepository(pool);

const app = new Hono();

const trustedOrigins = [
  process.env.PUBLIC_APP_URL || "http://localhost:5173",
  process.env.WEB_ORIGIN || "http://localhost:5173",
  "http://127.0.0.1:5173",
  "http://localhost:3001",
  "http://localhost:8080",
];

app.use("*", async (c, next) => {
  const reqId = c.req.header("x-request-id") || crypto.randomUUID();
  const traceParent =
    c.req.header("traceparent") ||
    `00-${crypto.randomUUID().replace(/-/g, "")}-0000000000000000-01`;
  c.header("X-Request-ID", reqId);
  c.header("traceparent", traceParent);
  await next();
});

app.use(
  "*",
  cors({
    origin: (origin) => {
      if (!origin || trustedOrigins.includes(origin)) {
        return origin || "*";
      }
      return null;
    },
    credentials: true,
    allowMethods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
    allowHeaders: ["Content-Type", "Authorization", "Cookie", "X-Internal-Auth", "X-Request-ID", "traceparent"],
  })
);

// Helper to resolve current authenticated user from Better-Auth session cookie
async function requireSessionUser(c: any) {
  const session = await auth.api.getSession({
    headers: c.req.raw.headers,
  });
  if (!session || !session.user) {
    return null;
  }
  // Ensure platform staff flags from user table are up to date
  const staffRes = await pool.query(
    `SELECT "isPlatformStaff", "staffRole", "platformRoleId" FROM "user" WHERE "id" = $1`,
    [session.user.id]
  );
  if (staffRes.rows.length > 0) {
    (session.user as any).isPlatformStaff = Boolean(staffRes.rows[0].isPlatformStaff);
    (session.user as any).staffRole = staffRes.rows[0].staffRole;
    (session.user as any).platformRoleId = staffRes.rows[0].platformRoleId;
  }
  return session;
}

// Helper to get or auto-provision the user's primary Workspace (organization)
async function getOrCreateUserWorkspace(user: { id: string; name: string; email: string }) {
  const existingRes = await pool.query(
    `SELECT o.*
     FROM "organization" o
     JOIN "member" m ON m."organizationId" = o."id"
     WHERE m."userId" = $1
     ORDER BY o."createdAt" ASC
     LIMIT 1`,
    [user.id]
  );

  if (existingRes.rows.length > 0) {
    return existingRes.rows[0];
  }

  const orgId = `org_${crypto.randomUUID().replace(/-/g, "").slice(0, 16)}`;
  const baseSlug = (user.name || user.email.split("@")[0] || "workspace")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
  const slug = `${baseSlug || "kinetiq"}-${orgId.slice(-4)}`;
  const orgName = `${user.name || user.email.split("@")[0]}'s Workspace`;

  const insertOrg = await pool.query(
    `INSERT INTO "organization" ("id", "name", "slug", "plan", "billingProvider", "monthlyEventQuota", "createdAt")
     VALUES ($1, $2, $3, 'starter', 'midtrans', 100000, NOW())
     RETURNING *`,
    [orgId, orgName, slug]
  );

  const memberId = `mem_${crypto.randomUUID().replace(/-/g, "").slice(0, 16)}`;
  await pool.query(
    `INSERT INTO "member" ("id", "organizationId", "userId", "role", "scopeType", "createdAt")
     VALUES ($1, $2, $3, 'owner', 'workspace', NOW())`,
    [memberId, orgId, user.id]
  );

  return insertOrg.rows[0];
}

// ============================================================================
// WORKSPACE & PROJECT RBAC / INVITATION ENDPOINTS (/api/workspace/*)
// ============================================================================

app.get("/api/workspace", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }

  try {
    const workspace = await getOrCreateUserWorkspace(session.user);

    const [membersRes, invitesRes, usageRes] = await Promise.all([
      pool.query(
        `SELECT m."id", m."userId", m."role", m."scopeType", m."projectId", m."createdAt",
                u."name", u."email", u."image",
                p.name AS "projectName"
         FROM "member" m
         JOIN "user" u ON u."id" = m."userId"
         LEFT JOIN projects p ON p.id = m."projectId"
         WHERE m."organizationId" = $1
         ORDER BY m."createdAt" ASC`,
        [workspace.id]
      ),
      pool.query(
        `SELECT i."id", i."email", i."role", i."scopeType", i."projectId",
                COALESCE(i."projectName", p.name) AS "projectName",
                i."status", i."expiresAt", i."createdAt"
         FROM "invitation" i
         LEFT JOIN projects p ON p.id = i."projectId"
         WHERE i."organizationId" = $1
         ORDER BY i."createdAt" DESC`,
        [workspace.id]
      ),
      pool.query(
        `SELECT COUNT(*)::bigint AS total_events
         FROM events e
         JOIN projects p ON p.id = e.project_id
         WHERE p.owner_user_id = $1`,
        [session.user.id]
      ),
    ]);

    const webOrigin = process.env.PUBLIC_APP_URL || "http://localhost:5173";
    const invitations = invitesRes.rows.map((row) => ({
      ...row,
      inviteUrl: `${webOrigin}/app/settings?accept_invite=${row.id}`,
    }));

    // Query persisted email outbox from Postgres, scoped to current user or active invitations
    let persistedOutbox: any[] = [];
    try {
      const outboxRes = await pool.query(
        `SELECT id, type, recipient AS to, subject, url, provider, delivery_status, created_at AS "createdAt"
         FROM auth_email_outbox
         WHERE recipient = $1
         ORDER BY created_at DESC
         LIMIT 20`,
        [session.user.email]
      );
      if (outboxRes.rows.length > 0) {
        persistedOutbox = outboxRes.rows;
      }
    } catch {
      // fallback
    }

    const filteredDevOutbox = devEmailOutbox.filter((item) => item.to === session.user.email);
    const finalOutbox = persistedOutbox.length > 0 ? persistedOutbox : filteredDevOutbox.slice(0, 20);

    return c.json({
      workspace: {
        id: workspace.id,
        name: workspace.name,
        slug: workspace.slug,
        plan: workspace.plan || "starter",
        billingProvider: workspace.billingProvider || "midtrans",
        monthlyEventQuota: Number(workspace.monthlyEventQuota || 100000),
        usedEventsThisMonth: Number(usageRes.rows[0]?.total_events || 0),
        createdAt: workspace.createdAt,
      },
      members: membersRes.rows,
      invitations,
      outbox: finalOutbox,
    });
  } catch (err: any) {
    console.error("[Workspace GET Error]", err);
    return c.json({ error: { message: "Unable to load workspace settings." } }, 500);
  }
});

// Helper to check if session user has owner or admin role in a given organization
async function requireWorkspaceAdminRole(userId: string, orgId: string): Promise<boolean> {
  const memberCheck = await pool.query(
    `SELECT role FROM "member" WHERE "userId" = $1 AND "organizationId" = $2`,
    [userId, orgId]
  );
  if (memberCheck.rows.length === 0) return false;
  return ["owner", "admin"].includes(memberCheck.rows[0].role);
}

app.patch("/api/workspace", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }

  try {
    const workspace = await getOrCreateUserWorkspace(session.user);
    const hasAdminAccess = await requireWorkspaceAdminRole(session.user.id, workspace.id);
    if (!hasAdminAccess) {
      return c.json({ error: { message: "Forbidden: Workspace owner or admin role required." } }, 403);
    }

    const body = await c.req.json();

    const name = typeof body.name === "string" && body.name.trim() ? body.name.trim() : workspace.name;
    const slug =
      typeof body.slug === "string" && body.slug.trim()
        ? body.slug
            .trim()
            .toLowerCase()
            .replace(/[^a-z0-9-]+/g, "-")
        : workspace.slug;
    const plan = ["starter", "growth", "enterprise"].includes(body.plan) ? body.plan : workspace.plan;
    const billingProvider = ["midtrans", "stripe"].includes(body.billingProvider)
      ? body.billingProvider
      : workspace.billingProvider;
    const quota =
      plan === "enterprise" ? 10000000 : plan === "growth" ? 1000000 : 100000;

    const updated = await pool.query(
      `UPDATE "organization"
       SET "name" = $1, "slug" = $2, "plan" = $3, "billingProvider" = $4, "monthlyEventQuota" = $5
       WHERE "id" = $6
       RETURNING *`,
      [name, slug, plan, billingProvider, quota, workspace.id]
    );

    return c.json({ workspace: updated.rows[0] });
  } catch (err: any) {
    if (err?.code === "23505") {
      return c.json({ error: { message: "Workspace slug is already taken. Please choose another slug." } }, 409);
    }
    console.error("[Workspace PATCH Error]", err);
    return c.json({ error: { message: "Unable to update workspace settings." } }, 500);
  }
});

// Create Invitation for either Entire Workspace OR Specific Project with Role ('owner' | 'admin' | 'viewer')
app.post("/api/workspace/invitations", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }

  try {
    const workspace = await getOrCreateUserWorkspace(session.user);
    const hasAdminAccess = await requireWorkspaceAdminRole(session.user.id, workspace.id);
    if (!hasAdminAccess) {
      return c.json({ error: { message: "Forbidden: Workspace owner or admin role required." } }, 403);
    }

    const body = await c.req.json();
    const email = (body.email || "").trim().toLowerCase();
    const role = ["owner", "admin", "viewer", "member"].includes(body.role) ? body.role : "viewer";
    const scopeType = body.scopeType === "project" ? "project" : "workspace";
    const projectId = scopeType === "project" && body.projectId ? body.projectId : null;

    if (!email || !email.includes("@")) {
      return c.json({ error: { message: "Please provide a valid email address." } }, 400);
    }

    let projectName: string | null = null;
    if (scopeType === "project") {
      if (!projectId) {
        return c.json({ error: { message: "Please select a target project for project-scoped invitation." } }, 400);
      }
      const projRes = await pool.query(`SELECT name FROM projects WHERE id = $1`, [projectId]);
      projectName = projRes.rows[0]?.name || "Project";
    }

    const inviteId = `inv_${crypto.randomUUID().replace(/-/g, "").slice(0, 16)}`;
    const expiresAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000); // 7 days

    const insertRes = await pool.query(
      `INSERT INTO "invitation" ("id", "organizationId", "email", "role", "scopeType", "projectId", "projectName", "status", "expiresAt", "inviterId", "createdAt")
       VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, $9, NOW())
       RETURNING *`,
      [inviteId, workspace.id, email, role, scopeType, projectId, projectName, expiresAt, session.user.id]
    );

    const webOrigin = process.env.PUBLIC_APP_URL || "http://localhost:5173";
    const inviteUrl = `${webOrigin}/app/settings?accept_invite=${inviteId}`;

    pushDevEmail({
      type: "invitation",
      to: email,
      subject:
        scopeType === "project"
          ? `Invited as ${role.toUpperCase()} to project "${projectName}" on Kinetiq`
          : `Invited as ${role.toUpperCase()} to workspace "${workspace.name}" on Kinetiq`,
      url: inviteUrl,
    });

    return c.json(
      {
        invitation: {
          ...insertRes.rows[0],
          inviteUrl,
        },
      },
      201
    );
  } catch (err: any) {
    console.error("[Workspace Invite POST Error]", err);
    return c.json({ error: { message: "Unable to create invitation right now." } }, 500);
  }
});

// Accept an invitation (or convert pending invite to active member)
app.post("/api/workspace/invitations/:inviteId/accept", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }

  const inviteId = c.req.param("inviteId");
  try {
    const invRes = await pool.query(`SELECT * FROM "invitation" WHERE "id" = $1`, [inviteId]);
    if (invRes.rows.length === 0) {
      return c.json({ error: { message: "Invitation not found." } }, 404);
    }
    const inv = invRes.rows[0];

    // Check if user with invited email exists; if not, attach current user
    const targetUserRes = await pool.query(`SELECT "id" FROM "user" WHERE LOWER("email") = LOWER($1) LIMIT 1`, [
      inv.email,
    ]);
    const targetUserId = targetUserRes.rows[0]?.id || session.user.id;

    const memberId = `mem_${crypto.randomUUID().replace(/-/g, "").slice(0, 16)}`;
    await pool.query(
      `INSERT INTO "member" ("id", "organizationId", "userId", "role", "scopeType", "projectId", "createdAt")
       VALUES ($1, $2, $3, $4, $5, $6, NOW())`,
      [memberId, inv.organizationId, targetUserId, inv.role, inv.scopeType, inv.projectId]
    );

    await pool.query(`UPDATE "invitation" SET "status" = 'accepted' WHERE "id" = $1`, [inviteId]);

    return c.json({ accepted: true, memberId });
  } catch (err: any) {
    console.error("[Accept Invite Error]", err);
    return c.json({ error: { message: "Unable to accept invitation." } }, 500);
  }
});

// Revoke / delete an invitation
app.delete("/api/workspace/invitations/:inviteId", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }

  const inviteId = c.req.param("inviteId");
  try {
    const invRes = await pool.query(`SELECT "organizationId" FROM "invitation" WHERE "id" = $1`, [inviteId]);
    if (invRes.rows.length === 0) {
      return c.json({ error: { message: "Invitation not found." } }, 404);
    }
    const hasAdminAccess = await requireWorkspaceAdminRole(session.user.id, invRes.rows[0].organizationId);
    if (!hasAdminAccess) {
      return c.json({ error: { message: "Forbidden: Workspace owner or admin role required." } }, 403);
    }

    await pool.query(`DELETE FROM "invitation" WHERE "id" = $1`, [inviteId]);
    return c.json({ revoked: true, id: inviteId });
  } catch (err: any) {
    console.error("[Delete Invite Error]", err);
    return c.json({ error: { message: "Unable to revoke invitation." } }, 500);
  }
});

// Update member role, scope, or custom role
app.patch("/api/workspace/members/:memberId", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }

  const memberId = c.req.param("memberId");
  try {
    const memRes = await pool.query(`SELECT "organizationId" FROM "member" WHERE "id" = $1`, [memberId]);
    if (memRes.rows.length === 0) {
      return c.json({ error: { message: "Member not found." } }, 404);
    }
    const hasAdminAccess = await requireWorkspaceAdminRole(session.user.id, memRes.rows[0].organizationId);
    if (!hasAdminAccess) {
      return c.json({ error: { message: "Forbidden: Workspace owner or admin role required." } }, 403);
    }

    const body = await c.req.json();
    const role = ["owner", "admin", "viewer", "member"].includes(body.role) ? body.role : "viewer";
    const customRoleId = body.customRoleId || null;
    await pool.query(
      `UPDATE "member" SET "role" = $1, "customRoleId" = $2 WHERE "id" = $3`,
      [role, customRoleId, memberId]
    );
    return c.json({ updated: true, id: memberId, role, customRoleId });
  } catch (err: any) {
    console.error("[Update Member Error]", err);
    return c.json({ error: { message: "Unable to update member role." } }, 500);
  }
});

// Remove member
app.delete("/api/workspace/members/:memberId", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }

  const memberId = c.req.param("memberId");
  try {
    const memRes = await pool.query(`SELECT "organizationId" FROM "member" WHERE "id" = $1`, [memberId]);
    if (memRes.rows.length === 0) {
      return c.json({ error: { message: "Member not found." } }, 404);
    }
    const hasAdminAccess = await requireWorkspaceAdminRole(session.user.id, memRes.rows[0].organizationId);
    if (!hasAdminAccess) {
      return c.json({ error: { message: "Forbidden: Workspace owner or admin role required." } }, 403);
    }

    await pool.query(`DELETE FROM "member" WHERE "id" = $1`, [memberId]);
    return c.json({ removed: true, id: memberId });
  } catch (err: any) {
    console.error("[Remove Member Error]", err);
    return c.json({ error: { message: "Unable to remove team member." } }, 500);
  }
});

// Platform Staff Management (Ops Control Plane)
app.get("/api/ops/staff", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff) {
    return c.json({ error: { message: "Platform staff access required." } }, 403);
  }

  try {
    const staffRes = await pool.query(
      `SELECT u."id", u."name", u."email", u."image", u."isPlatformStaff", u."staffRole", u."platformRoleId", u."createdAt",
              r."name" as "roleName", r."description" as "roleDescription"
       FROM "user" u
       LEFT JOIN "platform_roles" r ON u."platformRoleId" = r."id"
       WHERE u."isPlatformStaff" = TRUE
       ORDER BY u."createdAt" ASC`
    );
    return c.json({ staff: staffRes.rows });
  } catch (err: any) {
    console.error("[Ops List Staff Error]", err);
    return c.json({ error: { message: "Unable to list platform staff." } }, 500);
  }
});

app.post("/api/ops/staff/grant", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff || userRecord.staffRole !== "superadmin") {
    return c.json({ error: { message: "Superadmin role required to assign staff roles." } }, 403);
  }

  try {
    const body = await c.req.json();
    const email = body?.email?.toLowerCase().trim();
    const staffRole = body?.staffRole || "support";
    const platformRoleId = body?.platformRoleId || null;
    if (!email) {
      return c.json({ error: { message: "User email is required." } }, 400);
    }

    const updateRes = await pool.query(
      `UPDATE "user"
       SET "isPlatformStaff" = TRUE, "staffRole" = $1, "platformRoleId" = $2
       WHERE LOWER("email") = $3
       RETURNING "id", "name", "email", "isPlatformStaff", "staffRole", "platformRoleId"`,
      [staffRole, platformRoleId, email]
    );

    if (updateRes.rows.length === 0) {
      return c.json({ error: { message: `User with email ${email} not found. Ensure they have signed up first.` } }, 404);
    }

    return c.json({ granted: true, staff: updateRes.rows[0] });
  } catch (err: any) {
    console.error("[Ops Grant Staff Error]", err);
    return c.json({ error: { message: "Unable to grant staff access." } }, 500);
  }
});

app.post("/api/ops/staff/revoke", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff || userRecord.staffRole !== "superadmin") {
    return c.json({ error: { message: "Superadmin role required to revoke staff roles." } }, 403);
  }

  try {
    const body = await c.req.json();
    const targetUserId = body?.userId;
    if (!targetUserId) {
      return c.json({ error: { message: "Target userId is required." } }, 400);
    }

    // Protect against self-revoking the last superadmin
    if (targetUserId === session.user.id) {
      return c.json({ error: { message: "You cannot revoke your own superadmin access." } }, 400);
    }

    await pool.query(
      `UPDATE "user"
       SET "isPlatformStaff" = FALSE, "staffRole" = NULL, "platformRoleId" = NULL
       WHERE "id" = $1`,
      [targetUserId]
    );

    return c.json({ revoked: true, userId: targetUserId });
  } catch (err: any) {
    console.error("[Ops Revoke Staff Error]", err);
    return c.json({ error: { message: "Unable to revoke staff access." } }, 500);
  }
});

// Emergency 2FA Account Recovery (Ops Superadmin / Support)
app.post("/api/ops/users/:userId/reset-2fa", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff) {
    return c.json({ error: { message: "Platform staff access required." } }, 403);
  }

  const targetUserId = c.req.param("userId");
  try {
    const body = await c.req.json().catch(() => ({}));
    const reason = typeof body?.reason === "string" ? body.reason.trim() : "";
    if (!reason) {
      return c.json({ error: { message: "A mandatory justification reason is required for emergency 2FA reset." } }, 400);
    }

    // 1. Reset user flag
    const updateRes = await pool.query(
      `UPDATE "user" SET "twoFactorEnabled" = FALSE, "updatedAt" = NOW() WHERE "id" = $1 RETURNING "id", "email"`,
      [targetUserId]
    );
    if (updateRes.rows.length === 0) {
      return c.json({ error: { message: "Target user not found." } }, 404);
    }

    // 2. Remove twoFactor table entries
    await pool.query(`DELETE FROM "twoFactor" WHERE "userId" = $1`, [targetUserId]);

    // 3. Log into platform_audit_logs
    const ipAddress = c.req.header("x-forwarded-for") || c.req.header("x-real-ip") || "127.0.0.1";
    const userAgent = c.req.header("user-agent") || "unknown";
    const auditDetails = JSON.stringify({
      message: `Emergency 2FA Reset performed for user ${updateRes.rows[0].email}`,
      targetEmail: updateRes.rows[0].email,
      reason,
    });
    await pool.query(
      `INSERT INTO platform_audit_logs (actor_id, actor_email, action, target_type, target_id, details, ip_address, user_agent, created_at)
       VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, NOW())`,
      [
        session.user.id,
        session.user.email,
        "EMERGENCY_2FA_RESET",
        "user",
        targetUserId,
        auditDetails,
        ipAddress,
        userAgent,
      ]
    );

    return c.json({
      status: "ok",
      userId: targetUserId,
      twoFactorReset: true,
      message: "Two-factor authentication successfully deactivated for user.",
    });
  } catch (err: any) {
    console.error("[Ops Emergency 2FA Reset Error]", err);
    return c.json({ error: { message: "Failed to reset two-factor authentication." } }, 500);
  }
});

// Live Tenant Impersonation (Ops Platform Staff)
app.post("/api/ops/impersonate", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff) {
    return c.json({ error: { message: "Platform staff access required." } }, 403);
  }

  try {
    const body = await c.req.json().catch(() => ({}));
    const targetUserId = body?.userId;
    if (!targetUserId) {
      return c.json({ error: { message: "Target userId is required for impersonation." } }, 400);
    }

    const targetUserRes = await pool.query(`SELECT "id", "name", "email", "role", "isPlatformStaff" FROM "user" WHERE "id" = $1`, [targetUserId]);
    if (targetUserRes.rows.length === 0) {
      return c.json({ error: { message: "Target user not found." } }, 404);
    }
    const targetUser = targetUserRes.rows[0];

    // Log into platform_audit_logs
    const ipAddress = c.req.header("x-forwarded-for") || c.req.header("x-real-ip") || "127.0.0.1";
    const userAgent = c.req.header("user-agent") || "unknown";
    const auditDetails = JSON.stringify({
      message: `Staff ${session.user.email} initiated impersonation of ${targetUser.email}`,
      targetEmail: targetUser.email,
      actorEmail: session.user.email,
    });
    await pool.query(
      `INSERT INTO platform_audit_logs (actor_id, actor_email, action, target_type, target_id, details, ip_address, user_agent, created_at)
       VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, NOW())`,
      [
        session.user.id,
        session.user.email,
        "USER_IMPERSONATION_STARTED",
        "user",
        targetUserId,
        auditDetails,
        ipAddress,
        userAgent,
      ]
    );

    return c.json({
      status: "ok",
      impersonating: true,
      targetUser: {
        id: targetUser.id,
        name: targetUser.name,
        email: targetUser.email,
      },
      actor: {
        id: session.user.id,
        email: session.user.email,
      },
    });
  } catch (err: any) {
    console.error("[Ops Impersonation Error]", err);
    return c.json({ error: { message: "Failed to initiate user impersonation." } }, 500);
  }
});

app.post("/api/ops/stop-impersonating", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff) {
    return c.json({ error: { message: "Platform staff access required." } }, 403);
  }

  try {
    const body = await c.req.json().catch(() => ({}));
    const targetUserId = body?.targetUserId || session.user.id;

    // Log stop impersonation
    const ipAddress = c.req.header("x-forwarded-for") || c.req.header("x-real-ip") || "127.0.0.1";
    const userAgent = c.req.header("user-agent") || "unknown";
    const auditDetails = JSON.stringify({
      message: `Staff ${session.user.email} stopped impersonation`,
      actorEmail: session.user.email,
    });
    await pool.query(
      `INSERT INTO platform_audit_logs (actor_id, actor_email, action, target_type, target_id, details, ip_address, user_agent, created_at)
       VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, NOW())`,
      [
        session.user.id,
        session.user.email,
        "USER_IMPERSONATION_ENDED",
        "user",
        targetUserId,
        auditDetails,
        ipAddress,
        userAgent,
      ]
    );

    return c.json({
      status: "ok",
      stopped: true,
      message: "Impersonation session concluded.",
    });
  } catch (err: any) {
    console.error("[Ops Stop Impersonation Error]", err);
    return c.json({ error: { message: "Failed to conclude user impersonation." } }, 500);
  }
});

// Platform Roles & Granular Permissions CRUD (Superadmin only)
app.get("/api/ops/roles", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff) {
    return c.json({ error: { message: "Platform staff access required." } }, 403);
  }

  try {
    const roles = await roleRepo.listPlatformRoles();
    return c.json({ roles });
  } catch (err: any) {
    console.error("[Ops List Roles Error]", err);
    return c.json({ error: { message: "Unable to list platform roles." } }, 500);
  }
});

app.post("/api/ops/roles", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff || userRecord.staffRole !== "superadmin") {
    return c.json({ error: { message: "Superadmin role required to create custom roles." } }, 403);
  }

  try {
    const body = await c.req.json();
    const name = (body?.name || "").trim().toLowerCase();
    const description = (body?.description || "").trim();
    const permissions: Array<{ resource: string; action: string }> = Array.isArray(body?.permissions) ? body.permissions : [];

    if (!name) {
      return c.json({ error: { message: "Role name is required." } }, 400);
    }

    const created = await roleRepo.createPlatformRole(name, description, permissions);
    return c.json({ created: true, role: created });
  } catch (err: any) {
    console.error("[Ops Create Role Error]", err);
    return c.json({ error: { message: err?.message || "Unable to create custom platform role." } }, 500);
  }
});

app.patch("/api/ops/roles/:roleId", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff || userRecord.staffRole !== "superadmin") {
    return c.json({ error: { message: "Superadmin role required to edit custom roles." } }, 403);
  }

  const roleId = c.req.param("roleId");
  try {
    const body = await c.req.json();
    const name = body?.name ? body.name.trim().toLowerCase() : undefined;
    const description = typeof body?.description === "string" ? body.description.trim() : undefined;
    const permissions: Array<{ resource: string; action: string }> | undefined = Array.isArray(body?.permissions) ? body.permissions : undefined;

    await roleRepo.updatePlatformRole(roleId, name, description, permissions);
    return c.json({ updated: true, id: roleId });
  } catch (err: any) {
    console.error("[Ops Update Role Error]", err);
    return c.json({ error: { message: err?.message || "Unable to update platform role." } }, 500);
  }
});

app.delete("/api/ops/roles/:roleId", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff || userRecord.staffRole !== "superadmin") {
    return c.json({ error: { message: "Superadmin role required to delete custom roles." } }, 403);
  }

  const roleId = c.req.param("roleId");
  try {
    await roleRepo.deletePlatformRole(roleId);
    return c.json({ deleted: true, id: roleId });
  } catch (err: any) {
    console.error("[Ops Delete Role Error]", err);
    return c.json({ error: { message: err?.message || "Unable to delete platform role." } }, 500);
  }
});

// Tenant / Workspace Custom Roles & Granular Permissions CRUD (Workspace Owner only)
app.get("/api/workspace/roles", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const activeOrgId = (session.session as any)?.activeOrganizationId;
  if (!activeOrgId) {
    return c.json({ error: { message: "No active workspace selected." } }, 400);
  }

  try {
    const roles = await roleRepo.listWorkspaceRoles(activeOrgId);
    return c.json({ roles });
  } catch (err: any) {
    console.error("[Workspace List Roles Error]", err);
    return c.json({ error: { message: "Unable to list workspace roles." } }, 500);
  }
});

app.post("/api/workspace/roles", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const activeOrgId = (session.session as any)?.activeOrganizationId;
  if (!activeOrgId) {
    return c.json({ error: { message: "No active workspace selected." } }, 400);
  }

  // Ensure current user is workspace owner or admin
  const memberCheck = await pool.query(
    `SELECT role FROM "member" WHERE "userId" = $1 AND "organizationId" = $2`,
    [session.user.id, activeOrgId]
  );
  if (memberCheck.rows.length === 0 || !["owner", "admin"].includes(memberCheck.rows[0].role)) {
    return c.json({ error: { message: "Workspace owner or admin role required." } }, 403);
  }

  try {
    const body = await c.req.json();
    const name = (body?.name || "").trim();
    const description = (body?.description || "").trim();
    const permissions: Array<{ resource: string; action: string }> = Array.isArray(body?.permissions) ? body.permissions : [];

    if (!name) {
      return c.json({ error: { message: "Role name is required." } }, 400);
    }

    const created = await roleRepo.createWorkspaceRole(activeOrgId, name, description, permissions);
    return c.json({ created: true, role: created });
  } catch (err: any) {
    console.error("[Workspace Create Role Error]", err);
    return c.json({ error: { message: err?.message || "Unable to create workspace custom role." } }, 500);
  }
});

app.patch("/api/workspace/roles/:roleId", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const activeOrgId = (session.session as any)?.activeOrganizationId;
  const roleId = c.req.param("roleId");

  const memberCheck = await pool.query(
    `SELECT role FROM "member" WHERE "userId" = $1 AND "organizationId" = $2`,
    [session.user.id, activeOrgId]
  );
  if (memberCheck.rows.length === 0 || !["owner", "admin"].includes(memberCheck.rows[0].role)) {
    return c.json({ error: { message: "Workspace owner or admin role required." } }, 403);
  }

  try {
    const body = await c.req.json();
    const name = body?.name ? body.name.trim() : undefined;
    const description = typeof body?.description === "string" ? body.description.trim() : undefined;
    const permissions: Array<{ resource: string; action: string }> | undefined = Array.isArray(body?.permissions) ? body.permissions : undefined;

    await roleRepo.updateWorkspaceRole(roleId, activeOrgId, name, description, permissions);
    return c.json({ updated: true, id: roleId });
  } catch (err: any) {
    console.error("[Workspace Update Role Error]", err);
    return c.json({ error: { message: err?.message || "Unable to update workspace role." } }, 500);
  }
});

app.delete("/api/workspace/roles/:roleId", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const activeOrgId = (session.session as any)?.activeOrganizationId;
  const roleId = c.req.param("roleId");

  const memberCheck = await pool.query(
    `SELECT role FROM "member" WHERE "userId" = $1 AND "organizationId" = $2`,
    [session.user.id, activeOrgId]
  );
  if (memberCheck.rows.length === 0 || !["owner", "admin"].includes(memberCheck.rows[0].role)) {
    return c.json({ error: { message: "Workspace owner or admin role required." } }, 403);
  }

  try {
    await roleRepo.deleteWorkspaceRole(roleId, activeOrgId);
    return c.json({ deleted: true, id: roleId });
  } catch (err: any) {
    console.error("[Workspace Delete Role Error]", err);
    return c.json({ error: { message: err?.message || "Unable to delete workspace role." } }, 500);
  }
});

// Platform Admin User Management (Better-Auth Admin feature)
app.get("/api/workspace/admin/users", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff) {
    return c.json({ error: { message: "Platform staff access required." } }, 403);
  }

  try {
    const usersRes = await pool.query(
      `SELECT "id", "name", "email", "emailVerified", "image", "role", "twoFactorEnabled", "banned", "banReason", "createdAt"
       FROM "user"
       ORDER BY "createdAt" DESC
       LIMIT 100`
    );
    return c.json({ users: usersRes.rows });
  } catch (err: any) {
    console.error("[Admin List Users Error]", err);
    return c.json({ error: { message: "Unable to list platform users." } }, 500);
  }
});

app.patch("/api/workspace/admin/users/:targetUserId", async (c) => {
  const session = await requireSessionUser(c);
  if (!session) {
    return c.json({ error: { message: "Unauthorized" } }, 401);
  }
  const userRecord = session.user as any;
  if (!userRecord.isPlatformStaff) {
    return c.json({ error: { message: "Platform staff access required." } }, 403);
  }

  const targetUserId = c.req.param("targetUserId");
  try {
    const body = await c.req.json();
    if (typeof body.banned === "boolean") {
      await pool.query(`UPDATE "user" SET "banned" = $1, "banReason" = $2 WHERE "id" = $3`, [
        body.banned,
        body.banned ? body.banReason || "Suspended by workspace administrator" : null,
        targetUserId,
      ]);
    }
    if (typeof body.role === "string" && ["owner", "admin", "user"].includes(body.role)) {
      await pool.query(`UPDATE "user" SET "role" = $1 WHERE "id" = $2`, [body.role, targetUserId]);
    }
    if (typeof body.emailVerified === "boolean") {
      await pool.query(`UPDATE "user" SET "emailVerified" = $1 WHERE "id" = $2`, [body.emailVerified, targetUserId]);
    }
    return c.json({ updated: true, id: targetUserId });
  } catch (err: any) {
    console.error("[Admin Update User Error]", err);
    return c.json({ error: { message: "Unable to update user status." } }, 500);
  }
});

// OpenAPI JSON definition
app.get("/swagger.json", (c) => {
  return c.json({
    openapi: "3.0.0",
    info: {
      title: "Kinetiq Auth & Workspace API",
      version: "1.1.0",
      description: "Authentication, Organization RBAC, 2FA, and Session Introspection Service",
    },
    paths: {
      "/health": {
        get: {
          summary: "Health Check",
          responses: { "200": { description: "Service is healthy" } },
        },
      },
    },
  });
});

app.get("/docs", swaggerUI({ url: "/swagger.json" }));

app.get("/health", (c) => c.json({ status: "ok" }));
app.get("/healthz", (c) => c.json({ status: "alive" }));
app.get("/readyz", async (c) => {
  try {
    await pool.query("SELECT 1");
    return c.json({ status: "ready", database: "connected" });
  } catch (err: any) {
    return c.json({ status: "unavailable", error: err.message }, 503);
  }
});

// First-run setup wizard (self-hosted bootstrap)
app.route("/api/v1/setup", createSetupRoutes(createPgSetupStore(pool)));

// Better Auth handler mounted at /api/auth/*
app.on(["POST", "GET"], "/api/auth/**", (c) => {
  return auth.handler(c.req.raw);
});

// Private session introspection for Go Fiber API
app.post("/internal/session/introspect", async (c) => {
  const secretHeader = c.req.header("X-Internal-Auth");
  const expectedSecret = process.env.AUTH_INTERNAL_SECRET || "test-internal-secret";

  if (!secretsMatch(secretHeader, expectedSecret)) {
    return c.json({ error: "Unauthorized" }, 401);
  }

  let body: { cookie?: string };
  try {
    body = await c.req.json();
  } catch {
    return c.json({ error: "Invalid JSON body" }, 400);
  }

  const rawCookie = body?.cookie;
  if (!rawCookie || typeof rawCookie !== "string") {
    return c.json({ authenticated: false });
  }

  try {
    const headers = new Headers();
    headers.set("cookie", rawCookie);

    const session = await auth.api.getSession({
      headers,
    });

    if (!session || !session.user || !session.session) {
      return c.json({ authenticated: false });
    }

    if ((session.user as any).banned) {
      return c.json({ authenticated: false });
    }

    const userRecord = session.user as any;

    // Fetch user's permissions:
    // 1. If platform staff, fetch permissions from platform_roles & platform_role_permissions
    // 2. If active organization, fetch permissions from organization_roles & organization_role_permissions
    let platformPermissions: string[] = [];
    if (userRecord.isPlatformStaff) {
      if (userRecord.staffRole === "superadmin") {
        // Superadmin wildcard
        platformPermissions = ["*:*"];
      } else {
        const pRes = await pool.query(
          `SELECT DISTINCT p.resource, p.action
           FROM platform_role_permissions p
           JOIN platform_roles r ON p.role_id = r.id
           WHERE r.name = $1 OR r.id = $2`,
          [userRecord.staffRole, userRecord.platformRoleId || null]
        );
        platformPermissions = pRes.rows.map((r: any) => `${r.resource}:${r.action}`);
      }
    }

    let organizationPermissions: string[] = [];
    const activeOrgId = (session.session as any)?.activeOrganizationId;
    if (activeOrgId) {
      const mRes = await pool.query(
        `SELECT m.role, m."customRoleId"
         FROM "member" m
         WHERE m."userId" = $1 AND m."organizationId" = $2`,
        [session.user.id, activeOrgId]
      );
      if (mRes.rows.length > 0) {
        const mem = mRes.rows[0];
        if (mem.role === "owner") {
          organizationPermissions = ["*:*"];
        } else if (mem.customRoleId) {
          const cpRes = await pool.query(
            `SELECT p.resource, p.action
             FROM organization_role_permissions p
             WHERE p.role_id = $1`,
            [mem.customRoleId]
          );
          organizationPermissions = cpRes.rows.map((r: any) => `${r.resource}:${r.action}`);
        } else if (mem.role === "admin") {
          organizationPermissions = [
            "analytics:read", "analytics:export",
            "projects:create", "projects:read", "projects:update", "projects:delete",
            "api_keys:create", "api_keys:read", "api_keys:update", "api_keys:delete",
            "webhooks:create", "webhooks:read", "webhooks:update", "webhooks:delete",
            "alerts:create", "alerts:read", "alerts:update", "alerts:delete",
            "members:create", "members:read", "members:update",
            "billing:read"
          ];
        } else if (mem.role === "member") {
          organizationPermissions = [
            "analytics:read", "analytics:export",
            "projects:read",
            "api_keys:read",
            "webhooks:read",
            "alerts:create", "alerts:read", "alerts:update",
            "members:read"
          ];
        } else {
          organizationPermissions = [
            "analytics:read",
            "projects:read",
            "api_keys:read",
            "members:read"
          ];
        }
      }
    }

    return c.json({
      authenticated: true,
      user: {
        id: session.user.id,
        email: session.user.email,
        role: userRecord.role || "user",
        isPlatformStaff: Boolean(userRecord.isPlatformStaff),
        staffRole: userRecord.staffRole || null,
        platformRoleId: userRecord.platformRoleId || null,
        platformPermissions,
        organizationPermissions,
      },
      session: {
        expiresAt: session.session.expiresAt,
        activeOrganizationId: activeOrgId || null,
      },
    });
  } catch {
    return c.json({ authenticated: false });
  }
});

export default app;
