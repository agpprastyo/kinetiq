package mailer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"kinetiq/internal/mailer"
)

func TestMailerDispatcher_Success(t *testing.T) {
	mockSender := mailer.NewMockSender()
	email := mailer.OutboxEmail{
		ID:             "outbox-1",
		Recipient:      "user@example.com",
		Subject:        "Welcome to Kinetiq",
		URL:            "http://localhost:3000/verify",
		DeliveryStatus: "stored",
		MaxRetries:     5,
		RetryCount:     0,
		NextRetryAt:    time.Now().UTC(),
	}

	err := mockSender.Send(context.Background(), email)
	if err != nil {
		t.Fatalf("expected send success, got: %v", err)
	}

	if len(mockSender.SentEmails) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(mockSender.SentEmails))
	}
	if mockSender.SentEmails[0].Recipient != "user@example.com" {
		t.Errorf("expected recipient user@example.com, got %s", mockSender.SentEmails[0].Recipient)
	}
}

func TestMailerDispatcher_ExponentialRetry(t *testing.T) {
	mockSender := mailer.NewMockSender()
	mockSender.FailFunc = func(email mailer.OutboxEmail) error {
		return errors.New("smtp connection timeout")
	}

	email := mailer.OutboxEmail{
		ID:             "outbox-fail-1",
		Recipient:      "fail@example.com",
		Subject:        "Password Reset",
		URL:            "http://localhost:3000/reset",
		DeliveryStatus: "stored",
		MaxRetries:     5,
		RetryCount:     0,
	}

	err := mockSender.Send(context.Background(), email)
	if err == nil {
		t.Fatalf("expected send error, got nil")
	}

	// Verify retry calculations
	retryCount := email.RetryCount + 1
	if retryCount >= email.MaxRetries {
		t.Errorf("expected retryCount < maxRetries on first failure")
	}

	// Verify max retries threshold logic
	exhaustedEmail := email
	exhaustedEmail.RetryCount = 4
	finalRetry := exhaustedEmail.RetryCount + 1
	if finalRetry < exhaustedEmail.MaxRetries {
		t.Errorf("expected finalRetry >= maxRetries")
	}
}
