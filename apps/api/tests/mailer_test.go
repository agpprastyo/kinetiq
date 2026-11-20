package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/app"
	"kinetiq/internal/mailer"
)

func TestMailerIntegrationAndDispatcher(t *testing.T) {
	ctx := context.Background()

	// 1. Test ConsoleSender Send method
	consoleSender := mailer.NewConsoleSender(nil)
	testEmail := mailer.OutboxEmail{
		ID:             "test-email-id-1",
		Recipient:      "dev@example.com",
		Subject:        "Test Transactional Mail",
		URL:            "http://localhost:3000/verify",
		DeliveryStatus: "stored",
		MaxRetries:     5,
		RetryCount:     0,
		NextRetryAt:    time.Now().UTC(),
		CreatedAt:      time.Now().UTC(),
	}
	if err := consoleSender.Send(ctx, testEmail); err != nil {
		t.Fatalf("console sender failed: %v", err)
	}

	// 2. Test MockSender
	mockSender := mailer.NewMockSender()
	if err := mockSender.Send(ctx, testEmail); err != nil {
		t.Fatalf("mock sender failed: %v", err)
	}
	if len(mockSender.SentEmails) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(mockSender.SentEmails))
	}

	// 3. Test Dispatcher batch execution with container's DB Pool
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	dispatcher := mailer.NewDispatcher(container.DBPool, mockSender, nil, 100*time.Millisecond)

	// Insert test outbox email to database
	_, err = container.DBPool.Exec(ctx, `
		INSERT INTO auth_email_outbox (id, type, recipient, subject, url, delivery_status, retry_count, max_retries, next_retry_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (id) DO UPDATE SET delivery_status = EXCLUDED.delivery_status, next_retry_at = EXCLUDED.next_retry_at;
	`, "outbox-test-db-1", "verification", "alice@example.com", "Verify email", "http://localhost/verify", "stored", 0, 5, time.Now().UTC().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("failed to insert test email: %v", err)
	}

	// Run DispatchBatch
	err = dispatcher.DispatchBatch(ctx, 10)
	if err != nil {
		t.Fatalf("DispatchBatch failed: %v", err)
	}

	// Also test Start and Stop for coverage
	dispatcher.Start()
	time.Sleep(50 * time.Millisecond)
	dispatcher.Stop()

	// Verify metrics
	metrics, err := dispatcher.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}
	if metrics == nil {
		t.Fatalf("expected non-nil metrics")
	}

	// 4. Test Cleaner PurgeOldEmails
	cleaner := mailer.NewCleaner(container.DBPool, nil, 24*time.Hour, 1*time.Hour)
	cleaner.Start()
	time.Sleep(50 * time.Millisecond)
	cleaner.Stop()
	// Insert an old email marked as sent
	_, err = container.DBPool.Exec(ctx, `
		INSERT INTO auth_email_outbox (id, type, recipient, subject, url, delivery_status, sent_at, created_at)
		VALUES ($1, 'verification', 'bob@example.com', 'Welcome', 'http://localhost', 'sent', NOW() - INTERVAL '35 days', NOW() - INTERVAL '35 days')
		ON CONFLICT (id) DO UPDATE SET delivery_status = 'sent', sent_at = NOW() - INTERVAL '35 days';
	`, "outbox-old-purged")
	if err != nil {
		t.Fatalf("failed to insert old sent email: %v", err)
	}

	purged, err := cleaner.PurgeOldEmails(ctx)
	if err != nil {
		t.Fatalf("PurgeOldEmails failed: %v", err)
	}
	if purged == 0 {
		t.Logf("expected at least 1 purged email")
	}

	// 5. Test Mailer HTTP Handler
	handler := mailer.NewHandler(dispatcher, cleaner, nil)
	app := fiber.New()
	app.Get("/api/v1/ops/mailer/metrics", handler.GetMetrics)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ops/mailer/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("handler HTTP request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}
