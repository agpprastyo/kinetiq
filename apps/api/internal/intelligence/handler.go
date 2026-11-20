package intelligence

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/project"
	"kinetiq/internal/response"
)

type Handler struct {
	service    Service
	projectSvc project.Service
	logger     *slog.Logger
}

func NewHandler(service Service, projectSvc project.Service, logger *slog.Logger) *Handler {
	return &Handler{
		service:    service,
		projectSvc: projectSvc,
		logger:     logger,
	}
}

// GetWeeklyDigest returns the 7-day performance digest report for a project
func (h *Handler) GetWeeklyDigest(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_PROJECT_ID", "Invalid project ID format")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	report, err := h.service.GenerateProjectWeeklyDigest(c.UserContext(), projectID)
	if err != nil {
		h.logger.Error("failed to generate weekly digest", slog.String("error", err.Error()), slog.String("project_id", projectID.String()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate weekly digest")
	}

	return response.Success(c, fiber.StatusOK, report)
}

// TriggerDigestDispatch triggers immediate generation and outbox persistence of weekly performance digests (for manual ops or scheduled cron)
func (h *Handler) TriggerDigestDispatch(c *fiber.Ctx) error {
	count, err := h.service.DispatchWeeklyDigests(c.UserContext())
	if err != nil {
		h.logger.Error("failed to dispatch weekly digests", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to dispatch weekly digests")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"dispatched_count": count,
		"message":          "Weekly digests successfully dispatched to outbox",
	})
}

// CheckAnomaly checks if a specific event has dropped significantly vs its 3-day baseline
func (h *Handler) CheckAnomaly(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_PROJECT_ID", "Invalid project ID format")
	}

	eventName := c.Query("event_name")
	if eventName == "" {
		return response.Error(c, fiber.StatusBadRequest, "MISSING_EVENT_NAME", "Query parameter 'event_name' is required")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	res, err := h.service.CheckTrafficAnomaly(c.UserContext(), projectID, eventName)
	if err != nil {
		h.logger.Error("failed to check traffic anomaly", slog.String("error", err.Error()), slog.String("project_id", projectID.String()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to evaluate anomaly")
	}

	return response.Success(c, fiber.StatusOK, res)
}
