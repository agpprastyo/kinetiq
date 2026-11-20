-- name: CreateProject :one
INSERT INTO projects (
    id, owner_user_id, name, status
) VALUES (
    $1, $2, $3, $4
)
RETURNING id, owner_user_id, name, status, created_at, updated_at;

-- name: ListProjectsByOwner :many
SELECT id, owner_user_id, name, status, created_at, updated_at
FROM projects
WHERE owner_user_id = $1 AND status = 'active'
ORDER BY created_at DESC;

-- name: GetProjectByIDAndOwner :one
SELECT id, owner_user_id, name, status, created_at, updated_at
FROM projects
WHERE id = $1 AND owner_user_id = $2;

-- name: CreateProjectAPIKey :one
INSERT INTO project_api_keys (
    id, project_id, key_prefix, key_hash
) VALUES (
    $1, $2, $3, $4
)
RETURNING id, project_id, key_prefix, key_hash, created_at, revoked_at, last_used_at;

-- name: GetActiveKeyByPrefix :one
SELECT id, project_id, key_prefix, key_hash, created_at, revoked_at, last_used_at
FROM project_api_keys
WHERE key_prefix = $1 AND revoked_at IS NULL;

-- name: RevokeActiveKeysForProject :exec
UPDATE project_api_keys
SET revoked_at = NOW()
WHERE project_id = $1 AND revoked_at IS NULL;
