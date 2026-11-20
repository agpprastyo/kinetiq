package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
	"kinetiq/internal/worker"
)

func TestRetentionSweeperSeam(t *testing.T) {
	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	queries := db.New(container.DBPool)

	// Setup a test user and project
	userID := "retention-user-" + uuid.New().String()[:8]
	_, err = container.DBPool.Exec(ctx, `
		INSERT INTO "user" (id, name, email, "emailVerified", role, "twoFactorEnabled", banned, "isPlatformStaff", "createdAt", "updatedAt")
		VALUES ($1, 'Retention User', $2, true, 'owner', false, false, false, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING;
	`, userID, userID+"@example.com")
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	projectID := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: userID,
		Name:        "Retention Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	now := time.Now().UTC()

	// Seed 3 expired events (occurred 100 days ago) and 2 fresh events (occurred 5 days ago)
	for i := 0; i < 3; i++ {
		_, err := container.DBPool.Exec(ctx, `
			INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
			VALUES ($1, $2, 'old_event', 'device_old', '{}', $3, NOW())
		`, projectID, uuid.New().String(), now.AddDate(0, 0, -100-i))
		if err != nil {
			t.Fatalf("failed to seed expired event: %v", err)
		}
	}

	for i := 0; i < 2; i++ {
		_, err := container.DBPool.Exec(ctx, `
			INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
			VALUES ($1, $2, 'fresh_event', 'device_new', '{}', $3, NOW())
		`, projectID, uuid.New().String(), now.AddDate(0, 0, -5))
		if err != nil {
			t.Fatalf("failed to seed fresh event: %v", err)
		}
	}

	t.Run("PurgeExpiredEvents with non-positive retentionDays does nothing", func(t *testing.T) {
		purged, err := worker.PurgeExpiredEvents(ctx, container.DBPool, 0)
		if err != nil {
			t.Fatalf("PurgeExpiredEvents failed: %v", err)
		}
		if purged != 0 {
			t.Errorf("expected 0 purged events with retentionDays=0, got %d", purged)
		}

		purgedNeg, err := worker.PurgeExpiredEvents(ctx, container.DBPool, -10)
		if err != nil {
			t.Fatalf("PurgeExpiredEvents failed with negative days: %v", err)
		}
		if purgedNeg != 0 {
			t.Errorf("expected 0 purged events with negative days, got %d", purgedNeg)
		}
	})

	t.Run("PurgeExpiredEvents purges events older than 90 days cleanly", func(t *testing.T) {
		purged, err := worker.PurgeExpiredEvents(ctx, container.DBPool, 90)
		if err != nil {
			t.Fatalf("PurgeExpiredEvents failed: %v", err)
		}
		if purged < 3 {
			t.Errorf("expected at least 3 events purged, got %d", purged)
		}

		// Ensure fresh events still exist
		var freshCount int
		err = container.DBPool.QueryRow(ctx, `
			SELECT COUNT(*) FROM events WHERE project_id = $1 AND event_name = 'fresh_event'
		`, projectID).Scan(&freshCount)
		if err != nil {
			t.Fatalf("failed to count fresh events: %v", err)
		}
		if freshCount != 2 {
			t.Errorf("expected 2 fresh events remaining, got %d", freshCount)
		}

		// Ensure expired events are gone
		var expiredCount int
		err = container.DBPool.QueryRow(ctx, `
			SELECT COUNT(*) FROM events WHERE project_id = $1 AND event_name = 'old_event'
		`, projectID).Scan(&expiredCount)
		if err != nil {
			t.Fatalf("failed to count expired events: %v", err)
		}
		if expiredCount != 0 {
			t.Errorf("expected 0 expired events, got %d", expiredCount)
		}
	})

	t.Run("StartRetentionSweeper periodic lifecycle check", func(t *testing.T) {
		sweepCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Start retention sweeper with very short interval and verify cancellation exits cleanly
		worker.StartRetentionSweeper(sweepCtx, container.DBPool, container.Logger, 50*time.Millisecond, 90)
		time.Sleep(120 * time.Millisecond)
		cancel()
	})

	t.Run("StartIntelligenceSchedulers starts background tickers cleanly", func(t *testing.T) {
		schedCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Test nil service guard
		worker.StartIntelligenceSchedulers(schedCtx, nil, container.Logger)

		// Test with real intelligence service
		worker.StartIntelligenceSchedulers(schedCtx, container.IntelligenceService, container.Logger)
		time.Sleep(50 * time.Millisecond)
		cancel()
	})
}
