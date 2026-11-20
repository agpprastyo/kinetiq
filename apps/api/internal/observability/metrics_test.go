package observability

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func parseMetricInt(t *testing.T, output, prefix string) int64 {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix+" ") {
			valStr := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			v, err := strconv.ParseInt(valStr, 10, 64)
			if err != nil {
				t.Fatalf("failed to parse int for %q from line %q: %v", prefix, line, err)
			}
			return v
		}
	}
	t.Fatalf("metric line with prefix %q not found in output:\n%s", prefix, output)
	return 0
}

func TestFormatPrometheus_SingleFastRequestNotDoubleAccumulated(t *testing.T) {
	reg := NewMetricsRegistry()
	reg.RecordHTTPRequest("GET", "/health", 200, 1*time.Millisecond)

	out := reg.FormatPrometheus()

	for _, le := range defaultBuckets {
		key := fmt.Sprintf(`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="%.3g"}`, le)
		got := parseMetricInt(t, out, key)
		if got != 1 {
			t.Errorf("bucket %s = %d, want 1 (double-accumulation bug)", key, got)
		}
	}

	infKey := `http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="+Inf"}`
	countKey := `http_request_duration_seconds_count{method="GET",route="/health",status="200"}`
	infVal := parseMetricInt(t, out, infKey)
	countVal := parseMetricInt(t, out, countKey)
	if infVal != 1 || countVal != 1 {
		t.Errorf("expected +Inf=1 and count=1, got +Inf=%d count=%d", infVal, countVal)
	}
}

func TestFormatPrometheus_HistogramBucketsCumulativeAndMonotonic(t *testing.T) {
	reg := NewMetricsRegistry()
	// 1ms (<= 0.005), 50ms (<= 0.05), 15s (> 10, only in +Inf)
	reg.RecordHTTPRequest("POST", "/api/v1/ingest", 202, 1*time.Millisecond)
	reg.RecordHTTPRequest("POST", "/api/v1/ingest", 202, 50*time.Millisecond)
	reg.RecordHTTPRequest("POST", "/api/v1/ingest", 202, 15*time.Second)

	out := reg.FormatPrometheus()

	expected := map[float64]int64{
		0.005: 1,
		0.01:  1,
		0.025: 1,
		0.05:  2,
		0.1:   2,
		0.25:  2,
		0.5:   2,
		1:     2,
		2.5:   2,
		5:     2,
		10:    2,
	}

	var prev int64
	for _, le := range defaultBuckets {
		key := fmt.Sprintf(`http_request_duration_seconds_bucket{method="POST",route="/api/v1/ingest",status="202",le="%.3g"}`, le)
		got := parseMetricInt(t, out, key)
		if want := expected[le]; got != want {
			t.Errorf("bucket le=%.3g = %d, want %d", le, got, want)
		}
		if got < prev {
			t.Errorf("histogram buckets not monotonic at le=%.3g: %d < %d", le, got, prev)
		}
		prev = got
	}

	infVal := parseMetricInt(t, out, `http_request_duration_seconds_bucket{method="POST",route="/api/v1/ingest",status="202",le="+Inf"}`)
	countVal := parseMetricInt(t, out, `http_request_duration_seconds_count{method="POST",route="/api/v1/ingest",status="202"}`)
	if infVal != 3 || countVal != 3 {
		t.Errorf("expected +Inf=3 and count=3, got +Inf=%d count=%d", infVal, countVal)
	}
	if prev > infVal {
		t.Errorf("last finite bucket (%d) exceeds +Inf (%d)", prev, infVal)
	}
}

func TestMetricsHandler_BearerTokenAuth(t *testing.T) {
	t.Run("no auth token allows unauthenticated access", func(t *testing.T) {
		app := fiber.New()
		app.Get("/metrics", MetricsHandler())

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
			t.Errorf("unexpected Content-Type: %q", ct)
		}
	})

	t.Run("configured auth token enforces bearer auth", func(t *testing.T) {
		app := fiber.New()
		app.Get("/metrics", MetricsHandler("secret-metrics-token"))

		// Missing header -> 401
		reqMissing := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		respMissing, err := app.Test(reqMissing)
		if err != nil {
			t.Fatalf("missing-auth request failed: %v", err)
		}
		if respMissing.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for missing auth header, got %d", respMissing.StatusCode)
		}

		// Wrong token -> 401
		reqWrong := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		reqWrong.Header.Set("Authorization", "Bearer wrong-token")
		respWrong, err := app.Test(reqWrong)
		if err != nil {
			t.Fatalf("wrong-auth request failed: %v", err)
		}
		if respWrong.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for wrong token, got %d", respWrong.StatusCode)
		}

		// Valid token -> 200
		reqValid := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		reqValid.Header.Set("Authorization", "Bearer secret-metrics-token")
		respValid, err := app.Test(reqValid)
		if err != nil {
			t.Fatalf("valid-auth request failed: %v", err)
		}
		if respValid.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for valid token, got %d", respValid.StatusCode)
		}
		body, _ := io.ReadAll(respValid.Body)
		if !strings.Contains(string(body), "http_requests_total") {
			t.Errorf("expected Prometheus exposition body, got: %s", string(body))
		}
	})
}

func TestMetricsRegistry_FormatPrometheus_CoreAndDay2OpsMetrics(t *testing.T) {
	reg := NewMetricsRegistry()

	reg.RecordHTTPRequest("POST", "/api/v1/ingest", 202, 25*time.Millisecond)
	reg.IngestEventsReceived(50)
	reg.IngestEventsAccepted(48)
	reg.IngestRequestRejected("rate_limited")
	reg.WorkerEventPersisted(48)
	reg.WorkerRetry(2)
	reg.WorkerDeadLetter(1)
	reg.CacheHit()
	reg.CacheMiss()

	// Fase 31 Day-2 Ops metrics
	reg.SetWorkerQueueLag(12500)
	reg.SetDBPoolStats(18, 7, 25)
	reg.SetDegradedMode("ingest_buffering", true)
	reg.SetDegradedMode("read_only_dashboard", false)

	out := reg.FormatPrometheus()

	expectedSnippets := []string{
		`http_requests_total{method="POST",route="/api/v1/ingest",status="202"} 1`,
		`http_request_duration_seconds_count{method="POST",route="/api/v1/ingest",status="202"} 1`,
		`ingest_events_received_total 50`,
		`ingest_events_accepted_total 48`,
		`ingest_requests_rejected_total{reason="rate_limited"} 1`,
		`worker_events_persisted_total 48`,
		`worker_retries_total 2`,
		`worker_dead_letter_total 1`,
		`analytics_cache_hits_total 1`,
		`analytics_cache_misses_total 1`,
		`# TYPE worker_queue_lag gauge`,
		`worker_queue_lag 12500`,
		`# TYPE db_pool_acquired_conns gauge`,
		`db_pool_acquired_conns 18`,
		`# TYPE db_pool_idle_conns gauge`,
		`db_pool_idle_conns 7`,
		`# TYPE db_pool_max_conns gauge`,
		`db_pool_max_conns 25`,
		`# TYPE kinetiq_degraded_mode gauge`,
		`kinetiq_degraded_mode{mode="ingest_buffering"} 1`,
		`kinetiq_degraded_mode{mode="read_only_dashboard"} 0`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(out, snippet) {
			t.Errorf("expected Prometheus output to contain %q, got:\n%s", snippet, out)
		}
	}

	if !reg.IsDegraded() {
		t.Error("expected IsDegraded() to return true when ingest_buffering is active")
	}

	reg.SetDegradedMode("ingest_buffering", false)
	if reg.IsDegraded() {
		t.Error("expected IsDegraded() to return false after clearing all degraded modes")
	}
}

func TestMetricsHandler_AuthTokenEnforcement(t *testing.T) {
	app := fiber.New()
	app.Get("/metrics", MetricsHandler("secret-scrape-token"))

	reqNoAuth := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	respNoAuth, err := app.Test(reqNoAuth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respNoAuth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth token, got %d", respNoAuth.StatusCode)
	}

	reqAuth := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reqAuth.Header.Set("Authorization", "Bearer secret-scrape-token")
	respAuth, err := app.Test(reqAuth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respAuth.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with valid auth token, got %d", respAuth.StatusCode)
	}
	body, _ := io.ReadAll(respAuth.Body)
	if !strings.Contains(string(body), "worker_queue_lag") {
		t.Errorf("expected /metrics body to include worker_queue_lag, got: %s", string(body))
	}
}
