package importer

import (
	"time"

	"github.com/google/uuid"
)

type StandardEvent struct {
	EventID    string                 `json:"event_id"`
	EventName  string                 `json:"event_name"`
	DistinctID string                 `json:"distinct_id"`
	OccurredAt time.Time              `json:"occurred_at"`
	Properties map[string]interface{} `json:"properties"`
}

type ImportOptions struct {
	ProjectID uuid.UUID `json:"project_id"`
	DryRun    bool      `json:"dry_run"`
	BatchSize int       `json:"batch_size"`
}

type ImportStats struct {
	Source     string `json:"source"`
	TotalRead  int64  `json:"total_read"`
	Imported   int64  `json:"imported"`
	Skipped    int64  `json:"skipped"`
	ErrorCount int64  `json:"error_count"`
	DryRun     bool   `json:"dry_run"`
	DurationMs int64  `json:"duration_ms"`
}
