package insight

import (
	"time"

	"github.com/google/uuid"
)

type ProjectInsight struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	Title         string    `json:"title"`
	ChartType     string    `json:"chart_type"`
	EventName     string    `json:"event_name"`
	Aggregation   string    `json:"aggregation"`
	PropertyKey   *string   `json:"property_key,omitempty"`
	ComputedValue float64   `json:"computed_value"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateInsightRequest struct {
	Title       string  `json:"title" validate:"required,min=1,max=100"`
	ChartType   string  `json:"chart_type" validate:"required,oneof=metric bar line donut"`
	EventName   string  `json:"event_name" validate:"required,min=1,max=100"`
	Aggregation string  `json:"aggregation" validate:"required,oneof=count unique_users sum_property avg_property min_property max_property"`
	PropertyKey *string `json:"property_key,omitempty"`
}

type UpdateInsightRequest struct {
	Title       *string `json:"title,omitempty" validate:"omitempty,min=1,max=100"`
	ChartType   *string `json:"chart_type,omitempty" validate:"omitempty,oneof=metric bar line donut"`
	EventName   *string `json:"event_name,omitempty" validate:"omitempty,min=1,max=100"`
	Aggregation *string `json:"aggregation,omitempty" validate:"omitempty,oneof=count unique_users sum_property avg_property min_property max_property"`
	PropertyKey *string `json:"property_key,omitempty"`
}
