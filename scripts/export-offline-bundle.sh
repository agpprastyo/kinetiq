#!/usr/bin/env bash
set -euo pipefail
TAG="${1:-latest}"
OUT="kinetiq-offline-bundle-${TAG}.tar"
IMAGES=(ghcr.io/agpprastyo/kinetiq-api:${TAG} ghcr.io/agpprastyo/kinetiq-auth:${TAG} ghcr.io/agpprastyo/kinetiq-web:${TAG} ghcr.io/agpprastyo/kinetiq-worker:${TAG})
docker save "${IMAGES[@]}" -o "$OUT"
sha256sum "$OUT" > "${OUT}.sha256"
echo "Created $OUT"
