#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/kinetiq}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
BACKUP_S3_BUCKET="${BACKUP_S3_BUCKET:-}"
BACKUP_S3_PREFIX="${BACKUP_S3_PREFIX:-kinetiq-backups}"
BACKUP_S3_ENDPOINT="${BACKUP_S3_ENDPOINT:-}"
DATABASE_URL="${DATABASE_URL:-}"
DRY_RUN="false"

usage() {
  cat <<'EOF'
Usage: ./scripts/backup-postgres.sh [options]

Options:
  --database-url <url>      PostgreSQL connection string (default: $DATABASE_URL)
  --local-dir <dir>         Local backup staging/retention directory (default: /var/backups/kinetiq)
  --retention-days <days>   Delete backups older than N days (default: 14)
  --s3-bucket <bucket>      Target S3/R2/GCS bucket name (optional; local-only if omitted)
  --s3-prefix <prefix>      Key prefix inside bucket (default: kinetiq-backups)
  --s3-endpoint <url>       Custom S3-compatible endpoint URL (for Cloudflare R2 / MinIO / GCS)
  --dry-run                 Simulate dump, checksum, upload, and retention without requiring live DB/S3
  -h, --help                Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --database-url)
      DATABASE_URL="$2"
      shift 2
      ;;
    --local-dir)
      BACKUP_DIR="$2"
      shift 2
      ;;
    --retention-days)
      BACKUP_RETENTION_DAYS="$2"
      shift 2
      ;;
    --s3-bucket)
      BACKUP_S3_BUCKET="$2"
      shift 2
      ;;
    --s3-prefix)
      BACKUP_S3_PREFIX="$2"
      shift 2
      ;;
    --s3-endpoint)
      BACKUP_S3_ENDPOINT="$2"
      shift 2
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

if ! [[ "$BACKUP_RETENTION_DAYS" =~ ^[0-9]+$ ]]; then
  echo "ERROR: --retention-days must be a non-negative integer, got: $BACKUP_RETENTION_DAYS" >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"

TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
DUMP_FILE="$BACKUP_DIR/kinetiq-${TIMESTAMP}.dump"
CHECKSUM_FILE="${DUMP_FILE}.sha256"

echo "==> Kinetiq PostgreSQL Backup"
echo "    Timestamp:      $TIMESTAMP"
echo "    Local Dir:      $BACKUP_DIR"
echo "    Retention Days: $BACKUP_RETENTION_DAYS"
echo "    Dry-run:        $DRY_RUN"

if [[ "$DRY_RUN" == "true" ]]; then
  printf 'KINETIQ_DRY_RUN_PGDUMP_%s\n' "$TIMESTAMP" > "$DUMP_FILE"
  echo "    [dry-run] Created simulated dump: $DUMP_FILE"
else
  if [[ -z "$DATABASE_URL" ]]; then
    echo "ERROR: DATABASE_URL is required (or pass --database-url)" >&2
    exit 1
  fi
  if ! command -v pg_dump >/dev/null 2>&1; then
    echo "ERROR: pg_dump not found in PATH" >&2
    exit 1
  fi
  pg_dump --format=custom --compress=9 --no-owner --no-acl "$DATABASE_URL" --file="$DUMP_FILE"
  echo "    Created dump: $DUMP_FILE ($(du -h "$DUMP_FILE" | cut -f1))"
fi

sha256sum "$DUMP_FILE" > "$CHECKSUM_FILE"
echo "    Checksum written: $CHECKSUM_FILE"

if [[ -n "$BACKUP_S3_BUCKET" ]]; then
  S3_BASE="s3://${BACKUP_S3_BUCKET}/${BACKUP_S3_PREFIX}"
  AWS_ARGS=()
  if [[ -n "$BACKUP_S3_ENDPOINT" ]]; then
    AWS_ARGS+=(--endpoint-url "$BACKUP_S3_ENDPOINT")
  fi

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "    [dry-run] Would upload $(basename "$DUMP_FILE") and .sha256 to ${S3_BASE}/"
  else
    if ! command -v aws >/dev/null 2>&1; then
      echo "ERROR: aws CLI not found in PATH for S3 upload" >&2
      exit 1
    fi
    aws "${AWS_ARGS[@]}" s3 cp "$DUMP_FILE" "${S3_BASE}/$(basename "$DUMP_FILE")" --only-show-errors
    aws "${AWS_ARGS[@]}" s3 cp "$CHECKSUM_FILE" "${S3_BASE}/$(basename "$CHECKSUM_FILE")" --only-show-errors
    echo "    Uploaded to ${S3_BASE}/"

    # Remote retention rotation
    CUTOFF_EPOCH="$(date -u -d "-${BACKUP_RETENTION_DAYS} days" +%s)"
    aws "${AWS_ARGS[@]}" s3 ls "${S3_BASE}/" | while read -r d_date d_time _ d_name; do
      [[ -z "${d_name:-}" ]] && continue
      [[ "$d_name" != kinetiq-*.dump* ]] && continue
      OBJ_EPOCH="$(date -u -d "${d_date} ${d_time}" +%s 2>/dev/null || echo 0)"
      if [[ "$OBJ_EPOCH" -gt 0 && "$OBJ_EPOCH" -lt "$CUTOFF_EPOCH" ]]; then
        echo "    Pruning remote expired backup: $d_name"
        aws "${AWS_ARGS[@]}" s3 rm "${S3_BASE}/${d_name}" --only-show-errors
      fi
    done
  fi
fi

# Local retention rotation
PRUNED_COUNT=0
while IFS= read -r -d '' expired; do
  rm -f "$expired"
  PRUNED_COUNT=$((PRUNED_COUNT + 1))
done < <(find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'kinetiq-*.dump' -o -name 'kinetiq-*.dump.sha256' \) -mtime +"$BACKUP_RETENTION_DAYS" -print0)

echo "    Local retention rotation complete (pruned: $PRUNED_COUNT files)"
echo "==> Backup completed successfully: $DUMP_FILE"
