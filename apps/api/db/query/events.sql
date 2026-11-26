-- name: InsertEvent :exec
INSERT INTO events (
    project_id, event_id, event_name, occurred_at, received_at, distinct_id, properties
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (project_id, event_id) DO NOTHING;

-- name: InsertDeadLetter :one
INSERT INTO event_dead_letters (
    project_id, event_id, reason, payload, attempts, failed_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING id, project_id, event_id, reason, payload, attempts, failed_at;

-- name: CountEventsByProject :one
SELECT COUNT(*) FROM events WHERE project_id = $1;

-- name: GetAnalyticsSummary :one
SELECT 
    COUNT(*)::bigint AS total_events,
    COUNT(DISTINCT person_key)::bigint AS unique_distinct_ids
FROM resolved_events
WHERE project_id = $1
  AND occurred_at >= $2
  AND occurred_at < $3;

-- name: GetTopEvents :many
SELECT 
    event_name,
    COUNT(*)::bigint AS count
FROM events
WHERE project_id = $1
  AND occurred_at >= $2
  AND occurred_at < $3
GROUP BY event_name
ORDER BY count DESC, event_name ASC
LIMIT $4;

-- name: GetHourlyVolume :many
SELECT 
    date_trunc('hour', occurred_at)::timestamptz AS bucket,
    COUNT(*)::bigint AS count
FROM events
WHERE project_id = $1
  AND occurred_at >= $2
  AND occurred_at < $3
GROUP BY bucket
ORDER BY bucket ASC;

-- name: GetMinuteVolume :many
SELECT 
    date_trunc('minute', occurred_at)::timestamptz AS bucket,
    COUNT(*)::bigint AS count
FROM events
WHERE project_id = $1
  AND occurred_at >= $2
  AND occurred_at < $3
GROUP BY bucket
ORDER BY bucket ASC;

-- name: GetDailyVolume :many
SELECT 
    date_trunc('day', occurred_at)::timestamptz AS bucket,
    COUNT(*)::bigint AS count
FROM events
WHERE project_id = $1
  AND occurred_at >= $2
  AND occurred_at < $3
GROUP BY bucket
ORDER BY bucket ASC;

-- name: GetPropertyBreakdown :many
SELECT 
    COALESCE(properties ->> @property_key::text, 'unknown') AS property_value,
    COUNT(*)::bigint AS count
FROM events
WHERE project_id = $1
  AND event_name = $2
  AND occurred_at >= $3
  AND occurred_at < $4
GROUP BY property_value
ORDER BY count DESC, property_value ASC
LIMIT $5;

