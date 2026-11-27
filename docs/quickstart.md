# Self-Hosted Quickstart Guide

This guide walks you through deploying **Kinetiq** on your own server or local machine using Docker Compose, verifying stack health, and sending your first telemetry event.

---

## Prerequisites

- **Docker** (24.0+) and **Docker Compose** (`docker compose` v2+)
- **Git**
- *(Optional)* **Bun** (1.2+) or **Node.js** (18+) to run the SDK example

---

## 1. Clone the Repository

```bash
git clone https://github.com/agpprastyo/kinetiq.git
cd kinetiq
```

---

## 2. Configure Environment Variables

Copy the example environment file:

```bash
cp .env.example .env
```

For production deployments, generate strong random secrets for `BETTER_AUTH_SECRET` and `INTERNAL_AUTH_SECRET`:

```bash
openssl rand -hex 32
```

Update `.env` with your generated secrets and public URLs:

```ini
# Auth Secrets (minimum 32 characters)
BETTER_AUTH_SECRET=<your_generated_secret_1>
INTERNAL_AUTH_SECRET=<your_generated_secret_2>

# Public URLs (update to your domain if behind a reverse proxy)
AUTH_BASE_URL=http://localhost:3001
WEB_ORIGIN=http://localhost:3000
PUBLIC_API_URL=http://localhost:8080
PUBLIC_AUTH_URL=http://localhost:3001
```

---

## 3. Start the Stack

Launch PostgreSQL (TimescaleDB), Redis, migrations, Auth, API, Worker, and the Web Dashboard:

```bash
docker compose up -d
```

Docker Compose will automatically:
1. Start `kinetiq-postgres` (TimescaleDB PG16) and `kinetiq-redis` (Redis 7).
2. Run `kinetiq-migrate` to apply all SQL migrations in `migrations/app/`.
3. Start `kinetiq-auth` (`:3001`), `kinetiq-api` (`:8080`), `kinetiq-worker`, and `kinetiq-web` (`:3000`).

---

## 4. Verify Stack Health

Run `kinetiq doctor` to verify that all services, database connections, and Redis streams are healthy:

```bash
kinetiq doctor
```

You can also check container status and HTTP health endpoints directly:

```bash
docker compose ps
curl -s http://localhost:8080/readyz
curl -s http://localhost:3001/healthz
```

---

## 5. Create Your First Project & API Key

1. Open **[http://localhost:3000](http://localhost:3000)** in your browser.
2. Register your administrator account.
3. Create a new project (e.g., `My App`).
4. Navigate to **Project Settings → API Keys** and copy your project key (`tlk_live_...`).

---

## 6. Send Your First Event

Test ingestion immediately using `curl`:

```bash
curl -X POST http://localhost:8080/api/v1/ingest \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '[
    {
      "event": "quickstart_test",
      "user_id": "user_001",
      "properties": {
        "source": "curl"
      }
    }
  ]'
```

Return to **[http://localhost:3000](http://localhost:3000)** and open the **Live Stream** or **Overview** tab—your `quickstart_test` event will appear in real time.

---

## Next Steps

- Integrate `@agpprastyo/kinetiq-sdk` into your app: [SDK & Ingestion Guide](sdk.md)
- Set up automated database backups: [Backup & Restore Guide](operations/backup-restore.md)
- Learn how to upgrade safely: [Upgrade Guide](operations/upgrade.md)
