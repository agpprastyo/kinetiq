import { Hono } from "hono";

export interface SetupInput {
  name: string;
  email: string;
  password: string;
  workspaceName: string;
  appUrl: string;
  timezone: string;
}

export class AlreadyInitializedError extends Error {}

export interface SetupStore {
  isInitialized(): Promise<boolean>;
  /** Must atomically re-check initialization and throw AlreadyInitializedError if set up. */
  initialize(input: SetupInput): Promise<{ setCookie: string[] }>;
}

export const MIN_PASSWORD_LENGTH = 12;

export function validateSetupInput(body: any): { ok: true; value: SetupInput } | { ok: false; error: string } {
  const str = (v: unknown) => (typeof v === "string" ? v.trim() : "");
  const name = str(body?.name);
  const email = str(body?.email).toLowerCase();
  const password = typeof body?.password === "string" ? body.password : "";
  const workspaceName = str(body?.workspaceName);
  const appUrl = str(body?.appUrl);
  const timezone = str(body?.timezone) || "UTC";

  if (!name || name.length > 100) return { ok: false, error: "Name is required (max 100 characters)." };
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) || email.length > 254) {
    return { ok: false, error: "A valid email is required." };
  }
  if (password.length < MIN_PASSWORD_LENGTH || password.length > 128) {
    return { ok: false, error: `Password must be ${MIN_PASSWORD_LENGTH}-128 characters.` };
  }
  if (!workspaceName || workspaceName.length > 100) {
    return { ok: false, error: "Workspace name is required (max 100 characters)." };
  }
  try {
    const u = new URL(appUrl);
    if (u.protocol !== "http:" && u.protocol !== "https:") throw new Error();
  } catch {
    return { ok: false, error: "App URL must be a valid http(s) URL." };
  }
  try {
    new Intl.DateTimeFormat("en", { timeZone: timezone });
  } catch {
    return { ok: false, error: "Timezone is not a valid IANA timezone." };
  }
  return { ok: true, value: { name, email, password, workspaceName, appUrl: appUrl.replace(/\/+$/, ""), timezone } };
}

export function createSetupRoutes(store: SetupStore) {
  const r = new Hono();

  r.get("/status", async (c) => {
    const initialized = await store.isInitialized();
    return c.json({ initialized, is_setup_completed: initialized });
  });

  r.post("/", async (c) => {
    // Cheap pre-check; authoritative check happens atomically inside store.initialize.
    if (await store.isInitialized()) {
      return c.json({ error: { message: "Instance is already set up." } }, 403);
    }
    const body = await c.req.json().catch(() => null);
    const parsed = validateSetupInput(body);
    if (!parsed.ok) return c.json({ error: { message: parsed.error } }, 400);

    try {
      const { setCookie } = await store.initialize(parsed.value);
      for (const cookie of setCookie) c.header("Set-Cookie", cookie, { append: true });
      return c.json({ initialized: true }, 201);
    } catch (err: any) {
      if (err instanceof AlreadyInitializedError) {
        return c.json({ error: { message: "Instance is already set up." } }, 403);
      }
      console.error("[Setup Error]", err);
      return c.json({ error: { message: "Unable to initialize instance." } }, 500);
    }
  });

  return r;
}
