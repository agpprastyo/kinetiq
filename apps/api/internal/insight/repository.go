package insight

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, insight *ProjectInsight) error
	List(ctx context.Context, projectID uuid.UUID) ([]ProjectInsight, error)
	GetByID(ctx context.Context, projectID, insightID uuid.UUID) (*ProjectInsight, error)
	Update(ctx context.Context, insight *ProjectInsight) error
	Delete(ctx context.Context, projectID, insightID uuid.UUID) error
	ComputeValue(ctx context.Context, projectID uuid.UUID, eventName, aggregation string, propertyKey *string) (float64, error)
}

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

func (r *pgRepository) Create(ctx context.Context, i *ProjectInsight) error {
	query := `
		INSERT INTO project_insights (id, project_id, title, chart_type, event_name, aggregation, property_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	now := time.Now().UTC()
	i.CreatedAt = now
	i.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query, i.ID, i.ProjectID, i.Title, i.ChartType, i.EventName, i.Aggregation, i.PropertyKey, i.CreatedAt, i.UpdatedAt)
	return err
}

func (r *pgRepository) List(ctx context.Context, projectID uuid.UUID) ([]ProjectInsight, error) {
	query := `
		SELECT id, project_id, title, chart_type, event_name, aggregation, property_key, created_at, updated_at
		FROM project_insights
		WHERE project_id = $1
		ORDER BY created_at ASC, id ASC;
	`
	rows, err := r.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var insights []ProjectInsight
	for rows.Next() {
		var i ProjectInsight
		if err := rows.Scan(&i.ID, &i.ProjectID, &i.Title, &i.ChartType, &i.EventName, &i.Aggregation, &i.PropertyKey, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		insights = append(insights, i)
	}
	return insights, nil
}

func (r *pgRepository) GetByID(ctx context.Context, projectID, insightID uuid.UUID) (*ProjectInsight, error) {
	query := `
		SELECT id, project_id, title, chart_type, event_name, aggregation, property_key, created_at, updated_at
		FROM project_insights
		WHERE project_id = $1 AND id = $2;
	`
	var i ProjectInsight
	err := r.pool.QueryRow(ctx, query, projectID, insightID).Scan(
		&i.ID, &i.ProjectID, &i.Title, &i.ChartType, &i.EventName, &i.Aggregation, &i.PropertyKey, &i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (r *pgRepository) Update(ctx context.Context, i *ProjectInsight) error {
	query := `
		UPDATE project_insights
		SET title = $1, chart_type = $2, event_name = $3, aggregation = $4, property_key = $5, updated_at = NOW()
		WHERE project_id = $6 AND id = $7;
	`
	_, err := r.pool.Exec(ctx, query, i.Title, i.ChartType, i.EventName, i.Aggregation, i.PropertyKey, i.ProjectID, i.ID)
	return err
}

func (r *pgRepository) Delete(ctx context.Context, projectID, insightID uuid.UUID) error {
	query := `DELETE FROM project_insights WHERE project_id = $1 AND id = $2;`
	_, err := r.pool.Exec(ctx, query, projectID, insightID)
	return err
}

func (r *pgRepository) ComputeValue(ctx context.Context, projectID uuid.UUID, eventName, aggregation string, propertyKey *string) (float64, error) {
	var query string
	switch strings.ToLower(aggregation) {
	case "unique_users":
		query = `SELECT COALESCE(COUNT(DISTINCT person_key), 0)::float8 FROM resolved_events WHERE project_id = $1 AND event_name = $2;`
		var val float64
		err := r.pool.QueryRow(ctx, query, projectID, eventName).Scan(&val)
		return val, err
	case "sum_property":
		prop := "amount"
		if propertyKey != nil && *propertyKey != "" {
			prop = *propertyKey
		}
		query = fmt.Sprintf(`
			SELECT COALESCE(SUM((properties->>'%s')::numeric), 0)::float8
			FROM events
			WHERE project_id = $1 AND event_name = $2 AND (properties->>'%s') ~ '^-?[0-9]+(\.[0-9]+)?$';
		`, prop, prop)
		var val float64
		err := r.pool.QueryRow(ctx, query, projectID, eventName).Scan(&val)
		return val, err
	case "avg_property":
		prop := "amount"
		if propertyKey != nil && *propertyKey != "" {
			prop = *propertyKey
		}
		query = fmt.Sprintf(`
			SELECT COALESCE(AVG((properties->>'%s')::numeric), 0)::float8
			FROM events
			WHERE project_id = $1 AND event_name = $2 AND (properties->>'%s') ~ '^-?[0-9]+(\.[0-9]+)?$';
		`, prop, prop)
		var val float64
		err := r.pool.QueryRow(ctx, query, projectID, eventName).Scan(&val)
		return val, err
	case "min_property":
		prop := "amount"
		if propertyKey != nil && *propertyKey != "" {
			prop = *propertyKey
		}
		query = fmt.Sprintf(`
			SELECT COALESCE(MIN((properties->>'%s')::numeric), 0)::float8
			FROM events
			WHERE project_id = $1 AND event_name = $2 AND (properties->>'%s') ~ '^-?[0-9]+(\.[0-9]+)?$';
		`, prop, prop)
		var val float64
		err := r.pool.QueryRow(ctx, query, projectID, eventName).Scan(&val)
		return val, err
	case "max_property":
		prop := "amount"
		if propertyKey != nil && *propertyKey != "" {
			prop = *propertyKey
		}
		query = fmt.Sprintf(`
			SELECT COALESCE(MAX((properties->>'%s')::numeric), 0)::float8
			FROM events
			WHERE project_id = $1 AND event_name = $2 AND (properties->>'%s') ~ '^-?[0-9]+(\.[0-9]+)?$';
		`, prop, prop)
		var val float64
		err := r.pool.QueryRow(ctx, query, projectID, eventName).Scan(&val)
		return val, err
	default: // "count"
		query = `SELECT COALESCE(COUNT(*), 0)::float8 FROM events WHERE project_id = $1 AND event_name = $2;`
		var val float64
		err := r.pool.QueryRow(ctx, query, projectID, eventName).Scan(&val)
		return val, err
	}
}
