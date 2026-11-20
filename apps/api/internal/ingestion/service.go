package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	StreamKey = "events:stream"
	// MaxPipelineBatch defines maximum items sent to Redis in a single pipeline execution
	MaxPipelineBatch = 250
)

type Service interface {
	EnqueueEvents(ctx context.Context, projectID uuid.UUID, events []EventPayload) (int, error)
}

type IngestionService struct {
	redis  *redis.Client
	logger *slog.Logger
}

func NewIngestionService(redisClient *redis.Client, logger *slog.Logger) *IngestionService {
	return &IngestionService{
		redis:  redisClient,
		logger: logger,
	}
}

func (s *IngestionService) EnqueueEvents(ctx context.Context, projectID uuid.UUID, events []EventPayload) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	now := time.Now().UTC()

	// Chunk execution if event count exceeds MaxPipelineBatch to prevent high Redis latency & buffer allocation
	for chunkStart := 0; chunkStart < len(events); chunkStart += MaxPipelineBatch {
		chunkEnd := chunkStart + MaxPipelineBatch
		if chunkEnd > len(events) {
			chunkEnd = len(events)
		}
		chunk := events[chunkStart:chunkEnd]

		pipe := s.redis.Pipeline()
		for _, evt := range chunk {
			occurredAt := now
			if evt.OccurredAt != nil {
				occurredAt = evt.OccurredAt.UTC()
			}
			props := evt.Properties
			if props == nil {
				props = make(map[string]interface{})
			}

			envelope := QueueMessage{
				ProjectID:  projectID,
				EventID:    evt.EventID,
				EventName:  evt.EventName,
				OccurredAt: occurredAt,
				ReceivedAt: now,
				DistinctID: evt.DistinctID,
				Properties: props,
			}

			payloadBytes, err := json.Marshal(envelope)
			if err != nil {
				return chunkStart, fmt.Errorf("failed to marshal envelope: %w", err)
			}

			pipe.XAdd(ctx, &redis.XAddArgs{
				Stream: StreamKey,
				Values: map[string]interface{}{
					"data": string(payloadBytes),
				},
			})
		}

		_, err := pipe.Exec(ctx)
		if err != nil {
			if s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to enqueue events to redis stream",
					slog.String("stream", StreamKey),
					slog.String("error", err.Error()),
					slog.Int("processed", chunkStart),
				)
			}
			return chunkStart, err
		}
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx, "successfully enqueued events to redis stream",
			slog.String("project_id", projectID.String()),
			slog.Int("event_count", len(events)),
		)
	}

	return len(events), nil
}
