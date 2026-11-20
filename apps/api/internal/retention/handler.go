package retention

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/response"
)

// ProjectAccessFunc resolves the caller's role on a project ("owner", "admin", ...).
// It must return a non-nil error or an empty role when the caller has no access.
type ProjectAccessFunc func(ctx context.Context, userID string, projectID uuid.UUID) (role string, err error)

type Handler struct {
	service Service
	access  ProjectAccessFunc
	logger  *slog.Logger
}

func NewHandler(service Service, access ProjectAccessFunc, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		access:  access,
		logger:  logger,
	}
}

// authorize parses :projectID and verifies the caller may access it (write=true requires owner/admin).
// On failure it writes the response and returns ok=false.
func (h *Handler) authorize(c *fiber.Ctx, write bool) (uuid.UUID, bool, error) {
	userID, _ := c.Locals("userID").(string)
	if userID == "" {
		return uuid.Nil, false, response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}
	projectID, err := uuid.Parse(strings.TrimSpace(c.Params("projectID")))
	if err != nil {
		return uuid.Nil, false, response.Error(c, fiber.StatusBadRequest, "INVALID_PROJECT_ID", "Invalid project ID")
	}
	role := ""
	if h.access != nil {
		role, err = h.access(c.UserContext(), userID, projectID)
	}
	if err != nil || role == "" {
		return uuid.Nil, false, response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	if write && role != "owner" && role != "admin" {
		return uuid.Nil, false, response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Only project owner or admin can modify retention")
	}
	return projectID, true, nil
}

type UpdatePolicyRequest struct {
	RetentionDays int `json:"retention_days"`
}

// GetPolicy godoc
// @Summary Get project retention policy
// @Description Retrieve current retention period in days for a project
// @Tags retention
// @Produce json
// @Router /api/v1/projects/{projectID}/retention [get]
func (h *Handler) GetPolicy(c *fiber.Ctx) error {
	ctx := c.UserContext()
	projectID, ok, aerr := h.authorize(c, false)
	if !ok {
		return aerr
	}

	policy, err := h.service.GetPolicy(ctx, projectID)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(ctx, "failed to get retention policy", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve retention policy")
	}

	return response.Success(c, fiber.StatusOK, policy)
}

// UpdatePolicy godoc
// @Summary Update project retention policy
// @Description Configure retention period in days (0 or less = unlimited)
// @Tags retention
// @Accept json
// @Produce json
// @Router /api/v1/projects/{projectID}/retention [put]
func (h *Handler) UpdatePolicy(c *fiber.Ctx) error {
	ctx := c.UserContext()
	projectID, ok, aerr := h.authorize(c, true)
	if !ok {
		return aerr
	}

	var req UpdatePolicyRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload")
	}

	policy, err := h.service.UpdatePolicy(ctx, projectID, req.RetentionDays)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(ctx, "failed to update retention policy", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update retention policy")
	}

	return response.Success(c, fiber.StatusOK, policy)
}

// Preview godoc
// @Summary Preview affected row count before executing retention purge
// @Description Inspect number of events that would be deleted under current retention policy
// @Tags retention
// @Produce json
// @Router /api/v1/projects/{projectID}/retention/preview [get]
func (h *Handler) Preview(c *fiber.Ctx) error {
	ctx := c.UserContext()
	projectID, ok, aerr := h.authorize(c, false)
	if !ok {
		return aerr
	}

	result, err := h.service.Preview(ctx, projectID)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(ctx, "failed to preview retention", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate retention preview")
	}

	return response.Success(c, fiber.StatusOK, result)
}

// RunPurge godoc
// @Summary Manually trigger retention policy execution
// @Description Purge events older than project retention policy
// @Tags retention
// @Produce json
// @Router /api/v1/projects/{projectID}/retention/run [post]
func (h *Handler) RunPurge(c *fiber.Ctx) error {
	ctx := c.UserContext()
	projectID, ok, aerr := h.authorize(c, true)
	if !ok {
		return aerr
	}

	result, err := h.service.Purge(ctx, projectID)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(ctx, "failed to execute retention purge", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to execute retention purge")
	}

	return response.Success(c, fiber.StatusOK, result)
}
