package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/auth"
	"kinetiq/internal/observability"
	"kinetiq/internal/response"
	"kinetiq/internal/validator"
)

// Limiter throttles ingestion per project and invalid-key attempts per IP.
type Limiter interface {
	AllowProject(ctx context.Context, projectID uuid.UUID, n int) (bool, time.Duration)
	IPBlocked(ctx context.Context, ip string) (bool, time.Duration)
	RecordBadKey(ctx context.Context, ip string)
}

type Handler struct {
	limiter     Limiter
	service     Service
	keyAuth     auth.APIKeyValidator
	validator   *validator.CustomValidator
	enricher    Enricher
	logger      *slog.Logger
	maxBatchLen int
	maxPropLen  int64
}

func NewHandler(
	service Service,
	keyAuth auth.APIKeyValidator,
	v *validator.CustomValidator,
	logger *slog.Logger,
	maxBatchLen int,
	maxPropLen int64,
) *Handler {
	return &Handler{
		service:     service,
		keyAuth:     keyAuth,
		validator:   v,
		enricher:    NewEnricher(),
		logger:      logger,
		maxBatchLen: maxBatchLen,
		maxPropLen:  maxPropLen,
	}
}

func (h *Handler) SetLimiter(l Limiter) { h.limiter = l }

// tooManyRequests writes 429 + Retry-After (whole seconds, rounded up).
func tooManyRequests(c *fiber.Ctx, retry time.Duration, code, msg string) error {
	secs := int64((retry + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	c.Set("Retry-After", strconv.FormatInt(secs, 10))
	return response.Error(c, fiber.StatusTooManyRequests, code, msg)
}

// AuthenticateKey validates the Bearer API key, throttling IPs that keep
// presenting invalid keys. Returns a non-nil error response when it already replied.
func AuthenticateKey(c *fiber.Ctx, keys auth.APIKeyValidator, l Limiter) (uuid.UUID, bool, error) {
	ctx := c.UserContext()
	ip := c.IP()
	if l != nil {
		if blocked, retry := l.IPBlocked(ctx, ip); blocked {
			observability.DefaultMetrics.IngestRequestRejected("ip_throttled")
			return uuid.Nil, false, tooManyRequests(c, retry, "RATE_LIMITED", "Too many invalid API key attempts")
		}
	}
	authHeader := c.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		observability.DefaultMetrics.IngestRequestRejected("unauthorized")
		return uuid.Nil, false, response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Bearer API key")
	}
	projectID, err := keys.ValidateKey(ctx, strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")))
	if err != nil {
		if l != nil {
			l.RecordBadKey(ctx, ip)
		}
		observability.DefaultMetrics.IngestRequestRejected("invalid_key")
		return uuid.Nil, false, response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Invalid API key")
	}
	return projectID, true, nil
}

// IngestEvents godoc
// @Summary Ingest one or a batch of analytics events
// @Description Accepts single event or batch, authenticates with project API key, and enqueues to Redis
// @Tags ingestion
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param Authorization header string true "Project API Key (Bearer sp_live_...)"
// @Success 202 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 503 {object} response.Response
// @Router /api/v1/ingest [post]
func (h *Handler) IngestEvents(c *fiber.Ctx) error {
	ctx := c.UserContext()

	// 1. Authenticate API Key (IP-throttled on repeated invalid keys)
	projectID, ok, err := AuthenticateKey(c, h.keyAuth, h.limiter)
	if !ok {
		return err
	}

	// 2. Parse payload (single or batch)
	body := c.Body()
	events, err := NormalizeEvents(body)
	if err != nil || len(events) == 0 {
		observability.DefaultMetrics.IngestRequestRejected("invalid_body")
		return response.Error(c, fiber.StatusBadRequest, "INVALID_BODY", "Invalid event JSON payload")
	}

	observability.DefaultMetrics.IngestEventsReceived(len(events))

	// 3. Check batch limits
	if h.maxBatchLen > 0 && len(events) > h.maxBatchLen {
		observability.DefaultMetrics.IngestRequestRejected("batch_too_large")
		return response.Error(c, fiber.StatusBadRequest, "BATCH_TOO_LARGE", "Batch size exceeds configured limit")
	}

	// 3b. Per-project events/sec limit (projects.rate_limit_per_sec)
	if h.limiter != nil {
		if allowed, retry := h.limiter.AllowProject(ctx, projectID, len(events)); !allowed {
			observability.DefaultMetrics.IngestRequestRejected("rate_limited")
			return tooManyRequests(c, retry, "RATE_LIMITED", "Project rate limit exceeded")
		}
	}

	// 4. Atomic Validation: every event must be valid
	for idx, evt := range events {
		if errs := h.validator.Validate(evt); len(errs) > 0 {
			var details []response.ErrorDetail
			for _, e := range errs {
				details = append(details, response.ErrorDetail{
					Field:   e.Field,
					Message: e.Message,
				})
			}
			if h.logger != nil {
				h.logger.WarnContext(ctx, "ingestion batch rejected: validation failure",
					slog.Int("event_index", idx),
				)
			}
			observability.DefaultMetrics.IngestRequestRejected("validation_error")
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Validation failed on event", details...)
		}
	}

	// 4b. Per-event properties size cap (measured before enrichment adds server-side keys)
	if h.maxPropLen > 0 {
		for idx, evt := range events {
			if raw, _ := json.Marshal(evt.Properties); int64(len(raw)) > h.maxPropLen {
				observability.DefaultMetrics.IngestRequestRejected("properties_too_large")
				return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Validation failed on event",
					response.ErrorDetail{Field: fmt.Sprintf("events[%d].properties", idx), Message: fmt.Sprintf("properties exceed %d bytes", h.maxPropLen)})
			}
		}
	}

	// 5. Client Metadata Enrichment (Zero-alloc User-Agent & Geo-IP parsing)
	if h.enricher != nil {
		// c.IP() honours ProxyHeader only from TRUSTED_PROXIES (see app.newFiberConfig).
		clientIP := c.IP()
		ua := c.Get("User-Agent")
		cfCountry := c.Get("CF-IPCountry")
		meta := h.enricher.Enrich(clientIP, ua, cfCountry)

		for i := range events {
			if events[i].Properties == nil {
				events[i].Properties = make(map[string]interface{})
			}
			// Precedence rule: do not overwrite existing explicit developer values
			if _, exists := events[i].Properties["$geo_country"]; !exists && meta.Country != "" {
				events[i].Properties["$geo_country"] = meta.Country
			}
			if _, exists := events[i].Properties["$geo_city"]; !exists && meta.City != "" {
				events[i].Properties["$geo_city"] = meta.City
			}
			if _, exists := events[i].Properties["$device_type"]; !exists && meta.DeviceType != "" {
				events[i].Properties["$device_type"] = meta.DeviceType
			}
			if _, exists := events[i].Properties["$os"]; !exists && meta.OS != "" {
				events[i].Properties["$os"] = meta.OS
			}
			if _, exists := events[i].Properties["$browser"]; !exists && meta.Browser != "" {
				events[i].Properties["$browser"] = meta.Browser
			}
		}
	}

	// 6. Enqueue to Redis Stream
	accepted, err := h.service.EnqueueEvents(ctx, projectID, events)
	if err != nil {
		observability.DefaultMetrics.IngestRequestRejected("queue_unavailable")
		return response.Error(c, fiber.StatusServiceUnavailable, "QUEUE_UNAVAILABLE", "Queue service unavailable")
	}

	observability.DefaultMetrics.IngestEventsAccepted(accepted)

	return response.Success(c, fiber.StatusAccepted, IngestResponseData{
		Status:   "accepted",
		Accepted: accepted,
		Rejected: 0,
	})
}
