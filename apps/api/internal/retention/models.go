package retention

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Policy struct {
	ProjectID     uuid.UUID `json:"project_id"`
	RetentionDays int       `json:"retention_days"` // <= 0 means unlimited
	Enabled       bool      `json:"enabled"`
}

type PreviewResult struct {
	ProjectID      uuid.UUID  `json:"project_id"`
	RetentionDays  int        `json:"retention_days"`
	CutoffTime     *time.Time `json:"cutoff_time,omitempty"`
	AffectedEvents int64      `json:"affected_events"`
	Message        string     `json:"message"`
}

type PurgeResult struct {
	ProjectID     uuid.UUID `json:"project_id"`
	RetentionDays int       `json:"retention_days"`
	DeletedEvents int64     `json:"deleted_events"`
	DurationMs    int64     `json:"duration_ms"`
	Timestamp     time.Time `json:"timestamp"`
}

type Service interface {
	GetPolicy(ctx context.Context, projectID uuid.UUID) (*Policy, error)
	UpdatePolicy(ctx context.Context, projectID uuid.UUID, retentionDays int) (*Policy, error)
	Preview(ctx context.Context, projectID uuid.UUID) (*PreviewResult, error)
	Purge(ctx context.Context, projectID uuid.UUID) (*PurgeResult, error)
	SweepAll(ctx context.Context) ([]PurgeResult, error)
}
