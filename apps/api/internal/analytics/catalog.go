package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ponytail: catalog is computed on read; property/value catalogs sample the most recent
// catalogSampleSize events. Add a materialized catalog table maintained by the worker if this gets slow.
const catalogSampleSize = 10000

type CatalogEvent struct {
	EventName string    `json:"event_name"`
	Count     int64     `json:"count"`
	LastSeen  time.Time `json:"last_seen"`
}

type CatalogProperty struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type PropertyValueItem struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

func clampLimit(limit, def, max int) int {
	if limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}

func (s *AnalyticsService) ListEvents(ctx context.Context, projectID uuid.UUID, search string, limit int) ([]CatalogEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT event_name, COUNT(*)::bigint, MAX(occurred_at)
		FROM events
		WHERE project_id = $1 AND ($2 = '' OR strpos(lower(event_name), lower($2)) > 0)
		GROUP BY event_name
		ORDER BY COUNT(*) DESC, event_name ASC
		LIMIT $3`, projectID, search, clampLimit(limit, 100, 500))
	if err != nil {
		return nil, fmt.Errorf("failed to query event catalog: %w", err)
	}
	defer rows.Close()
	out := []CatalogEvent{}
	for rows.Next() {
		var e CatalogEvent
		if err := rows.Scan(&e.EventName, &e.Count, &e.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// sampleSource returns a sub-select yielding a `properties` column for the scope.
// Args: $1 project, $2 event name (” = any).
func sampleSource(scope string) string {
	if scope == "person" {
		return fmt.Sprintf(`(SELECT properties FROM persons WHERE project_id = $1 AND $2::text IS NOT NULL ORDER BY updated_at DESC LIMIT %d)`, catalogSampleSize)
	}
	return fmt.Sprintf(`(SELECT properties FROM events WHERE project_id = $1 AND ($2::text = '' OR event_name = $2::text) ORDER BY occurred_at DESC LIMIT %d)`, catalogSampleSize)
}

func (s *AnalyticsService) ListProperties(ctx context.Context, projectID uuid.UUID, scope, eventName, search string, limit int) ([]CatalogProperty, error) {
	if scope != "person" {
		scope = "event"
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT kv.key, array_agg(DISTINCT jsonb_typeof(kv.value)), COUNT(*)::bigint
		FROM %s src, jsonb_each(CASE WHEN jsonb_typeof(src.properties) = 'object' THEN src.properties ELSE '{}'::jsonb END) kv
		WHERE $3 = '' OR strpos(lower(kv.key), lower($3)) > 0
		GROUP BY kv.key
		ORDER BY COUNT(*) DESC, kv.key ASC
		LIMIT $4`, sampleSource(scope)), projectID, eventName, search, clampLimit(limit, 100, 500))
	if err != nil {
		return nil, fmt.Errorf("failed to query property catalog: %w", err)
	}
	defer rows.Close()
	out := []CatalogProperty{}
	for rows.Next() {
		var p CatalogProperty
		var types []string
		if err := rows.Scan(&p.Name, &types, &p.Count); err != nil {
			return nil, err
		}
		p.Scope, p.Type = scope, InferPropertyType(types)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *AnalyticsService) ListPropertyValues(ctx context.Context, projectID uuid.UUID, scope, property, eventName, search string, limit int) ([]PropertyValueItem, error) {
	if err := ValidatePropertyName(property); err != nil {
		return nil, err
	}
	if scope != "person" {
		scope = "event"
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT src.properties ->> $3::text AS v, COUNT(*)::bigint
		FROM %s src
		WHERE src.properties ->> $3::text IS NOT NULL
		  AND ($4 = '' OR strpos(lower(src.properties ->> $3::text), lower($4)) > 0)
		GROUP BY v
		ORDER BY COUNT(*) DESC, v ASC
		LIMIT $5`, sampleSource(scope)), projectID, eventName, property, search, clampLimit(limit, 20, 100))
	if err != nil {
		return nil, fmt.Errorf("failed to query property values: %w", err)
	}
	defer rows.Close()
	out := []PropertyValueItem{}
	for rows.Next() {
		var v PropertyValueItem
		if err := rows.Scan(&v.Value, &v.Count); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
