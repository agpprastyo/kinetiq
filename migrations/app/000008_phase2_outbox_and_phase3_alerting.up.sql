-- 1. Create auth_email_outbox table for transactional mailer persistence
CREATE TABLE IF NOT EXISTS auth_email_outbox (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    url TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'outbox',
    delivery_status TEXT NOT NULL DEFAULT 'stored',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_email_outbox_created_at ON auth_email_outbox (created_at DESC);

-- 2. Extend alert_rules with multi-channel support (Slack, Discord, Telegram, Webhook)
ALTER TABLE alert_rules
    ADD COLUMN IF NOT EXISTS channel_type VARCHAR(32) NOT NULL DEFAULT 'webhook',
    ADD COLUMN IF NOT EXISTS telegram_chat_id VARCHAR(100) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS cooldown_minutes INT NOT NULL DEFAULT 15,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
