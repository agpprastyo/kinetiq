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
	"kinetiq/internal/analytics"
	"kinetiq/internal/app"
)

func TestRetentionCohortSeam(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))
	queries := db.New(container.DBPool)

	// Create test project
	userCookie := "user1_session_cookie"
	projectID := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Retention Cohort Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	baseDate := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	// Seed cohort events:
	// User A: Day 0 signup, Day 1 active, Day 2 active
	// User B: Day 0 signup, Day 1 active
	// User C: Day 0 signup (no return)
	// User D: Day 1 signup (Cohort 2), Day 2 active
	eventsToSeed := []struct {
		distinctID string
		eventName  string
		occurredAt time.Time
	}{
		// Day 0 (2026-10-10)
		{"user_A", "signup", baseDate},
		{"user_B", "signup", baseDate.Add(1 * time.Hour)},
		{"user_C", "signup", baseDate.Add(2 * time.Hour)},

		// Day 1 (2026-10-11)
		{"user_A", "app_opened", baseDate.Add(24 * time.Hour)},
		{"user_B", "app_opened", baseDate.Add(25 * time.Hour)},
		{"user_D", "signup", baseDate.Add(26 * time.Hour)}, // Day 1 cohort

		// Day 2 (2026-10-12)
		{"user_A", "app_opened", baseDate.Add(48 * time.Hour)},
		{"user_D", "app_opened", baseDate.Add(49 * time.Hour)},
	}

	for _, ev := range eventsToSeed {
		_, err := container.DBPool.Exec(ctx, `
			INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
		`, projectID, uuid.New().String(), ev.eventName, ev.distinctID, "{}", ev.occurredAt)
		if err != nil {
			t.Fatalf("failed to seed retention event: %v", err)
		}
	}

	t.Run("calculates daily retention cohorts accurately", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("/api/v1/projects/%s/analytics/retention?from=2026-10-10T00:00:00Z&to=2026-10-13T00:00:00Z&first_event=signup&return_event=app_opened&granularity=day", projectID.String()),
			nil,
		)
		req.Header.Set("Cookie", userCookie)

		resp, err := server.Test(req, 5000)
		if err != nil {
			t.Fatalf("unexpected request error: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var wrapper struct {
			Success bool                        `json:"success"`
			Data    analytics.RetentionResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
			t.Fatalf("failed to decode retention response: %v", err)
		}
		result := wrapper.Data

		if result.Granularity != "day" {
			t.Errorf("expected granularity 'day', got '%s'", result.Granularity)
		}

		if len(result.Cohorts) < 2 {
			t.Fatalf("expected at least 2 cohorts, got %d", len(result.Cohorts))
		}

		// Cohort 2026-10-10 has 3 users (user_A, user_B, user_C)
		cohort0 := result.Cohorts[0]
		if cohort0.CohortDate != "2026-10-10" {
			t.Errorf("expected first cohort date 2026-10-10, got %s", cohort0.CohortDate)
		}
		if cohort0.TotalUsers != 3 {
			t.Errorf("expected 3 total users in 2026-10-10 cohort, got %d", cohort0.TotalUsers)
		}

		// Day 0 retention (100%)
		// Day 1 retention: user_A & user_B = 2 / 3 (66.67%)
		// Day 2 retention: user_A = 1 / 3 (33.33%)
		day1Retention := cohort0.Periods[1]
		if day1Retention.Count != 2 {
			t.Errorf("expected 2 users retained on Day 1, got %d", day1Retention.Count)
		}
		if day1Retention.Percentage < 66.0 || day1Retention.Percentage > 67.0 {
			t.Errorf("expected ~66.67%% on Day 1, got %.2f%%", day1Retention.Percentage)
		}

		day2Retention := cohort0.Periods[2]
		if day2Retention.Count != 1 {
			t.Errorf("expected 1 user retained on Day 2, got %d", day2Retention.Count)
		}
	})
}
