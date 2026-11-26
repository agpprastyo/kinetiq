# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- **OpenTelemetry Instrumentation & Monitoring Stack (Fase 24 — #93, PR #141)**:
  - Opt-in OTel config (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_TRACES_SAMPLER_ARG`).
  - `http_request_duration_seconds` histogram for p95/p99; `/metrics` Bearer protection via `METRICS_AUTH_TOKEN`.
  - Grafana dashboard `deploy/monitoring/grafana/kinetiq-overview.json` with provisioned Prometheus datasource.
- **Self-Hosted Deployment & Operations (Fase 27 — #99, PR #143)**:
  - `scripts/install.sh` 1-click installer (Docker/Podman detection, disk/RAM/port preflight, `--offline <bundle>`).
  - Air-gapped bundle `scripts/export-offline-bundle.sh` with SHA-256 manifest; Caddy TLS profile `deploy/tls/`; `docker-compose.healthcheck.yml`.
  - `kinetiq backup` / `kinetiq restore` CLI (`.env` archive + checksum validation).
- **Self-Hosted Data Lifecycle (Fase 30 — #102, PR #151, #165)**:
  - Per-project retention API (`/api/v1/projects/:projectID/retention`, `/preview`, `/run`) and `kinetiq retention preview|run`.
  - GDPR delete/pseudonymize `DELETE /api/v1/projects/:projectID/compliance/users/:distinctID?mode=delete|pseudonymize` with audit log.
  - `kinetiq import posthog|mixpanel` idempotent JSONL importer, `kinetiq vacuum`, TimescaleDB compression policy.
- **First-Run Setup Wizard (#144, PR #167)**: `/setup` stepper UI and Auth endpoints `GET /api/v1/setup/status`, `POST /api/v1/setup` (locked with `403` once any user exists).
- **`kinetiq doctor` Diagnostics (#146, PR #163)**: env, RAM/disk, container engine, ports, PostgreSQL/TimescaleDB/migrations, Redis stream, Auth health, and TLS expiry checks with `PASS`/`WARN`/`FAIL`, remediation hints, and `--json`.

### Changed
- **Standardized pagination (#182) — BREAKING response shape**:
  - Offset (`?page=1&limit=20`, max 100) returning `{ items, pagination: { page, limit, total, total_pages } }` for `GET /projects`, `/projects/:id/keys`, `/projects/:id/alerts`, `/projects/:id/insights`, `/billing/transactions`, `/projects/:id/analytics/users`, `/workspace/audit-logs`, `/ops/tenants`, `/ops/audit-logs`. Endpoints that returned bare arrays (or `{tenants|logs, count}`) now return `{ items, pagination }`.
  - Cursor (`?cursor=&limit=50`, max 200) returning `{ items, pagination: { limit, next_cursor, has_more } }` for user timeline and `/ops/dlq` (keyset on `(timestamp, id)`; malformed cursor -> `400 INVALID_CURSOR`; DLQ keeps `summary`).
  - Deprecated, still accepted for one release: `?offset=` (analytics users, `/ops/tenants`, `/ops/audit-logs`, `/ops/dlq`) and timeline `?before=`.
  - Web UI updated (timeline/DLQ "Load more"). See docs `api-reference/pagination`.

### Security
- Retention and GDPR deletion endpoints enforce project ownership (owner/admin) and return `404` for inaccessible projects (PR #165).

---

## [v0.6.0] — 2026-11-19

### Added
- **Self-Hosted Scalability & High Availability (Fase 28 — #100, PR #164)**:
  - Optional Redis Sentinel failover client (`REDIS_SENTINEL_ADDRS`, `REDIS_SENTINEL_MASTER`); DB/credentials still read from `REDIS_URL`.
  - Reference HA stack `deploy/ha/docker-compose.ha.yml` (Postgres primary + streaming replica, Redis master/replica + 3 Sentinels, 2x api, 2x worker).
  - Helm chart `deploy/helm/kinetiq` with HPA, PodDisruptionBudgets, pod anti-affinity, and `existingSecret` credentials.
- **Self-Hosted Observability & Backup Automation (Fase 31 — #103, PR #162)**:
  - Monitoring stack `deploy/monitoring/docker-compose.monitoring.yml` (Prometheus, Alertmanager, Grafana, node/postgres/redis exporters) with alert rules and `kinetiq_degraded_mode` gauge.
  - Scheduled backups (`deploy/backup/docker-compose.backup.yml`), `scripts/backup-postgres.sh` / `scripts/restore-postgres.sh` with SHA-256 checksums, S3/R2/GCS upload, and retention.
  - DR & incident runbook `docs/runbooks/SELF_HOSTED_DR_AND_INCIDENT_RUNBOOK.md`.
- **Safe In-Place Upgrade Runner (#145, PR #166)**:
  - `kinetiq upgrade [--dry-run] [--skip-health]`: pre-migration `pg_dump` + `.env` snapshot, `/healthz` + `/readyz` verification, automatic down-migration rollback with `pg_restore` guidance.

---

## [v0.5.0] — 2026-11-19

### Added
- **Plugin & Webhook Extensibility Framework (Fase 26 — #98, PR #142)**:
  - Outbound webhook delivery engine (`apps/api/internal/webhook/delivery.go`) with configurable timeouts, 5x exponential backoff retries, and atomic `CircuitBreaker` (`StateClosed` / `StateOpen` / `StateHalfOpen`).
  - HMAC SHA-256 request signing & constant-time replay-protected verification (`X-Kinetiq-Signature`, `X-Kinetiq-Timestamp`).
  - Strict DNS-validated SSRF guard (`IsSafeTargetURL`) blocking loopback, RFC1918 private networks, IPv6 ULA, and cloud metadata (`169.254.169.254`).
  - Database migration `000021_create_project_webhooks` provisioning `project_webhooks` and `webhook_deliveries`.
- **Self-Hosted Enterprise Authentication: SSO SAML/OIDC, LDAP & SCIM 2.0 (Fase 29 — #101, PR #149)**:
  - Environment-driven enterprise SSO configuration & SSO-only policy enforcement (`SSO_ENABLED`, `SSO_PROVIDER`, `SSO_ONLY`) in `apps/auth/src/sso-plugins.ts`.
  - SCIM 2.0 Hono provisioning router (`GET/POST /Users`, `PATCH /Users/:id`, `GET /Groups`) in `apps/auth/src/scim.ts`.

---

## [v0.4.0] — 2026-10-25

### Added
- **Database Query Performance & Read Replica Routing (Fase 19 — #72, PR #121)**:
  - Split primary/replica PostgreSQL connection pools via `database.Router` (`Writer()` / `Reader()`).
  - Automatic health-checked failover to Primary writer pool when read replica is unavailable.
  - Configurable pool sizing (`DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`), `DATABASE_READ_REPLICA_URL`, and slow-query threshold logging (`DB_SLOW_QUERY_THRESHOLD_MS`).
- **SDK Telemetry Resilience & Offline Persistence (Fase 20 — #74, PR #128)**:
  - Persistent storage adapters (`IndexedDBStorage`, `LocalStorageAdapter`, `MemoryStorage`) in `@kinetiq/sdk`.
  - Bounded queue protection (`maxQueueSize`), automatic online flush (`autoSyncOnOnline`), keepalive delivery on `visibilitychange` / `pagehide` (`flushOnPageHide`), and exponential backoff with jitter.

---

## [v0.3.0] — 2026-10-24

### Added
- **Transactional Mailer Engine & Outbox Reliability Dispatcher (Fase 14 — #56, PR #75)**:
  - Transactional outbox polling worker (`auth_email_outbox`) with exponential backoff retries, retention cleaner, `EmailSender` abstraction (`ConsoleSender`, `MockSender`), and Ops metrics endpoint `GET /api/v1/ops/mailer/metrics`.
- **Deep Product Analytics: User Explorer & Activity Timeline (Fase 15 — #58, PR #79)**:
  - Project User Directory UI (`/app/projects/[projectId]/users`), `distinct_id` database index, and REST APIs (`GET /api/v1/projects/:projectID/analytics/users`, `/users/:distinctId/timeline`).
- **Client Fingerprinting & Geo-IP / Device Breakdown (Fase 16 — #60, PR #85)**:
  - Privacy-first ingestion enricher (`FastEnricher`) for `device_type`, `os`, `browser`, `country`, and bot detection without persisting raw IPs.
  - Analytics breakdown endpoints (`/analytics/breakdown/devices`, `/analytics/breakdown/geo`) and `DeviceAndGeoBreakdown` dashboard widget.
- **Enterprise Data Portability: Automated Cloud Storage Sync (Fase 17 — #62, PR #89)**:
  - Scheduled gzip-compressed exports to S3-compatible buckets (AWS S3, Cloudflare R2, GCS, MinIO) with secret masking (`********`), `HeadBucket` connection validation, and `GET/PUT /storage-sync`, `POST /storage-sync/test` endpoints.
- **Enterprise Compliance: Workspace Audit Trail & Activity Logs (Fase 18 — #65, PR #104)**:
  - Append-only `workspace_audit_logs` table, RBAC-protected `GET /api/v1/workspace/audit-logs` (`owner`/`admin`/`superadmin`), and `WorkspaceAuditSection` settings UI.

---

## [v0.2.0] — 2026-10-23

### Fixed & Security
- **Security Hardening & Client Resilience (Fase 11 — #43, #45)**:
  - Remediated cross-tenant data leakage (OWASP A01:2021) by removing workspace-level admin user endpoints and isolating outbox & DLQ controls inside `/ops`.
  - Added graceful fallback handling in the Web Dashboard when auth service is unreachable.

---

## [v0.1.1] — 2026-10-21

### Added
- **Automated Intelligence & Anomaly Sweeper (Fase 10)**: Traffic anomaly detector (>50% drop) and weekly intelligence digest generator.

---

## [v0.1.0] — 2026-10-15

### Added
- **Initial MVP Engine (Fase 0–9)**: Redis Streams ingestion pipeline, PostgreSQL 16 batch worker, analytics suite (trends, funnels, cohort retention, custom widget builder), Better-Auth multi-tenant RBAC, SSE live tail, and TypeScript client SDK.
