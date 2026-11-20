package ingestion

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type EventPayload struct {
	EventID    string                 `json:"event_id" validate:"required,min=1,max=64"`
	EventName  string                 `json:"event_name" validate:"required,min=1,max=64"`
	OccurredAt *time.Time             `json:"occurred_at,omitempty"`
	DistinctID string                 `json:"distinct_id,omitempty" validate:"omitempty,max=128"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type BatchPayload struct {
	Events []EventPayload `json:"events" validate:"required,min=1,max=100,dive"`
}

// QueueMessage is the normalized envelope written to Redis Streams
type QueueMessage struct {
	ProjectID  uuid.UUID              `json:"project_id"`
	EventID    string                 `json:"event_id"`
	EventName  string                 `json:"event_name"`
	OccurredAt time.Time              `json:"occurred_at"`
	ReceivedAt time.Time              `json:"received_at"`
	DistinctID string                 `json:"distinct_id,omitempty"`
	Properties map[string]interface{} `json:"properties"`
}

type IngestResponseData struct {
	Status   string `json:"status" example:"accepted"`
	Accepted int    `json:"accepted" example:"1"`
	Rejected int    `json:"rejected" example:"0"`
}

// NormalizeEvents parses raw body into a slice of EventPayload, handling both single event and batch
func NormalizeEvents(rawBody []byte) ([]EventPayload, error) {
	// Try parsing as batch first
	var batch BatchPayload
	if err := json.Unmarshal(rawBody, &batch); err == nil && len(batch.Events) > 0 {
		return batch.Events, nil
	}

	// Try parsing as single event
	var single EventPayload
	if err := json.Unmarshal(rawBody, &single); err == nil && (single.EventID != "" || single.EventName != "") {
		return []EventPayload{single}, nil
	}

	return nil, json.Unmarshal(rawBody, &single)
}
