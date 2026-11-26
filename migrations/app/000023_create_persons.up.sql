CREATE TABLE IF NOT EXISTS persons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    properties JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_identified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_persons_project ON persons(project_id);

CREATE TABLE IF NOT EXISTS person_distinct_ids (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    distinct_id TEXT NOT NULL,
    person_id UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, distinct_id)
);

CREATE INDEX IF NOT EXISTS idx_person_distinct_ids_person ON person_distinct_ids(person_id);

-- Query-time identity resolution (no event rewrite). Unmapped distinct ids fall back to themselves.
CREATE OR REPLACE VIEW resolved_events AS
SELECT e.*,
       CASE
           WHEN e.distinct_id IS NULL THEN NULL
           WHEN m.person_id IS NOT NULL THEN 'p:' || m.person_id::text
           ELSE 'd:' || e.distinct_id
       END AS person_key
FROM events e
LEFT JOIN person_distinct_ids m
       ON m.project_id = e.project_id AND m.distinct_id = e.distinct_id;
