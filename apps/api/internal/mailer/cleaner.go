package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Cleaner struct {
	pool      *pgxpool.Pool
	logger    *slog.Logger
	retention time.Duration
	interval  time.Duration
	stopChan  chan struct{}
	wg        sync.WaitGroup
	isStarted bool
	mu        sync.Mutex
}

func NewCleaner(pool *pgxpool.Pool, logger *slog.Logger, retention, interval time.Duration) *Cleaner {
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	if interval <= 0 {
		interval = 12 * time.Hour
	}
	return &Cleaner{
		pool:      pool,
		logger:    logger,
		retention: retention,
		interval:  interval,
		stopChan:  make(chan struct{}),
	}
}

func (c *Cleaner) Start() {
	c.mu.Lock()
	if c.isStarted {
		c.mu.Unlock()
		return
	}
	c.isStarted = true
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-c.stopChan:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_, _ = c.PurgeOldEmails(ctx)
				cancel()
			}
		}
	}()
}

func (c *Cleaner) Stop() {
	c.mu.Lock()
	if !c.isStarted {
		c.mu.Unlock()
		return
	}
	close(c.stopChan)
	c.isStarted = false
	c.mu.Unlock()
	c.wg.Wait()
}

// PurgeOldEmails deletes emails marked as 'sent' older than the retention threshold
func (c *Cleaner) PurgeOldEmails(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-c.retention)
	query := `
		DELETE FROM auth_email_outbox
		WHERE delivery_status = 'sent'
		  AND sent_at IS NOT NULL
		  AND sent_at < $1;
	`
	tag, err := c.pool.Exec(ctx, query, cutoff)
	if err != nil {
		if c.logger != nil {
			c.logger.ErrorContext(ctx, "failed to purge old sent emails from outbox", slog.String("error", err.Error()))
		}
		return 0, fmt.Errorf("failed to purge sent emails: %w", err)
	}

	deleted := tag.RowsAffected()
	if c.logger != nil && deleted > 0 {
		c.logger.InfoContext(ctx, "successfully purged old sent emails from outbox",
			slog.Int64("deleted_count", deleted),
			slog.Time("cutoff", cutoff),
		)
	}
	return deleted, nil
}
