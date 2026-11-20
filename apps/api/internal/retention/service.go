package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RetentionService struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewRetentionService(pool *pgxpool.Pool, logger *slog.Logger) *RetentionService {
	return &RetentionService{
		pool:   pool,
		logger: logger,
	}
}

func (s *RetentionService) GetPolicy(ctx context.Context, projectID uuid.UUID) (*Policy, error) {
	if projectID == uuid.Nil {
		return nil, fmt.Errorf("project_id is required")
	}

	query := `SELECT retention_days FROM projects WHERE id = $1;`
	var retentionDays int
	if err := s.pool.QueryRow(ctx, query, projectID).Scan(&retentionDays); err != nil {
		return nil, fmt.Errorf("failed to get project retention policy: %w", err)
	}

	return &Policy{
		ProjectID:     projectID,
		RetentionDays: retentionDays,
		Enabled:       retentionDays > 0,
	}, nil
}

func (s *RetentionService) UpdatePolicy(ctx context.Context, projectID uuid.UUID, retentionDays int) (*Policy, error) {
	if projectID == uuid.Nil {
		return nil, fmt.Errorf("project_id is required")
	}
	if retentionDays < 0 {
		retentionDays = 0 // normalize to unlimited
	}

	query := `UPDATE projects SET retention_days = $1, updated_at = NOW() WHERE id = $2;`
	cmdTag, err := s.pool.Exec(ctx, query, retentionDays, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to update project retention policy: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return nil, fmt.Errorf("project not found")
	}

	return &Policy{
		ProjectID:     projectID,
		RetentionDays: retentionDays,
		Enabled:       retentionDays > 0,
	}, nil
}

func (s *RetentionService) Preview(ctx context.Context, projectID uuid.UUID) (*PreviewResult, error) {
	policy, err := s.GetPolicy(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if policy.RetentionDays <= 0 {
		return &PreviewResult{
			ProjectID:      projectID,
			RetentionDays:  policy.RetentionDays,
			CutoffTime:     nil,
			AffectedEvents: 0,
			Message:        "Project retention policy is unlimited. No events will be deleted.",
		}, nil
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -policy.RetentionDays)
	countQuery := `SELECT COUNT(*) FROM events WHERE project_id = $1 AND occurred_at < $2;`
	var count int64
	if err := s.pool.QueryRow(ctx, countQuery, projectID, cutoff).Scan(&count); err != nil {
		return nil, fmt.Errorf("failed to count expired events: %w", err)
	}

	return &PreviewResult{
		ProjectID:      projectID,
		RetentionDays:  policy.RetentionDays,
		CutoffTime:     &cutoff,
		AffectedEvents: count,
		Message:        fmt.Sprintf("%d events eligible for deletion older than %s (%d days)", count, cutoff.Format(time.RFC3339), policy.RetentionDays),
	}, nil
}

func (s *RetentionService) Purge(ctx context.Context, projectID uuid.UUID) (*PurgeResult, error) {
	start := time.Now()
	policy, err := s.GetPolicy(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if policy.RetentionDays <= 0 {
		return &PurgeResult{
			ProjectID:     projectID,
			RetentionDays: policy.RetentionDays,
			DeletedEvents: 0,
			DurationMs:    time.Since(start).Milliseconds(),
			Timestamp:     start.UTC(),
		}, nil
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -policy.RetentionDays)
	deleteQuery := `DELETE FROM events WHERE project_id = $1 AND occurred_at < $2;`
	cmdTag, err := s.pool.Exec(ctx, deleteQuery, projectID, cutoff)
	if err != nil {
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to purge expired events", slog.String("error", err.Error()), slog.String("project_id", projectID.String()))
		}
		return nil, fmt.Errorf("failed to delete expired events: %w", err)
	}

	deleted := cmdTag.RowsAffected()
	dur := time.Since(start).Milliseconds()

	if s.logger != nil {
		s.logger.InfoContext(ctx, "project retention purge completed",
			slog.String("project_id", projectID.String()),
			slog.Int("retention_days", policy.RetentionDays),
			slog.Int64("deleted_events", deleted),
			slog.Int64("duration_ms", dur),
		)
	}

	return &PurgeResult{
		ProjectID:     projectID,
		RetentionDays: policy.RetentionDays,
		DeletedEvents: deleted,
		DurationMs:    dur,
		Timestamp:     start.UTC(),
	}, nil
}

func (s *RetentionService) SweepAll(ctx context.Context) ([]PurgeResult, error) {
	query := `SELECT id FROM projects WHERE retention_days > 0;`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects for retention sweep: %w", err)
	}
	defer rows.Close()

	var projectIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan project id: %w", err)
		}
		projectIDs = append(projectIDs, id)
	}

	var results []PurgeResult
	for _, id := range projectIDs {
		res, err := s.Purge(ctx, id)
		if err != nil {
			if s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to sweep project retention", slog.String("project_id", id.String()), slog.String("error", err.Error()))
			}
			continue
		}
		results = append(results, *res)
	}

	return results, nil
}
