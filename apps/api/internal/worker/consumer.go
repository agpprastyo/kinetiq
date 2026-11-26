package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/alerting"
	"kinetiq/internal/ingestion"
	"kinetiq/internal/observability"
	"kinetiq/internal/person"
)

type PublishedEvent struct {
	EventID    string                 `json:"event_id"`
	ProjectID  string                 `json:"project_id"`
	Name       string                 `json:"name"`
	DistinctID string                 `json:"distinct_id,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Timestamp  string                 `json:"timestamp"`
}

func PublishLiveEvent(ctx context.Context, rdb *redis.Client, projectID string, event PublishedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return rdb.Publish(ctx, fmt.Sprintf("events:pubsub:%s", projectID), payload).Err()
}

type Config struct {
	StreamKey    string
	GroupName    string
	ConsumerName string
	StartPos     string
	BatchSize    int64
	BlockTimeout time.Duration
	MinIdleTime  time.Duration
	MaxRetries   int32
}

func DefaultConfig() Config {
	return Config{
		StreamKey:    "events:stream",
		GroupName:    "kinetiq-workers",
		ConsumerName: fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		StartPos:     "$",
		BatchSize:    100,
		BlockTimeout: 2 * time.Second,
		MinIdleTime:  30 * time.Second,
		MaxRetries:   5,
	}
}

type ConsumerWorker struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	redis       *redis.Client
	logger      *slog.Logger
	cfg         Config
	alertingSvc alerting.Service
	persons     *person.Service
}

func (w *ConsumerWorker) SetAlertingService(svc alerting.Service) {
	w.alertingSvc = svc
}

func NewConsumerWorker(pool *pgxpool.Pool, redisClient *redis.Client, logger *slog.Logger, cfg Config) (*ConsumerWorker, error) {
	if cfg.StreamKey == "" {
		cfg.StreamKey = "events:stream"
	}
	if cfg.GroupName == "" {
		cfg.GroupName = "kinetiq-workers"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.BlockTimeout <= 0 {
		cfg.BlockTimeout = 2 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.MinIdleTime < 0 {
		cfg.MinIdleTime = 30 * time.Second
	}

	w := &ConsumerWorker{
		pool:    pool,
		queries: db.New(pool),
		persons: person.NewService(pool),
		redis:   redisClient,
		logger:  logger,
		cfg:     cfg,
	}

	// Ensure consumer group exists
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.ensureGroup(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize consumer group: %w", err)
	}

	return w, nil
}

func (w *ConsumerWorker) ensureGroup(ctx context.Context) error {
	startPos := w.cfg.StartPos
	if startPos == "" {
		startPos = "$"
	}
	err := w.redis.XGroupCreateMkStream(ctx, w.cfg.StreamKey, w.cfg.GroupName, startPos).Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

// RecoverPendingMessages claims messages left pending by crashed or timed out consumers
func (w *ConsumerWorker) RecoverPendingMessages(ctx context.Context) (int, error) {
	claimed, _, err := w.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   w.cfg.StreamKey,
		Group:    w.cfg.GroupName,
		Consumer: w.cfg.ConsumerName,
		MinIdle:  w.cfg.MinIdleTime,
		Start:    "0-0",
		Count:    w.cfg.BatchSize,
	}).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}

	if len(claimed) == 0 {
		return 0, nil
	}

	processedCount := 0
	for _, msg := range claimed {
		if err := w.processMessage(ctx, msg); err != nil {
			if w.logger != nil {
				w.logger.ErrorContext(ctx, "failed to process recovered pending message",
					slog.String("message_id", msg.ID),
					slog.String("error", err.Error()),
				)
			}
			continue
		}
		processedCount++
	}

	return processedCount, nil
}

// ProcessBatch reads a batch of messages from the consumer group and persists them
func (w *ConsumerWorker) ProcessBatch(ctx context.Context) (int, error) {
	streams, err := w.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.cfg.GroupName,
		Consumer: w.cfg.ConsumerName,
		Streams:  []string{w.cfg.StreamKey, ">"},
		Count:    w.cfg.BatchSize,
		Block:    w.cfg.BlockTimeout,
	}).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return 0, nil
	}

	messages := streams[0].Messages
	processedCount := 0

	// Finish (and ack) every message already read even if shutdown was requested;
	// cmd/worker bounds the wait. Unfinished ones stay in the PEL and are recovered.
	ctx = context.WithoutCancel(ctx)
	for _, msg := range messages {
		if err := w.processMessage(ctx, msg); err != nil {
			if w.logger != nil {
				w.logger.ErrorContext(ctx, "failed to process message",
					slog.String("message_id", msg.ID),
					slog.String("error", err.Error()),
				)
			}
			continue
		}
		processedCount++
	}

	// Rolling stream trimming: keep approx up to 500k events in the stream buffer to avoid unbounded memory growth
	_ = w.redis.XTrimMaxLenApprox(ctx, w.cfg.StreamKey, 500000, 0).Err()

	return processedCount, nil
}

func (w *ConsumerWorker) processMessage(ctx context.Context, msg redis.XMessage) error {
	rawPayload, ok := msg.Values["data"].(string)
	if !ok {
		return w.handleDeadLetter(ctx, msg, "missing or invalid data field in stream message", []byte(fmt.Sprintf("%v", msg.Values)), 1, nil, "")
	}

	var envelope ingestion.QueueMessage
	if err := json.Unmarshal([]byte(rawPayload), &envelope); err != nil {
		// Poison message: cannot parse envelope JSON -> route to dead letter storage
		return w.handleDeadLetter(ctx, msg, fmt.Sprintf("invalid JSON envelope: %v", err), []byte(rawPayload), 1, nil, "")
	}

	propsBytes, err := json.Marshal(envelope.Properties)
	if err != nil {
		propsBytes = []byte("{}")
	}

	distinctID := pgtype.Text{Valid: false}
	if envelope.DistinctID != "" {
		distinctID = pgtype.Text{String: envelope.DistinctID, Valid: true}
	}

	// Retry loop with exponential backoff & jitter for transient DB errors
	var persistErr error
	for attempt := int32(1); attempt <= w.cfg.MaxRetries; attempt++ {
		persistErr = w.queries.InsertEvent(ctx, db.InsertEventParams{
			ProjectID:  pgtype.UUID{Bytes: envelope.ProjectID, Valid: true},
			EventID:    envelope.EventID,
			EventName:  envelope.EventName,
			OccurredAt: pgtype.Timestamptz{Time: envelope.OccurredAt, Valid: true},
			ReceivedAt: pgtype.Timestamptz{Time: envelope.ReceivedAt, Valid: true},
			DistinctID: distinctID,
			Properties: propsBytes,
		})
		if persistErr == nil {
			observability.DefaultMetrics.WorkerEventPersisted(1)
			break
		}

		observability.DefaultMetrics.WorkerRetry(1)

		if w.logger != nil {
			w.logger.WarnContext(ctx, "transient error inserting event, retrying with backoff",
				slog.String("event_id", envelope.EventID),
				slog.Int("attempt", int(attempt)),
				slog.String("error", persistErr.Error()),
			)
		}

		// Exponential backoff: base 50ms * 2^(attempt-1) + jitter
		backoff := time.Duration(50*(1<<uint(attempt-1)))*time.Millisecond + time.Duration(rand.Intn(20))*time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	if persistErr != nil {
		// Exhausted retries -> route to dead letter
		deadLetterErr := w.handleDeadLetter(ctx, msg, fmt.Sprintf("database persistence failed after %d retries: %v", w.cfg.MaxRetries, persistErr), []byte(rawPayload), w.cfg.MaxRetries, &envelope.ProjectID, envelope.EventID)
		if deadLetterErr != nil {
			return fmt.Errorf("failed to dead-letter message: %w (original error: %v)", deadLetterErr, persistErr)
		}
		return nil
	}

	// Resolve person + apply $set/$set_once. Event is already persisted, so failures are logged, not dead-lettered.
	if envelope.DistinctID != "" {
		set, _ := envelope.Properties["$set"].(map[string]interface{})
		setOnce, _ := envelope.Properties["$set_once"].(map[string]interface{})
		if err := w.persons.Capture(ctx, envelope.ProjectID, envelope.DistinctID, set, setOnce); err != nil && !errors.Is(err, person.ErrInvalidID) && w.logger != nil {
			w.logger.WarnContext(ctx, "failed to resolve person for event",
				slog.String("event_id", envelope.EventID),
				slog.String("error", err.Error()),
			)
		}
	}

	// Broadcast to real-time pubsub subscribers
	liveEvt := PublishedEvent{
		EventID:    envelope.EventID,
		ProjectID:  envelope.ProjectID.String(),
		Name:       envelope.EventName,
		DistinctID: envelope.DistinctID,
		Properties: envelope.Properties,
		Timestamp:  envelope.OccurredAt.UTC().Format(time.RFC3339),
	}
	_ = PublishLiveEvent(ctx, w.redis, envelope.ProjectID.String(), liveEvt)

	// Evaluate alert rules if alerting engine is enabled
	if w.alertingSvc != nil {
		go func(pID uuid.UUID, evName string) {
			evalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = w.alertingSvc.TrackEventAndEvaluate(evalCtx, pID, evName)
		}(envelope.ProjectID, envelope.EventName)
	}

	// Acknowledge after successful commit
	if err := w.redis.XAck(ctx, w.cfg.StreamKey, w.cfg.GroupName, msg.ID).Err(); err != nil {
		return fmt.Errorf("failed to ack message %s: %w", msg.ID, err)
	}

	return nil
}

func (w *ConsumerWorker) handleDeadLetter(ctx context.Context, msg redis.XMessage, reason string, payload []byte, attempts int32, projectID *uuid.UUID, eventID string) error {
	observability.DefaultMetrics.WorkerDeadLetter(1)
	if w.logger != nil {
		w.logger.WarnContext(ctx, "routing message to dead-letter storage",
			slog.String("message_id", msg.ID),
			slog.String("reason", reason),
			slog.Int("attempts", int(attempts)),
		)
	}

	// Ensure payload is valid JSONB
	jsonPayload := payload
	var js json.RawMessage
	if err := json.Unmarshal(payload, &js); err != nil {
		wrapped, err := json.Marshal(map[string]string{
			"raw": string(payload),
		})
		if err == nil {
			jsonPayload = wrapped
		} else {
			jsonPayload = []byte("{}")
		}
	}

	pgProjectID := pgtype.UUID{Valid: false}
	if projectID != nil {
		pgProjectID = pgtype.UUID{Bytes: *projectID, Valid: true}
	}
	pgEventID := pgtype.Text{Valid: false}
	if eventID != "" {
		pgEventID = pgtype.Text{String: eventID, Valid: true}
	}

	_, err := w.queries.InsertDeadLetter(ctx, db.InsertDeadLetterParams{
		ProjectID: pgProjectID,
		EventID:   pgEventID,
		Reason:    reason,
		Payload:   jsonPayload,
		Attempts:  attempts,
		FailedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		// If project_id foreign key constraint failed, fall back to NULL project_id
		if strings.Contains(err.Error(), "foreign key") || strings.Contains(err.Error(), "23503") {
			_, err = w.queries.InsertDeadLetter(ctx, db.InsertDeadLetterParams{
				ProjectID: pgtype.UUID{Valid: false},
				EventID:   pgEventID,
				Reason:    reason,
				Payload:   jsonPayload,
				Attempts:  attempts,
				FailedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			})
		}
		if err != nil {
			return fmt.Errorf("failed to insert dead letter: %w", err)
		}
	}

	// Acknowledge the poisoned message so stream doesn't block
	return w.redis.XAck(ctx, w.cfg.StreamKey, w.cfg.GroupName, msg.ID).Err()
}

// Start runs the worker loop until context is canceled, recovering pending messages periodically
func (w *ConsumerWorker) Start(ctx context.Context) {
	if w.logger != nil {
		w.logger.Info("starting stream worker consumer loop",
			slog.String("group", w.cfg.GroupName),
			slog.String("consumer", w.cfg.ConsumerName),
			slog.String("stream", w.cfg.StreamKey),
		)
	}

	recoveryTicker := time.NewTicker(30 * time.Second)
	defer recoveryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			if w.logger != nil {
				w.logger.Info("shutting down stream worker loop")
			}
			return
		case <-recoveryTicker.C:
			if _, err := w.RecoverPendingMessages(ctx); err != nil && !errors.Is(err, context.Canceled) {
				if w.logger != nil {
					w.logger.Error("error recovering pending messages", slog.String("error", err.Error()))
				}
			}
		default:
			_, err := w.ProcessBatch(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				if w.logger != nil {
					w.logger.Error("error processing batch from stream", slog.String("error", err.Error()))
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}
