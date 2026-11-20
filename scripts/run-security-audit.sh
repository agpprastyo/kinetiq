#!/usr/bin/env bash
# scripts/run-security-audit.sh
# Automated Security Verification Suite for Kinetiq (kinetiq-cloud)

set -e

echo "================================================================="
echo "  [Kinetiq Security] Running Security Audit & Verification Suite   "
echo "================================================================="

# 1. Verify apps/auth dependencies
if [ ! -d "apps/auth/node_modules" ]; then
  echo "Installing apps/auth dependencies via bun..."
  cd apps/auth && bun install && cd ../..
fi

# 2. Run automated security regression test suite
echo ""
echo "-> [Step 1/2] Executing Access Control & Token Leakage Test Suite (apps/auth)..."
cd apps/auth
bun test src/security-audit.test.ts
cd ../..

# 3. Check for exposed ops/internal routes in web client code
echo ""
echo "-> [Step 2/2] Scanning Web Client for Leaked Internal /ops Endpoints..."
EXPOSED_OPS=$(grep -rn "api/v1/ops" apps/web/src/routes/app || true)
if [ -n "$EXPOSED_OPS" ]; then
  echo "[WARNING] Found references to internal /ops endpoints inside client workspace routes (apps/web/src/routes/app):"
  echo "$EXPOSED_OPS"
  echo ""
  echo "Resolution: Ensure /ops endpoints are removed from client workspace settings (Issue #45)."
else
  echo "[PASS] No internal /ops endpoints exposed in client workspace routes."
fi

echo ""
echo "================================================================="
echo "  [Kinetiq Security] Audit Complete. Check results above.          "
echo "================================================================="
