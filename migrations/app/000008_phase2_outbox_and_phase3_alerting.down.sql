ALTER TABLE alert_rules
    DROP COLUMN IF EXISTS channel_type,
    DROP COLUMN IF EXISTS telegram_chat_id,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS cooldown_minutes,
    DROP COLUMN IF EXISTS updated_at;

DROP TABLE IF EXISTS auth_email_outbox;
