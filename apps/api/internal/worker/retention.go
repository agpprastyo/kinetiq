package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PurgeExpiredEvents deletes events where occurred_at is older than retentionDays
func PurgeExpiredEvents(ctx context.Context, pool *pgxpool.Pool, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	threshold := time.Now().UTC().AddDate(0, 0, -retentionDays)
	tag, err := pool.Exec(ctx, `
		DELETE FROM events
		WHERE occurred_at < $1
	`, threshold)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// StartRetentionSweeper runs periodic purge in background
func StartRetentionSweeper(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, interval time.Duration, retentionDays int) *sync.WaitGroup {
	ticker := time.NewTicker(interval)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				purged, err := PurgeExpiredEvents(ctx, pool, retentionDays)
				if err != nil && logger != nil {
					logger.ErrorContext(ctx, "failed to run retention cleanup", slog.String("error", err.Error()))
				} else if purged > 0 && logger != nil {
					logger.InfoContext(ctx, "retention cleanup completed", slog.Int64("purged_events", purged))
				}
			}
		}
	}()
	return wg
}
