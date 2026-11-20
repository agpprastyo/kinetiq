package analytics

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

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

	summary, err := h.analyticsService.GetSummary(c.UserContext(), projectID, from, to)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to get analytics summary", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return c.Status(fiber.StatusOK).JSON(summary)
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

	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 || parsedLimit > 50 {
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Limit must be an integer between 1 and 50")
		}
		limit = parsedLimit
	}

	topEvents, err := h.analyticsService.GetTopEvents(c.UserContext(), projectID, from, to, limit)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to get top events", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return c.Status(fiber.StatusOK).JSON(topEvents)
}

// GetVolume godoc
// @Summary Event volume time-series
// @Description Event volume bucketed by hour or day
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param from query string true "Start timestamp RFC3339"
// @Param to query string true "End timestamp RFC3339"
// @Param interval query string false "Bucket interval: hour or day"
// @Router /api/v1/projects/{projectID}/analytics/volume [get]
func (h *Handler) GetVolume(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	interval := c.Query("interval", "hour")
	if interval != "minute" && interval != "hour" && interval != "day" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Interval must be 'minute', 'hour', or 'day'")
	}

	volume, err := h.analyticsService.GetVolume(c.UserContext(), projectID, from, to, interval)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to get volume", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return c.Status(fiber.StatusOK).JSON(volume)
}

// GetFunnel godoc
// @Summary Funnel analytics
// @Description Multi-step conversion rates and step counts
// @Tags analytics
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param from query string true "Start timestamp RFC3339"
// @Param to query string true "End timestamp RFC3339"
// @Param steps query string true "Comma-separated event names (2-3 steps)"
// @Router /api/v1/projects/{projectID}/analytics/funnel [get]
func (h *Handler) GetFunnel(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	stepsParam := c.Query("steps")
	if stepsParam == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Query parameter 'steps' is required")
	}

	rawSteps := strings.Split(stepsParam, ",")
	steps := make([]string, 0, len(rawSteps))
	for _, s := range rawSteps {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			steps = append(steps, trimmed)
		}
	}

	if len(steps) < 2 || len(steps) > 5 {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Funnel steps must contain between 2 and 5 comma-separated event names")
	}

	funnel, err := h.analyticsService.GetFunnel(c.UserContext(), projectID, from, to, steps)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to compute funnel", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return c.Status(fiber.StatusOK).JSON(funnel)
}

func (h *Handler) GetPropertyBreakdown(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
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

	items, err := h.analyticsService.GetPropertyBreakdown(c.UserContext(), projectID, eventName, property, from, to, limit)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to compute property breakdown", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
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
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to compute device breakdown", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
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
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to compute geo breakdown", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return response.Success(c, fiber.StatusOK, items)
}

func (h *Handler) GetRetention(c *fiber.Ctx) error {
	projectID, from, to, httpErr := h.parseRangeAndVerifyOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	firstEvent := strings.TrimSpace(c.Query("first_event", "signup"))
	returnEvent := strings.TrimSpace(c.Query("return_event", "app_opened"))
	granularity := strings.TrimSpace(c.Query("granularity", "day"))

	res, err := h.analyticsService.GetRetention(c.UserContext(), projectID, from, to, firstEvent, returnEvent, granularity)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to compute retention", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return response.Success(c, fiber.StatusOK, res)
}

func (h *Handler) ListUsers(c *fiber.Ctx) error {
	projectID, httpErr := h.verifyProjectOwnership(c)
	if httpErr != nil {
		return response.Error(c, httpErr.Status, httpErr.Code, httpErr.Message)
	}

	search := strings.TrimSpace(c.Query("search"))
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	offset := 0
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	users, err := h.analyticsService.ListUsers(c.UserContext(), projectID, search, limit, offset)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to list analytics users", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return response.Success(c, fiber.StatusOK, users)
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

	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var before *time.Time
	if b := c.Query("before"); b != "" {
		if parsed, err := time.Parse(time.RFC3339, b); err == nil {
			before = &parsed
		}
	}

	timeline, err := h.analyticsService.GetUserTimeline(c.UserContext(), projectID, distinctID, limit, before)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(c.UserContext(), "failed to get user timeline", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return response.Success(c, fiber.StatusOK, timeline)
}
