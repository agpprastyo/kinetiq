#!/usr/bin/env bash
set -euo pipefail

SEED_FILE="${1:-loadtest/.env.loadtest}"

echo "Provisioning load test seed project..."
cd "$(dirname "$0")/../apps/api"

OUTPUT=$(go run cmd/seed/main.go)

cd ../..

echo "$OUTPUT" > "$SEED_FILE"

echo "Load test credentials generated and saved to $SEED_FILE:"
cat "$SEED_FILE"
