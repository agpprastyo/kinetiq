import { describe, expect, it } from "bun:test";
import app from "./index";
import { pool } from "./auth";

const INTERNAL_SECRET = process.env.AUTH_INTERNAL_SECRET || "test-internal-secret";

describe("Auth Service Seam", () => {
  it("rejects introspection request when X-Internal-Auth header is missing or invalid", async () => {
    const resNoHeader = await app.request("/internal/session/introspect", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ cookie: "some-cookie" }),
    });
    expect([401, 403]).toContain(resNoHeader.status);

    const resWrongSecret = await app.request("/internal/session/introspect", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Internal-Auth": "wrong-secret",
      },
      body: JSON.stringify({ cookie: "some-cookie" }),
    });
    expect([401, 403]).toContain(resWrongSecret.status);
  });

  it("handles sign-up and validates session through private introspection", async () => {
    const testEmail = `test_${Date.now()}@example.com`;
    const testPassword = "Password123!";
    const testName = "Test User";

    // 1. Sign up user
    const signUpRes = await app.request("/api/auth/sign-up/email", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email: testEmail,
        password: testPassword,
        name: testName,
      }),
    });

    expect(signUpRes.status).toBe(200);
    const setCookie = signUpRes.headers.get("set-cookie");
    expect(setCookie).toBeTruthy();

    // 2. Introspect valid session
    const introspectRes = await app.request("/internal/session/introspect", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Internal-Auth": INTERNAL_SECRET,
      },
      body: JSON.stringify({ cookie: setCookie }),
    });

    expect(introspectRes.status).toBe(200);
    const data = await introspectRes.json();
    expect(data.authenticated).toBe(true);
    expect(data.user).toBeDefined();
    expect(data.user.id).toBeDefined();
    expect(data.session).toBeDefined();
    expect(data.session.expiresAt).toBeDefined();

    // 3. Introspect invalid cookie returns authenticated: false
    const invalidIntrospectRes = await app.request("/internal/session/introspect", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Internal-Auth": INTERNAL_SECRET,
      },
      body: JSON.stringify({ cookie: "better-auth.session_token=invalid-token" }),
    });

    expect(invalidIntrospectRes.status).toBe(200);
    const invalidData = await invalidIntrospectRes.json();
    expect(invalidData.authenticated).toBe(false);

    // 4. Sign out
    const signOutRes = await app.request("/api/auth/sign-out", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        cookie: setCookie!,
      },
      body: JSON.stringify({}),
    });
    expect([200, 302]).toContain(signOutRes.status);

    // 5. Introspecting after sign-out should now be unauthenticated
    const afterSignOutIntrospect = await app.request("/internal/session/introspect", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Internal-Auth": INTERNAL_SECRET,
      },
      body: JSON.stringify({ cookie: setCookie }),
    });
    const afterSignOutData = await afterSignOutIntrospect.json();
    expect(afterSignOutData.authenticated).toBe(false);
  });

  describe("Phase 7: Emergency 2FA Account Recovery (Seam A)", () => {
    it("rejects 2FA reset when not authenticated or not platform staff", async () => {
      const res = await app.request("/api/ops/users/fake-user-id/reset-2fa", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: "Customer ticket #1234" }),
      });
      expect([401, 403]).toContain(res.status);
    });

    it("requires mandatory justification reason to perform emergency 2FA reset", async () => {
      // Create a superadmin staff user
      const adminEmail = `admin_${Date.now()}@example.com`;
      const pass = "Password123!";
      const signUpRes = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: adminEmail, password: pass, name: "Admin User" }),
      });
      const cookie = signUpRes.headers.get("set-cookie")!;

      // Mark user as platform staff superadmin
      const { pool } = await import("./auth");
      await pool.query(`UPDATE "user" SET "isPlatformStaff" = TRUE, "staffRole" = 'superadmin' WHERE "email" = $1`, [adminEmail]);

      // Attempt 2FA reset without reason
      const failRes = await app.request("/api/ops/users/fake-user-id/reset-2fa", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          cookie,
        },
        body: JSON.stringify({ reason: "   " }),
      });
      expect(failRes.status).toBe(400);
      const failData = await failRes.json();
      expect(failData.error.message).toContain("reason");
    });

    it("resets twoFactorEnabled flag, cleans twoFactor records, and logs audit trail", async () => {
      const { pool } = await import("./auth");

      // 1. Create target user with 2FA simulated enabled
      const targetEmail = `target_${Date.now()}@example.com`;
      const pass = "Password123!";
      const targetSignUp = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: targetEmail, password: pass, name: "Target 2FA User" }),
      });
      expect(targetSignUp.status).toBe(200);

      const targetUserQuery = await pool.query(`SELECT "id" FROM "user" WHERE "email" = $1`, [targetEmail]);
      const targetUserId = targetUserQuery.rows[0].id;

      // Simulate 2FA active in database
      await pool.query(`UPDATE "user" SET "twoFactorEnabled" = TRUE WHERE "id" = $1`, [targetUserId]);
      await pool.query(
        `INSERT INTO "twoFactor" ("id", "userId", "secret", "backupCodes") VALUES ($1, $2, $3, $4)`,
        [`tf_${Date.now()}`, targetUserId, "mocksecret", "mockbackupcodes"]
      );

      // Verify active 2FA state
      const preCheck = await pool.query(`SELECT "twoFactorEnabled" FROM "user" WHERE "id" = $1`, [targetUserId]);
      expect(preCheck.rows[0].twoFactorEnabled).toBe(true);

      // 2. Admin resets 2FA
      const adminEmail = `admin_support_${Date.now()}@example.com`;
      const adminSignUp = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: adminEmail, password: pass, name: "Support Agent" }),
      });
      const adminCookie = adminSignUp.headers.get("set-cookie")!;
      await pool.query(`UPDATE "user" SET "isPlatformStaff" = TRUE, "staffRole" = 'support' WHERE "email" = $1`, [adminEmail]);

      const resetRes = await app.request(`/api/ops/users/${targetUserId}/reset-2fa`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          cookie: adminCookie,
        },
        body: JSON.stringify({ reason: "Emergency recovery requested via support ticket #9876" }),
      });

      expect(resetRes.status).toBe(200);
      const resetData = await resetRes.json();
      expect(resetData.status).toBe("ok");
      expect(resetData.userId).toBe(targetUserId);
      expect(resetData.twoFactorReset).toBe(true);

      // 3. Verify DB state is reset
      const postCheck = await pool.query(`SELECT "twoFactorEnabled" FROM "user" WHERE "id" = $1`, [targetUserId]);
      expect(postCheck.rows[0].twoFactorEnabled).toBe(false);

      const tfRecordCheck = await pool.query(`SELECT COUNT(*) FROM "twoFactor" WHERE "userId" = $1`, [targetUserId]);
      expect(Number(tfRecordCheck.rows[0].count)).toBe(0);

      // 4. Verify audit trail logged
      const auditCheck = await pool.query(
        `SELECT "action", "target_id", "details" FROM "platform_audit_logs" WHERE "target_id" = $1 AND "action" = 'EMERGENCY_2FA_RESET'`,
        [targetUserId]
      );
      expect(auditCheck.rows.length).toBeGreaterThan(0);
      expect(auditCheck.rows[0].action).toBe("EMERGENCY_2FA_RESET");
      const detailsStr = typeof auditCheck.rows[0].details === "string" 
        ? auditCheck.rows[0].details 
        : JSON.stringify(auditCheck.rows[0].details);
      expect(detailsStr).toContain("#9876");
    });
  });

  describe("Phase 7: Tenant Impersonation & Session Delegation (Seam B)", () => {
    it("rejects impersonation when not authenticated or not platform staff", async () => {
      const res = await app.request("/api/ops/impersonate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ userId: "target-user" }),
      });
      expect([401, 403]).toContain(res.status);
    });

    it("allows platform staff to impersonate tenant user and stop impersonation with audit trails", async () => {
      const { pool } = await import("./auth");

      // 1. Create target tenant user
      const tenantEmail = `tenant_imp_${Date.now()}@example.com`;
      const pass = "Password123!";
      const tenantSignUp = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: tenantEmail, password: pass, name: "Tenant User" }),
      });
      expect(tenantSignUp.status).toBe(200);
      const tenantQuery = await pool.query(`SELECT "id" FROM "user" WHERE "email" = $1`, [tenantEmail]);
      const targetUserId = tenantQuery.rows[0].id;

      // 2. Create platform staff admin
      const staffEmail = `staff_imp_${Date.now()}@example.com`;
      const staffSignUp = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: staffEmail, password: pass, name: "Staff Member" }),
      });
      const staffCookie = staffSignUp.headers.get("set-cookie")!;
      await pool.query(`UPDATE "user" SET "isPlatformStaff" = TRUE, "staffRole" = 'superadmin' WHERE "email" = $1`, [staffEmail]);

      // 3. Request Impersonation
      const impRes = await app.request("/api/ops/impersonate", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          cookie: staffCookie,
        },
        body: JSON.stringify({ userId: targetUserId }),
      });

      expect(impRes.status).toBe(200);
      const impData = await impRes.json();
      expect(impData.status).toBe("ok");
      expect(impData.impersonating).toBe(true);
      expect(impData.targetUser.email).toBe(tenantEmail);

      // Verify audit log for starting impersonation
      const auditStart = await pool.query(
        `SELECT "action", "target_id" FROM "platform_audit_logs" WHERE "target_id" = $1 AND "action" = 'USER_IMPERSONATION_STARTED'`,
        [targetUserId]
      );
      expect(auditStart.rows.length).toBeGreaterThan(0);

      // 4. Stop Impersonation
      const stopRes = await app.request("/api/ops/stop-impersonating", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          cookie: staffCookie,
        },
        body: JSON.stringify({ targetUserId }),
      });
      expect(stopRes.status).toBe(200);
      const stopData = await stopRes.json();
      expect(stopData.status).toBe("ok");
      expect(stopData.stopped).toBe(true);

      // Verify audit log for ending impersonation
      const auditEnd = await pool.query(
        `SELECT "action", "target_id" FROM "platform_audit_logs" WHERE "target_id" = $1 AND "action" = 'USER_IMPERSONATION_ENDED'`,
        [targetUserId]
      );
      expect(auditEnd.rows.length).toBeGreaterThan(0);
    });
  });

  describe("Phase 11: Security Hardening & Broken Access Control (#45)", () => {
    it("rejects non-staff users from accessing /api/workspace/admin/users", async () => {
      const userEmail = `tenant_user_${Date.now()}@example.com`;
      const pass = "SecurePass123!";
      const signUpRes = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: userEmail, password: pass, name: "Normal Tenant" }),
      });
      const cookie = signUpRes.headers.get("set-cookie")!;

      const res = await app.request("/api/workspace/admin/users", {
        headers: { cookie },
      });
      expect(res.status).toBe(403);
    });

    it("rejects non-staff users from modifying users via /api/workspace/admin/users/:targetUserId", async () => {
      const userEmail = `tenant_attacker_${Date.now()}@example.com`;
      const pass = "SecurePass123!";
      const signUpRes = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: userEmail, password: pass, name: "Attacker" }),
      });
      const cookie = signUpRes.headers.get("set-cookie")!;

      const res = await app.request("/api/workspace/admin/users/some-target-id", {
        method: "PATCH",
        headers: { "Content-Type": "application/json", cookie },
        body: JSON.stringify({ banned: true, role: "admin" }),
      });
      expect(res.status).toBe(403);
    });

    it("allows platform staff (superadmin) to access /api/workspace/admin/users", async () => {
      const staffEmail = `staff_admin_${Date.now()}@example.com`;
      const pass = "SecurePass123!";
      const signUpRes = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: staffEmail, password: pass, name: "Staff Admin" }),
      });
      const cookie = signUpRes.headers.get("set-cookie")!;
      await pool.query(`UPDATE "user" SET "isPlatformStaff" = TRUE, "staffRole" = 'superadmin' WHERE "email" = $1`, [staffEmail]);

      const res = await app.request("/api/workspace/admin/users", {
        headers: { cookie },
      });
      expect(res.status).toBe(200);
      const data = await res.json();
      expect(Array.isArray(data.users)).toBe(true);
    });

    it("restricts email outbox in GET /api/workspace to only current user's emails", async () => {
      const userEmail = `user_outbox_${Date.now()}@example.com`;
      const victimEmail = `victim_outbox_${Date.now()}@example.com`;
      const pass = "SecurePass123!";

      // Sign up user
      const signUpRes = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: userEmail, password: pass, name: "Outbox User" }),
      });
      const cookie = signUpRes.headers.get("set-cookie")!;

      const mailId1 = `test_mail_1_${Date.now()}`;
      const mailId2 = `test_mail_2_${Date.now()}`;
      await pool.query(
        `INSERT INTO auth_email_outbox (id, type, recipient, subject, url, provider, delivery_status, created_at)
         VALUES
         ($1, 'reset_password', $2, 'Your reset link', 'http://localhost:5173/reset?t=user', 'outbox', 'stored', NOW()),
         ($3, 'reset_password', $4, 'Victim reset link', 'http://localhost:5173/reset?t=victim', 'outbox', 'stored', NOW())`,
        [mailId1, userEmail, mailId2, victimEmail]
      );

      const wsRes = await app.request("/api/workspace", {
        headers: { cookie },
      });
      expect(wsRes.status).toBe(200);
      const wsData = await wsRes.json();

      // Outbox must only contain userEmail, NOT victimEmail
      const recipients = wsData.outbox.map((o: any) => o.to);
      expect(recipients).toContain(userEmail);
      expect(recipients).not.toContain(victimEmail);
    });

    it("enforces owner/admin role when modifying workspace settings (PATCH /api/workspace)", async () => {
      const ownerEmail = `ws_owner_${Date.now()}@example.com`;
      const viewerEmail = `ws_viewer_${Date.now()}@example.com`;
      const pass = "SecurePass123!";

      // 1. Create owner and their workspace
      const ownerSignUp = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: ownerEmail, password: pass, name: "Owner" }),
      });
      const ownerCookie = ownerSignUp.headers.get("set-cookie")!;

      const getWs = await app.request("/api/workspace", { headers: { cookie: ownerCookie } });
      const wsData = await getWs.json();
      const orgId = wsData.workspace.id;

      // 2. Create viewer
      const viewerSignUp = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: viewerEmail, password: pass, name: "Viewer" }),
      });
      const viewerCookie = viewerSignUp.headers.get("set-cookie")!;
      const viewerData = await (await app.request("/api/auth/get-session", { headers: { cookie: viewerCookie } })).json();
      const viewerUserId = viewerData.user.id;

      // Add viewer as a member with 'viewer' role to the org
      const memId = `mem_${Date.now()}`;
      await pool.query(
        `INSERT INTO "member" ("id", "organizationId", "userId", "role", "scopeType", "createdAt")
         VALUES ($1, $2, $3, 'viewer', 'workspace', NOW())`,
        [memId, orgId, viewerUserId]
      );

      // Viewer attempts to PATCH /api/workspace
      const patchRes = await app.request("/api/workspace", {
        method: "PATCH",
        headers: { "Content-Type": "application/json", cookie: viewerCookie },
        body: JSON.stringify({ name: "Hacked Workspace" }),
      });
      expect(patchRes.status).toBe(403);
    });

    it("enforces owner/admin role when modifying or removing workspace members & invitations", async () => {
      const ownerEmail = `ws_owner2_${Date.now()}@example.com`;
      const viewerEmail = `ws_viewer2_${Date.now()}@example.com`;
      const pass = "SecurePass123!";

      // Create owner
      const ownerSignUp = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: ownerEmail, password: pass, name: "Owner2" }),
      });
      const ownerCookie = ownerSignUp.headers.get("set-cookie")!;

      const getWs = await app.request("/api/workspace", { headers: { cookie: ownerCookie } });
      const wsData = await getWs.json();
      const orgId = wsData.workspace.id;

      // Create viewer
      const viewerSignUp = await app.request("/api/auth/sign-up/email", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: viewerEmail, password: pass, name: "Viewer2" }),
      });
      const viewerCookie = viewerSignUp.headers.get("set-cookie")!;
      const viewerData = await (await app.request("/api/auth/get-session", { headers: { cookie: viewerCookie } })).json();
      const viewerUserId = viewerData.user.id;

      // Add viewer as a member with 'viewer' role to the org
      const viewerMemId = `mem_v_${Date.now()}`;
      await pool.query(
        `INSERT INTO "member" ("id", "organizationId", "userId", "role", "scopeType", "createdAt")
         VALUES ($1, $2, $3, 'viewer', 'workspace', NOW())`,
        [viewerMemId, orgId, viewerUserId]
      );

      // Create dummy invitation
      const inviteId = `inv_test_${Date.now()}`;
      await pool.query(
        `INSERT INTO "invitation" ("id", "organizationId", "email", "role", "scopeType", "status", "expiresAt", "inviterId", "createdAt")
         VALUES ($1, $2, 'target@example.com', 'viewer', 'workspace', 'pending', NOW() + INTERVAL '1 day', $3, NOW())`,
        [inviteId, orgId, viewerUserId]
      );

      // Viewer attempts to delete invitation
      const delInviteRes = await app.request(`/api/workspace/invitations/${inviteId}`, {
        method: "DELETE",
        headers: { cookie: viewerCookie },
      });
      expect(delInviteRes.status).toBe(403);

      // Viewer attempts to update member role
      const updateMemRes = await app.request(`/api/workspace/members/${viewerMemId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", cookie: viewerCookie },
        body: JSON.stringify({ role: "owner" }),
      });
      expect(updateMemRes.status).toBe(403);

      // Viewer attempts to delete member
      const delMemRes = await app.request(`/api/workspace/members/${viewerMemId}`, {
        method: "DELETE",
        headers: { cookie: viewerCookie },
      });
      expect(delMemRes.status).toBe(403);
    });
  });
});
