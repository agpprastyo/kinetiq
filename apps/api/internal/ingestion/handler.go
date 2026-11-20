package ingestion

import (
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/auth"
	"kinetiq/internal/observability"
	"kinetiq/internal/response"
	"kinetiq/internal/validator"
)

type Handler struct {
	service     Service
	keyAuth     auth.APIKeyValidator
	validator   *validator.CustomValidator
	enricher    Enricher
	logger      *slog.Logger
	maxBatchLen int
}

func NewHandler(
	service Service,
	keyAuth auth.APIKeyValidator,
	v *validator.CustomValidator,
	logger *slog.Logger,
	maxBatchLen int,
) *Handler {
	return &Handler{
		service:     service,
		keyAuth:     keyAuth,
		validator:   v,
		enricher:    NewEnricher(),
		logger:      logger,
		maxBatchLen: maxBatchLen,
	}
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

	// 1. Authenticate API Key
	authHeader := c.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		observability.DefaultMetrics.IngestRequestRejected("unauthorized")
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Bearer API key")
	}
	rawKey := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))

	projectID, err := h.keyAuth.ValidateKey(ctx, rawKey)
	if err != nil {
		observability.DefaultMetrics.IngestRequestRejected("invalid_key")
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Invalid API key")
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

	// 5. Client Metadata Enrichment (Zero-alloc User-Agent & Geo-IP parsing)
	if h.enricher != nil {
		clientIP := c.IP()
		if fwd := c.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
				clientIP = strings.TrimSpace(parts[0])
			}
		}
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
