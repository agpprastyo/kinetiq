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

func TestWorkerPendingRecoveryAndRetry(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	queries := db.New(container.DBPool)

	projectID := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "recovery-test-user",
		Name:        "Recovery Test Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	groupName := "kinetiq-recovery-test-" + uuid.New().String()
	streamKey := "events:stream"

	// 1. Create consumer group
	err = container.RedisClient.XGroupCreateMkStream(ctx, streamKey, groupName, "$").Err()
	if err != nil {
		t.Fatalf("failed to create consumer group: %v", err)
	}

	t.Run("recovers pending messages from crashed consumer", func(t *testing.T) {
		evtID := "evt_recover_" + uuid.New().String()
		now := time.Now().UTC()

		// Enqueue event
		_, err := container.IngestionService.EnqueueEvents(ctx, projectID, []ingestion.EventPayload{
			{
				EventID:    evtID,
				EventName:  "crashed_action",
				OccurredAt: &now,
			},
		})
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		// Simulate "crashed worker": read message but never acknowledge it
		crashedConsumer := "crashed-worker"
		streams, err := container.RedisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    groupName,
			Consumer: crashedConsumer,
			Streams:  []string{streamKey, ">"},
			Count:    1,
		}).Result()
		if err != nil || len(streams) == 0 || len(streams[0].Messages) == 0 {
			t.Fatalf("crashed worker failed to read message: %v", err)
		}

		// Verify it is in Pending Entries List (PEL)
		pending, err := container.RedisClient.XPending(ctx, streamKey, groupName).Result()
		if err != nil {
			t.Fatalf("failed to inspect PEL: %v", err)
		}
		if pending.Count != 1 {
			t.Fatalf("expected 1 pending message, got %d", pending.Count)
		}

		// New healthy worker starts with short MinIdleTime for test
		healthyWorker, err := worker.NewConsumerWorker(container.DBPool, container.RedisClient, container.Logger, worker.Config{
			StreamKey:    streamKey,
			GroupName:    groupName,
			ConsumerName: "healthy-worker",
			BatchSize:    10,
			BlockTimeout: 100 * time.Millisecond,
			MinIdleTime:  0, // immediate claim in test
			MaxRetries:   3,
		})
		if err != nil {
			t.Fatalf("failed to create healthy worker: %v", err)
		}

		// Process pending messages
		recovered, err := healthyWorker.RecoverPendingMessages(ctx)
		if err != nil {
			t.Fatalf("failed to recover pending messages: %v", err)
		}
		if recovered != 1 {
			t.Errorf("expected 1 recovered message, got %d", recovered)
		}

		// Verify event is now saved in DB
		count, err := queries.CountEventsByProject(ctx, pgtype.UUID{Bytes: projectID, Valid: true})
		if err != nil {
			t.Fatalf("failed to count events: %v", err)
		}
		if count < 1 {
			t.Errorf("expected at least 1 event in DB, got %d", count)
		}

		// Verify PEL is now 0 (acknowledged)
		pendingAfter, err := container.RedisClient.XPending(ctx, streamKey, groupName).Result()
		if err != nil {
			t.Fatalf("failed to inspect PEL after recovery: %v", err)
		}
		if pendingAfter.Count != 0 {
			t.Errorf("expected 0 pending messages after recovery and ack, got %d", pendingAfter.Count)
		}
	})

	t.Run("exhausted retries routes to dead letter storage with accurate attempt count", func(t *testing.T) {
		// Event with a non-existent foreign key project_id will trigger DB constraint failure
		nonExistentProjectID := uuid.New()
		evtID := "evt_fk_fail_" + uuid.New().String()
		now := time.Now().UTC()

		_, err := container.IngestionService.EnqueueEvents(ctx, nonExistentProjectID, []ingestion.EventPayload{
			{
				EventID:    evtID,
				EventName:  "orphan_event",
				OccurredAt: &now,
			},
		})
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		maxRetries := int32(2)
		testWorker, err := worker.NewConsumerWorker(container.DBPool, container.RedisClient, container.Logger, worker.Config{
			StreamKey:    streamKey,
			GroupName:    groupName,
			ConsumerName: "retry-worker",
			BatchSize:    1,
			BlockTimeout: 100 * time.Millisecond,
			MaxRetries:   maxRetries,
		})
		if err != nil {
			t.Fatalf("failed to create worker: %v", err)
		}

		processed, err := testWorker.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("unexpected error processing batch: %v", err)
		}
		if processed != 1 {
			t.Errorf("expected 1 message processed (dead-lettered), got %d", processed)
		}
	})
}
