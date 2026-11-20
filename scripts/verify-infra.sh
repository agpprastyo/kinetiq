#!/usr/bin/env bash
set -euo pipefail

echo "=== Verifying Docker Infrastructure Health ==="

# Check Postgres container status
echo -n "Checking Postgres container health... "
for i in {1..30}; do
  status=$(docker inspect --format='{{json .State.Health.Status}}' kinetiq-postgres 2>/dev/null || echo "null")
  if [ "$status" = "\"healthy\"" ]; then
    echo "OK (Healthy)"
    break
  fi
  sleep 1
  if [ "$i" -eq 30 ]; then
    echo "FAILED: Postgres container is not healthy: $status"
    exit 1
  fi
done

# Check Redis container status
echo -n "Checking Redis container health... "
for i in {1..30}; do
  status=$(docker inspect --format='{{json .State.Health.Status}}' kinetiq-redis 2>/dev/null || echo "null")
  if [ "$status" = "\"healthy\"" ]; then
    echo "OK (Healthy)"
    break
  fi
  sleep 1
  if [ "$i" -eq 30 ]; then
    echo "FAILED: Redis container is not healthy: $status"
    exit 1
  fi
done

echo "=== All infrastructure services are UP and HEALTHY ==="
