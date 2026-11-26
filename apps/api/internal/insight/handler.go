package insight

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/project"
	"kinetiq/internal/response"
	"kinetiq/internal/validator"
)

type Handler struct {
	service    Service
	projectSvc project.Service
	validator  *validator.CustomValidator
	logger     *slog.Logger
}

func NewHandler(service Service, projectSvc project.Service, validator *validator.CustomValidator, logger *slog.Logger) *Handler {
	return &Handler{
		service:    service,
		projectSvc: projectSvc,
		validator:  validator,
		logger:     logger,
	}
}

// CreateInsight handles creating a new custom insight chart widget
func (h *Handler) CreateInsight(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	var req CreateInsightRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "Invalid JSON payload")
	}

	if h.validator != nil {
		if errs := h.validator.Validate(req); len(errs) > 0 {
			var details []response.ErrorDetail
			for _, e := range errs {
				details = append(details, response.ErrorDetail{
					Field:   e.Field,
					Message: e.Message,
				})
			}
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", details...)
		}
	}

	insight, err := h.service.CreateInsight(c.UserContext(), projectID, &req)
	if err != nil {
		h.logger.Error("failed to create insight", slog.String("error", err.Error()), slog.String("project_id", projectID.String()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create insight")
	}

	return response.Success(c, fiber.StatusCreated, insight)
}

// ListInsights returns all custom insights for a project with computed real-time values
func (h *Handler) ListInsights(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	params := response.ParseOffsetParams(c, false)
	insights, total, err := h.service.ListInsights(c.UserContext(), projectID, params)
	if err != nil {
		h.logger.Error("failed to list insights", slog.String("error", err.Error()), slog.String("project_id", projectID.String()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list insights")
	}

	return response.Success(c, fiber.StatusOK, response.Paginated(insights, params, total))
}

// UpdateInsight updates an existing custom insight
func (h *Handler) UpdateInsight(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	insightID, err := uuid.Parse(c.Params("insightId"))
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_INSIGHT_ID", "Invalid insight ID format")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	var req UpdateInsightRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "Invalid JSON payload")
	}

	if h.validator != nil {
		if errs := h.validator.Validate(req); len(errs) > 0 {
			var details []response.ErrorDetail
			for _, e := range errs {
				details = append(details, response.ErrorDetail{
					Field:   e.Field,
					Message: e.Message,
				})
			}
			return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", details...)
		}
	}

	insight, err := h.service.UpdateInsight(c.UserContext(), projectID, insightID, &req)
	if err != nil {
		h.logger.Error("failed to update insight", slog.String("error", err.Error()), slog.String("insight_id", insightID.String()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update insight")
	}

	return response.Success(c, fiber.StatusOK, insight)
}

// DeleteInsight removes a custom insight
func (h *Handler) DeleteInsight(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	insightID, err := uuid.Parse(c.Params("insightId"))
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_INSIGHT_ID", "Invalid insight ID format")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	if err := h.service.DeleteInsight(c.UserContext(), projectID, insightID); err != nil {
		h.logger.Error("failed to delete insight", slog.String("error", err.Error()), slog.String("insight_id", insightID.String()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete insight")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"message": "Insight deleted successfully",
	})
}
