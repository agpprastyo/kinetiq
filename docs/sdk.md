# SDK & Ingestion API Guide

Kinetiq provides the official `@agpprastyo/kinetiq-sdk` for TypeScript/JavaScript (Browser, Bun, and Node.js) as well as a standard HTTP REST ingestion endpoint.

---

## 1. Installation

```bash
bun add @agpprastyo/kinetiq-sdk
# or
npm install @agpprastyo/kinetiq-sdk
```

---

## 2. Initialize the Client

```typescript
import { KinetiqClient } from "@agpprastyo/kinetiq-sdk";

const kinetiq = new KinetiqClient({
  endpoint: "http://localhost:8080", // Your self-hosted Kinetiq API URL
  apiKey: "tlk_live_your_project_api_key_here",
  maxBatchSize: 50,
  autocapture: true, // Automatically captures pageviews & clicks in browsers
});
```

### Configuration Options

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `apiKey` | `string` | *(required)* | Project API key (`tlk_live_...`) |
| `endpoint` | `string` | `"http://localhost:8080"` | Base URL of your Kinetiq API server |
| `maxBatchSize` | `number` | `50` | Maximum events per batch flush |
| `maxQueueSize` | `number` | `1000` | Maximum queued events before dropping oldest |
| `storage` | `"memory" \| "localstorage" \| "indexeddb"` | `"memory"` | Persistence backend for offline/retry queueing |
| `autocapture` | `boolean` | `true` (browser) | Enable automatic pageview/click capture in browsers |
| `respectDoNotTrack` | `boolean` | `true` | Honor browser `Do Not Track` header |

---

## 3. Track Events & Identify Users

```typescript
// Track a custom event with properties
kinetiq.track("order_completed", {
  currency: "USD",
  amount: 49.99,
  items_count: 2,
});

// Associate subsequent events with a user ID and traits
kinetiq.identify("usr_99812", {
  email: "alex@company.com",
  plan: "pro",
});

// Manually flush queued events (useful in short-lived scripts/serverless)
await kinetiq.flush();
```

---

## 4. Direct HTTP REST API (`POST /api/v1/ingest`)

If you are sending events from Go, Python, Rust, or any backend service without the JS SDK, send a JSON array to `/api/v1/ingest` with your project API key:

```bash
curl -X POST http://localhost:8080/api/v1/ingest \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '[
    {
      "event": "order_completed",
      "user_id": "usr_99812",
      "properties": {
        "currency": "USD",
        "amount": 49.99
      },
      "timestamp": "2026-11-27T09:00:00Z"
    }
  ]'
```

A successful request returns `202 Accepted` and queues the batch into Redis Streams for asynchronous processing into PostgreSQL.
