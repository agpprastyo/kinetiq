#!/usr/bin/env bash
set -euo pipefail

SCENARIO="${1:-ingest}"
VUS="${VUS:-10}"
DURATION="${DURATION:-30s}"
ENV_FILE="loadtest/.env.loadtest"

if [ ! -f "$ENV_FILE" ]; then
  echo "Seed environment file not found at $ENV_FILE. Running setup-seed.sh..."
  ./loadtest/setup-seed.sh
fi

export $(grep -v '^#' "$ENV_FILE" | xargs)

SCRIPT_PATH="loadtest/scenarios/${SCENARIO}.js"
if [ ! -f "$SCRIPT_PATH" ]; then
  echo "Error: Scenario script $SCRIPT_PATH does not exist."
  echo "Available scenarios: ingest, analytics, mixed"
  exit 1
fi

echo "============================================================"
echo "Running k6 Scenario: $SCENARIO"
echo "VUs: $VUS | Duration: $DURATION"
echo "Project ID: $PROJECT_ID"
echo "============================================================"

docker run --rm -i \
  --network=host \
  -v "$(pwd):/workspace" \
  -w /workspace \
  -e BASE_URL="http://127.0.0.1:8080" \
  -e PROJECT_ID="$PROJECT_ID" \
  -e API_KEY="$API_KEY" \
  -e COOKIE="$COOKIE" \
  -e VUS="$VUS" \
  -e DURATION="$DURATION" \
  -e MODE="${MODE:-warm}" \
  grafana/k6:latest run "$SCRIPT_PATH"
