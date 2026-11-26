package analytics

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"kinetiq/internal/project"
	"kinetiq/internal/response"
)

type Handler struct {
	analyticsService Service
	projectService   project.Service
	logger           *slog.Logger
}

func NewHandler(analyticsService Service, projectService project.Service, logger *slog.Logger) *Handler {
	return &Handler{
		analyticsService: analyticsService,
		projectService:   projectService,
		logger:           logger,
	}
}

type httpError struct {
	Status  int
	Code    string
	Message string
}

func (e *httpError) Error() string {
	return e.Message
}

func (h *Handler) verifyProjectOwnership(c *fiber.Ctx) (uuid.UUID, *httpError) {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return uuid.Nil, &httpError{Status: fiber.StatusUnauthorized, Code: "UNAUTHORIZED", Message: "Unauthorized"}
	}

	projectIDStr := c.Params("projectID")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return uuid.Nil, &httpError{Status: fiber.StatusNotFound, Code: "NOT_FOUND", Message: "Project not found"}
	}

	// Verify project ownership
	proj, err := h.projectService.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return uuid.Nil, &httpError{Status: fiber.StatusNotFound, Code: "NOT_FOUND", Message: "Project not found"}
	}

	return projectID, nil
}

func (h *Handler) parseRangeAndVerifyOwnership(c *fiber.Ctx) (uuid.UUID, time.Time, time.Time, *httpError) {
	projectID, httpErr := h.verifyProjectOwnership(c)
	if httpErr != nil {
		return uuid.Nil, time.Time{}, time.Time{}, httpErr
	}

	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		return uuid.Nil, time.Time{}, time.Time{}, &httpError{Status: fiber.StatusBadRequest, Code: "INVALID_DATE_RANGE", Message: "Both 'from' and 'to' query parameters are required"}
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return uuid.Nil, time.Time{}, time.Time{}, &httpError{Status: fiber.StatusBadRequest, Code: "INVALID_DATE_RANGE", Message: "Invalid 'from' timestamp format. Must be RFC3339"}
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return uuid.Nil, time.Time{}, time.Time{}, &httpError{Status: fiber.StatusBadRequest, Code: "INVALID_DATE_RANGE", Message: "Invalid 'to' timestamp format. Must be RFC3339"}
	}

	if !from.Before(to) {
		return uuid.Nil, time.Time{}, time.Time{}, &httpError{Status: fiber.StatusBadRequest, Code: "INVALID_DATE_RANGE", Message: "'from' timestamp must be before 'to' timestamp"}
	}

	// Max range: 90 days
	if to.Sub(from) > 90*24*time.Hour {
		return uuid.Nil, time.Time{}, time.Time{}, &httpError{Status: fiber.StatusBadRequest, Code: "INVALID_DATE_RANGE", Message: "Date range exceeds maximum limit of 90 days"}
	}

	return projectID, from.UTC(), to.UTC(), nil
}

// GetSummary godoc
// @Summary Analytics summary
// @Description Total events and unique distinct IDs in date range
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param from query string true "Start timestamp RFC3339"
// @Param to query string true "End timestamp RFC3339"
// @Router /api/v1/projects/{projectID}/analytics/summary [get]
func (h *Handler) GetSummary(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	opts, optErr := parseQueryOptions(c)
	if optErr != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", optErr.Error())
	}

	summary, err := h.analyticsService.GetSummary(c.UserContext(), projectID, from, to, opts)
	if err != nil {
		return h.fail(c, "failed to get analytics summary", err)
	}

	return response.Success(c, fiber.StatusOK, summary)
}

// GetTopEvents godoc
// @Summary Top events
// @Description Top event names by frequency
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param from query string true "Start timestamp RFC3339"
// @Param to query string true "End timestamp RFC3339"
// @Param limit query int false "Max results (1-50)"
// @Router /api/v1/projects/{projectID}/analytics/top-events [get]
func (h *Handler) GetTopEvents(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	opts, optErr := parseQueryOptions(c)
	if optErr != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", optErr.Error())
	}

	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 || parsedLimit > 50 {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Limit must be an integer between 1 and 50")
		}
		limit = parsedLimit
	}

	topEvents, err := h.analyticsService.GetTopEvents(c.UserContext(), projectID, from, to, limit, opts)
	if err != nil {
		return h.fail(c, "failed to get top events", err)
	}

	return response.Success(c, fiber.StatusOK, topEvents)
}

// GetVolume godoc
// @Summary Event volume time-series
// @Description Event volume bucketed by hour or day
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param from query string true "Start timestamp RFC3339"
// @Param to query string true "End timestamp RFC3339"
// @Param interval query string false "Bucket interval: minute, hour or day"
// @Param tz query string false "IANA time zone for bucketing (default UTC)"
// @Router /api/v1/projects/{projectID}/analytics/volume [get]
func (h *Handler) GetVolume(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	opts, optErr := parseQueryOptions(c)
	if optErr != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", optErr.Error())
	}

	interval := c.Query("interval", "hour")
	if interval != "minute" && interval != "hour" && interval != "day" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Interval must be 'minute', 'hour', or 'day'")
	}

	volume, err := h.analyticsService.GetVolume(c.UserContext(), projectID, from, to, interval, opts)
	if err != nil {
		return h.fail(c, "failed to get volume", err)
	}

	return response.Success(c, fiber.StatusOK, volume)
}

// GetFunnel godoc
// @Summary Funnel analytics
// @Description Per-step person counts, conversion rate and median time-to-convert, computed in SQL
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param from query string true "Start timestamp RFC3339"
// @Param to query string true "End timestamp RFC3339"
// @Param steps query string true "Comma-separated event names (2-5 steps)"
// @Param window query string false "Conversion window from step 1, e.g. 30m, 12h, 7d (1s-90d, default 14d)"
// @Param order query string false "ordered (default) or strict (no other events between steps)"
// @Param filter query string false "Filter DSL JSON"
// @Param breakdown_by query string false "Property to break down by (step-1 event value)"
// @Router /api/v1/projects/{projectID}/analytics/funnel [get]
func (h *Handler) GetFunnel(c *fiber.Ctx) error {
	projectID, from, to, p, opts, httpErr := h.parseFunnelRequest(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}
	funnel, err := h.analyticsService.GetFunnel(c.UserContext(), projectID, from, to, p, opts)
	if err != nil {
		return h.catalogFail(c, "compute funnel", err)
	}
	return response.Success(c, fiber.StatusOK, funnel)
}

// GetFunnelDropoff godoc
// @Summary Funnel drop-off users
// @Description Persons who reached `step` but not the next step, paginated by person key
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param from query string true "Start timestamp RFC3339"
// @Param to query string true "End timestamp RFC3339"
// @Param steps query string true "Comma-separated event names (2-5 steps)"
// @Param step query int true "1-based step the users stopped at (1..len(steps)-1)"
// @Param window query string false "Conversion window (default 14d)"
// @Param order query string false "ordered or strict"
// @Param limit query int false "Page size (1-200, default 50)"
// @Param offset query int false "Offset (default 0)"
// @Router /api/v1/projects/{projectID}/analytics/funnel/dropoff [get]
func (h *Handler) GetFunnelDropoff(c *fiber.Ctx) error {
	projectID, from, to, p, opts, httpErr := h.parseFunnelRequest(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}
	step, err := strconv.Atoi(c.Query("step"))
	if err != nil || step < 1 || step >= len(p.Steps) {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "step must be an integer between 1 and "+strconv.Itoa(len(p.Steps)-1))
	}
	limit, offset := DefaultDropoffPageSz, 0
	if v := c.Query("limit"); v != "" {
		if limit, err = strconv.Atoi(v); err != nil || limit < 1 || limit > MaxDropoffPageSize {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "limit must be an integer between 1 and 200")
		}
	}
	if v := c.Query("offset"); v != "" {
		if offset, err = strconv.Atoi(v); err != nil || offset < 0 {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "offset must be a non-negative integer")
		}
	}
	opts.BreakdownBy = ""
	res, err := h.analyticsService.GetFunnelDropoff(c.UserContext(), projectID, from, to, p, step, limit, offset, opts)
	if err != nil {
		return h.catalogFail(c, "list funnel drop-off", err)
	}
	return response.Success(c, fiber.StatusOK, res)
}

func (h *Handler) parseFunnelRequest(c *fiber.Ctx) (uuid.UUID, time.Time, time.Time, FunnelParams, QueryOptions, *httpError) {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return uuid.Nil, from, to, FunnelParams{}, QueryOptions{}, httpErr
	}
	bad := func(msg string) *httpError {
		return &httpError{Status: fiber.StatusBadRequest, Code: "VALIDATION_ERROR", Message: msg}
	}
	opts, err := parseQueryOptions(c)
	if err != nil {
		return uuid.Nil, from, to, FunnelParams{}, opts, bad(err.Error())
	}
	if c.Query("steps") == "" {
		return uuid.Nil, from, to, FunnelParams{}, opts, bad("Query parameter 'steps' is required")
	}
	p, err := ParseFunnelParams(c.Query("steps"), c.Query("window"), c.Query("order"))
	if err != nil {
		return uuid.Nil, from, to, p, opts, bad(err.Error())
	}
	return projectID, from, to, p, opts, nil
}

func (h *Handler) GetPropertyBreakdown(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	opts, optErr := parseQueryOptions(c)
	if optErr != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", optErr.Error())
	}

	eventName := c.Query("event_name")
	if eventName == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Query parameter 'event_name' is required")
	}

	property := c.Query("property")
	if property == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Query parameter 'property' is required")
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	items, err := h.analyticsService.GetPropertyBreakdown(c.UserContext(), projectID, eventName, property, from, to, limit, opts)
	if err != nil {
		return h.fail(c, "failed to compute property breakdown", err)
	}

	return response.Success(c, fiber.StatusOK, items)
}

func (h *Handler) GetDeviceBreakdown(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	items, err := h.analyticsService.GetDeviceBreakdown(c.UserContext(), projectID, from, to)
	if err != nil {
		return h.fail(c, "failed to compute device breakdown", err)
	}

	return response.Success(c, fiber.StatusOK, items)
}

func (h *Handler) GetGeoBreakdown(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	items, err := h.analyticsService.GetGeoBreakdown(c.UserContext(), projectID, from, to, limit)
	if err != nil {
		return h.fail(c, "failed to compute geo breakdown", err)
	}

	return response.Success(c, fiber.StatusOK, items)
}

func (h *Handler) GetRetention(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	opts, optErr := parseQueryOptions(c)
	if optErr != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", optErr.Error())
	}

	firstEvent := strings.TrimSpace(c.Query("first_event", "signup"))
	returnEvent := strings.TrimSpace(c.Query("return_event", "app_opened"))
	granularity := strings.TrimSpace(c.Query("granularity", "day"))

	res, err := h.analyticsService.GetRetention(c.UserContext(), projectID, from, to, firstEvent, returnEvent, granularity, opts)
	if err != nil {
		return h.fail(c, "failed to compute retention", err)
	}

	return response.Success(c, fiber.StatusOK, res)
}

func (h *Handler) ListUsers(c *fiber.Ctx) error {
	projectID, httpErr := h.verifyProjectOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	search := strings.TrimSpace(c.Query("search"))
	params := response.ParseOffsetParams(c, true)

	users, total, err := h.analyticsService.ListUsers(c.UserContext(), projectID, search, params.Limit, params.Offset)
	if err != nil {
		return h.fail(c, "failed to list analytics users", err)
	}

	return response.Success(c, fiber.StatusOK, response.Paginated(users, params, total))
}

func (h *Handler) GetUserTimeline(c *fiber.Ctx) error {
	projectID, httpErr := h.verifyProjectOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	distinctID := strings.TrimSpace(c.Params("distinctId"))
	if distinctID == "" {
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Missing distinctId parameter")
	}

	params, err := response.ParseCursorParams(c, timelineDefaultLimit, timelineMaxLimit)
	if err != nil {
		return response.InvalidCursor(c)
	}

	var before *time.Time
	if params.Cursor == nil {
		if b := c.Query("before"); b != "" {
			if parsed, perr := time.Parse(time.RFC3339, b); perr == nil {
				slog.Warn("deprecated query param 'before'; use 'cursor'", slog.String("path", c.Path()))
				before = &parsed
			}
		}
	}

	timeline, hasMore, err := h.analyticsService.GetUserTimeline(c.UserContext(), projectID, distinctID, params.Limit, params.Cursor, before)
	if err != nil {
		return h.fail(c, "failed to get user timeline", err)
	}

	var last *response.Cursor
	if n := len(timeline); n > 0 {
		last = &response.Cursor{T: timeline[n-1].OccurredAt, ID: timeline[n-1].EventID}
	}
	return response.Success(c, fiber.StatusOK, response.CursorPaginated(timeline, params.Limit, last, hasMore))
}

// GetSessions godoc
// @Summary Sessions
// @Description Sessions derived from events (30-min inactivity timeout by default)
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param from query string true "Start timestamp RFC3339"
// @Param to query string true "End timestamp RFC3339"
// @Param timeout_minutes query int false "Inactivity timeout in minutes (1-1440, default 30)"
// @Param limit query int false "Max sessions listed (1-200, default 50)"
// @Router /api/v1/projects/{projectID}/analytics/sessions [get]
func (h *Handler) GetSessions(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	timeout := DefaultSessionTimeout
	if v := c.Query("timeout_minutes"); v != "" {
		m, err := strconv.Atoi(v)
		if err != nil || m < 1 || m > 1440 {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "timeout_minutes must be an integer between 1 and 1440")
		}
		timeout = time.Duration(m) * time.Minute
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil || l < 1 || l > 200 {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "limit must be an integer between 1 and 200")
		}
		limit = l
	}

	res, err := h.analyticsService.GetSessions(c.UserContext(), projectID, from, to, timeout, limit)
	if err != nil {
		return h.fail(c, "failed to get sessions", err)
	}
	return response.Success(c, fiber.StatusOK, res)
}

func parseQueryOptions(c *fiber.Ctx) (QueryOptions, error) {
	o, err := ParseQueryOptions(c.Query("filter"), c.Query("breakdown_by"), c.Query("breakdown_scope"), c.Query("breakdown_limit"))
	if err != nil {
		return o, err
	}
	o.TZ, err = ParseTZ(c.Query("tz"))
	return o, err
}

func (h *Handler) catalogLimit(c *fiber.Ctx, def int) (int, error) {
	v := c.Query("limit")
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 500 {
		return 0, errors.New("limit must be an integer between 1 and 500")
	}
	return n, nil
}

func (h *Handler) catalogScope(c *fiber.Ctx) (string, error) {
	scope := c.Query("scope", "event")
	if scope != "event" && scope != "person" {
		return "", errors.New("scope must be 'event' or 'person'")
	}
	return scope, nil
}

func (h *Handler) catalogFail(c *fiber.Ctx, what string, err error) error {
	return h.fail(c, "failed to "+what, err)
}

// fail maps query deadline/cancel-by-timeout to 504 QUERY_TIMEOUT, everything else to a logged 500.
func (h *Handler) fail(c *fiber.Ctx, msg string, err error) error {
	var pgErr *pgconn.PgError
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &pgErr) && pgErr.Code == "57014") {
		return response.Error(c, fiber.StatusGatewayTimeout, "QUERY_TIMEOUT", "Query timed out; narrow the date range or filters")
	}
	if h.logger != nil {
		h.logger.ErrorContext(c.UserContext(), msg, slog.String("error", err.Error()))
	}
	return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}

// QueryTimeout bounds the request context so slow analytics queries are cancelled.
func QueryTimeout(d time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), d)
		defer cancel()
		c.SetUserContext(ctx)
		return c.Next()
	}
}

// ListEvents godoc
// @Summary Event catalog
// @Description Event names for the project with count and last-seen
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param search query string false "Case-insensitive name substring"
// @Param limit query int false "Max results (1-500, default 100)"
// @Router /api/v1/projects/{projectID}/catalog/events [get]
func (h *Handler) ListEvents(c *fiber.Ctx) error {
	projectID, httpErr := h.verifyProjectOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}
	limit, err := h.catalogLimit(c, 100)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	}
	items, err := h.analyticsService.ListEvents(c.UserContext(), projectID, c.Query("search"), limit)
	if err != nil {
		return h.catalogFail(c, "list event catalog", err)
	}
	return response.Success(c, fiber.StatusOK, items)
}

// ListProperties godoc
// @Summary Property catalog
// @Description Event or person properties with inferred type
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param scope query string false "event (default) or person"
// @Param event_name query string false "Restrict event properties to one event"
// @Param search query string false "Case-insensitive name substring"
// @Param limit query int false "Max results (1-500, default 100)"
// @Router /api/v1/projects/{projectID}/catalog/properties [get]
func (h *Handler) ListProperties(c *fiber.Ctx) error {
	projectID, httpErr := h.verifyProjectOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}
	scope, err := h.catalogScope(c)
	if err == nil {
		var limit int
		if limit, err = h.catalogLimit(c, 100); err == nil {
			items, qerr := h.analyticsService.ListProperties(c.UserContext(), projectID, scope, c.Query("event_name"), c.Query("search"), limit)
			if qerr != nil {
				return h.catalogFail(c, "list property catalog", qerr)
			}
			return response.Success(c, fiber.StatusOK, items)
		}
	}
	return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", err.Error())
}

// ListPropertyValues godoc
// @Summary Sample property values
// @Description Most frequent values of a property
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param property query string true "Property name"
// @Param scope query string false "event (default) or person"
// @Param event_name query string false "Restrict to one event"
// @Param search query string false "Case-insensitive value substring"
// @Param limit query int false "Max results (1-500, default 20)"
// @Router /api/v1/projects/{projectID}/catalog/property-values [get]
func (h *Handler) ListPropertyValues(c *fiber.Ctx) error {
	projectID, httpErr := h.verifyProjectOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}
	property := c.Query("property")
	if err := ValidatePropertyName(property); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	}
	scope, err := h.catalogScope(c)
	if err == nil {
		var limit int
		if limit, err = h.catalogLimit(c, 20); err == nil {
			items, qerr := h.analyticsService.ListPropertyValues(c.UserContext(), projectID, scope, property, c.Query("event_name"), c.Query("search"), limit)
			if qerr != nil {
				return h.catalogFail(c, "list property values", qerr)
			}
			return response.Success(c, fiber.StatusOK, items)
		}
	}
	return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", err.Error())
}
