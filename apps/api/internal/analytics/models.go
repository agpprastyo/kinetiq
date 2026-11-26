package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"

	"kinetiq/internal/response"
)

const (
	timelineDefaultLimit = 50
	timelineMaxLimit     = 200
)

type SummaryResponse struct {
	From              string                  `json:"from"`
	To                string                  `json:"to"`
	TotalEvents       int64                   `json:"total_events"`
	UniqueDistinctIds int64                   `json:"unique_distinct_ids"`
	Breakdown         []PropertyBreakdownItem `json:"breakdown,omitempty"`
}

type TopEventItem struct {
	EventName string `json:"event_name"`
	Count     int64  `json:"count"`
}

type TopEventsResponse struct {
	Items []TopEventItem `json:"items"`
}

type VolumePoint struct {
	Timestamp string `json:"timestamp"`
	Count     int64  `json:"count"`
}

type VolumeResponse struct {
	Interval string         `json:"interval"`
	Points   []VolumePoint  `json:"points"`
	Series   []VolumeSeries `json:"series,omitempty"`
}

type PropertyBreakdownItem struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type RetentionPeriod struct {
	Period     int     `json:"period"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

type CohortItem struct {
	CohortDate string            `json:"cohort_date"`
	TotalUsers int64             `json:"total_users"`
	Periods    []RetentionPeriod `json:"periods"`
}

type RetentionResponse struct {
	Granularity string       `json:"granularity"`
	FirstEvent  string       `json:"first_event"`
	ReturnEvent string       `json:"return_event"`
	From        string       `json:"from"`
	To          string       `json:"to"`
	Cohorts     []CohortItem `json:"cohorts"`
}

type UserExplorerItem struct {
	DistinctID  string    `json:"distinct_id"`
	TotalEvents int64     `json:"total_events"`
	FirstSeen   time.Time `json:"first_seen"`
	LastActive  time.Time `json:"last_active"`
}

type UserTimelineItem struct {
	EventID    string                 `json:"event_id"`
	EventName  string                 `json:"event_name"`
	OccurredAt time.Time              `json:"occurred_at"`
	Properties map[string]interface{} `json:"properties"`
}

type DeviceBreakdownItem struct {
	DeviceType string `json:"device_type"`
	Count      int64  `json:"count"`
}

type GeoBreakdownItem struct {
	CountryCode string `json:"country_code"`
	Count       int64  `json:"count"`
}

type Service interface {
	GetSummary(ctx context.Context, projectID uuid.UUID, from, to time.Time, opts QueryOptions) (*SummaryResponse, error)
	GetTopEvents(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int, opts QueryOptions) (*TopEventsResponse, error)
	GetVolume(ctx context.Context, projectID uuid.UUID, from, to time.Time, interval string, opts QueryOptions) (*VolumeResponse, error)
	GetFunnel(ctx context.Context, projectID uuid.UUID, from, to time.Time, p FunnelParams, opts QueryOptions) (*FunnelResponse, error)
	GetFunnelDropoff(ctx context.Context, projectID uuid.UUID, from, to time.Time, p FunnelParams, step, limit, offset int, opts QueryOptions) (*FunnelDropoffResponse, error)
	GetPropertyBreakdown(ctx context.Context, projectID uuid.UUID, eventName, property string, from, to time.Time, limit int, opts QueryOptions) ([]PropertyBreakdownItem, error)
	GetDeviceBreakdown(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]DeviceBreakdownItem, error)
	GetGeoBreakdown(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int) ([]GeoBreakdownItem, error)
	GetRetention(ctx context.Context, projectID uuid.UUID, from, to time.Time, firstEvent, returnEvent, granularity string, opts QueryOptions) (*RetentionResponse, error)
	ListEvents(ctx context.Context, projectID uuid.UUID, search string, limit int) ([]CatalogEvent, error)
	ListProperties(ctx context.Context, projectID uuid.UUID, scope, eventName, search string, limit int) ([]CatalogProperty, error)
	ListPropertyValues(ctx context.Context, projectID uuid.UUID, scope, property, eventName, search string, limit int) ([]PropertyValueItem, error)
	ListUsers(ctx context.Context, projectID uuid.UUID, search string, limit, offset int) ([]UserExplorerItem, int64, error)
	GetUserTimeline(ctx context.Context, projectID uuid.UUID, distinctID string, limit int, cursor *response.Cursor, before *time.Time) ([]UserTimelineItem, bool, error)
	GetSessions(ctx context.Context, projectID uuid.UUID, from, to time.Time, timeout time.Duration, limit int) (*SessionsResponse, error)
}
