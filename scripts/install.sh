#!/usr/bin/env bash
set -euo pipefail

OFFLINE=""
BUNDLE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --offline) OFFLINE=1; BUNDLE="$2"; shift 2;;
    *) echo "Unknown arg: $1"; exit 1;;
  esac
done

echo "=== Kinetiq Self-Hosted Installer ==="

# Preflight
if command -v docker >/dev/null 2>&1; then RUNTIME=docker; elif command -v podman >/dev/null 2>&1; then RUNTIME=podman; else echo "Docker/Podman required"; exit 1; fi
echo "Runtime: $RUNTIME"

# disk check
AVAIL_KB=$(df -k . | awk 'NR==2{print $4}')
if [[ $AVAIL_KB -lt $((20*1024*1024)) ]]; then echo "Need >=20GB disk"; exit 1; fi
# ram check
MEM_KB=$(grep MemTotal /proc/meminfo | awk '{print $2}')
if [[ $MEM_KB -lt $((2*1024*1024)) ]]; then echo "Need >=2GB RAM"; exit 1; fi

# port check
for p in 80 443 8080; do
  if ss -ltn 2>/dev/null | grep -q ":$p "; then echo "Port $p in use"; exit 1; fi
done

if [[ -n "$OFFLINE" ]]; then
  echo "Offline mode: loading bundle $BUNDLE"
  $RUNTIME load -i "$BUNDLE"
else
  echo "Pulling images..."
  $RUNTIME compose pull
fi

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Generated .env from .env.example"
fi

$RUNTIME compose up -d
echo "Kinetiq started. Open http://localhost to complete setup."
