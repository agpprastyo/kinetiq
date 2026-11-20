-- Migration 000012: Platform Superadmin Audit Log
CREATE TABLE IF NOT EXISTS platform_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id TEXT NOT NULL,
    actor_email TEXT,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    details JSONB DEFAULT '{}'::jsonb,
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_platform_audit_logs_created_at ON platform_audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_platform_audit_logs_actor ON platform_audit_logs (actor_id);
CREATE INDEX IF NOT EXISTS idx_platform_audit_logs_target ON platform_audit_logs (target_type, target_id);
