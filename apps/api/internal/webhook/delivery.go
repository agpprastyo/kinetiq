package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

// DeliveryConfig holds webhook delivery settings
type DeliveryConfig struct {
	Timeout        time.Duration
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// DefaultConfig returns default delivery config
func DefaultConfig() DeliveryConfig {
	return DeliveryConfig{
		Timeout:        5 * time.Second,
		MaxRetries:     5,
		InitialBackoff: 5 * time.Second,
		MaxBackoff:     30 * time.Minute,
	}
}

// Payload represents webhook payload
type Payload struct {
	EventID   string
	EventName string
	ProjectID string
	Data      []byte
}

// DeliveryResult holds result of delivery attempt
type DeliveryResult struct {
	StatusCode     int
	ResponseTimeMs int
	Error          error
	Success        bool
}

// Dispatcher handles webhook delivery
type Dispatcher struct {
	config DeliveryConfig
	client *http.Client
}

// NewDispatcher creates a new dispatcher
func NewDispatcher(cfg DeliveryConfig) *Dispatcher {
	return &Dispatcher{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// SignPayloadHMAC signs payload with HMAC-SHA256
func (d *Dispatcher) SignPayloadHMAC(secret, payload string) (string, string) {
	ts := time.Now()
	timestamp := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return "sha256=" + sig, timestamp
}

// ComputeBackoff computes exponential backoff for attempt
func (d *Dispatcher) ComputeBackoff(attempt int) time.Duration {
	backoff := d.config.InitialBackoff * time.Duration(1<<uint(attempt-1))
	if backoff > d.config.MaxBackoff {
		backoff = d.config.MaxBackoff
	}
	return backoff
}

// CircuitState represents circuit breaker state
type CircuitState int32

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implements simple circuit breaker
type CircuitBreaker struct {
	state            int32
	failCount        int32
	successCount     int32
	failThreshold    int32
	successThreshold int32
	openUntil        int64
}

// NewCircuitBreaker creates circuit breaker
func NewCircuitBreaker(failThreshold, successThreshold int32) *CircuitBreaker {
	return &CircuitBreaker{
		failThreshold:    failThreshold,
		successThreshold: successThreshold,
		state:            int32(StateClosed),
	}
}

// GetState returns current state
func (cb *CircuitBreaker) GetState() CircuitState {
	if atomic.LoadInt32(&cb.state) == int32(StateOpen) {
		if time.Now().UnixNano() > atomic.LoadInt64(&cb.openUntil) {
			atomic.StoreInt32(&cb.state, int32(StateHalfOpen))
		}
	}
	return CircuitState(atomic.LoadInt32(&cb.state))
}

// RecordSuccess records success
func (cb *CircuitBreaker) RecordSuccess() {
	state := cb.GetState()
	if state == StateHalfOpen {
		if atomic.AddInt32(&cb.successCount, 1) >= cb.successThreshold {
			atomic.StoreInt32(&cb.state, int32(StateClosed))
			atomic.StoreInt32(&cb.failCount, 0)
			atomic.StoreInt32(&cb.successCount, 0)
		}
	} else if state == StateClosed {
		atomic.StoreInt32(&cb.failCount, 0)
	}
}

// RecordFailure records failure
func (cb *CircuitBreaker) RecordFailure() {
	state := cb.GetState()
	if state == StateHalfOpen {
		cb.Open()
		return
	}
	if atomic.AddInt32(&cb.failCount, 1) >= cb.failThreshold {
		cb.Open()
	}
}

func (cb *CircuitBreaker) Open() {
	atomic.StoreInt32(&cb.state, int32(StateOpen))
	atomic.StoreInt64(&cb.openUntil, time.Now().Add(5*time.Minute).UnixNano())
	atomic.StoreInt32(&cb.failCount, 0)
	atomic.StoreInt32(&cb.successCount, 0)
}

// Deliver sends webhook with retries
func (d *Dispatcher) Deliver(ctx context.Context, targetURL, secret string, payload []byte) DeliveryResult {
	start := time.Now()
	var lastErr error
	var lastStatus int

	for attempt := 1; attempt <= d.config.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, nil)
		if err != nil {
			lastErr = err
			if attempt < d.config.MaxRetries {
				time.Sleep(d.ComputeBackoff(attempt))
			}
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		sig, ts := d.SignPayloadHMAC(secret, string(payload))
		req.Header.Set("X-Kinetiq-Signature", sig)
		req.Header.Set("X-Kinetiq-Timestamp", ts)

		resp, err := d.client.Do(req)
		elapsed := time.Since(start).Milliseconds()
		if err != nil {
			lastErr = err
			if attempt < d.config.MaxRetries {
				time.Sleep(d.ComputeBackoff(attempt))
			}
			continue
		}
		lastStatus = resp.StatusCode
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return DeliveryResult{StatusCode: resp.StatusCode, ResponseTimeMs: int(elapsed), Success: true}
		}
		lastErr = fmt.Errorf("bad status: %d", resp.StatusCode)
		if attempt < d.config.MaxRetries {
			time.Sleep(d.ComputeBackoff(attempt))
		}
	}

	return DeliveryResult{StatusCode: lastStatus, ResponseTimeMs: int(time.Since(start).Milliseconds()), Error: lastErr, Success: false}
}
