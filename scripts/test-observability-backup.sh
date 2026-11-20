#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1" >&2; exit 1; }

echo "=== Test Suite: Fase 31 Observability & Backup Automation ==="

echo "[1/5] Validating Docker Compose stacks..."
if command -v docker >/dev/null 2>&1; then
  docker compose -f "$ROOT/deploy/monitoring/docker-compose.monitoring.yml" config --quiet
  pass "deploy/monitoring/docker-compose.monitoring.yml valid"
  docker compose -f "$ROOT/deploy/backup/docker-compose.backup.yml" config --quiet
  pass "deploy/backup/docker-compose.backup.yml valid"
else
  echo "  WARN: docker not found, skipping compose config check"
fi

echo "[2/5] Validating Prometheus, Alertmanager, and Grafana configs..."
python3 - "$ROOT" << 'PY'
import json
import os
import sys

root = sys.argv[1]

# 1. Validate Grafana JSON dashboard
dash_path = os.path.join(root, "deploy/monitoring/grafana/kinetiq-overview.json")
with open(dash_path, "r", encoding="utf-8") as f:
    dash = json.load(f)

panels = dash.get("panels", [])
assert len(panels) >= 6, f"expected >= 6 panels, got {len(panels)}"
all_exprs = " ".join(
    t.get("expr", "")
    for p in panels
    for t in p.get("targets", [])
)
for metric in [
    "ingest_events_received_total",
    "worker_queue_lag",
    "worker_dead_letter_total",
    "http_request_duration_seconds_bucket",
    "analytics_cache_hits_total",
    "db_pool_acquired_conns",
]:
    assert metric in all_exprs, f"dashboard missing metric: {metric}"

# 2. Validate Prometheus alerts content
alerts_path = os.path.join(root, "deploy/monitoring/prometheus/alerts.yml")
alerts_txt = open(alerts_path, "r", encoding="utf-8").read()
for alert in [
    "KinetiqWorkerHighLag",
    "KinetiqDLQGrowth",
    "KinetiqHostDiskUsageHigh",
    "KinetiqDBConnectionExhaustion",
    "KinetiqDatabaseDown",
    "KinetiqRedisDown",
]:
    assert f"alert: {alert}" in alerts_txt, f"alerts.yml missing rule: {alert}"

# 3. Validate Alertmanager receivers
am_path = os.path.join(root, "deploy/monitoring/alertmanager/alertmanager.yml")
am_txt = open(am_path, "r", encoding="utf-8").read()
for receiver in ["slack-ops", "email-ops", "pagerduty-critical"]:
    assert f"name: '{receiver}'" in am_txt, f"alertmanager.yml missing receiver: {receiver}"

# 4. Validate Prometheus scrape jobs
prom_path = os.path.join(root, "deploy/monitoring/prometheus/prometheus.yml")
prom_txt = open(prom_path, "r", encoding="utf-8").read()
for job in ["kinetiq-api", "node-exporter", "postgres-exporter", "redis-exporter"]:
    assert job in prom_txt, f"prometheus.yml missing scrape job: {job}"

print("  PASS: Grafana dashboard, Prometheus rules/scrape jobs, and Alertmanager receivers verified")
PY

echo "[3/5] Validating shell script syntax (bash -n)..."
bash -n "$ROOT/scripts/backup-postgres.sh"
bash -n "$ROOT/scripts/restore-postgres.sh"
pass "backup-postgres.sh & restore-postgres.sh syntax valid"

echo "[4/5] Testing backup dry-run, checksum, retention rotation, and restore drill..."
BACKUP_TEST_DIR="$TMP_DIR/backups"
mkdir -p "$BACKUP_TEST_DIR"

# Seed an expired dump older than 14 days to verify retention pruning
EXPIRED_DUMP="$BACKUP_TEST_DIR/kinetiq-20200101T000000Z.dump"
touch -d "30 days ago" "$EXPIRED_DUMP"

bash "$ROOT/scripts/backup-postgres.sh" \
  --dry-run \
  --local-dir "$BACKUP_TEST_DIR" \
  --retention-days 14 \
  --s3-bucket "test-bucket" \
  --s3-endpoint "https://r2.example.com"

if [[ -e "$EXPIRED_DUMP" ]]; then
  fail "expired backup was not pruned by retention rotation"
fi
pass "expired backup pruned by retention rotation"

NEW_DUMP="$(find "$BACKUP_TEST_DIR" -maxdepth 1 -name 'kinetiq-*.dump' | head -1)"
[[ -n "$NEW_DUMP" && -f "$NEW_DUMP" && -f "${NEW_DUMP}.sha256" ]] || fail "dry-run dump or .sha256 not created"
pass "dry-run dump and .sha256 checksum created"

bash "$ROOT/scripts/restore-postgres.sh" --dry-run --file "$NEW_DUMP"
pass "restore-postgres.sh dry-run checksum verification passed"

# Negative test: corrupt dump should fail checksum verification
echo "CORRUPTED" >> "$NEW_DUMP"
if bash "$ROOT/scripts/restore-postgres.sh" --dry-run --file "$NEW_DUMP" >/dev/null 2>&1; then
  fail "restore-postgres.sh should have rejected corrupted dump checksum"
fi
pass "restore-postgres.sh rejected corrupted dump checksum"

echo "[5/5] Verifying DR & Incident Response Runbook..."
RUNBOOK="$ROOT/docs/runbooks/SELF_HOSTED_DR_AND_INCIDENT_RUNBOOK.md"
[[ -f "$RUNBOOK" ]] || fail "missing DR runbook: $RUNBOOK"
for heading in "RPO" "RTO" "Restore Drill" "High Worker Lag" "Redis Down" "Disk Full" "Graceful Degradation"; do
  grep -qi "$heading" "$RUNBOOK" || fail "runbook missing section/topic: $heading"
done
pass "DR & Incident Response Runbook verified"

echo "=== ALL FASE 31 OBSERVABILITY & BACKUP TESTS PASSED ==="
