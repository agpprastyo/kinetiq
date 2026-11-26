import { describe, expect, test } from "bun:test";
import { assertProductionSecrets, secretsMatch } from "./secrets";

describe("assertProductionSecrets", () => {
  test("throws in production when unset or default", () => {
    expect(() => assertProductionSecrets({ NODE_ENV: "production" })).toThrow();
    expect(() =>
      assertProductionSecrets({
        NODE_ENV: "production",
        AUTH_INTERNAL_SECRET: "test-internal-secret",
        BETTER_AUTH_SECRET: "x".repeat(40),
      }),
    ).toThrow(/AUTH_INTERNAL_SECRET/);
    expect(() =>
      assertProductionSecrets({
        APP_ENV: "production",
        AUTH_INTERNAL_SECRET: "real",
        BETTER_AUTH_SECRET: "test-secret-at-least-32-characters-long",
      }),
    ).toThrow(/BETTER_AUTH_SECRET/);
  });
  test("passes with real secrets or non-production", () => {
    expect(() => assertProductionSecrets({})).not.toThrow();
    expect(() =>
      assertProductionSecrets({ NODE_ENV: "production", AUTH_INTERNAL_SECRET: "a", BETTER_AUTH_SECRET: "b" }),
    ).not.toThrow();
  });
});

describe("secretsMatch", () => {
  test("matches only identical secrets", () => {
    expect(secretsMatch("abc", "abc")).toBe(true);
    expect(secretsMatch("abd", "abc")).toBe(false);
    expect(secretsMatch("ab", "abc")).toBe(false);
    expect(secretsMatch(undefined, "abc")).toBe(false);
  });
});
