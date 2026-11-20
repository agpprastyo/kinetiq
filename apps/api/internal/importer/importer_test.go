package importer_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"kinetiq/internal/importer"
)

func TestParsePostHog(t *testing.T) {
	line := `{"event":"$pageview","distinct_id":"user_42","timestamp":"2026-10-15T08:30:00Z","uuid":"ph-evt-100","properties":{"$current_url":"https://example.com/pricing"}}`
	evt, err := importer.ParsePostHog([]byte(line))
	if err != nil {
		t.Fatalf("unexpected error parsing posthog: %v", err)
	}

	if evt.EventName != "$pageview" {
		t.Errorf("expected $pageview, got %s", evt.EventName)
	}
	if evt.DistinctID != "user_42" {
		t.Errorf("expected user_42, got %s", evt.DistinctID)
	}
	if evt.EventID != "ph-evt-100" {
		t.Errorf("expected ph-evt-100, got %s", evt.EventID)
	}
	expectedTime, _ := time.Parse(time.RFC3339, "2026-10-15T08:30:00Z")
	if !evt.OccurredAt.Equal(expectedTime) {
		t.Errorf("expected %v, got %v", expectedTime, evt.OccurredAt)
	}
	if evt.Properties["$current_url"] != "https://example.com/pricing" {
		t.Errorf("expected pricing url in properties, got %v", evt.Properties["$current_url"])
	}
}

func TestParseMixpanel(t *testing.T) {
	line := `{"event":"Button Clicked","properties":{"time":1729000000,"distinct_id":"mp_user_99","$insert_id":"mp-evt-200","button_name":"Signup"}}`
	evt, err := importer.ParseMixpanel([]byte(line))
	if err != nil {
		t.Fatalf("unexpected error parsing mixpanel: %v", err)
	}

	if evt.EventName != "Button Clicked" {
		t.Errorf("expected 'Button Clicked', got %s", evt.EventName)
	}
	if evt.DistinctID != "mp_user_99" {
		t.Errorf("expected mp_user_99, got %s", evt.DistinctID)
	}
	if evt.EventID != "mp-evt-200" {
		t.Errorf("expected mp-evt-200, got %s", evt.EventID)
	}
	if evt.OccurredAt.Unix() != 1729000000 {
		t.Errorf("expected timestamp 1729000000, got %d", evt.OccurredAt.Unix())
	}
	if evt.Properties["button_name"] != "Signup" {
		t.Errorf("expected Signup in properties, got %v", evt.Properties["button_name"])
	}
}

func TestImportStream_DryRun(t *testing.T) {
	posthogData := `
{"event":"$pageview","distinct_id":"u1","timestamp":"2026-10-01T10:00:00Z","uuid":"evt-1","properties":{"path":"/home"}}
{"event":"click","distinct_id":"u1","timestamp":"2026-10-01T10:05:00Z","uuid":"evt-2","properties":{"button":"cta"}}
corrupt line that should be skipped as error
{"event":"purchase","distinct_id":"u2","timestamp":"2026-10-01T11:00:00Z","uuid":"evt-3","properties":{"amount":99}}
`
	imp := importer.NewImporter(nil)
	projectID := uuid.New()

	progressCalls := 0
	stats, err := imp.ImportStream(context.Background(), "posthog", strings.NewReader(posthogData), importer.ImportOptions{
		ProjectID: projectID,
		DryRun:    true,
		BatchSize: 2,
	}, func(read int64) {
		progressCalls++
	})

	if err != nil {
		t.Fatalf("unexpected import error: %v", err)
	}

	if stats.TotalRead != 4 {
		t.Errorf("expected 4 total read lines, got %d", stats.TotalRead)
	}
	if stats.Imported != 3 {
		t.Errorf("expected 3 valid imported events, got %d", stats.Imported)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("expected 1 error line, got %d", stats.ErrorCount)
	}
	if !stats.DryRun {
		t.Errorf("expected DryRun true")
	}
}
