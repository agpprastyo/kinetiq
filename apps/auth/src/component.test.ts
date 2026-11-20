import { describe, expect, it } from "bun:test";
import app from "./index";

const INTERNAL_SECRET = process.env.AUTH_INTERNAL_SECRET || "test-internal-secret";

describe("Auth Component & Comprehensive Scenarios", () => {
  it("serves Swagger OpenAPI schema and UI docs endpoint", async () => {
    const swaggerRes = await app.request("/swagger.json");
    expect(swaggerRes.status).toBe(200);
    const schema = await swaggerRes.json();
    expect(schema.openapi).toBe("3.0.0");
    expect(schema.info.title).toContain("Kinetiq Auth");

    const docsRes = await app.request("/docs");
    expect(docsRes.status).toBe(200);
    const html = await docsRes.text();
    expect(html).toContain("swagger");
  });

  it("serves /healthz and /readyz endpoints with service health", async () => {
    const healthz = await app.request("/healthz");
    expect(healthz.status).toBe(200);
    const hzBody = await healthz.json();
    expect(hzBody.status).toBe("alive");

    const readyz = await app.request("/readyz");
    expect(readyz.status).toBe(200);
    const rzBody = await readyz.json();
    expect(rzBody.status).toBe("ready");
    expect(rzBody.database).toBe("connected");
  });

  it("handles duplicate email registration conflict", async () => {
    const uniqueEmail = `dup_${Date.now()}@example.com`;

    const res1 = await app.request("/api/auth/sign-up/email", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email: uniqueEmail,
        password: "Password123!",
        name: "First User",
      }),
    });
    expect(res1.status).toBe(200);

    // Register with same email again
    const res2 = await app.request("/api/auth/sign-up/email", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email: uniqueEmail,
        password: "AnotherPassword123!",
        name: "Second User",
      }),
    });
    // Better Auth returns 400/422 for existing email
    expect([400, 422]).toContain(res2.status);
  });

  it("fails sign-in with wrong password", async () => {
    const uniqueEmail = `signin_fail_${Date.now()}@example.com`;

    // 1. Sign up
    await app.request("/api/auth/sign-up/email", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email: uniqueEmail,
        password: "CorrectPassword123!",
        name: "Valid User",
      }),
    });

    // 2. Sign in with wrong password
    const failRes = await app.request("/api/auth/sign-in/email", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email: uniqueEmail,
        password: "WrongPassword!",
      }),
    });
    expect([400, 401]).toContain(failRes.status);
  });

  it("handles malformed JSON body gracefully on introspection", async () => {
    const res = await app.request("/internal/session/introspect", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Internal-Auth": INTERNAL_SECRET,
      },
      body: "invalid-json-content",
    });
    expect(res.status).toBe(400);
  });
});
