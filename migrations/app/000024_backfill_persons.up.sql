-- Backfill: one anonymous person per existing, not yet mapped (project_id, distinct_id). Idempotent.
WITH d AS (
    SELECT x.project_id, x.distinct_id, gen_random_uuid() AS person_id
    FROM (
        SELECT DISTINCT e.project_id, e.distinct_id
        FROM events e
        WHERE e.distinct_id IS NOT NULL AND e.distinct_id <> ''
          AND NOT EXISTS (
              SELECT 1 FROM person_distinct_ids m
              WHERE m.project_id = e.project_id AND m.distinct_id = e.distinct_id
          )
    ) x
), ins AS (
    INSERT INTO persons (id, project_id)
    SELECT person_id, project_id FROM d
)
INSERT INTO person_distinct_ids (project_id, distinct_id, person_id)
SELECT project_id, distinct_id, person_id FROM d;
