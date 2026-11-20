package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const StreamKey = "events:stream"

var (
	ErrNotFound = errors.New("dead letter record not found")
)

type DeadLetterItem struct {
	ID          int64           `json:"id"`
	ProjectID   *string         `json:"project_id"`
	ProjectName *string         `json:"project_name,omitempty"`
	EventID     *string         `json:"event_id"`
	Reason      string          `json:"reason"`
	Payload     json.RawMessage `json:"payload"`
	Attempts    int32           `json:"attempts"`
	Status      string          `json:"status"`
	FailedAt    time.Time       `json:"failed_at"`
	ReplayedAt  *time.Time      `json:"replayed_at,omitempty"`
}

type DLQSummary struct {
	TotalItems    int64 `json:"total_items"`
	PendingItems  int64 `json:"pending_items"`
	ReplayedItems int64 `json:"replayed_items"`
}

type Service interface {
	List(ctx context.Context, limit int, offset int) ([]DeadLetterItem, *DLQSummary, error)
	Replay(ctx context.Context, id int64) error
	ReplayAll(ctx context.Context) (int, error)
	Purge(ctx context.Context, id *int64) (int, error)
}

type DLQService struct {
	pool   *pgxpool.Pool
	redis  *redis.Client
	logger *slog.Logger
}

func NewService(pool *pgxpool.Pool, redisClient *redis.Client, logger *slog.Logger) *DLQService {
	return &DLQService{
		pool:   pool,
		redis:  redisClient,
		logger: logger,
	}
}

func (s *DLQService) List(ctx context.Context, limit int, offset int) ([]DeadLetterItem, *DLQSummary, error) {
	if s.pool == nil {
		return []DeadLetterItem{}, &DLQSummary{}, nil
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	// 1. Get summary counts
	var total, pending, replayed int64
	_ = s.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*)::bigint,
			COUNT(CASE WHEN status = 'failed' THEN 1 END)::bigint,
			COUNT(CASE WHEN status = 'replayed' THEN 1 END)::bigint
		FROM event_dead_letters
	`).Scan(&total, &pending, &replayed)

	summary := &DLQSummary{
		TotalItems:    total,
		PendingItems:  pending,
		ReplayedItems: replayed,
	}

	// 2. Fetch records
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.project_id::text, p.name, d.event_id, d.reason,
		       d.payload, d.attempts, d.status, d.failed_at, d.replayed_at
		FROM event_dead_letters d
		LEFT JOIN projects p ON p.id = d.project_id
		ORDER BY d.failed_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, summary, err
	}
	defer rows.Close()

	var items []DeadLetterItem
	for rows.Next() {
		var item DeadLetterItem
		var rawPayload []byte
		if err := rows.Scan(
			&item.ID, &item.ProjectID, &item.ProjectName, &item.EventID, &item.Reason,
			&rawPayload, &item.Attempts, &item.Status, &item.FailedAt, &item.ReplayedAt,
		); err != nil {
			continue
		}
		item.Payload = rawPayload
		items = append(items, item)
	}

	if items == nil {
		items = []DeadLetterItem{}
	}

	return items, summary, nil
}

func (s *DLQService) Replay(ctx context.Context, id int64) error {
	if s.pool == nil || s.redis == nil {
		return errors.New("database or redis unavailable")
	}

	var rawPayload []byte
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT payload, status FROM event_dead_letters WHERE id = $1
	`, id).Scan(&rawPayload, &status)
	if err != nil {
		return ErrNotFound
	}

	// Enqueue back into Redis Stream
	err = s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamKey,
		Values: map[string]interface{}{
			"data": string(rawPayload),
		},
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to replay to redis stream: %w", err)
	}

	// Mark as replayed
	_, err = s.pool.Exec(ctx, `
		UPDATE event_dead_letters
		SET status = 'replayed', replayed_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (s *DLQService) ReplayAll(ctx context.Context) (int, error) {
	if s.pool == nil || s.redis == nil {
		return 0, errors.New("database or redis unavailable")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, payload FROM event_dead_letters
		WHERE status = 'failed'
		ORDER BY failed_at ASC
		LIMIT 500
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type pendingItem struct {
		id      int64
		payload []byte
	}
	var batch []pendingItem
	for rows.Next() {
		var item pendingItem
		if err := rows.Scan(&item.id, &item.payload); err == nil {
			batch = append(batch, item)
		}
	}

	if len(batch) == 0 {
		return 0, nil
	}

	pipe := s.redis.Pipeline()
	for _, item := range batch {
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: StreamKey,
			Values: map[string]interface{}{
				"data": string(item.payload),
			},
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to batch enqueue to stream: %w", err)
	}

	var ids []int64
	for _, b := range batch {
		ids = append(ids, b.id)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE event_dead_letters
		SET status = 'replayed', replayed_at = NOW()
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return len(batch), err
	}

	return len(batch), nil
}

func (s *DLQService) Purge(ctx context.Context, id *int64) (int, error) {
	if s.pool == nil {
		return 0, nil
	}

	if id != nil {
		res, err := s.pool.Exec(ctx, `DELETE FROM event_dead_letters WHERE id = $1`, *id)
		if err != nil {
			return 0, err
		}
		return int(res.RowsAffected()), nil
	}

	res, err := s.pool.Exec(ctx, `DELETE FROM event_dead_letters`)
	if err != nil {
		return 0, err
	}
	return int(res.RowsAffected()), nil
}
