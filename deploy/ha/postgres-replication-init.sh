#!/bin/sh
# Creates the replication role and allows it in pg_hba.conf (runs once on primary init).
set -e
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -c "CREATE ROLE replicator WITH REPLICATION LOGIN PASSWORD '$REPLICATION_PASSWORD'"
echo "host replication replicator all scram-sha-256" >> "$PGDATA/pg_hba.conf"
