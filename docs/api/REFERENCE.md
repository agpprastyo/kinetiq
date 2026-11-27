# Kinetiq Public API Reference

This document provides `curl` examples and response schemas for Kinetiq Community Edition (CE) public endpoints.
For the machine-readable specification, see [`openapi.yaml`](./openapi.yaml).

## Authentication

Kinetiq uses two authentication schemes:
1. **API Key (`X-API-Key`)**: Used for event ingestion (`/api/v1/ingest`, `/api/v1/identify`, `/api/v1/alias`).
2. **Bearer Token (`Authorization: Bearer <token>`)**: Used for project management, analytics queries, and exports.

---

## 1. Healthcheck

### `GET /healthz`
```bash
curl -s http://localhost:8080/healthz
```
Response:
```json
{ "status": "ok" }
```

### `GET /readyz`
```bash
curl -s http://localhost:8080/readyz
```
Response:
```json
{ "status": "ready" }
```

---

## 2. Ingestion & Person Identity

### `POST /api/v1/ingest`
Ingest a single event or a batch of up to 100 events.

```bash
curl -X POST http://localhost:8080/api/v1/ingest \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $KINETIQ_API_KEY" \
  -d '{
    "event_id": "evt_01J9X8K7M2N3P4Q5R6S7T8U9V0",
    "event_name": "page_view",
    "distinct_id": "user_12345",
    "occurred_at": "2026-10-14T12:00:00Z",
    "properties": {
      "path": "/pricing",
      "referrer": "https://google.com"
    }
  }'
```
Response (`202 Accepted`):
```json
{
  "status": "accepted",
  "accepted": 1,
  "rejected": 0
}
```

### `POST /api/v1/identify`
Link an anonymous ID to an identified user and set person properties.

```bash
curl -X POST http://localhost:8080/api/v1/identify \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $KINETIQ_API_KEY" \
  -d '{
    "distinct_id": "user_12345",
    "anonymous_id": "anon_98765",
    "$set": { "email": "alice@example.com", "plan": "pro" },
    "$set_once": { "signup_source": "organic" }
  }'
```
Response (`200 OK`):
```json
{
  "person_id": "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a22",
  "is_identified": true,
  "properties": {
    "email": "alice@example.com",
    "plan": "pro",
    "signup_source": "organic"
  }
}
```

### `POST /api/v1/alias`
Link two known distinct IDs to the same person.

```bash
curl -X POST http://localhost:8080/api/v1/alias \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $KINETIQ_API_KEY" \
  -d '{
    "distinct_id": "user_12345",
    "alias": "legacy_user_99"
  }'
```

---

## 3. Projects & API Keys

### `POST /api/v1/projects`
```bash
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"name": "My Web App"}'
```
Response (`201 Created`):
```json
{
  "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
  "name": "My Web App",
  "status": "active",
  "created_at": "2026-10-14T12:00:00Z",
  "api_key": "sp_live_example_key_placeholder"
}
```

### `GET /api/v1/projects`
```bash
curl -s http://localhost:8080/api/v1/projects \
  -H "Authorization: Bearer <token>"
```

### `GET /api/v1/projects/{projectID}` / `PATCH` / `DELETE`
```bash
curl -s http://localhost:8080/api/v1/projects/a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11 \
  -H "Authorization: Bearer <token>"
```

### `POST /api/v1/projects/{projectID}/keys`
```bash
curl -X POST http://localhost:8080/api/v1/projects/a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11/keys \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"name": "Production Web SDK", "environment": "live"}'
```

---

## 4. Analytics & Catalog

All analytics endpoints require `Authorization: Bearer <token>` and accept optional `from` and `to` ISO-8601 query parameters.

```bash
# Summary
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/summary?from=2026-10-01T00:00:00Z&to=2026-10-14T23:59:59Z" \
  -H "Authorization: Bearer <token>"

# Top events
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/top-events" \
  -H "Authorization: Bearer <token>"

# Volume time series
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/volume" \
  -H "Authorization: Bearer <token>"

# Funnel & dropoff
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/funnel?steps=page_view,signup,purchase" \
  -H "Authorization: Bearer <token>"

# Property / Device / Geo breakdowns
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/breakdown?property=path" \
  -H "Authorization: Bearer <token>"
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/breakdown/devices" \
  -H "Authorization: Bearer <token>"
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/breakdown/geo" \
  -H "Authorization: Bearer <token>"

# Retention & Sessions
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/retention" \
  -H "Authorization: Bearer <token>"
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/sessions" \
  -H "Authorization: Bearer <token>"

# Event & Property Catalog
curl -s "http://localhost:8080/api/v1/projects/<projectID>/catalog/events" \
  -H "Authorization: Bearer <token>"
curl -s "http://localhost:8080/api/v1/projects/<projectID>/catalog/properties" \
  -H "Authorization: Bearer <token>"

# Users & Timeline
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/users" \
  -H "Authorization: Bearer <token>"
curl -s "http://localhost:8080/api/v1/projects/<projectID>/analytics/users/user_12345/timeline" \
  -H "Authorization: Bearer <token>"
```

---

## 5. Live Stream & Export

```bash
# Live SSE stream
curl -N "http://localhost:8080/api/v1/projects/<projectID>/live" \
  -H "Authorization: Bearer <token>"

# Export events
curl -s "http://localhost:8080/api/v1/projects/<projectID>/export?from=2026-10-01T00:00:00Z" \
  -H "Authorization: Bearer <token>" -o events.json
```
