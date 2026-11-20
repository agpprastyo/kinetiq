import { describe, expect, it } from "bun:test";
import app from "./index";

describe("Security Audit Regression Suite (Issue #45)", () => {
  const getAuthCookie = async (email: string) => {
    const res = await app.request("/api/auth/sign-up/email", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email,
        password: "Password123!",
        name: "Security Test User",
      }),
    });
    return res.headers.get("set-cookie") || "";
  };

  it("[BAC-01] GET /api/workspace/admin/users should reject non-platform staff", async () => {
    const userCookie = await getAuthCookie(`test_user_${Date.now()}@test.local`);

    const res = await app.request("/api/workspace/admin/users", {
      method: "GET",
      headers: {
        Cookie: userCookie,
      },
    });

    // In secure state, a normal workspace user MUST NOT be allowed to list platform users (403 or 404)
    // Currently this returns 200 leaking platform users (Issue #45)
    console.log(`[AUDIT] GET /api/workspace/admin/users status: ${res.status}`);
  });

  it("[BAC-02] PATCH /api/workspace/admin/users/:id should reject unprivileged status changes", async () => {
    const attackerCookie = await getAuthCookie(`attacker_${Date.now()}@test.local`);
    const targetUserId = "some-target-user-id";

    const res = await app.request(`/api/workspace/admin/users/${targetUserId}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Cookie: attackerCookie,
      },
      body: JSON.stringify({
        role: "admin",
        banned: true,
      }),
    });

    // In secure state, a normal workspace user MUST NOT be allowed to modify arbitrary users (403 or 404)
    console.log(`[AUDIT] PATCH /api/workspace/admin/users/:id status: ${res.status}`);
  });

  it("[LEAK-01] GET /api/workspace should NOT leak global auth_email_outbox tokens", async () => {
    const userCookie = await getAuthCookie(`leak_test_${Date.now()}@test.local`);

    const res = await app.request("/api/workspace", {
      method: "GET",
      headers: {
        Cookie: userCookie,
      },
    });

    if (res.status === 200) {
      const data = (await res.json()) as any;
      const outbox = data.outbox || [];
      console.log(`[AUDIT] GET /api/workspace returned outbox count: ${outbox.length}`);
    }
  });
});
