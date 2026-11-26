package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

func TestAnalyticsSessions(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))
	queries := db.New(container.DBPool)

	projectID := uuid.New()
	if _, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Sessions Project " + projectID.String()[:8],
		Status:      "active",
	}); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	insert := func(distinct, name string, ago time.Duration, props string) {
		at := now.Add(-ago)
		if err := queries.InsertEvent(ctx, db.InsertEventParams{
			ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
			EventID:    uuid.New().String(),
			EventName:  name,
			DistinctID: pgtype.Text{String: distinct, Valid: true},
			OccurredAt: pgtype.Timestamptz{Time: at, Valid: true},
			ReceivedAt: pgtype.Timestamptz{Time: at, Valid: true},
			Properties: []byte(props),
		}); err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}

	// u1: one session (10m), then a new one after >30m gap.
	insert("u1", "$pageview", 120*time.Minute, `{"$pathname":"/","utm_source":"google","$referring_domain":"google.com"}`)
	insert("u1", "$pageview", 110*time.Minute, `{"$pathname":"/pricing"}`)
	insert("u1", "$pageview", 10*time.Minute, `{"$pathname":"/docs"}`)
	// u2: one single-event session.
	insert("u2", "$pageview", 5*time.Minute, `{"$pathname":"/"}`)

	from := url.QueryEscape(now.Add(-3 * time.Hour).Format(time.RFC3339))
	to := url.QueryEscape(now.Add(time.Hour).Format(time.RFC3339))
	reqURL := fmt.Sprintf("/api/v1/projects/%s/analytics/sessions?from=%s&to=%s", projectID, from, to)
	req := httptest.NewRequest(http.MethodGet, reqURL, nil)
	req.Header.Set("Cookie", "user1_session_cookie")

	resp, err := server.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var res struct {
		Data struct {
			TotalSessions      int64 `json:"total_sessions"`
			AvgDurationSeconds int64 `json:"avg_duration_seconds"`
			TopEntryPages      []struct {
				Value string `json:"value"`
				Count int64  `json:"count"`
			} `json:"top_entry_pages"`
			Sessions []struct {
				DistinctID      string `json:"distinct_id"`
				DurationSeconds int64  `json:"duration_seconds"`
				EntryPage       string `json:"entry_page"`
				ExitPage        string `json:"exit_page"`
				UTMSource       string `json:"utm_source"`
			} `json:"sessions"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Data.TotalSessions != 3 {
		t.Fatalf("expected 3 sessions, got %d", res.Data.TotalSessions)
	}
	if res.Data.AvgDurationSeconds != 200 { // (600+0+0)/3
		t.Errorf("expected avg 200s, got %d", res.Data.AvgDurationSeconds)
	}
	if len(res.Data.TopEntryPages) == 0 || res.Data.TopEntryPages[0].Value != "/" || res.Data.TopEntryPages[0].Count != 2 {
		t.Errorf("unexpected top entry pages: %+v", res.Data.TopEntryPages)
	}

	// invalid range -> 400
	bad := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/sessions", projectID), nil)
	bad.Header.Set("Cookie", "user1_session_cookie")
	badResp, _ := server.Test(bad)
	if badResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 without range, got %d", badResp.StatusCode)
	}
}
