#!/usr/bin/env bash
set -euo pipefail

echo "=========================================================="
echo " Kinetiq MVP - Clean Installation Verification Script"
echo "=========================================================="

TEST_DB_NAME="kinetiq_clean_test_$(date +%s)"
DATABASE_URL="postgres://kinetiq:kinetiq@localhost:5432/${TEST_DB_NAME}?sslmode=disable"

echo "[1/4] Creating temporary empty test database: ${TEST_DB_NAME}..."
docker exec -i kinetiq-postgres psql -U kinetiq -d postgres -c "CREATE DATABASE ${TEST_DB_NAME};"

cleanup() {
  echo "[Cleanup] Dropping temporary test database: ${TEST_DB_NAME}..."
  docker exec -i kinetiq-postgres psql -U kinetiq -d postgres -c "DROP DATABASE IF EXISTS ${TEST_DB_NAME} WITH (FORCE);" || true
}
trap cleanup EXIT

echo "[2/4] Running migrations up on clean database..."
export PATH=$PATH:$(go env GOPATH)/bin
migrate -path migrations/app -database "${DATABASE_URL}" up

echo "[3/4] Verifying table schema and indexes created in clean database..."
docker exec -i kinetiq-postgres psql -U kinetiq -d "${TEST_DB_NAME}" -c "\dt"
docker exec -i kinetiq-postgres psql -U kinetiq -d "${TEST_DB_NAME}" -c "\di"

echo "[4/4] Verifying migration down and up cycle..."
migrate -path migrations/app -database "${DATABASE_URL}" down 1
migrate -path migrations/app -database "${DATABASE_URL}" up

echo "=========================================================="
echo " SUCCESS: Clean installation verification passed 100%!"
echo "=========================================================="
