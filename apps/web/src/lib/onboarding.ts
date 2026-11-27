export interface OnboardingSnippets {
  sdk: string;
  curl: string;
  fetch: string;
}

export interface ReceivedEventPreview {
  event_name: string;
  timestamp: string;
}

export type OnboardingListenerStatus = "idle" | "waiting" | "received" | "timeout";

export interface OnboardingListenerState {
  status: OnboardingListenerStatus;
  receivedEvent: ReceivedEventPreview | null;
  error: string | null;
}

export interface FirstEventListenerOptions {
  fetchLatestEvent: () => Promise<ReceivedEventPreview | null>;
  pollIntervalMs?: number;
  timeoutMs?: number;
  onStateChange?: (state: OnboardingListenerState) => void;
}

/**
 * Generates copy-pasteable onboarding snippets for JS/TS (@agpprastyo/kinetiq-sdk),
 * cURL, and Fetch API pre-filled with the project's API key and ingest endpoint.
 */
export function buildOnboardingSnippets(
  apiKey: string,
  endpoint: string
): OnboardingSnippets {
  const key = apiKey || "tlk_live_your_api_key";
  const ingestEndpoint = endpoint || "http://localhost:8080/api/v1/ingest";
  const sampleTimestamp = new Date().toISOString();

  return {
    sdk: `import { KinetiqClient } from "@agpprastyo/kinetiq-sdk";

const kinetiq = new KinetiqClient({
  apiKey: "${key}",
  endpoint: "${ingestEndpoint}"
});

// Track user event
kinetiq.track("page_view", {
  distinct_id: "usr_100",
  path: "/pricing",
  plan: "growth"
});`,
    curl: `curl -X POST ${ingestEndpoint} \\
  -H "Authorization: Bearer ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "events": [
      {
        "event_id": "evt_sample_01",
        "event_name": "page_view",
        "distinct_id": "usr_100",
        "occurred_at": "${sampleTimestamp}",
        "properties": { "path": "/pricing" }
      }
    ]
  }'`,
    fetch: `await fetch("${ingestEndpoint}", {
  method: "POST",
  headers: {
    "Authorization": "Bearer ${key}",
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    events: [
      {
        event_id: crypto.randomUUID(),
        event_name: "page_view",
        distinct_id: "usr_100",
        occurred_at: new Date().toISOString(),
        properties: { path: "/pricing" }
      }
    ]
  })
});`,
  };
}

/**
 * Creates a lightweight polling listener that waits for the first event
 * on a newly created project, pausing when the browser tab is hidden and
 * timing out after `timeoutMs` (default 5 minutes).
 */
export function createFirstEventListener(options: FirstEventListenerOptions) {
  const pollIntervalMs = options.pollIntervalMs ?? 2000;
  const timeoutMs = options.timeoutMs ?? 5 * 60 * 1000;

  let state: OnboardingListenerState = {
    status: "idle",
    receivedEvent: null,
    error: null,
  };

  let pollTimer: ReturnType<typeof setInterval> | null = null;
  let timeoutTimer: ReturnType<typeof setTimeout> | null = null;
  let isPolling = false;
  let isHidden = false;

  function emit() {
    options.onStateChange?.({ ...state });
  }

  function clearTimers() {
    if (pollTimer !== null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    if (timeoutTimer !== null) {
      clearTimeout(timeoutTimer);
      timeoutTimer = null;
    }
  }

  async function pollOnce() {
    if (isPolling || state.status !== "waiting" || isHidden) return;
    isPolling = true;
    try {
      const event = await options.fetchLatestEvent();
      if (state.status !== "waiting") return;

      if (event) {
        clearTimers();
        state = {
          status: "received",
          receivedEvent: event,
          error: null,
        };
        emit();
      } else if (state.error !== null) {
        state = { ...state, error: null };
        emit();
      }
    } catch {
      if (state.status === "waiting") {
        state = {
          ...state,
          error: "Connection hiccup while checking for events. Retrying...",
        };
        emit();
      }
    } finally {
      isPolling = false;
    }
  }

  function startTimers() {
    clearTimers();
    pollTimer = setInterval(() => {
      void pollOnce();
    }, pollIntervalMs);
    timeoutTimer = setTimeout(() => {
      if (state.status === "waiting") {
        clearTimers();
        state = { ...state, status: "timeout", error: null };
        emit();
      }
    }, timeoutMs);
  }

  function start() {
    state = {
      status: "waiting",
      receivedEvent: null,
      error: null,
    };
    emit();
    if (!isHidden) {
      startTimers();
      void pollOnce();
    }
  }

  function stop() {
    clearTimers();
  }

  function checkAgain() {
    start();
  }

  function handleVisibilityChange(hidden: boolean) {
    isHidden = hidden;
    if (state.status !== "waiting") return;
    if (hidden) {
      clearTimers();
    } else {
      startTimers();
      void pollOnce();
    }
  }

  return {
    getState: () => ({ ...state }),
    start,
    stop,
    checkAgain,
    handleVisibilityChange,
  };
}
