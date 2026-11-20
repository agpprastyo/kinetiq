package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type posthogRaw struct {
	Event      string                 `json:"event"`
	DistinctID string                 `json:"distinct_id"`
	Timestamp  interface{}            `json:"timestamp"`
	UUID       string                 `json:"uuid"`
	ID         string                 `json:"id"`
	Properties map[string]interface{} `json:"properties"`
}

type mixpanelRaw struct {
	Event      string                 `json:"event"`
	Properties map[string]interface{} `json:"properties"`
}

func ParsePostHog(line []byte) (*StandardEvent, error) {
	var raw posthogRaw
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("invalid posthog json: %w", err)
	}

	eventName := strings.TrimSpace(raw.Event)
	if eventName == "" {
		return nil, fmt.Errorf("missing event name")
	}

	occurredAt, err := parseTimestamp(raw.Timestamp)
	if err != nil {
		occurredAt = time.Now().UTC()
	}

	eventID := strings.TrimSpace(raw.UUID)
	if eventID == "" {
		eventID = strings.TrimSpace(raw.ID)
	}
	if eventID == "" {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", eventName, raw.DistinctID, occurredAt.UnixNano())))
		eventID = hex.EncodeToString(h[:16])
	}

	props := raw.Properties
	if props == nil {
		props = make(map[string]interface{})
	}

	return &StandardEvent{
		EventID:    eventID,
		EventName:  eventName,
		DistinctID: strings.TrimSpace(raw.DistinctID),
		OccurredAt: occurredAt,
		Properties: props,
	}, nil
}

func ParseMixpanel(line []byte) (*StandardEvent, error) {
	var raw mixpanelRaw
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("invalid mixpanel json: %w", err)
	}

	eventName := strings.TrimSpace(raw.Event)
	if eventName == "" {
		return nil, fmt.Errorf("missing event name")
	}

	props := raw.Properties
	if props == nil {
		props = make(map[string]interface{})
	}

	var distinctID string
	if did, ok := props["distinct_id"].(string); ok {
		distinctID = strings.TrimSpace(did)
	}

	var eventID string
	if ins, ok := props["$insert_id"].(string); ok && strings.TrimSpace(ins) != "" {
		eventID = strings.TrimSpace(ins)
	}

	var occurredAt time.Time
	if tVal, ok := props["time"]; ok {
		occurredAt, _ = parseTimestamp(tVal)
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	if eventID == "" {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", eventName, distinctID, occurredAt.UnixNano())))
		eventID = hex.EncodeToString(h[:16])
	}

	return &StandardEvent{
		EventID:    eventID,
		EventName:  eventName,
		DistinctID: distinctID,
		OccurredAt: occurredAt,
		Properties: props,
	}, nil
}

func parseTimestamp(v interface{}) (time.Time, error) {
	if v == nil {
		return time.Time{}, fmt.Errorf("nil timestamp")
	}

	switch val := v.(type) {
	case float64:
		sec := int64(val)
		if sec > 1e11 { // milliseconds
			return time.UnixMilli(sec).UTC(), nil
		}
		return time.Unix(sec, 0).UTC(), nil
	case int64:
		if val > 1e11 {
			return time.UnixMilli(val).UTC(), nil
		}
		return time.Unix(val, 0).UTC(), nil
	case string:
		// Try RFC3339
		if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
			return t.UTC(), nil
		}
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return t.UTC(), nil
		}
		// Try numeric string
		if num, err := strconv.ParseInt(val, 10, 64); err == nil {
			return parseTimestamp(num)
		}
	}

	return time.Time{}, fmt.Errorf("unknown timestamp format: %v", v)
}
