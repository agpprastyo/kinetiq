ALTER TABLE auth_email_outbox
ADD COLUMN IF NOT EXISTS retry_count INT NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS max_retries INT NOT NULL DEFAULT 5,
ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ DEFAULT NOW(),
ADD COLUMN IF NOT EXISTS sent_at TIMESTAMPTZ DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_auth_email_outbox_dispatch 
ON auth_email_outbox (delivery_status, next_retry_at) 
WHERE delivery_status IN ('stored', 'retrying');
