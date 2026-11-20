CREATE INDEX IF NOT EXISTS idx_events_properties_gin ON events USING GIN (properties);
