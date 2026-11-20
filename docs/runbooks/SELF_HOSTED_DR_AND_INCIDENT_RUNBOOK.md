# Self-Hosted Disaster Recovery (DR) & Incident Response Runbook

> **Applies to**: Kinetiq Self-Hosted (Docker Compose / Kubernetes)  
> **Related Stack**: `deploy/monitoring/docker-compose.monitoring.yml`, `deploy/backup/docker-compose.backup.yml`, `scripts/backup-postgres.sh`, `scripts/restore-postgres.sh`

---

## 1. Recovery Objectives (RPO & RTO)

| Failure Scenario | Backup / Resilience Mechanism | Target RPO | Target RTO |
| :--- | :--- | :--- | :--- |
| **Short PostgreSQL Outage (< 30 min)** | Redis Stream ingestion buffering (`worker_queue_lag`) | **0 events** (buffered in Redis AOF) | **< 5 min** (restart / failover DB) |
| **PostgreSQL Corruption / Accidental Drop** | Scheduled daily `pg_dump` to S3/R2/GCS (`0 2 * * *`) | **<= 24 hours** (<= 1 hour with hourly cron) | **<= 30 minutes** |
| **Host Disk Exhaustion** | TimescaleDB chunk drop + backup rotation + volume expansion | **0 events** (if caught at 85% warning) | **<= 15 minutes** |
| **Redis Outage** | Redis AOF persistence + restart; dashboard falls back to DB queries | **<= 1 second** (AOF `appendfsync everysec`) | **<= 5 minutes** |

---

## 2. Monitoring Stack Quickstart

```bash
# 1. Export required credentials
export GF_SECURITY_ADMIN_PASSWORD="<strong-grafana-password>"
export POSTGRES_EXPORTER_DSN="postgresql://<user>:<pass>@kinetiq-db:5432/kinetiq?sslmode=disable"

# 2. Launch Prometheus, Alertmanager, Grafana, node-exporter, postgres-exporter, redis-exporter
docker compose -f deploy/monitoring/docker-compose.monitoring.yml up -d
```

- **Grafana**: `http://localhost:3002` (auto-provisioned dashboard: **Kinetiq Self-Hosted Overview**)
- **Prometheus**: `http://localhost:9090` (alerts loaded from `deploy/monitoring/prometheus/alerts.yml`)
- **Alertmanager**: `http://localhost:9093` (Slack / Email / PagerDuty routing in `deploy/monitoring/alertmanager/alertmanager.yml`)

---

## 3. Automated Backup & Restore Drill

### 3.1 Enable Scheduled S3/R2/GCS Backups

```bash
export DATABASE_URL="postgres://<user>:<pass>@kinetiq-db:5432/kinetiq?sslmode=disable"
export BACKUP_S3_BUCKET="my-kinetiq-backups"
export BACKUP_S3_ENDPOINT="https://<account-id>.r2.cloudflarestorage.com" # omit for AWS S3
export BACKUP_RETENTION_DAYS="14"

docker compose -f deploy/backup/docker-compose.backup.yml up -d
```

### 3.2 Manual On-Demand Backup

```bash
./scripts/backup-postgres.sh \
  --database-url "$DATABASE_URL" \
  --s3-bucket "$BACKUP_S3_BUCKET" \
  --s3-endpoint "$BACKUP_S3_ENDPOINT" \
  --retention-days 14
```

### 3.3 Quarterly Restore Drill (Staging Verification)

1. Provision a disposable PostgreSQL container (`kinetiq-restore-drill`) so production is untouched.
2. Restore the latest backup with SHA-256 checksum verification:
   ```bash
   ./scripts/restore-postgres.sh \
     --s3-uri "s3://${BACKUP_S3_BUCKET}/kinetiq-backups/kinetiq-YYYYMMDDTHHMMSSZ.dump" \
     --s3-endpoint "$BACKUP_S3_ENDPOINT" \
     --target-db "postgres://kinetiq:<pass>@localhost:5439/kinetiq_drill?sslmode=disable"
   ```
3. Verify:
   - `pg_restore` exits `0`
   - `schema_migrations` latest version matches production and `dirty = false`
   - Row counts on `projects` and `events` match expected snapshot totals
4. Record elapsed time (verify **<= 30 min RTO**) and destroy the disposable container.

---

## 4. Incident Response Triage Playbooks

### 1. High Worker Lag

**Alerts**: `KinetiqWorkerHighLag` (`> 10,000` pending for 5m), `KinetiqWorkerCriticalLag` (`> 50,000` pending for 5m), `KinetiqDLQGrowth`

1. **Inspect lag & worker logs**:
   ```bash
   docker compose logs --tail=100 kinetiq-worker
   docker compose exec kinetiq-redis redis-cli XLEN kinetiq:events
   ```
2. **Check PostgreSQL write health**:
   - Look at Grafana panel **PostgreSQL Connection Pool** and **HTTP p95 / p99 Latency**.
   - Check active locks: `SELECT pid, state, wait_event_type, query FROM pg_stat_activity WHERE state != 'idle';`
3. **Mitigate**:
   - Scale worker replicas horizontally: `docker compose up -d --scale kinetiq-worker=3`
   - If DLQ is growing (`KinetiqDLQGrowth`), inspect dead letters via Ops DLQ endpoint or `SELECT * FROM dead_letter_events ORDER BY created_at DESC LIMIT 20;` and replay after fixing the root cause.

### 2. PostgreSQL / TimescaleDB Down or Exhausted

**Alerts**: `KinetiqDatabaseDown` (`pg_up == 0`), `KinetiqDBConnectionExhaustion` (`> 85%` pool/backends)

1. **Verify container & logs**:
   ```bash
   docker compose ps kinetiq-db
   docker compose logs --tail=150 kinetiq-db
   ```
2. **If connection exhaustion**:
   - Terminate runaway analytics queries (`pg_terminate_backend(pid)` for long-running queries > 60s).
   - Enable/tune PgBouncer transaction pooling (`deploy/ha/pgbouncer.ini`) or increase `DB_MAX_CONNS`.
3. **If database is down**:
   - Confirm Redis is up so `/api/v1/ingest` continues buffering events into Redis Streams (**Ingest Buffering** degraded mode).
   - Restart PostgreSQL (`docker compose restart kinetiq-db`). If volume is corrupted, execute Section 3.3 Restore procedure.

### 3. Redis Down

**Alerts**: `KinetiqRedisDown` (`redis_up == 0`)

1. **Verify container & memory/AOF health**:
   ```bash
   docker compose ps kinetiq-redis
   docker compose logs --tail=100 kinetiq-redis
   ```
2. **Impact**:
   - Analytics queries bypass cache and hit PostgreSQL directly (higher read latency).
   - `/api/v1/ingest` returns `503 Service Unavailable` (`queue_unavailable`); SDK clients (`@kinetiq/sdk`) automatically buffer and retry events with exponential backoff.
3. **Mitigate**:
   - Restart Redis (`docker compose restart kinetiq-redis`). If AOF is truncated, run `redis-check-aof --fix /data/appendonly.aof` before restart.

### 4. Disk Full

**Alerts**: `KinetiqHostDiskUsageHigh` (`> 85%`), `KinetiqHostDiskUsageCritical` (`> 92%`)

1. **Identify largest consumers**:
   ```bash
   df -h
   du -sh /var/lib/docker/volumes/* /var/backups/kinetiq 2>/dev/null | sort -hr | head -10
   ```
2. **Immediate relief**:
   - Prune unused Docker build cache/images: `docker system prune -f`
   - Run local backup retention rotation with tighter window: `./scripts/backup-postgres.sh --dry-run --retention-days 3` (or remove verified-uploaded local dumps).
   - Drop expired TimescaleDB chunks via retention policy if configured.
3. **Permanent fix**: Expand host volume / move `kinetiq_postgres_data` to dedicated block storage.

---

## 5. Graceful Degradation Modes

Kinetiq tracks active degradation modes via the Prometheus gauge `kinetiq_degraded_mode{mode="..."}` (alerted by `KinetiqDegradedModeActive`):

1. **`ingest_buffering` (Database Temporarily Unavailable)**:
   - `/api/v1/ingest` continues accepting and enqueueing events into Redis Streams without blocking on PostgreSQL.
   - Workers pause/backoff batch inserts until PostgreSQL recovers, then drain the backlog automatically with zero event loss.
2. **`read_only_dashboard` (Writer DB Unavailable / Maintenance Window)**:
   - Analytics read endpoints serve cached Redis responses and read-replica queries (`DBRouter.Reader()`).
   - Mutating configuration actions return `503 Service Unavailable` with `Retry-After` until primary writer recovers.
