CREATE INDEX IF NOT EXISTS idx_events_project_distinct_occurred
ON events (project_id, distinct_id, occurred_at DESC)
WHERE distinct_id IS NOT NULL;
