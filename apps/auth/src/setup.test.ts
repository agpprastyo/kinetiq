import { describe, expect, it } from "bun:test";
import { Hono } from "hono";
import { AlreadyInitializedError, createSetupRoutes, validateSetupInput, type SetupStore } from "./setup";

// Throwaway test-only credential generated at runtime.
const pw = () => `${crypto.randomUUID()}Aa1`;

function fakeStore(initial = false) {
  let initialized = initial;
  const calls: unknown[] = [];
  const store: SetupStore = {
    isInitialized: async () => initialized,
    async initialize(input) {
      if (initialized) throw new AlreadyInitializedError();
      calls.push(input);
      initialized = true;
      return { setCookie: ["session=abc; Path=/; HttpOnly"] };
    },
  };
  const app = new Hono().route("/api/v1/setup", createSetupRoutes(store));
  return { app, calls };
}

const valid = () => ({
  name: "Admin",
  email: "Admin@Example.com",
  password: pw(),
  workspaceName: "Acme",
  appUrl: "https://kinetiq.example.com/",
  timezone: "Asia/Jakarta",
});

const post = (app: Hono, body: unknown) =>
  app.request("/api/v1/setup", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

describe("setup status", () => {
  it("reports fresh instance", async () => {
    const { app } = fakeStore(false);
    const res = await app.request("/api/v1/setup/status");
    expect(res.status).toBe(200);
    expect((await res.json()).initialized).toBe(false);
  });

  it("reports initialized instance", async () => {
    const { app } = fakeStore(true);
    const body = await (await app.request("/api/v1/setup/status")).json();
    expect(body.initialized).toBe(true);
    expect(body.is_setup_completed).toBe(true);
  });
});

describe("setup initialization lock", () => {
  it("initializes once, sets session cookie, then returns 403", async () => {
    const { app, calls } = fakeStore(false);
    const res = await post(app, valid());
    expect(res.status).toBe(201);
    expect(res.headers.get("set-cookie")).toContain("session=abc");
    expect(calls).toHaveLength(1);
    expect((calls[0] as any).email).toBe("admin@example.com");
    expect((calls[0] as any).appUrl).toBe("https://kinetiq.example.com");

    const again = await post(app, valid());
    expect(again.status).toBe(403);
    expect(calls).toHaveLength(1);
  });

  it("returns 403 when store loses a race (AlreadyInitializedError)", async () => {
    const store: SetupStore = {
      isInitialized: async () => false,
      initialize: async () => {
        throw new AlreadyInitializedError();
      },
    };
    const raced = new Hono().route("/api/v1/setup", createSetupRoutes(store));
    expect((await post(raced, valid())).status).toBe(403);
  });

  it("returns 403 immediately when already initialized, even with invalid body", async () => {
    const { app } = fakeStore(true);
    expect((await post(app, {})).status).toBe(403);
  });

  it("does not leak internal errors", async () => {
    const store: SetupStore = {
      isInitialized: async () => false,
      initialize: async () => {
        throw new Error("db password=hunter2");
      },
    };
    const app = new Hono().route("/api/v1/setup", createSetupRoutes(store));
    const res = await post(app, valid());
    expect(res.status).toBe(500);
    expect(JSON.stringify(await res.json())).not.toContain("hunter2");
  });
});

describe("setup validation", () => {
  it("rejects bad input with 400 and does not initialize", async () => {
    const bad = [
      { ...valid(), name: "" },
      { ...valid(), email: "nope" },
      { ...valid(), password: "short" },
      { ...valid(), workspaceName: "  " },
      { ...valid(), appUrl: "ftp://x.com" },
      { ...valid(), appUrl: "not a url" },
      { ...valid(), timezone: "Mars/Olympus" },
    ];
    for (const b of bad) {
      const { app, calls } = fakeStore(false);
      expect((await post(app, b)).status).toBe(400);
      expect(calls).toHaveLength(0);
    }
  });

  it("rejects non-JSON body", async () => {
    const { app } = fakeStore(false);
    const res = await app.request("/api/v1/setup", { method: "POST", body: "garbage" });
    expect(res.status).toBe(400);
  });

  it("defaults timezone to UTC", () => {
    const r = validateSetupInput({ ...valid(), timezone: undefined });
    expect(r.ok && r.value.timezone).toBe("UTC");
  });
});
