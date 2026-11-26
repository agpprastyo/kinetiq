package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
	"kinetiq/internal/intelligence"
)

func TestIntelligenceSeam(t *testing.T) {
	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	queries := db.New(container.DBPool)

	// Setup a test user and project
	userID := "intel-user-" + uuid.New().String()[:8]
	_, err = container.DBPool.Exec(ctx, `
		INSERT INTO "user" (id, name, email, "emailVerified", role, "twoFactorEnabled", banned, "isPlatformStaff", "createdAt", "updatedAt")
		VALUES ($1, 'Intel User', $2, true, 'owner', false, false, false, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING;
	`, userID, userID+"@example.com")
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	projectID := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: userID,
		Name:        "Intelligence Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	service := intelligence.NewService(container.DBPool, container.RedisClient, container.AlertingService, container.Logger)

	t.Run("GenerateWeeklyDigest compiles 7-day stats and persists to auth_email_outbox", func(t *testing.T) {
		now := time.Now().UTC()
		// Seed 10 events across last 7 days
		for i := 0; i < 10; i++ {
			_, err := container.DBPool.Exec(ctx, `
				INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
				VALUES ($1, $2, $3, $4, $5, $6, NOW())
			`, projectID, uuid.New().String(), "checkout", fmt.Sprintf("u_%d", i%3), `{"price": 50}`, now.Add(-time.Duration(i*12)*time.Hour))
			if err != nil {
				t.Fatalf("failed to seed event: %v", err)
			}
		}

		digest, err := service.GenerateProjectWeeklyDigest(ctx, projectID)
		if err != nil {
			t.Fatalf("GenerateProjectWeeklyDigest failed: %v", err)
		}
		if digest == nil {
			t.Fatalf("expected non-nil digest")
		}
		if digest.TotalEvents != 10 {
			t.Errorf("expected 10 events, got %d", digest.TotalEvents)
		}
		if digest.UniqueUsers != 3 {
			t.Errorf("expected 3 unique users, got %d", digest.UniqueUsers)
		}

		// Dispatch weekly digest to outbox
		count, err := service.DispatchWeeklyDigests(ctx)
		if err != nil {
			t.Fatalf("DispatchWeeklyDigests failed: %v", err)
		}
		if count == 0 {
			t.Errorf("expected at least 1 digest dispatched, got 0")
		}

		// Verify email was stored in auth_email_outbox
		var emailCount int
		err = container.DBPool.QueryRow(ctx, `
			SELECT COUNT(*) FROM auth_email_outbox WHERE type = 'weekly_digest' AND recipient = $1
		`, userID+"@example.com").Scan(&emailCount)
		if err != nil {
			t.Fatalf("failed to query auth_email_outbox: %v", err)
		}
		if emailCount == 0 {
			t.Errorf("expected weekly_digest email in outbox for %s", userID+"@example.com")
		}
	})

	t.Run("DetectTrafficAnomaly identifies drop compared to 3-day baseline", func(t *testing.T) {
		now := time.Now().UTC()
		anomalyProjectID := uuid.New()
		_, err = queries.CreateProject(ctx, db.CreateProjectParams{
			ID:          pgtype.UUID{Bytes: anomalyProjectID, Valid: true},
			OwnerUserID: userID,
			Name:        "Anomaly Project " + anomalyProjectID.String()[:8],
			Status:      "active",
		})
		if err != nil {
			t.Fatalf("failed to create anomaly project: %v", err)
		}

		// Baseline: 1 day ago, 2 days ago, 3 days ago at the exact same hour had 20 events each
		for day := 1; day <= 3; day++ {
			for i := 0; i < 20; i++ {
				_, err := container.DBPool.Exec(ctx, `
					INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
					VALUES ($1, $2, 'app_ping', 'device_1', '{}', $3, NOW())
				`, anomalyProjectID, uuid.New().String(), now.AddDate(0, 0, -day))
				if err != nil {
					t.Fatalf("failed to seed baseline event: %v", err)
				}
			}
		}

		// Current hour only has 2 events (90% drop vs baseline of 20)
		for i := 0; i < 2; i++ {
			_, err := container.DBPool.Exec(ctx, `
				INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
				VALUES ($1, $2, 'app_ping', 'device_1', '{}', $3, NOW())
			`, anomalyProjectID, uuid.New().String(), now.Add(-5*time.Minute))
			if err != nil {
				t.Fatalf("failed to seed current event: %v", err)
			}
		}

		// Register an alert rule for app_ping
		_, err = container.DBPool.Exec(ctx, `
			INSERT INTO alert_rules (id, project_id, name, event_name, condition, threshold_count, window_minutes, webhook_url, webhook_secret, channel_type, is_active, cooldown_minutes)
			VALUES ($1, $2, 'Anomaly Alert', 'app_ping', 'drop_lt', 5, 60, 'http://localhost:0/alert', '', 'webhook', true, 60)
		`, uuid.New().String(), anomalyProjectID)
		if err != nil {
			t.Fatalf("failed to insert alert rule: %v", err)
		}

		anomaly, err := service.CheckTrafficAnomaly(ctx, anomalyProjectID, "app_ping")
		if err != nil {
			t.Fatalf("CheckTrafficAnomaly failed: %v", err)
		}
		if anomaly == nil {
			t.Fatalf("expected anomaly to be detected")
		}
		if !anomaly.IsAnomaly {
			t.Errorf("expected IsAnomaly=true, baseline=%.1f, current=%d, drop=%.1f%%", anomaly.BaselineHourly, anomaly.CurrentHourly, anomaly.DropPercentage)
		}
	})

	t.Run("API HTTP Endpoints for Intelligence", func(t *testing.T) {
		server := app.SetupWithContainer(withFakeAuth(container))
		userCookie := "user1_session_cookie"

		// Mock project ownership for user-1-id
		httpProjID := uuid.New()
		_, err = queries.CreateProject(ctx, db.CreateProjectParams{
			ID:          pgtype.UUID{Bytes: httpProjID, Valid: true},
			OwnerUserID: "user-1-id",
			Name:        "HTTP Intel Project " + httpProjID.String()[:8],
			Status:      "active",
		})
		if err != nil {
			t.Fatalf("failed to create http project: %v", err)
		}

		// GET /projects/:id/intelligence/digest
		req := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("/api/v1/projects/%s/intelligence/digest", httpProjID.String()),
			nil,
		)
		req.Header.Set("Cookie", userCookie)
		resp, err := server.Test(req, 5000)
		if err != nil {
			t.Fatalf("request error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		// GET /projects/:id/intelligence/anomaly?event_name=app_ping
		reqAnomaly := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("/api/v1/projects/%s/intelligence/anomaly?event_name=app_ping", httpProjID.String()),
			nil,
		)
		reqAnomaly.Header.Set("Cookie", userCookie)
		respAnomaly, err := server.Test(reqAnomaly, 5000)
		if err != nil {
			t.Fatalf("anomaly request error: %v", err)
		}
		if respAnomaly.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", respAnomaly.StatusCode)
		}

		// Digest dispatch is ops-only: tenant user -> 403, ops staff -> 200
		dispatch := func(cookie string) int {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/ops/intelligence/dispatch-digests", nil)
			req.Header.Set("Cookie", cookie)
			resp, err := server.Test(req, 5000)
			if err != nil {
				t.Fatalf("dispatch request error: %v", err)
			}
			return resp.StatusCode
		}
		if code := dispatch(userCookie); code != http.StatusForbidden {
			t.Fatalf("expected 403 for tenant dispatch, got %d", code)
		}
		if code := dispatch("ops_session_cookie"); code != http.StatusOK {
			t.Fatalf("expected 200 for ops dispatch, got %d", code)
		}

		legacy := httptest.NewRequest(http.MethodPost, "/api/v1/intelligence/dispatch-digests", nil)
		legacy.Header.Set("Cookie", userCookie)
		respLegacy, err := server.Test(legacy, 5000)
		if err != nil {
			t.Fatalf("legacy dispatch request error: %v", err)
		}
		if respLegacy.StatusCode == http.StatusOK {
			t.Fatalf("legacy tenant dispatch route must be removed")
		}
	})

	t.Run("RunAnomalySweep evaluates active alert rules across all projects", func(t *testing.T) {
		sweepProjectID := uuid.New()
		_, err = queries.CreateProject(ctx, db.CreateProjectParams{
			ID:          pgtype.UUID{Bytes: sweepProjectID, Valid: true},
			OwnerUserID: userID,
			Name:        "Sweep Project " + sweepProjectID.String()[:8],
			Status:      "active",
		})
		if err != nil {
			t.Fatalf("failed to create sweep project: %v", err)
		}

		now := time.Now().UTC()
		// Seed baseline events for "order_placed"
		for day := 1; day <= 3; day++ {
			for i := 0; i < 15; i++ {
				_, err := container.DBPool.Exec(ctx, `
					INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
					VALUES ($1, $2, 'order_placed', 'customer_1', '{}', $3, NOW())
				`, sweepProjectID, uuid.New().String(), now.AddDate(0, 0, -day))
				if err != nil {
					t.Fatalf("failed to seed sweep baseline event: %v", err)
				}
			}
		}

		// Seed only 1 event in current hour
		_, err = container.DBPool.Exec(ctx, `
			INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
			VALUES ($1, $2, 'order_placed', 'customer_1', '{}', $3, NOW())
		`, sweepProjectID, uuid.New().String(), now.Add(-10*time.Minute))
		if err != nil {
			t.Fatalf("failed to seed sweep current event: %v", err)
		}

		// Insert active alert rule for "order_placed"
		_, err = container.DBPool.Exec(ctx, `
			INSERT INTO alert_rules (id, project_id, name, event_name, condition, threshold_count, window_minutes, webhook_url, webhook_secret, channel_type, is_active, cooldown_minutes)
			VALUES ($1, $2, 'Order Drop Alert', 'order_placed', 'drop_lt', 5, 60, 'http://localhost:0/order-alert', '', 'webhook', true, 60)
		`, uuid.New().String(), sweepProjectID)
		if err != nil {
			t.Fatalf("failed to insert sweep alert rule: %v", err)
		}

		anomalies, err := service.RunAnomalySweep(ctx)
		if err != nil {
			t.Fatalf("RunAnomalySweep failed: %v", err)
		}

		found := false
		for _, a := range anomalies {
			if a.ProjectID == sweepProjectID && a.EventName == "order_placed" {
				found = true
				if !a.IsAnomaly {
					t.Errorf("expected IsAnomaly to be true for order_placed")
				}
			}
		}
		if !found {
			t.Errorf("expected to find anomaly for project %s and event 'order_placed'", sweepProjectID.String())
		}
	})
}
