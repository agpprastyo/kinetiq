-- Phase 5: DLQ Indexing and Resilience Optimization
CREATE INDEX IF NOT EXISTS idx_dead_letters_failed_at ON event_dead_letters(failed_at DESC);
CREATE INDEX IF NOT EXISTS idx_dead_letters_project_failed ON event_dead_letters(project_id, failed_at DESC);

-- Add replayed_at column to support auditing replayed dead letters if kept
ALTER TABLE event_dead_letters ADD COLUMN IF NOT EXISTS replayed_at TIMESTAMPTZ NULL;
ALTER TABLE event_dead_letters ADD COLUMN IF NOT EXISTS status VARCHAR(50) NOT NULL DEFAULT 'failed';

-- Performance index for time-series range scanning on events
CREATE INDEX IF NOT EXISTS idx_events_occurred_at_brin ON events USING brin (occurred_at);
