#!/bin/sh
set -e

echo "=== Running PostgreSQL initialization scripts for Kinetiq ==="

# 1. Run Better-Auth migrations
if [ -f "/docker-entrypoint-initdb.d/auth/0000_red_rumiko_fujikawa.sql" ]; then
  echo "Applying Better-Auth baseline schema..."
  sed 's/--> statement-breakpoint//g' /docker-entrypoint-initdb.d/auth/0000_red_rumiko_fujikawa.sql | psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB"
fi

# 2. Run App migrations in numeric order
if [ -d "/docker-entrypoint-initdb.d/app" ]; then
  echo "Applying App schema migrations..."
  for sql_file in $(ls -1 /docker-entrypoint-initdb.d/app/*.up.sql | sort); do
    echo "Applying $sql_file..."
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$sql_file"
  done
fi

echo "=== PostgreSQL initialization complete ==="
