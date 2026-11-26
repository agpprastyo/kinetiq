package ingestion_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/ingestion"
	"kinetiq/internal/validator"
)

type fakeKeys struct{ ok bool }

func (f fakeKeys) ValidateKey(context.Context, string) (uuid.UUID, error) {
	if !f.ok {
		return uuid.Nil, context.Canceled
	}
	return uuid.New(), nil
}

type fakeSvc struct{}

func (fakeSvc) EnqueueEvents(_ context.Context, _ uuid.UUID, e []ingestion.EventPayload) (int, error) {
	return len(e), nil
}

type fakeLimiter struct {
	allow    bool
	blocked  bool
	badKeys  int
	lastIP   string
	retryFor time.Duration
}

func (f *fakeLimiter) AllowProject(context.Context, uuid.UUID, int) (bool, time.Duration) {
	return f.allow, f.retryFor
}
func (f *fakeLimiter) IPBlocked(_ context.Context, ip string) (bool, time.Duration) {
	f.lastIP = ip
	return f.blocked, f.retryFor
}
func (f *fakeLimiter) RecordBadKey(context.Context, string) { f.badKeys++ }

func do(t *testing.T, keysOK bool, l *fakeLimiter) (int, string, string) {
	t.Helper()
	h := ingestion.NewHandler(fakeSvc{}, fakeKeys{keysOK}, validator.New(), nil, 100, 0)
	h.SetLimiter(l)
	app := fiber.New()
	app.Post("/ingest", h.IngestEvents)
	req := httptest.NewRequest("POST", "/ingest", strings.NewReader(`{"event_id":"e1","event_name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tlk_live_abc_def")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	return resp.StatusCode, resp.Header.Get("Retry-After"), string(buf[:n])
}

func TestIngestProjectRateLimitReturns429WithRetryAfter(t *testing.T) {
	code, retry, body := do(t, true, &fakeLimiter{allow: false, retryFor: 1500 * time.Millisecond})
	if code != 429 {
		t.Fatalf("want 429 got %d", code)
	}
	if retry != "2" {
		t.Fatalf("want Retry-After 2 got %q", retry)
	}
	if !strings.Contains(body, `"RATE_LIMITED"`) || !strings.Contains(body, `"success":false`) {
		t.Fatalf("want JSON error envelope, got %s", body)
	}
}

func TestIngestWithinLimitAccepted(t *testing.T) {
	if code, _, _ := do(t, true, &fakeLimiter{allow: true}); code != 202 {
		t.Fatalf("want 202 got %d", code)
	}
}

func TestInvalidKeyRecordedAndBlockedIPThrottled(t *testing.T) {
	l := &fakeLimiter{allow: true}
	if code, _, _ := do(t, false, l); code != 401 || l.badKeys != 1 {
		t.Fatalf("want 401 + recorded bad key, got %d / %d", code, l.badKeys)
	}
	l = &fakeLimiter{allow: true, blocked: true, retryFor: 30 * time.Second}
	code, retry, _ := do(t, false, l)
	if code != 429 || retry != "30" {
		t.Fatalf("blocked IP want 429/30 got %d/%s", code, retry)
	}
	if l.badKeys != 0 {
		t.Fatal("blocked request must not be validated further")
	}
}
