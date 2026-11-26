import { describe, expect, it } from "bun:test";
import { auth } from "./auth";

describe("auth rate limiting (#187)", () => {
  it("throttles sign-in and sign-up", () => {
    const rl = auth.options.rateLimit!;
    expect(rl.customRules?.["/sign-in/email"]).toEqual({ window: 60, max: 5 });
    expect(rl.customRules?.["/sign-up/email"]).toEqual({ window: 60, max: 3 });
  });
});
