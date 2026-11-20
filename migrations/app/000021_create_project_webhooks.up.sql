CREATE TABLE IF NOT EXISTS project_webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    target_url TEXT NOT NULL,
    secret_key_enc TEXT NOT NULL,
    event_filter JSONB NOT NULL DEFAULT '{"all": true}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    status VARCHAR(20) NOT NULL DEFAULT 'healthy',
    failure_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhooks_project_id ON project_webhooks(project_id) WHERE is_active = true;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES project_webhooks(id) ON DELETE CASCADE,
    event_id VARCHAR(100) NOT NULL,
    event_name VARCHAR(100) NOT NULL,
    status_code INT,
    response_time_ms INT,
    error_message TEXT,
    attempt INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_created
ON webhook_deliveries (webhook_id, created_at DESC);
