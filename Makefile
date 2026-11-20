.PHONY: help run dev dev-all stop status seed run-auth run-api run-worker run-web infra-up infra-down clean test lint pentest security

help:
	@echo "================================================================="
	@echo "  Perintah Makefile yang Tersedia:"
	@echo "================================================================="
	@echo "  make run          - Jalankan SELURUH project (DB + Redis + Migrasi + Auth + API + Worker + Web)"
	@echo "  make stop         - Hentikan semua proses aplikasi (Auth, API, Worker, Web)"
	@echo "  make status       - Cek status kesehatan (healthcheck) seluruh servis"
	@echo "  make seed         - Buat akun & project demo beserta sample data"
	@echo "  make infra-up     - Nyalakan container PostgreSQL & Redis saja"
	@echo "  make infra-down   - Matikan container PostgreSQL & Redis"
	@echo "  make migrate-up   - Jalankan migrasi database SQL"
	@echo "  make test         - Jalankan seluruh unit & component test"
	@echo "  make lint         - Jalankan seluruh linter & typecheck (Go + Auth + Web)"
	@echo "  make security     - Jalankan automated security audit regression suite"
	@echo "  make docs-dev     - Jalankan portal web dokumentasi Astro Starlight (:4321)"
	@echo "  make docs-build   - Build web portal dokumentasi produksi"
	@echo "================================================================="

# Menjalankan seluruh project dalam 1 perintah (Ctrl+C untuk mematikan semua)
run:
	@chmod +x scripts/run-all.sh
	@bash scripts/run-all.sh

dev-all: run

# Menjalankan servis secara individual jika dibutuhkan
run-auth:
	@cd apps/auth && PORT=3001 bun run src/index.ts

run-api:
	@cd apps/api && go run cmd/api/main.go

run-worker:
	@cd apps/api && go run cmd/worker/main.go

run-web:
	@cd apps/web && bun run dev --port 5173

# Menghentikan seluruh servis aplikasi yang berjalan di background/port
stop:
	@echo "Menghentikan servis pada port 3001, 8080, 5173 dan worker..."
	@-fuser -k 3001/tcp 2>/dev/null || true
	@-fuser -k 8080/tcp 2>/dev/null || true
	@-fuser -k 5173/tcp 2>/dev/null || true
	@-pkill -f "cmd/worker/main.go" 2>/dev/null || true
	@echo "Seluruh servis aplikasi dihentikan."

# Cek status kesehatan seluruh komponen
status:
	@echo "=== Status Infrastruktur & Servis ==="
	@docker ps --filter "name=kinetiq-" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
	@echo ""
	@printf "Auth Service (:3001) : " && (curl -s http://localhost:3001/healthz || echo "OFFLINE")
	@printf "API Server   (:8080) : " && (curl -s http://localhost:8080/readyz || echo "OFFLINE")
	@printf "Web UI       (:5173) : " && (curl -s -o /dev/null -w "HTTP %{http_code}\n" http://localhost:5173 || echo "OFFLINE")

seed:
	@cd apps/api && go run cmd/seed/main.go

infra-up:
	docker compose up -d

infra-down:
	docker compose down

dev: infra-up
	@echo "Local infrastructure running. Postgres: 5432, Redis: 6379"
	@echo "Tip: Gunakan 'make run' untuk menjalankan seluruh aplikasi sekaligus."

MIGRATE_CMD = export PATH=$$PATH:$$(go env GOPATH)/bin && migrate -path migrations/app -database "postgres://kinetiq:kinetiq@localhost:5432/kinetiq?sslmode=disable"
SQLC_CMD = export PATH=$$PATH:$$(go env GOPATH)/bin && sqlc
SWAG_CMD = export PATH=$$PATH:$$(go env GOPATH)/bin && swag

migrate-up:
	@$(MIGRATE_CMD) up

migrate-down:
	@$(MIGRATE_CMD) down 1

generate:
	@echo "Generating sqlc queries..."
	@$(SQLC_CMD) generate
	@echo "Generating swagger docs..."
	@cd apps/api && $(SWAG_CMD) init -g internal/app/app.go -o docs/swagger

test: test-unit test-component test-sdk test-web

test-unit:
	@echo "Running Unit Tests..."
	@cd apps/api && go test -v ./internal/...

test-component:
	@echo "Running Component & Integration Tests..."
	@cd apps/auth && bun test
	@cd apps/api && go test -v ./tests/...

test-sdk:
	@echo "Running SDK Tests..."
	@cd packages/sdk && bun test

test-web:
	@echo "Running Web Frontend Tests..."
	@cd apps/web && bun test

test-coverage:
	@echo "Checking test coverage..."
	@bash scripts/check-coverage.sh

test-clean-install:
	@bash scripts/verify-clean-install.sh

# Target untuk E2E AI Agent
e2e-up:
	docker compose -f docker-compose.e2e.yml up -d --build --wait

e2e-down:
	docker compose -f docker-compose.e2e.yml down -v --remove-orphans

security:
	@bash scripts/run-security-audit.sh

pentest: security

loadtest-seed:
	@bash loadtest/setup-seed.sh

loadtest-smoke:
	@VUS=10 DURATION=30s bash loadtest/run-k6.sh ingest

loadtest:
	@bash loadtest/run-k6.sh ingest
	@bash loadtest/run-k6.sh analytics
	@bash loadtest/run-k6.sh mixed

benchmark: loadtest-seed
	@echo "Running Ingestion Benchmark (10 VUs smoke)..."
	@VUS=10 DURATION=30s bash loadtest/run-k6.sh ingest
	@echo "Running Analytics Benchmark (Warm Cache)..."
	@MODE=warm VUS=10 DURATION=30s bash loadtest/run-k6.sh analytics
	@echo "Running Analytics Benchmark (Cold Cache)..."
	@MODE=cold VUS=10 DURATION=30s bash loadtest/run-k6.sh analytics
	@echo "Running Mixed Traffic Benchmark (10 VUs)..."
	@VUS=10 DURATION=30s bash loadtest/run-k6.sh mixed

lint:
	@echo "Running linters..."
	@cd apps/api && go vet ./...
	@cd apps/api && test -z "$$(gofmt -l .)" || (echo "Unformatted Go files found:" && gofmt -l . && exit 1)
	@echo "Go lint passed."
	@echo "Running Auth TypeScript typecheck..."
	@cd apps/auth && bunx tsc --noEmit
	@echo "TypeScript typecheck passed."
	@echo "Running Web SvelteKit check & build..."
	@cd apps/web && bun run check && bun run build
	@echo "Web check passed."

test-e2e:
	@echo "Running Security & API Regression E2E Suite..."
	@bash scripts/run-e2e-pntc.sh
	@echo "Running Interactive Browser E2E Suite via Orca CLI..."
	@bash scripts/run-browser-e2e.sh

clean:
	docker compose down -v

# Portal Web Dokumentasi Resmi (Astro + Starlight Bilingual)
docs-dev:
	@cd apps/docs && bun run dev

docs-build:
	@cd apps/docs && bun run build

docs-preview:
	@cd apps/docs && bun run preview


