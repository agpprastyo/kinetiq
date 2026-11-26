package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/response"
)

var defaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type httpHistogram struct {
	buckets []int64
	sum     float64
	count   int64
}

type MetricsRegistry struct {
	mu sync.RWMutex

	// HTTP metrics
	httpRequestsTotal map[string]*int64 // key: method:route:status
	httpDuration      map[string]*httpHistogram

	// Ingestion metrics
	ingestEventsReceivedTotal   int64
	ingestEventsAcceptedTotal   int64
	ingestRequestsRejectedTotal map[string]*int64 // key: reason

	// Worker metrics
	workerEventsPersistedTotal int64
	workerRetriesTotal         int64
	workerDeadLetterTotal      int64

	// Analytics cache metrics
	analyticsCacheHitsTotal   int64
	analyticsCacheMissesTotal int64

	// Day-2 Ops gauges (Fase 31)
	workerQueueLag      int64
	dbPoolAcquiredConns int64
	dbPoolIdleConns     int64
	dbPoolMaxConns      int64
	degradedModes       map[string]int64
}

var DefaultMetrics = NewMetricsRegistry()

func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		httpRequestsTotal:           make(map[string]*int64),
		httpDuration:                make(map[string]*httpHistogram),
		ingestRequestsRejectedTotal: make(map[string]*int64),
		degradedModes:               make(map[string]int64),
	}
}

func (m *MetricsRegistry) RecordHTTPRequest(method, route string, status int, duration time.Duration) {
	if route == "" {
		route = "unknown"
	}
	key := fmt.Sprintf("%s:%s:%d", method, route, status)

	m.mu.Lock()
	ptr, exists := m.httpRequestsTotal[key]
	if !exists {
		var n int64
		ptr = &n
		m.httpRequestsTotal[key] = ptr
	}
	hist, existsDur := m.httpDuration[key]
	if !existsDur {
		h := &httpHistogram{buckets: make([]int64, len(defaultBuckets)+1)}
		m.httpDuration[key] = h
		hist = h
	}
	secs := duration.Seconds()
	hist.sum += secs
	hist.count++
	for i, le := range defaultBuckets {
		if secs <= le {
			hist.buckets[i]++
		}
	}
	hist.buckets[len(defaultBuckets)]++

	m.mu.Unlock()

	atomic.AddInt64(ptr, 1)
}

func (m *MetricsRegistry) IngestEventsReceived(n int) {
	atomic.AddInt64(&m.ingestEventsReceivedTotal, int64(n))
}

func (m *MetricsRegistry) IngestEventsAccepted(n int) {
	atomic.AddInt64(&m.ingestEventsAcceptedTotal, int64(n))
}

func (m *MetricsRegistry) IngestRequestRejected(reason string) {
	m.mu.Lock()
	ptr, exists := m.ingestRequestsRejectedTotal[reason]
	if !exists {
		var n int64
		ptr = &n
		m.ingestRequestsRejectedTotal[reason] = ptr
	}
	m.mu.Unlock()

	atomic.AddInt64(ptr, 1)
}

func (m *MetricsRegistry) WorkerEventPersisted(n int) {
	atomic.AddInt64(&m.workerEventsPersistedTotal, int64(n))
}

func (m *MetricsRegistry) WorkerRetry(n int) {
	atomic.AddInt64(&m.workerRetriesTotal, int64(n))
}

func (m *MetricsRegistry) WorkerDeadLetter(n int) {
	atomic.AddInt64(&m.workerDeadLetterTotal, int64(n))
}

func (m *MetricsRegistry) CacheHit() {
	atomic.AddInt64(&m.analyticsCacheHitsTotal, 1)
}

func (m *MetricsRegistry) CacheMiss() {
	atomic.AddInt64(&m.analyticsCacheMissesTotal, 1)
}

func (m *MetricsRegistry) SetWorkerQueueLag(lag int64) {
	if lag < 0 {
		lag = 0
	}
	atomic.StoreInt64(&m.workerQueueLag, lag)
}

func (m *MetricsRegistry) SetDBPoolStats(acquired, idle, max int32) {
	atomic.StoreInt64(&m.dbPoolAcquiredConns, int64(acquired))
	atomic.StoreInt64(&m.dbPoolIdleConns, int64(idle))
	atomic.StoreInt64(&m.dbPoolMaxConns, int64(max))
}

func (m *MetricsRegistry) SetDegradedMode(mode string, active bool) {
	if mode == "" {
		return
	}
	val := int64(0)
	if active {
		val = 1
	}
	m.mu.Lock()
	m.degradedModes[mode] = val
	m.mu.Unlock()
}

func (m *MetricsRegistry) IsDegraded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, v := range m.degradedModes {
		if v > 0 {
			return true
		}
	}
	return false
}

// FormatPrometheus generates standard Prometheus exposition text
func (m *MetricsRegistry) FormatPrometheus() string {
	var sb strings.Builder

	sb.WriteString("# HELP http_requests_total Total number of HTTP requests processed\n")
	sb.WriteString("# TYPE http_requests_total counter\n")
	m.mu.RLock()
	for k, v := range m.httpRequestsTotal {
		parts := strings.Split(k, ":")
		if len(parts) == 3 {
			sb.WriteString(fmt.Sprintf("http_requests_total{method=\"%s\",route=\"%s\",status=\"%s\"} %d\n", parts[0], parts[1], parts[2], atomic.LoadInt64(v)))
		}
	}

	sb.WriteString("# HELP http_request_duration_seconds Duration of HTTP requests in seconds\n")
	sb.WriteString("# TYPE http_request_duration_seconds histogram\n")
	for k, hist := range m.httpDuration {
		parts := strings.Split(k, ":")
		if len(parts) == 3 {
			for i, le := range defaultBuckets {
				sb.WriteString(fmt.Sprintf("http_request_duration_seconds_bucket{method=\"%s\",route=\"%s\",status=\"%s\",le=\"%.3g\"} %d\n", parts[0], parts[1], parts[2], le, hist.buckets[i]))
			}
			sb.WriteString(fmt.Sprintf("http_request_duration_seconds_bucket{method=\"%s\",route=\"%s\",status=\"%s\",le=\"+Inf\"} %d\n", parts[0], parts[1], parts[2], hist.count))
			sb.WriteString(fmt.Sprintf("http_request_duration_seconds_sum{method=\"%s\",route=\"%s\",status=\"%s\"} %.6f\n", parts[0], parts[1], parts[2], hist.sum))
			sb.WriteString(fmt.Sprintf("http_request_duration_seconds_count{method=\"%s\",route=\"%s\",status=\"%s\"} %d\n", parts[0], parts[1], parts[2], hist.count))
		}
	}

	sb.WriteString("# HELP ingest_events_received_total Total event objects received by ingestion endpoint\n")
	sb.WriteString("# TYPE ingest_events_received_total counter\n")
	sb.WriteString(fmt.Sprintf("ingest_events_received_total %d\n", atomic.LoadInt64(&m.ingestEventsReceivedTotal)))

	sb.WriteString("# HELP ingest_events_accepted_total Total event objects accepted and enqueued\n")
	sb.WriteString("# TYPE ingest_events_accepted_total counter\n")
	sb.WriteString(fmt.Sprintf("ingest_events_accepted_total %d\n", atomic.LoadInt64(&m.ingestEventsAcceptedTotal)))

	sb.WriteString("# HELP ingest_requests_rejected_total Total ingestion requests rejected by reason\n")
	sb.WriteString("# TYPE ingest_requests_rejected_total counter\n")
	for reason, v := range m.ingestRequestsRejectedTotal {
		sb.WriteString(fmt.Sprintf("ingest_requests_rejected_total{reason=\"%s\"} %d\n", reason, atomic.LoadInt64(v)))
	}
	m.mu.RUnlock()

	sb.WriteString("# HELP worker_events_persisted_total Total events successfully persisted to PostgreSQL\n")
	sb.WriteString("# TYPE worker_events_persisted_total counter\n")
	sb.WriteString(fmt.Sprintf("worker_events_persisted_total %d\n", atomic.LoadInt64(&m.workerEventsPersistedTotal)))

	sb.WriteString("# HELP worker_retries_total Total retry attempts executed by consumer worker\n")
	sb.WriteString("# TYPE worker_retries_total counter\n")
	sb.WriteString(fmt.Sprintf("worker_retries_total %d\n", atomic.LoadInt64(&m.workerRetriesTotal)))

	sb.WriteString("# HELP worker_dead_letter_total Total messages routed to dead_letter_events table\n")
	sb.WriteString("# TYPE worker_dead_letter_total counter\n")
	sb.WriteString(fmt.Sprintf("worker_dead_letter_total %d\n", atomic.LoadInt64(&m.workerDeadLetterTotal)))

	sb.WriteString("# HELP analytics_cache_hits_total Total analytics query cache hits from Redis\n")
	sb.WriteString("# TYPE analytics_cache_hits_total counter\n")
	sb.WriteString(fmt.Sprintf("analytics_cache_hits_total %d\n", atomic.LoadInt64(&m.analyticsCacheHitsTotal)))

	sb.WriteString("# HELP analytics_cache_misses_total Total analytics query cache misses requiring DB queries\n")
	sb.WriteString("# TYPE analytics_cache_misses_total counter\n")
	sb.WriteString(fmt.Sprintf("analytics_cache_misses_total %d\n", atomic.LoadInt64(&m.analyticsCacheMissesTotal)))

	sb.WriteString("# HELP worker_queue_lag Current pending message lag in Redis ingestion stream\n")
	sb.WriteString("# TYPE worker_queue_lag gauge\n")
	sb.WriteString(fmt.Sprintf("worker_queue_lag %d\n", atomic.LoadInt64(&m.workerQueueLag)))

	sb.WriteString("# HELP db_pool_acquired_conns Current number of acquired PostgreSQL pool connections\n")
	sb.WriteString("# TYPE db_pool_acquired_conns gauge\n")
	sb.WriteString(fmt.Sprintf("db_pool_acquired_conns %d\n", atomic.LoadInt64(&m.dbPoolAcquiredConns)))

	sb.WriteString("# HELP db_pool_idle_conns Current number of idle PostgreSQL pool connections\n")
	sb.WriteString("# TYPE db_pool_idle_conns gauge\n")
	sb.WriteString(fmt.Sprintf("db_pool_idle_conns %d\n", atomic.LoadInt64(&m.dbPoolIdleConns)))

	sb.WriteString("# HELP db_pool_max_conns Configured maximum PostgreSQL pool connections\n")
	sb.WriteString("# TYPE db_pool_max_conns gauge\n")
	sb.WriteString(fmt.Sprintf("db_pool_max_conns %d\n", atomic.LoadInt64(&m.dbPoolMaxConns)))

	sb.WriteString("# HELP kinetiq_degraded_mode Active graceful degradation mode indicator (1 = active, 0 = inactive)\n")
	sb.WriteString("# TYPE kinetiq_degraded_mode gauge\n")
	m.mu.RLock()
	modes := make([]string, 0, len(m.degradedModes))
	for mode := range m.degradedModes {
		modes = append(modes, mode)
	}
	sort.Strings(modes)
	for _, mode := range modes {
		sb.WriteString(fmt.Sprintf("kinetiq_degraded_mode{mode=\"%s\"} %d\n", mode, m.degradedModes[mode]))
	}
	m.mu.RUnlock()

	return sb.String()
}

// MetricsHandler returns a Fiber handler exposing Prometheus formatted metrics.
// If authToken is non-empty, requests must include "Authorization: Bearer <token>".
func MetricsHandler(authToken ...string) fiber.Handler {
	token := ""
	if len(authToken) > 0 {
		token = authToken[0]
	}
	return func(c *fiber.Ctx) error {
		if token != "" {
			auth := c.Get("Authorization")
			if auth != "Bearer "+token {
				return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
			}
		}
		c.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		return c.SendString(DefaultMetrics.FormatPrometheus())
	}
}
