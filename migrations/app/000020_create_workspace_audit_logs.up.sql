CREATE TABLE IF NOT EXISTS workspace_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id TEXT NOT NULL,
    project_id UUID NULL REFERENCES projects(id) ON DELETE SET NULL,
    actor_id TEXT NOT NULL,
    actor_name TEXT,
    actor_email TEXT,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workspace_audit_logs_workspace_created
ON workspace_audit_logs (workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_workspace_audit_logs_project_created
ON workspace_audit_logs (project_id, created_at DESC)
WHERE project_id IS NOT NULL;
