# Upgrade Guide

This guide describes how to upgrade a self-hosted **Kinetiq** deployment to a newer release without losing data.

---

## 1. Pre-Upgrade Backup

Always take a PostgreSQL backup before upgrading:

```bash
./scripts/backup-postgres.sh --local-dir /var/backups/kinetiq
```

---

## 2. Pull Latest Changes & Rebuild

```bash
# 1. Pull the latest repository changes (or checkout a specific release tag)
git fetch origin master
git pull origin master

# 2. Pull updated base images and rebuild service containers
docker compose pull
docker compose up -d --build
```

Persistent data is stored in Docker volumes (including `kinetiq_secrets` and container data volumes), so your database and Redis state are preserved across container recreations.

---

## 3. Database Migrations

When `docker compose up -d` runs, the `kinetiq-migrate` service automatically applies any new SQL migrations in `migrations/app/` before `kinetiq-auth`, `kinetiq-api`, and `kinetiq-web` start.

To inspect migration logs:

```bash
docker compose logs kinetiq-migrate
```

---

## 4. Post-Upgrade Verification

Verify that all services are healthy:

```bash
kinetiq doctor
docker compose ps
curl -s http://localhost:8080/readyz
```

---

## 5. Rollback Procedure

If an upgrade encounters an issue:

1. Stop the updated containers:
   ```bash
   docker compose down
   ```
2. Checkout the previous git tag or commit:
   ```bash
   git checkout <previous-tag-or-commit>
   ```
3. If a migration changed the schema, restore your pre-upgrade database backup:
   ```bash
   docker compose up -d postgres
   ./scripts/restore-postgres.sh --file /var/backups/kinetiq/<backup-file>.dump
   ```
4. Start the remaining services:
   ```bash
   docker compose up -d --build
   ```
