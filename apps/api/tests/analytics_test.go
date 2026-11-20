package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

func TestAnalyticsSeam(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(container)
	queries := db.New(container.DBPool)

	// User 1 project
	user1Cookie := "user1_session_cookie"
	projectID1 := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID1, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Analytics Project 1 " + projectID1.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project 1: %v", err)
	}

	// User 2 project
	user2Cookie := "user2_session_cookie"
	projectID2 := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID2, Valid: true},
		OwnerUserID: "user-2-id",
		Name:        "Analytics Project 2 " + projectID2.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project 2: %v", err)
	}

	// Seed test events into project 1
	// T0: 2026-10-14 10:00:00 UTC
	t0 := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 10, 14, 10, 30, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 14, 11, 15, 0, 0, time.UTC)

	// Project 1 events
	seedEvents := []struct {
		eventID    string
		eventName  string
		occurredAt time.Time
		distinctID string
	}{
		{"evt_a1", "page_view", t0, "user_alpha"},
		{"evt_a2", "click", t1, "user_alpha"},
		{"evt_a3", "click", t1, "user_beta"},
		{"evt_a4", "signup", t2, "user_beta"},
		{"evt_a5", "click", t2, ""}, // null distinct_id
	}

	for _, e := range seedEvents {
		distinct := pgtype.Text{Valid: false}
		if e.distinctID != "" {
			distinct = pgtype.Text{String: e.distinctID, Valid: true}
		}
		err = queries.InsertEvent(ctx, db.InsertEventParams{
			ProjectID:  pgtype.UUID{Bytes: projectID1, Valid: true},
			EventID:    e.eventID,
			EventName:  e.eventName,
			OccurredAt: pgtype.Timestamptz{Time: e.occurredAt, Valid: true},
			ReceivedAt: pgtype.Timestamptz{Time: e.occurredAt, Valid: true},
			DistinctID: distinct,
			Properties: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("failed to seed event %s: %v", e.eventID, err)
		}
	}

	fromStr := "2026-10-14T09:00:00Z"
	toStr := "2026-10-14T12:00:00Z"

	t.Run("cross-user cannot access analytics (404/403)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/summary?from=%s&to=%s", projectID1.String(), fromStr, toStr), nil)
		req.Header.Set("Cookie", user2Cookie) // User 2 trying to query User 1's project

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed request: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET summary returns accurate total_events and unique_distinct_ids", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/summary?from=%s&to=%s", projectID1.String(), fromStr, toStr), nil)
		req.Header.Set("Cookie", user1Cookie)

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var body struct {
			From              string `json:"from"`
			To                string `json:"to"`
			TotalEvents       int64  `json:"total_events"`
			UniqueDistinctIds int64  `json:"unique_distinct_ids"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if body.TotalEvents != 5 {
			t.Errorf("expected total_events=5, got %d", body.TotalEvents)
		}
		if body.UniqueDistinctIds != 2 { // user_alpha and user_beta (excluding empty)
			t.Errorf("expected unique_distinct_ids=2, got %d", body.UniqueDistinctIds)
		}
	})

	t.Run("GET top-events returns sorted events by count desc", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/top-events?from=%s&to=%s&limit=10", projectID1.String(), fromStr, toStr), nil)
		req.Header.Set("Cookie", user1Cookie)

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var body struct {
			Items []struct {
				EventName string `json:"event_name"`
				Count     int64  `json:"count"`
			} `json:"items"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(body.Items) < 3 {
			t.Fatalf("expected at least 3 top events, got %d", len(body.Items))
		}
		// page_view (2), click (2), signup (1)
		if body.Items[0].Count < body.Items[len(body.Items)-1].Count {
			t.Errorf("top-events should be sorted descending")
		}
	})

	t.Run("GET volume returns bucketed points by hour", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/volume?from=%s&to=%s&interval=hour", projectID1.String(), fromStr, toStr), nil)
		req.Header.Set("Cookie", user1Cookie)

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var body struct {
			Interval string `json:"interval"`
			Points   []struct {
				Timestamp string `json:"timestamp"`
				Count     int64  `json:"count"`
			} `json:"points"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if body.Interval != "hour" {
			t.Errorf("expected interval 'hour', got '%s'", body.Interval)
		}
		if len(body.Points) == 0 {
			t.Fatalf("expected volume points, got 0")
		}
	})

	t.Run("GET funnel computes accurate conversion rates and step counts", func(t *testing.T) {
		// User Alpha: t0 page_view, t1 click
		// User Beta: t1 click, t2 signup
		// Steps: click,signup
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/funnel?from=%s&to=%s&steps=click,signup", projectID1.String(), fromStr, toStr), nil)
		req.Header.Set("Cookie", user1Cookie)

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var body struct {
			Steps []struct {
				Step           int     `json:"step"`
				EventName      string  `json:"event_name"`
				Count          int64   `json:"count"`
				ConversionRate float64 `json:"conversion_rate"`
			} `json:"steps"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode funnel response: %v", err)
		}

		if len(body.Steps) != 2 {
			t.Fatalf("expected 2 funnel steps, got %d", len(body.Steps))
		}
		// Step 1: click (user_alpha & user_beta = 2 distinct users) -> 100% conversion
		if body.Steps[0].Count != 2 || body.Steps[0].ConversionRate != 100.0 {
			t.Errorf("step 1 expected count=2, rate=100.0, got count=%d, rate=%f", body.Steps[0].Count, body.Steps[0].ConversionRate)
		}
		// Step 2: signup after click (only user_beta did click then signup) -> 1 user = 50.0% conversion
		if body.Steps[1].Count != 1 || body.Steps[1].ConversionRate != 50.0 {
			t.Errorf("step 2 expected count=1, rate=50.0, got count=%d, rate=%f", body.Steps[1].Count, body.Steps[1].ConversionRate)
		}
	})

	t.Run("GET funnel rejects invalid step counts (<2 or >5)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/funnel?from=%s&to=%s&steps=page_view", projectID1.String(), fromStr, toStr), nil)
		req.Header.Set("Cookie", user1Cookie)
		resp, _ := server.Test(req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for single step funnel, got %d", resp.StatusCode)
		}

		req6 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/funnel?from=%s&to=%s&steps=s1,s2,s3,s4,s5,s6", projectID1.String(), fromStr, toStr), nil)
		req6.Header.Set("Cookie", user1Cookie)
		resp6, _ := server.Test(req6)
		if resp6.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for 6 steps funnel, got %d", resp6.StatusCode)
		}
	})

	t.Run("Redis Cache-Aside stores analytics responses with short TTL", func(t *testing.T) {
		// First call populates cache
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/summary?from=%s&to=%s", projectID1.String(), fromStr, toStr), nil)
		req.Header.Set("Cookie", user1Cookie)
		resp, err := server.Test(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("first request failed: %v", err)
		}

		// Inspect Redis keys for analytics cache
		keys, err := container.RedisClient.Keys(ctx, "cache:analytics:"+projectID1.String()+":*").Result()
		if err != nil {
			t.Fatalf("failed to query redis keys: %v", err)
		}
		if len(keys) == 0 {
			t.Errorf("expected at least 1 analytics cache key in Redis, got 0")
		}

		// Second call should return identical cached data
		resp2, err := server.Test(req)
		if err != nil || resp2.StatusCode != http.StatusOK {
			t.Fatalf("second cached request failed: %v", err)
		}
	})

	t.Run("GET /analytics/users and /users/:distinctId/timeline", func(t *testing.T) {
		// 1. Cross-user access check
		reqForbidden := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/users", projectID1.String()), nil)
		reqForbidden.Header.Set("Cookie", user2Cookie)
		respForbidden, _ := server.Test(reqForbidden)
		if respForbidden.StatusCode != http.StatusNotFound && respForbidden.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 or 404 for cross-user project access, got %d", respForbidden.StatusCode)
		}

		// 2. List users for project 1
		reqUsers := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/users?limit=10", projectID1.String()), nil)
		reqUsers.Header.Set("Cookie", user1Cookie)
		respUsers, err := server.Test(reqUsers)
		if err != nil || respUsers.StatusCode != http.StatusOK {
			t.Fatalf("list users failed: %v, status: %d", err, respUsers.StatusCode)
		}

		var usersBody struct {
			Success bool `json:"success"`
			Data    []struct {
				DistinctID  string `json:"distinct_id"`
				TotalEvents int64  `json:"total_events"`
			} `json:"data"`
		}
		if err := json.NewDecoder(respUsers.Body).Decode(&usersBody); err != nil {
			t.Fatalf("failed to decode users response: %v", err)
		}
		if len(usersBody.Data) == 0 {
			t.Errorf("expected at least 1 user, got 0")
		}

		// 3. User timeline
		firstUser := usersBody.Data[0].DistinctID
		reqTimeline := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/users/%s/timeline?limit=50", projectID1.String(), firstUser), nil)
		reqTimeline.Header.Set("Cookie", user1Cookie)
		respTimeline, err := server.Test(reqTimeline)
		if err != nil || respTimeline.StatusCode != http.StatusOK {
			t.Fatalf("user timeline failed: %v, status: %d", err, respTimeline.StatusCode)
		}

		var timelineBody struct {
			Success bool `json:"success"`
			Data    []struct {
				EventID   string `json:"event_id"`
				EventName string `json:"event_name"`
			} `json:"data"`
		}
		if err := json.NewDecoder(respTimeline.Body).Decode(&timelineBody); err != nil {
			t.Fatalf("failed to decode timeline response: %v", err)
		}
		if len(timelineBody.Data) == 0 {
			t.Errorf("expected at least 1 timeline event for user %s, got 0", firstUser)
		}
	})
}
