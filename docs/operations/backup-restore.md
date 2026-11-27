# Backup & Restore Guide (PostgreSQL)

Kinetiq stores all telemetry events, projects, users, and analytics aggregates in PostgreSQL (TimescaleDB). This guide covers automated backups and restores using the built-in scripts in `scripts/`.

---

## 1. Automated Backup (`scripts/backup-postgres.sh`)

The `scripts/backup-postgres.sh` script creates compressed `pg_dump` archives with SHA-256 checksums, enforces local retention, and optionally uploads to any S3-compatible bucket (AWS S3, Cloudflare R2, MinIO, GCS).

### Local Backup

```bash
export DATABASE_URL="postgres://kinetiq:kinetiq_secure_password@localhost:5432/kinetiq?sslmode=disable"

./scripts/backup-postgres.sh \
  --local-dir /var/backups/kinetiq \
  --retention-days 14
```

### S3 / Cloudflare R2 / MinIO Backup

```bash
./scripts/backup-postgres.sh \
  --local-dir /var/backups/kinetiq \
  --s3-bucket my-kinetiq-backups \
  --s3-prefix prod \
  --s3-endpoint https://<account-id>.r2.cloudflarestorage.com
```

### Dry Run

Verify options and retention logic without touching the database or S3:

```bash
./scripts/backup-postgres.sh --dry-run
```

---

## 2. Restore from Backup (`scripts/restore-postgres.sh`)

The `scripts/restore-postgres.sh` script verifies the `.sha256` checksum (unless `--skip-checksum` is passed) and restores the `.dump` archive into your target PostgreSQL instance.

### Restore from Local File

```bash
export DATABASE_URL="postgres://kinetiq:kinetiq_secure_password@localhost:5432/kinetiq?sslmode=disable"

./scripts/restore-postgres.sh --file /var/backups/kinetiq/kinetiq_20261127_020000.dump
```

### Restore from S3 / R2

```bash
./scripts/restore-postgres.sh \
  --s3-uri s3://my-kinetiq-backups/prod/kinetiq_20261127_020000.dump \
  --s3-endpoint https://<account-id>.r2.cloudflarestorage.com
```

---

## 3. Manual Docker Compose Backup & Restore

If you prefer running `pg_dump` directly against the `kinetiq-postgres` container:

```bash
# Backup
docker compose exec -T kinetiq-db pg_dump -U kinetiq -Fc kinetiq > kinetiq_backup.dump

# Restore
cat kinetiq_backup.dump | docker compose exec -T kinetiq-db pg_restore -U kinetiq -d kinetiq --clean --if-exists
```
