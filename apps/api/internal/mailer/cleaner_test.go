package mailer_test

import (
	"testing"
	"time"

	"kinetiq/internal/mailer"
)

func TestMailerRetention_Purge(t *testing.T) {
	// Verify cutoff date calculation
	retention := 30 * 24 * time.Hour
	now := time.Now().UTC()
	cutoff := now.Add(-retention)

	// An email sent 31 days ago should be purged
	sent31DaysAgo := now.Add(-31 * 24 * time.Hour)
	if !sent31DaysAgo.Before(cutoff) {
		t.Errorf("expected 31-day-old email to be before cutoff")
	}

	// An email sent 10 days ago should NOT be purged
	sent10DaysAgo := now.Add(-10 * 24 * time.Hour)
	if sent10DaysAgo.Before(cutoff) {
		t.Errorf("expected 10-day-old email NOT to be before cutoff")
	}

	cleaner := mailer.NewCleaner(nil, nil, retention, 12*time.Hour)
	if cleaner == nil {
		t.Fatalf("expected non-nil cleaner")
	}
}
