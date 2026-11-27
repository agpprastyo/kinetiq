import { describe, expect, test } from "bun:test";
import {
  buildOnboardingSnippets,
  createFirstEventListener,
  type OnboardingListenerState,
} from "../src/lib/onboarding";

describe("buildOnboardingSnippets", () => {
  test("includes apiKey and self-hosted endpoint in sdk, curl, and fetch snippets", () => {
    const snippets = buildOnboardingSnippets(
      "example-key",
      "https://analytics.example.com/api/v1/ingest"
    );

    expect(snippets.sdk).toContain("@agpprastyo/kinetiq-sdk");
    expect(snippets.sdk).toContain('apiKey: "example-key"');
    expect(snippets.sdk).toContain('endpoint: "https://analytics.example.com/api/v1/ingest"');

    expect(snippets.curl).toContain("curl -X POST https://analytics.example.com/api/v1/ingest");
    expect(snippets.curl).toContain("Authorization: Bearer example-key");

    expect(snippets.fetch).toContain('fetch("https://analytics.example.com/api/v1/ingest"');
    expect(snippets.fetch).toContain('"Authorization": "Bearer example-key"');
  });
});

describe("createFirstEventListener", () => {
  test("transitions waiting -> received when an event is detected", async () => {
    const states: OnboardingListenerState[] = [];
    let calls = 0;

    const listener = createFirstEventListener({
      pollIntervalMs: 10,
      timeoutMs: 500,
      fetchLatestEvent: async () => {
        calls += 1;
        if (calls >= 2) {
          return { event_name: "page_view", timestamp: "2026-11-27T08:00:00Z" };
        }
        return null;
      },
      onStateChange: (state) => {
        states.push({ ...state });
      },
    });

    listener.start();
    expect(listener.getState().status).toBe("waiting");

    await new Promise((r) => setTimeout(r, 60));

    const finalState = listener.getState();
    expect(finalState.status).toBe("received");
    expect(finalState.receivedEvent).toEqual({
      event_name: "page_view",
      timestamp: "2026-11-27T08:00:00Z",
    });
    expect(finalState.error).toBeNull();
    listener.stop();
  });

  test("transitions waiting -> timeout and supports checkAgain()", async () => {
    let calls = 0;
    const listener = createFirstEventListener({
      pollIntervalMs: 15,
      timeoutMs: 40,
      fetchLatestEvent: async () => {
        calls += 1;
        return null;
      },
    });

    listener.start();
    await new Promise((r) => setTimeout(r, 70));

    expect(listener.getState().status).toBe("timeout");
    const callsAtTimeout = calls;

    await new Promise((r) => setTimeout(r, 40));
    expect(calls).toBe(callsAtTimeout);

    listener.checkAgain();
    expect(listener.getState().status).toBe("waiting");
    await new Promise((r) => setTimeout(r, 25));
    expect(calls).toBeGreaterThan(callsAtTimeout);

    listener.stop();
  });

  test("handles transient network errors without stopping waiting state", async () => {
    let calls = 0;
    const listener = createFirstEventListener({
      pollIntervalMs: 15,
      timeoutMs: 500,
      fetchLatestEvent: async () => {
        calls += 1;
        if (calls === 1) {
          throw new Error("Network error");
        }
        return { event_name: "signup", timestamp: "2026-11-27T08:05:00Z" };
      },
    });

    listener.start();
    await new Promise((r) => setTimeout(r, 5));
    // First poll runs immediately on start()
    expect(listener.getState().status).toBe("waiting");
    expect(listener.getState().error).toBeTruthy();

    await new Promise((r) => setTimeout(r, 40));
    expect(listener.getState().status).toBe("received");
    expect(listener.getState().error).toBeNull();

    listener.stop();
  });

  test("pauses polling when tab is hidden and resumes when visible", async () => {
    let calls = 0;
    const listener = createFirstEventListener({
      pollIntervalMs: 15,
      timeoutMs: 500,
      fetchLatestEvent: async () => {
        calls += 1;
        return null;
      },
    });

    listener.start();
    await new Promise((r) => setTimeout(r, 20));
    listener.handleVisibilityChange(true);
    const callsWhileHidden = calls;

    await new Promise((r) => setTimeout(r, 50));
    expect(calls).toBe(callsWhileHidden);

    listener.handleVisibilityChange(false);
    await new Promise((r) => setTimeout(r, 25));
    expect(calls).toBeGreaterThan(callsWhileHidden);

    listener.stop();
  });
});
