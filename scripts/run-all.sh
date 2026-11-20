#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Colors for prefixed logs
C_RESET="\033[0m"
C_AUTH="\033[1;35m"   # Magenta
C_API="\033[1;36m"    # Cyan
C_WORKER="\033[1;33m" # Yellow
C_WEB="\033[1;32m"    # Green
C_INFO="\033[1;34m"   # Blue

PIDS=()

cleanup() {
  echo ""
  echo -e "${C_INFO}[SHUTDOWN] Menghentikan seluruh servis (Auth, API, Worker, Web)...${C_RESET}"
  for pid in "${PIDS[@]:-}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  wait 2>/dev/null || true
  echo -e "${C_INFO}[SHUTDOWN] Semua servis aplikasi telah berhenti.${C_RESET}"
}

trap cleanup SIGINT SIGTERM EXIT

echo -e "${C_INFO}==============================================================${C_RESET}"
echo -e "${C_INFO}  Menjalankan Seluruh Stack Aplikasi (Infra + Migrations + Apps)${C_RESET}"
echo -e "${C_INFO}==============================================================${C_RESET}"

# 1. Start Docker Infrastructure
echo -e "${C_INFO}[1/5] Menyalakan PostgreSQL & Redis (Docker Compose)...${C_RESET}"
docker compose up -d

echo -n "      Menunggu PostgreSQL siap..."
until docker exec kinetiq-postgres pg_isready -U kinetiq -d kinetiq >/dev/null 2>&1; do
  echo -n "."
  sleep 1
done
echo " OK!"

echo -n "      Menunggu Redis siap..."
until docker exec kinetiq-redis redis-cli ping >/dev/null 2>&1; do
  echo -n "."
  sleep 1
done
echo " OK!"

# 2. Run Auth & App Database Migrations
echo -e "${C_INFO}[2/5] Menjalankan migrasi database (Auth & App)...${C_RESET}"
# Ensure Better Auth tables exist idempotently
docker exec -i kinetiq-postgres psql -U kinetiq -d kinetiq -v ON_ERROR_STOP=0 >/dev/null 2>&1 <<'EOF' || true
SELECT 1 FROM "user" LIMIT 1;
EOF

USER_TABLE_EXISTS=$(docker exec kinetiq-postgres psql -U kinetiq -d kinetiq -tAc "SELECT to_regclass('public.user');" || echo "")
if [ "$USER_TABLE_EXISTS" != "user" ] && [ "$USER_TABLE_EXISTS" != "\"user\"" ]; then
  echo "      Menginisialisasi skema tabel Auth..."
  sed 's/--> statement-breakpoint//g' migrations/auth/0000_red_rumiko_fujikawa.sql | \
    docker exec -i kinetiq-postgres psql -U kinetiq -d kinetiq >/dev/null
fi

export PATH="$PATH:$(go env GOPATH)/bin"
if command -v migrate >/dev/null 2>&1; then
  migrate -path migrations/app -database "postgres://kinetiq:kinetiq@localhost:5432/kinetiq?sslmode=disable" up || true
else
  echo "      [WARN] CLI 'migrate' tidak ditemukan di PATH, melewati migrate CLI."
fi

# 3. Ensure dependencies are installed
echo -e "${C_INFO}[3/5] Memeriksa dependensi paket...${C_RESET}"
if [ ! -d "apps/auth/node_modules" ]; then
  echo "      Menginstal dependensi apps/auth..."
  (cd apps/auth && bun install)
fi
if [ ! -d "apps/web/node_modules" ]; then
  echo "      Menginstal dependensi apps/web..."
  (cd apps/web && bun install)
fi

# 4. Free up ports if old orphan processes exist
for port in 3001 8080 5173; do
  if command -v fuser >/dev/null 2>&1; then
    fuser -k "${port}/tcp" >/dev/null 2>&1 || true
  fi
done

# 5. Launch all 4 services with prefixed output
echo -e "${C_INFO}[4/5] Menjalankan ke-4 servis secara paralel...${C_RESET}"

(
  cd "$ROOT_DIR/apps/auth"
  PORT=3001 bun run src/index.ts 2>&1 | sed -u "s/^/$(printf "${C_AUTH}[AUTH   :3001]${C_RESET} ")/"
) &
PIDS+=($!)

(
  cd "$ROOT_DIR/apps/api"
  go run cmd/api/main.go 2>&1 | sed -u "s/^/$(printf "${C_API}[API    :8080]${C_RESET} ")/"
) &
PIDS+=($!)

(
  cd "$ROOT_DIR/apps/api"
  go run cmd/worker/main.go 2>&1 | sed -u "s/^/$(printf "${C_WORKER}[WORKER      ]${C_RESET} ")/"
) &
PIDS+=($!)

(
  cd "$ROOT_DIR/apps/web"
  bun run dev --port 5173 2>&1 | sed -u "s/^/$(printf "${C_WEB}[WEB    :5173]${C_RESET} ")/"
) &
PIDS+=($!)

echo -e "${C_INFO}[5/5] Semua servis telah diluncurkan!${C_RESET}"
echo -e "${C_INFO}--------------------------------------------------------------${C_RESET}"
echo -e "  🌐 Web Dashboard : http://localhost:5173"
echo -e "  ⚡ Go API Server : http://localhost:8080  (Swagger: /swagger/index.html)"
echo -e "  🔐 Auth Service  : http://localhost:3001  (Docs:    /docs)"
echo -e "  ⚙️  Worker Daemon : Active (Redis Stream 'events:stream')"
echo -e "  🛑 Tekan Ctrl+C untuk menghentikan seluruh servis sekaligus."
echo -e "${C_INFO}--------------------------------------------------------------${C_RESET}"

wait
