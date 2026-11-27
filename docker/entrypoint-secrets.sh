#!/bin/sh
set -e

SECRETS_FILE="/run/kinetiq-secrets/env"

gen_secret() {
  head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
}

is_weak() {
  case "$1" in
    ""|changeme_*) return 0 ;;
    *) return 1 ;;
  esac
}

# If running as kinetiq-init (no args), generate secrets file if needed.
if [ "$#" -eq 0 ]; then
  mkdir -p /run/kinetiq-secrets
  if [ -f "$SECRETS_FILE" ]; then
    # shellcheck disable=SC1090
    . "$SECRETS_FILE"
  fi

  if is_weak "${BETTER_AUTH_SECRET:-}"; then
    BETTER_AUTH_SECRET="$(gen_secret)"
  fi
  if is_weak "${INTERNAL_AUTH_SECRET:-}"; then
    INTERNAL_AUTH_SECRET="$(gen_secret)"
  fi

  cat > "$SECRETS_FILE" <<EOF
BETTER_AUTH_SECRET=$BETTER_AUTH_SECRET
INTERNAL_AUTH_SECRET=$INTERNAL_AUTH_SECRET
AUTH_INTERNAL_SECRET=$INTERNAL_AUTH_SECRET
EOF
  chmod 644 "$SECRETS_FILE"
  exit 0
fi

# For app containers: load generated secrets if env vars are unset or weak.
if [ -f "$SECRETS_FILE" ]; then
  # shellcheck disable=SC1090
  . "$SECRETS_FILE"
  export BETTER_AUTH_SECRET INTERNAL_AUTH_SECRET AUTH_INTERNAL_SECRET
fi

exec "$@"
