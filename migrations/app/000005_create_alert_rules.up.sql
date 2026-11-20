CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    event_name VARCHAR(100) NOT NULL,
    condition VARCHAR(50) NOT NULL CHECK (condition IN ('spike_gt', 'drop_lt')),
    threshold_count BIGINT NOT NULL,
    window_minutes INT NOT NULL DEFAULT 5,
    webhook_url TEXT NOT NULL,
    webhook_secret TEXT NOT NULL,
    last_triggered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
