-- Phase 30: Storage Compression and TimescaleDB Retention Policy Support

-- Enable TimescaleDB hypertable compression if TimescaleDB extension is available
DO $$
BEGIN
    -- Check if timescaledb extension is loaded
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        -- Alter table settings to enable compression on events
        BEGIN
            PERFORM alter_table_set_compressed_chunk_time_interval('events', INTERVAL '7 days');
        EXCEPTION WHEN undefined_function THEN
            NULL;
        END;

        BEGIN
            ALTER TABLE events SET (
                timescaledb.compress,
                timescaledb.compress_segmentby = 'project_id, event_name',
                timescaledb.compress_orderby = 'occurred_at DESC'
            );
        EXCEPTION WHEN OTHERS THEN
            NULL;
        END;

        -- Add automatic compression policy for chunks older than 30 days
        BEGIN
            PERFORM add_compression_policy('events', INTERVAL '30 days', if_not_exists => true);
        EXCEPTION WHEN OTHERS THEN
            NULL;
        END;
    END IF;
END $$;

-- Optimize index on events for retention scanning if not exists
CREATE INDEX IF NOT EXISTS idx_events_project_occurred_retention
ON events (project_id, occurred_at);
