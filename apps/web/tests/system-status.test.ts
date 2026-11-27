import { describe, expect, test } from "bun:test";
import { healthBadgeClasses, queueHealth } from "../src/lib/system-status";

describe("system-status helpers", () => {
  test("healthBadgeClasses maps healthy, degraded, and down", () => {
    expect(healthBadgeClasses("healthy")).toContain("emerald");
    expect(healthBadgeClasses("degraded")).toContain("amber");
    expect(healthBadgeClasses("down")).toContain("red");
  });

  test("queueHealth returns degraded when PEL or lag exceeds threshold", () => {
    expect(queueHealth(0, 0)).toBe("healthy");
    expect(queueHealth(1001, 0)).toBe("degraded");
    expect(queueHealth(0, 5001)).toBe("degraded");
  });
});
