CREATE TABLE IF NOT EXISTS events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    event_name TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    distinct_id TEXT NULL,
    properties JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT uq_project_event_id UNIQUE (project_id, event_id)
);

CREATE INDEX IF NOT EXISTS idx_events_project_occurred
ON events(project_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_events_project_name_occurred
ON events(project_id, event_name, occurred_at DESC);

CREATE TABLE IF NOT EXISTS event_dead_letters (
    id BIGSERIAL PRIMARY KEY,
    project_id UUID NULL REFERENCES projects(id) ON DELETE SET NULL,
    event_id TEXT NULL,
    reason TEXT NOT NULL,
    payload JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 1,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
