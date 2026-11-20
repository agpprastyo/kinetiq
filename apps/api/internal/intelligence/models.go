package intelligence

import (
	"time"

	"github.com/google/uuid"
)

type DigestTopEvent struct {
	EventName string `json:"event_name"`
	Count     int64  `json:"count"`
}

type WeeklyDigestReport struct {
	ProjectID      uuid.UUID        `json:"project_id"`
	ProjectName    string           `json:"project_name"`
	TotalEvents    int64            `json:"total_events"`
	UniqueUsers    int64            `json:"unique_users"`
	TopEvents      []DigestTopEvent `json:"top_events"`
	ConversionRate float64          `json:"conversion_rate"`
	PeriodStart    time.Time        `json:"period_start"`
	PeriodEnd      time.Time        `json:"period_end"`
}

type AnomalyResult struct {
	ProjectID      uuid.UUID `json:"project_id"`
	EventName      string    `json:"event_name"`
	CurrentHourly  int64     `json:"current_hourly"`
	BaselineHourly float64   `json:"baseline_hourly"`
	DropPercentage float64   `json:"drop_percentage"`
	IsAnomaly      bool      `json:"is_anomaly"`
	EvaluatedAt    time.Time `json:"evaluated_at"`
}
