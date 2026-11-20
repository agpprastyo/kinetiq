package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MailerMetrics struct {
	TotalStored   int64 `json:"total_stored"`
	TotalRetrying int64 `json:"total_retrying"`
	TotalSent     int64 `json:"total_sent"`
	TotalFailed   int64 `json:"total_failed"`
}

type Dispatcher struct {
	pool      *pgxpool.Pool
	sender    EmailSender
	logger    *slog.Logger
	interval  time.Duration
	stopChan  chan struct{}
	wg        sync.WaitGroup
	isStarted bool
	mu        sync.Mutex
}

func NewDispatcher(pool *pgxpool.Pool, sender EmailSender, logger *slog.Logger, interval time.Duration) *Dispatcher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Dispatcher{
		pool:     pool,
		sender:   sender,
		logger:   logger,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (d *Dispatcher) Start() {
	d.mu.Lock()
	if d.isStarted {
		d.mu.Unlock()
		return
	}
	d.isStarted = true
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()

		for {
			select {
			case <-d.stopChan:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				_ = d.DispatchBatch(ctx, 50)
				cancel()
			}
		}
	}()
}

func (d *Dispatcher) Stop() {
	d.mu.Lock()
	if !d.isStarted {
		d.mu.Unlock()
		return
	}
	close(d.stopChan)
	d.isStarted = false
	d.mu.Unlock()
	d.wg.Wait()
}

// DispatchBatch processes a batch of emails using row-level locking (FOR UPDATE SKIP LOCKED)
func (d *Dispatcher) DispatchBatch(ctx context.Context, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 50
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		SELECT id, type, recipient, subject, url, provider, delivery_status, error_message, retry_count, max_retries, next_retry_at, sent_at, created_at
		FROM auth_email_outbox
		WHERE delivery_status IN ('stored', 'retrying')
		  AND next_retry_at <= NOW()
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED;
	`

	rows, err := tx.Query(ctx, query, batchSize)
	if err != nil {
		return fmt.Errorf("failed to select outbox batch: %w", err)
	}
	defer rows.Close()

	var emails []OutboxEmail
	for rows.Next() {
		var em OutboxEmail
		if err := rows.Scan(
			&em.ID, &em.Type, &em.Recipient, &em.Subject, &em.URL,
			&em.Provider, &em.DeliveryStatus, &em.ErrorMessage,
			&em.RetryCount, &em.MaxRetries, &em.NextRetryAt, &em.SentAt, &em.CreatedAt,
		); err != nil {
			return fmt.Errorf("failed to scan outbox row: %w", err)
		}
		emails = append(emails, em)
	}
	rows.Close()

	if len(emails) == 0 {
		return tx.Commit(ctx)
	}

	for _, em := range emails {
		sendErr := d.sender.Send(ctx, em)
		if sendErr == nil {
			// Delivery succeeded
			_, err = tx.Exec(ctx, `
				UPDATE auth_email_outbox
				SET delivery_status = 'sent',
				    sent_at = NOW(),
				    error_message = NULL
				WHERE id = $1;
			`, em.ID)
			if err != nil {
				return fmt.Errorf("failed to update sent status: %w", err)
			}
		} else {
			// Delivery failed -> calculate exponential backoff with jitter
			nextRetryCount := em.RetryCount + 1
			if nextRetryCount >= em.MaxRetries {
				errMsg := sendErr.Error()
				_, err = tx.Exec(ctx, `
					UPDATE auth_email_outbox
					SET delivery_status = 'failed',
					    retry_count = $2,
					    error_message = $3
					WHERE id = $1;
				`, em.ID, nextRetryCount, errMsg)
				if err != nil {
					return fmt.Errorf("failed to mark email failed: %w", err)
				}
			} else {
				errMsg := sendErr.Error()
				// 2^retry_count * 30 seconds + jitter (0-5s)
				backoffSec := int(math.Pow(2, float64(nextRetryCount))) * 30
				jitter := rand.Intn(5)
				nextRetryTime := time.Now().UTC().Add(time.Duration(backoffSec+jitter) * time.Second)

				_, err = tx.Exec(ctx, `
					UPDATE auth_email_outbox
					SET delivery_status = 'retrying',
					    retry_count = $2,
					    next_retry_at = $3,
					    error_message = $4
					WHERE id = $1;
				`, em.ID, nextRetryCount, nextRetryTime, errMsg)
				if err != nil {
					return fmt.Errorf("failed to mark email retrying: %w", err)
				}
			}
		}
	}

	return tx.Commit(ctx)
}

// GetMetrics returns statistics on the current auth_email_outbox state
func (d *Dispatcher) GetMetrics(ctx context.Context) (*MailerMetrics, error) {
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN delivery_status = 'stored' THEN 1 ELSE 0 END), 0) AS total_stored,
			COALESCE(SUM(CASE WHEN delivery_status = 'retrying' THEN 1 ELSE 0 END), 0) AS total_retrying,
			COALESCE(SUM(CASE WHEN delivery_status = 'sent' THEN 1 ELSE 0 END), 0) AS total_sent,
			COALESCE(SUM(CASE WHEN delivery_status = 'failed' THEN 1 ELSE 0 END), 0) AS total_failed
		FROM auth_email_outbox;
	`
	var m MailerMetrics
	err := d.pool.QueryRow(ctx, query).Scan(&m.TotalStored, &m.TotalRetrying, &m.TotalSent, &m.TotalFailed)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	return &m, nil
}
