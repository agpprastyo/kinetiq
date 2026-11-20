-- Phase 30 Down Migration
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        BEGIN
            PERFORM remove_compression_policy('events', if_exists => true);
        EXCEPTION WHEN OTHERS THEN
            NULL;
        END;
    END IF;
END $$;

DROP INDEX IF EXISTS idx_events_project_occurred_retention;
