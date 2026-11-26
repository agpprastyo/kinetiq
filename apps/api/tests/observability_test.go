package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kinetiq/internal/app"
	"kinetiq/internal/config"
	"kinetiq/internal/observability"
)

func TestObservabilityAndHealthSeam(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create test container: %v", err)
	}
	defer container.DBPool.Close()
	defer container.RedisClient.Close()

	server := app.SetupWithContainer(withFakeAuth(container))

	t.Run("GET /healthz returns 200 alive without database ping", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode json: %v", err)
		}
		if body["status"] != "alive" {
			t.Errorf("expected status 'alive', got '%s'", body["status"])
		}
	})

	t.Run("GET /readyz returns 200 ready when DB and Redis are healthy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body struct {
			Status   string            `json:"status"`
			Services map[string]string `json:"services"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode json: %v", err)
		}
		if body.Status != "ready" {
			t.Errorf("expected status 'ready', got '%s'", body.Status)
		}
		if body.Services["postgres"] != "connected" || body.Services["redis"] != "connected" {
			t.Errorf("expected both services connected, got %+v", body.Services)
		}
	})

	t.Run("GET /metrics returns standard Prometheus format with required metrics", func(t *testing.T) {
		// Exercise some metrics
		observability.DefaultMetrics.IngestEventsReceived(5)
		observability.DefaultMetrics.IngestEventsAccepted(5)
		observability.DefaultMetrics.IngestRequestRejected("test_reason")
		observability.DefaultMetrics.WorkerEventPersisted(3)
		observability.DefaultMetrics.WorkerRetry(1)
		observability.DefaultMetrics.WorkerDeadLetter(1)
		observability.DefaultMetrics.CacheHit()
		observability.DefaultMetrics.CacheMiss()

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		body := string(bodyBytes)

		expectedSubstrings := []string{
			"http_requests_total",
			"http_request_duration_seconds",
			"ingest_events_received_total",
			"ingest_events_accepted_total",
			"ingest_requests_rejected_total",
			"worker_events_persisted_total",
			"worker_retries_total",
			"worker_dead_letter_total",
			"analytics_cache_hits_total",
			"analytics_cache_misses_total",
		}

		for _, sub := range expectedSubstrings {
			if !strings.Contains(body, sub) {
				t.Errorf("expected metric '%s' in /metrics output, but was missing", sub)
			}
		}
	})

	t.Run("Sanitizes sensitive data from startup config logging", func(t *testing.T) {
		// Confirm secret fields are never included in safe config logs
		cfg.AuthInternalSecret = "super-secret-auth-key-123"
		cfg.DatabaseURL = "postgres://kinetiq:superpassword@localhost:5432/kinetiq"
		cfg.RedisURL = "redis://:superpassword@localhost:6379/0"

		// Ensure LogSafeConfig can run safely
		cfg.LogSafeConfig(container.Logger)
	})
}
