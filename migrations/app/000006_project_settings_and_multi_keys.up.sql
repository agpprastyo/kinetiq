-- 1. Deduplicate any pre-existing projects with identical names under the same owner
WITH ranked AS (
    SELECT id, name,
        ROW_NUMBER() OVER (PARTITION BY owner_user_id, LOWER(TRIM(name)) ORDER BY created_at ASC) AS rn
    FROM projects
)
UPDATE projects p
SET name = p.name || ' (' || SUBSTRING(p.id::text FROM 1 FOR 4) || ')'
FROM ranked r
WHERE p.id = r.id AND r.rn > 1;

-- 2. Enforce case-insensitive unique project name per owner
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_owner_unique_name
ON projects (owner_user_id, LOWER(TRIM(name)));

-- 3. Add per-project configuration settings
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS retention_days INT NOT NULL DEFAULT 90,
    ADD COLUMN IF NOT EXISTS allowed_origins TEXT NOT NULL DEFAULT '*',
    ADD COLUMN IF NOT EXISTS rate_limit_per_sec INT NOT NULL DEFAULT 500,
    ADD COLUMN IF NOT EXISTS anonymize_ip BOOLEAN NOT NULL DEFAULT true;

-- 4. Remove single-active-key restriction to support Multi-API Keys per project
DROP INDEX IF EXISTS idx_project_api_keys_one_active;

-- 5. Add metadata columns to project_api_keys
ALTER TABLE project_api_keys
    ADD COLUMN IF NOT EXISTS name VARCHAR(100) NOT NULL DEFAULT 'Default Live Key',
    ADD COLUMN IF NOT EXISTS environment VARCHAR(32) NOT NULL DEFAULT 'live' CHECK (environment IN ('live', 'test')),
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL;
