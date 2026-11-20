import type { Pool } from "pg";
import { auth } from "./auth";
import { AlreadyInitializedError, type SetupInput, type SetupStore } from "./setup";

const SETUP_LOCK_KEY = 144144;

// ponytail: "initialized" == any user row exists (no extra flag table). Safe for existing
// instances (never re-opens); add a system_settings row if a explicit flag is ever needed.
export function createPgSetupStore(pool: Pool): SetupStore {
  const hasUsers = async (q: Pick<Pool, "query">) =>
    ((await q.query(`SELECT EXISTS (SELECT 1 FROM "user") AS e`)).rows[0].e as boolean);

  return {
    isInitialized: () => hasUsers(pool),

    async initialize(input: SetupInput) {
      const client = await pool.connect();
      try {
        // Session-level advisory lock serializes concurrent setup attempts.
        await client.query(`SELECT pg_advisory_lock($1)`, [SETUP_LOCK_KEY]);
        if (await hasUsers(client)) throw new AlreadyInitializedError();

        const { headers, response } = await auth.api.signUpEmail({
          body: { name: input.name, email: input.email, password: input.password },
          returnHeaders: true,
        });
        const userId = response.user.id;

        try {
          await client.query(
            `UPDATE "user" SET "isPlatformStaff" = TRUE, "staffRole" = 'superadmin', "emailVerified" = TRUE WHERE "id" = $1`,
            [userId]
          );
          const orgId = `org_${crypto.randomUUID().replace(/-/g, "").slice(0, 16)}`;
          const base = input.workspaceName.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
          await client.query(
            `INSERT INTO "organization" ("id", "name", "slug", "plan", "billingProvider", "monthlyEventQuota", "metadata", "createdAt")
             VALUES ($1, $2, $3, 'enterprise', 'midtrans', 100000, $4, NOW())`,
            [orgId, input.workspaceName, `${base || "workspace"}-${orgId.slice(-4)}`, JSON.stringify({ appUrl: input.appUrl, timezone: input.timezone })]
          );
          await client.query(
            `INSERT INTO "member" ("id", "organizationId", "userId", "role", "scopeType", "createdAt")
             VALUES ($1, $2, $3, 'owner', 'workspace', NOW())`,
            [`mem_${crypto.randomUUID().replace(/-/g, "").slice(0, 16)}`, orgId, userId]
          );
        } catch (err) {
          // Roll back the half-created admin so setup can be retried.
          await client.query(`DELETE FROM "user" WHERE "id" = $1`, [userId]).catch(() => {});
          throw err;
        }

        return { setCookie: headers.getSetCookie() };
      } finally {
        await client.query(`SELECT pg_advisory_unlock($1)`, [SETUP_LOCK_KEY]).catch(() => {});
        client.release();
      }
    },
  };
}
