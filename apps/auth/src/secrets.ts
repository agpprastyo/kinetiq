import { timingSafeEqual } from "node:crypto";

const DEFAULT_SECRETS = new Set([
  "test-internal-secret",
  "test-secret-at-least-32-characters-long",
]);

export function assertProductionSecrets(env: Record<string, string | undefined> = process.env): void {
  if (env.NODE_ENV !== "production" && env.APP_ENV !== "production") return;
  for (const name of ["AUTH_INTERNAL_SECRET", "BETTER_AUTH_SECRET"]) {
    const v = env[name];
    if (!v || DEFAULT_SECRETS.has(v)) {
      throw new Error(`${name} must be set to a non-default value in production`);
    }
  }
}

export function secretsMatch(provided: string | undefined, expected: string): boolean {
  if (!provided) return false;
  const a = Buffer.from(provided);
  const b = Buffer.from(expected);
  return a.length === b.length && timingSafeEqual(a, b);
}
