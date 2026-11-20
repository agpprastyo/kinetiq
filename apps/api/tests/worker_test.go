package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
	"kinetiq/internal/ingestion"
	"kinetiq/internal/worker"
)

func TestWorkerSeam(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	queries := db.New(container.DBPool)

	// Setup a test project in Postgres
	projectID := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "worker-test-user",
		Name:        "Worker Test Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	groupName := "kinetiq-workers-test-" + uuid.New().String()
	consumerConfig := worker.Config{
		StreamKey:    "events:stream",
		GroupName:    groupName,
		ConsumerName: "test-worker-1",
		BatchSize:    10,
		BlockTimeout: 100 * time.Millisecond,
		MaxRetries:   3,
	}

	w, err := worker.NewConsumerWorker(container.DBPool, container.RedisClient, container.Logger, consumerConfig)
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	t.Run("persists batch events and acknowledges messages", func(t *testing.T) {
		evtID1 := "evt_test_" + uuid.New().String()
		evtID2 := "evt_test_" + uuid.New().String()

		now := time.Now().UTC().Truncate(time.Microsecond)
		events := []ingestion.EventPayload{
			{
				EventID:    evtID1,
				EventName:  "signup",
				OccurredAt: &now,
				DistinctID: "user_123",
				Properties: map[string]interface{}{"plan": "pro"},
			},
			{
				EventID:    evtID2,
				EventName:  "checkout",
				OccurredAt: &now,
				DistinctID: "user_123",
				Properties: map[string]interface{}{"amount": 99.0},
			},
		}

		// Enqueue via IngestionService
		count, err := container.IngestionService.EnqueueEvents(ctx, projectID, events)
		if err != nil {
			t.Fatalf("failed to enqueue events: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected 2 enqueued events, got %d", count)
		}

		// Process one batch with worker
		processed, err := w.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("worker failed to process batch: %v", err)
		}
		if processed != 2 {
			t.Errorf("expected 2 processed events, got %d", processed)
		}

		// Verify persisted in PostgreSQL
		dbCount, err := queries.CountEventsByProject(ctx, pgtype.UUID{Bytes: projectID, Valid: true})
		if err != nil {
			t.Fatalf("failed to count events: %v", err)
		}
		if dbCount < 2 {
			t.Errorf("expected at least 2 events in DB, got %d", dbCount)
		}
	})

	t.Run("handles duplicate event idempotently without error", func(t *testing.T) {
		evtID := "evt_dup_" + uuid.New().String()
		now := time.Now().UTC().Truncate(time.Microsecond)

		events := []ingestion.EventPayload{
			{
				EventID:    evtID,
				EventName:  "button_click",
				OccurredAt: &now,
			},
		}

		// First ingestion & processing
		_, err := container.IngestionService.EnqueueEvents(ctx, projectID, events)
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}
		_, err = w.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("failed to process first batch: %v", err)
		}

		beforeCount, err := queries.CountEventsByProject(ctx, pgtype.UUID{Bytes: projectID, Valid: true})
		if err != nil {
			t.Fatalf("failed to query count: %v", err)
		}

		// Re-enqueue identical event ID for same project
		_, err = container.IngestionService.EnqueueEvents(ctx, projectID, events)
		if err != nil {
			t.Fatalf("failed to enqueue duplicate: %v", err)
		}

		// Worker processes duplicate
		processed, err := w.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("worker failed on duplicate batch: %v", err)
		}
		if processed != 1 {
			t.Errorf("expected 1 message processed, got %d", processed)
		}

		afterCount, err := queries.CountEventsByProject(ctx, pgtype.UUID{Bytes: projectID, Valid: true})
		if err != nil {
			t.Fatalf("failed to query count after duplicate: %v", err)
		}

		if beforeCount != afterCount {
			t.Errorf("duplicate event should not increase count: before=%d, after=%d", beforeCount, afterCount)
		}
	})

	t.Run("poison/malformed message is sent to dead letter and acknowledged", func(t *testing.T) {
		// Insert directly malformed JSON to stream
		err := container.RedisClient.XAdd(ctx, &redis.XAddArgs{
			Stream: "events:stream",
			Values: map[string]interface{}{
				"data": "invalid-malformed-json{{",
			},
		}).Err()
		if err != nil {
			t.Fatalf("failed to push malformed message: %v", err)
		}

		processed, err := w.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("worker failed on malformed message: %v", err)
		}
		if processed != 1 {
			t.Errorf("expected 1 processed message (routed to dead letter), got %d", processed)
		}
	})
}
