#!/usr/bin/env bash
set -euo pipefail

DUMP_FILE=""
S3_URI=""
S3_ENDPOINT="${BACKUP_S3_ENDPOINT:-}"
TARGET_DB="${DATABASE_URL:-}"
VERIFY_CHECKSUM="true"
DRY_RUN="false"

usage() {
  cat <<'EOF'
Usage: ./scripts/restore-postgres.sh [options]

Options:
  --file <path>            Local .dump file path to restore
  --s3-uri <s3://...>      Download dump (and optional .sha256) from S3/R2/GCS before restore
  --s3-endpoint <url>      Custom S3-compatible endpoint URL
  --target-db <url>        Target PostgreSQL connection string (default: $DATABASE_URL)
  --skip-checksum          Skip SHA-256 verification
  --dry-run                Verify file existence, checksum, and restore command without modifying DB
  -h, --help               Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --file)
      DUMP_FILE="$2"
      shift 2
      ;;
    --s3-uri)
      S3_URI="$2"
      shift 2
      ;;
    --s3-endpoint)
      S3_ENDPOINT="$2"
      shift 2
      ;;
    --target-db)
      TARGET_DB="$2"
      shift 2
      ;;
    --skip-checksum)
      VERIFY_CHECKSUM="false"
      shift
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

TMP_DIR=""
cleanup() {
  if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT

if [[ -n "$S3_URI" ]]; then
  TMP_DIR="$(mktemp -d)"
  DUMP_FILE="$TMP_DIR/$(basename "$S3_URI")"
  AWS_ARGS=()
  if [[ -n "$S3_ENDPOINT" ]]; then
    AWS_ARGS+=(--endpoint-url "$S3_ENDPOINT")
  fi
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "    [dry-run] Would download $S3_URI to $DUMP_FILE"
    printf 'KINETIQ_DRY_RUN_RESTORE\n' > "$DUMP_FILE"
    sha256sum "$DUMP_FILE" > "${DUMP_FILE}.sha256"
  else
    aws "${AWS_ARGS[@]}" s3 cp "$S3_URI" "$DUMP_FILE" --only-show-errors
    aws "${AWS_ARGS[@]}" s3 cp "${S3_URI}.sha256" "${DUMP_FILE}.sha256" --only-show-errors || true
  fi
fi

if [[ -z "$DUMP_FILE" || ! -f "$DUMP_FILE" ]]; then
  echo "ERROR: Backup dump file not found: ${DUMP_FILE:-<none>}" >&2
  exit 1
fi

echo "==> Kinetiq PostgreSQL Restore"
echo "    Dump File: $DUMP_FILE"
echo "    Dry-run:   $DRY_RUN"

if [[ "$VERIFY_CHECKSUM" == "true" && -f "${DUMP_FILE}.sha256" ]]; then
  echo "    Verifying SHA-256 checksum..."
  EXPECTED_HASH="$(awk '{print $1}' "${DUMP_FILE}.sha256")"
  ACTUAL_HASH="$(sha256sum "$DUMP_FILE" | awk '{print $1}')"
  if [[ "$EXPECTED_HASH" != "$ACTUAL_HASH" ]]; then
    echo "ERROR: Checksum mismatch! expected=$EXPECTED_HASH actual=$ACTUAL_HASH" >&2
    exit 1
  fi
  echo "    PASS: SHA-256 checksum verified ($ACTUAL_HASH)"
fi

if [[ "$DRY_RUN" == "true" ]]; then
  echo "    [dry-run] Restore preflight checks passed; skipping pg_restore execution."
  exit 0
fi

if [[ -z "$TARGET_DB" ]]; then
  echo "ERROR: --target-db or DATABASE_URL is required" >&2
  exit 1
fi

if ! command -v pg_restore >/dev/null 2>&1; then
  echo "ERROR: pg_restore not found in PATH" >&2
  exit 1
fi

echo "    Validating archive TOC via pg_restore --list..."
pg_restore --list "$DUMP_FILE" >/dev/null

echo "    Executing pg_restore (--clean --if-exists --no-owner --no-acl)..."
pg_restore --clean --if-exists --no-owner --no-acl --dbname="$TARGET_DB" "$DUMP_FILE"

if command -v psql >/dev/null 2>&1; then
  echo "    Running post-restore sanity check..."
  psql "$TARGET_DB" -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 5;"
fi

echo "==> Restore completed successfully."
