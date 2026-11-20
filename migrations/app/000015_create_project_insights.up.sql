-- Migration 000015: Project Insights & Custom Widgets
CREATE TABLE IF NOT EXISTS project_insights (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title VARCHAR(100) NOT NULL,
    chart_type VARCHAR(30) NOT NULL DEFAULT 'metric',
    event_name VARCHAR(100) NOT NULL,
    aggregation VARCHAR(30) NOT NULL DEFAULT 'count',
    property_key VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_project_insights_project_id ON project_insights (project_id);
