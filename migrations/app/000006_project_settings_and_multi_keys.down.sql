ALTER TABLE project_api_keys
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS environment,
    DROP COLUMN IF EXISTS name;

-- Keep only the newest active key per project before restoring the single-key index
WITH ranked_keys AS (
    SELECT id,
        ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY created_at DESC) AS rn
    FROM project_api_keys
    WHERE revoked_at IS NULL
)
UPDATE project_api_keys k
SET revoked_at = NOW()
FROM ranked_keys rk
WHERE k.id = rk.id AND rk.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_project_api_keys_one_active
ON project_api_keys(project_id)
WHERE revoked_at IS NULL;

ALTER TABLE projects
    DROP COLUMN IF EXISTS anonymize_ip,
    DROP COLUMN IF EXISTS rate_limit_per_sec,
    DROP COLUMN IF EXISTS allowed_origins,
    DROP COLUMN IF EXISTS retention_days;

DROP INDEX IF EXISTS idx_projects_owner_unique_name;
