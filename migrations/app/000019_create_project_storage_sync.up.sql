-- Migration 000019: Create project_storage_sync table for automated cloud storage export
CREATE TABLE IF NOT EXISTS project_storage_sync (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL DEFAULT 's3', -- 's3', 'r2', 'gcs', 'minio'
    bucket_name TEXT NOT NULL,
    endpoint_url TEXT NOT NULL DEFAULT '', -- custom endpoint for R2 / MinIO
    region VARCHAR(64) NOT NULL DEFAULT 'us-east-1',
    access_key_id TEXT NOT NULL,
    secret_access_key TEXT NOT NULL,
    prefix_path TEXT NOT NULL DEFAULT 'kinetiq-events',
    frequency VARCHAR(32) NOT NULL DEFAULT 'daily', -- 'hourly', 'daily'
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_sync_at TIMESTAMPTZ DEFAULT NULL,
    last_sync_status VARCHAR(32) NOT NULL DEFAULT 'idle', -- 'idle', 'syncing', 'success', 'failed'
    last_error_message TEXT DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_project_storage UNIQUE (project_id)
);

CREATE INDEX IF NOT EXISTS idx_project_storage_sync_active
ON project_storage_sync (is_enabled, frequency, last_sync_at);
