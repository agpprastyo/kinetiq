DROP INDEX IF EXISTS idx_auth_email_outbox_dispatch;

ALTER TABLE auth_email_outbox
DROP COLUMN IF EXISTS retry_count,
DROP COLUMN IF EXISTS max_retries,
DROP COLUMN IF EXISTS next_retry_at,
DROP COLUMN IF EXISTS sent_at;
