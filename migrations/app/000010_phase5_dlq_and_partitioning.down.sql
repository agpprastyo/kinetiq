DROP INDEX IF EXISTS idx_events_occurred_at_brin;
DROP INDEX IF EXISTS idx_dead_letters_project_failed;
DROP INDEX IF EXISTS idx_dead_letters_failed_at;
ALTER TABLE event_dead_letters DROP COLUMN IF EXISTS replayed_at;
ALTER TABLE event_dead_letters DROP COLUMN IF EXISTS status;
