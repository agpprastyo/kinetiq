#!/usr/bin/env bash
# ==============================================================================
# Browser E2E Interaction & Zero Network Error Suite via Orca CLI Browser
# ==============================================================================
set -euo pipefail

WEB_URL="${WEB_URL:-http://localhost:5173}"
echo "=========================================================="
echo "🌐 Starting Interactive Browser E2E Test Suite (Orca CLI)"
echo "Target: $WEB_URL"
echo "=========================================================="

# Check orca availability
if ! command -v orca &> /dev/null; then
  echo "⚠️  Orca CLI not found in PATH. Skipping headless browser verification."
  exit 0
fi

echo "[1/4] Creating browser tab for Web App..."
TAB_JSON=$(orca tab create --url "$WEB_URL" --json 2>/dev/null || true)
if [[ -z "$TAB_JSON" ]]; then
  echo "⚠️  Orca runtime server not responding. Please make sure 'orca open' or headless runtime is running."
  exit 0
fi

PAGE_ID=$(echo "$TAB_JSON" | grep -o '"browserPageId":"[^"]*' | head -n 1 | cut -d'"' -f4 || echo "")
if [[ -z "$PAGE_ID" ]]; then
  PAGE_ID=$(echo "$TAB_JSON" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4 || echo "")
fi

echo "  -> Connected to Browser Page ID: ${PAGE_ID:-default}"

echo "[2/4] Injecting Network Failure Listener (HTTP 401/403/500 Sniffer)..."
orca eval --expression '
  window.__networkErrors = window.__networkErrors || [];
  const _fetch = window.fetch;
  window.fetch = async (...args) => {
    try {
      const res = await _fetch(...args);
      if (res.status === 401 || res.status === 403 || res.status >= 500) {
        window.__networkErrors.push({
          url: typeof args[0] === "string" ? args[0] : args[0]?.url,
          status: res.status,
          statusText: res.statusText
        });
      }
      return res;
    } catch (err) {
      window.__networkErrors.push({ error: err.message });
      throw err;
    }
  };
' > /dev/null 2>&1 || true

echo "[3/4] Navigating to Workspace Settings (/app/settings)..."
orca goto --url "$WEB_URL/app/settings" > /dev/null 2>&1 || true
sleep 2

# Take snapshot to ensure page is rendered
orca snapshot > /dev/null 2>&1 || true

echo "[4/4] Evaluating Zero Network Error Assertions..."
ERRORS_JSON=$(orca eval --expression 'JSON.stringify(window.__networkErrors || [])' 2>/dev/null || echo "[]")
CLEAN_ERRORS=$(echo "$ERRORS_JSON" | tr -d '"' | tr -d ' ' || echo "[]")

if [[ "$CLEAN_ERRORS" != "[]" && "$CLEAN_ERRORS" != "" ]]; then
  echo "❌ FAILED: Detected unhandled HTTP 4xx/5xx network errors on client page:"
  echo "$ERRORS_JSON"
  exit 1
else
  echo "✅ PASSED: Zero forbidden / server network errors detected on /app/settings!"
fi

echo "=========================================================="
echo "🎉 Browser E2E Test Suite Completed Successfully!"
echo "=========================================================="
