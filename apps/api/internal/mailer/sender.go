package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type OutboxEmail struct {
	ID             string     `json:"id"`
	Type           string     `json:"type"`
	Recipient      string     `json:"recipient"`
	Subject        string     `json:"subject"`
	URL            string     `json:"url"`
	Provider       string     `json:"provider"`
	DeliveryStatus string     `json:"delivery_status"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	RetryCount     int        `json:"retry_count"`
	MaxRetries     int        `json:"max_retries"`
	NextRetryAt    time.Time  `json:"next_retry_at"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type EmailSender interface {
	Send(ctx context.Context, email OutboxEmail) error
}

type MockSender struct {
	SentEmails []OutboxEmail
	FailFunc   func(email OutboxEmail) error
}

func NewMockSender() *MockSender {
	return &MockSender{
		SentEmails: make([]OutboxEmail, 0),
	}
}

func (m *MockSender) Send(ctx context.Context, email OutboxEmail) error {
	if m.FailFunc != nil {
		if err := m.FailFunc(email); err != nil {
			return err
		}
	}
	m.SentEmails = append(m.SentEmails, email)
	return nil
}

type ConsoleSender struct {
	logger *slog.Logger
}

func NewConsoleSender(logger *slog.Logger) *ConsoleSender {
	return &ConsoleSender{logger: logger}
}

func (c *ConsoleSender) Send(ctx context.Context, email OutboxEmail) error {
	if c.logger != nil {
		c.logger.InfoContext(ctx, "dispatching transactional email via console sender",
			slog.String("id", email.ID),
			slog.String("recipient", email.Recipient),
			slog.String("subject", email.Subject),
			slog.String("type", email.Type),
			slog.String("url", email.URL),
		)
	} else {
		fmt.Printf("[MAILER] To: %s | Subject: %s | URL: %s\n", email.Recipient, email.Subject, email.URL)
	}
	return nil
}
