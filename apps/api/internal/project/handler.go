package project

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/compliance"
	"kinetiq/internal/response"
	"kinetiq/internal/validator"
)

type CreateProjectRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100" example:"My Web App"`
}

type CreateAPIKeyRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100" example:"Production Web SDK"`
	Environment string `json:"environment" example:"live"`
}

type ProjectResponse struct {
	ID        string   `json:"id" example:"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"`
	Name      string   `json:"name" example:"My Web App"`
	Status    string   `json:"status" example:"active"`
	CreatedAt string   `json:"created_at" example:"2026-10-14T12:00:00Z"`
	APIKey    string   `json:"api_key,omitempty" example:"sp_live_abcdef123456_7890abcdef"`
	Project   *Project `json:"project,omitempty"`
}

type Handler struct {
	service   Service
	audit     compliance.Service
	validator *validator.CustomValidator
	logger    *slog.Logger
}

func NewHandler(service Service, v *validator.CustomValidator, logger *slog.Logger) *Handler {
	return &Handler{
		service:   service,
		validator: v,
		logger:    logger,
	}
}

func (h *Handler) SetAuditService(audit compliance.Service) {
	h.audit = audit
}

// CreateProject godoc
// @Summary Create a new analytics project
// @Description Creates a new project and returns the initial raw ingestion API key (shown only once)
// @Tags projects
// @Accept json
// @Produce json
// @Param request body CreateProjectRequest true "Project details"
// @Success 201 {object} ProjectResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/projects [post]
func (h *Handler) CreateProject(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	var req CreateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_BODY", "Invalid request body")
	}

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

	proj, err := h.service.Create(c.UserContext(), userID, req.Name)
	if err != nil {
		if errors.Is(err, ErrDuplicateProjectName) {
			return response.Error(c, fiber.StatusConflict, "DUPLICATE_PROJECT_NAME", "A project with this name already exists in your workspace")
		}
		h.logger.Error("failed to create project", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Unable to create project. Please check your input and try again.")
	}

	projCopy := *proj
	projCopy.APIKey = ""

	if h.audit != nil {
		actorEmail, _ := c.Locals("userEmail").(string)
		actorName, _ := c.Locals("userName").(string)
		pID := proj.ID
		_ = h.audit.Record(c.UserContext(), compliance.RecordAuditParams{
			WorkspaceID: userID,
			ProjectID:   &pID,
			ActorID:     userID,
			ActorName:   actorName,
			ActorEmail:  actorEmail,
			Action:      "project.create",
			TargetType:  "project",
			TargetID:    proj.ID.String(),
			Metadata:    map[string]interface{}{"name": proj.Name},
			IPAddress:   c.IP(),
			UserAgent:   c.Get("User-Agent"),
		})
	}

	return response.Success(c, fiber.StatusCreated, ProjectResponse{
		ID:        proj.ID.String(),
		Name:      proj.Name,
		Status:    proj.Status,
		CreatedAt: proj.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		APIKey:    proj.APIKey,
		Project:   &projCopy,
	})
}

// ListProjects godoc
// @Summary List projects
// @Description Returns all projects owned by the currently authenticated user
// @Tags projects
// @Produce json
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/projects [get]
func (h *Handler) ListProjects(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projects, err := h.service.List(c.UserContext(), userID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list projects")
	}

	return response.Success(c, fiber.StatusOK, projects)
}

// GetProject godoc
// @Summary Get project details
// @Description Returns project details by ID for the owner (never returns API key hash)
// @Tags projects
// @Produce json
// @Param projectID path string true "Project UUID"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/projects/{projectID} [get]
func (h *Handler) GetProject(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectIDStr := c.Params("projectID")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	proj, err := h.service.Get(c.UserContext(), userID, projectID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Unable to load project details")
	}
	if proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	return response.Success(c, fiber.StatusOK, proj)
}

// UpdateProject godoc
// @Summary Update project settings and name
// @Tags projects
// @Accept json
// @Produce json
// @Param projectID path string true "Project UUID"
// @Success 200 {object} response.Response
// @Router /api/v1/projects/{projectID} [patch]
func (h *Handler) UpdateProject(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	var input UpdateProjectInput
	if err := c.BodyParser(&input); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_BODY", "Invalid project settings format")
	}

	updated, err := h.service.Update(c.UserContext(), userID, projectID, input)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
		}
		if errors.Is(err, ErrForbidden) {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "You do not have permission to modify settings for this project")
		}
		if errors.Is(err, ErrDuplicateProjectName) {
			return response.Error(c, fiber.StatusConflict, "DUPLICATE_PROJECT_NAME", "A project with this name already exists in your workspace")
		}
		h.logger.Error("failed to update project", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Unable to update project settings. Please check your configuration values.")
	}

	return response.Success(c, fiber.StatusOK, updated)
}

// DeleteProject godoc
// @Summary Delete a project permanently
// @Tags projects
// @Param projectID path string true "Project UUID"
// @Success 200 {object} response.Response
// @Router /api/v1/projects/{projectID} [delete]
func (h *Handler) DeleteProject(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	if err := h.service.Delete(c.UserContext(), userID, projectID); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
		}
		if errors.Is(err, ErrForbidden) {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Only project owners can permanently delete a project")
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete project")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{"deleted": true, "id": projectID.String()})
}

// PurgeEvents godoc
// @Summary Purge all events for a project without deleting the project
// @Tags projects
// @Param projectID path string true "Project UUID"
// @Success 200 {object} response.Response
// @Router /api/v1/projects/{projectID}/purge-events [post]
func (h *Handler) PurgeEvents(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	deletedCount, err := h.service.PurgeEvents(c.UserContext(), userID, projectID)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
		}
		if errors.Is(err, ErrForbidden) {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "You do not have permission to purge events for this project")
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to purge events")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"purged_events": deletedCount,
		"purged_count":  deletedCount,
	})
}

// RotateKey godoc
// @Summary Rotate project API key
// @Description Revokes active API keys for project and generates a brand new key
// @Tags projects
// @Produce json
// @Param projectID path string true "Project UUID"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/projects/{projectID}/rotate-key [post]
func (h *Handler) RotateKey(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectIDStr := c.Params("projectID")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	proj, newKey, err := h.service.RotateKey(c.UserContext(), userID, projectID)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
		}
		if errors.Is(err, ErrForbidden) {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "You do not have permission to rotate API keys for this project")
		}
		h.logger.Error("failed to rotate API key", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Unable to rotate API key right now")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"project": proj,
		"api_key": newKey,
	})
}

// ListAPIKeys godoc
// @Summary List all API keys for a project
// @Tags projects
// @Produce json
// @Param projectID path string true "Project UUID"
// @Success 200 {object} response.Response
// @Router /api/v1/projects/{projectID}/keys [get]
func (h *Handler) ListAPIKeys(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	keys, err := h.service.ListKeys(c.UserContext(), userID, projectID)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list API keys")
	}

	return response.Success(c, fiber.StatusOK, keys)
}

// CreateAPIKey godoc
// @Summary Create an additional named API key for a project
// @Tags projects
// @Accept json
// @Produce json
// @Param projectID path string true "Project UUID"
// @Success 201 {object} response.Response
// @Router /api/v1/projects/{projectID}/keys [post]
func (h *Handler) CreateAPIKey(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	var req CreateAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_BODY", "Invalid request body")
	}

	created, err := h.service.CreateKey(c.UserContext(), userID, projectID, req.Name, req.Environment)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
		}
		if errors.Is(err, ErrForbidden) {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "You do not have permission to create API keys for this project")
		}
		h.logger.Error("failed to create API key", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Unable to create API key")
	}

	if h.audit != nil {
		actorEmail, _ := c.Locals("userEmail").(string)
		actorName, _ := c.Locals("userName").(string)
		pID := projectID
		_ = h.audit.Record(c.UserContext(), compliance.RecordAuditParams{
			WorkspaceID: userID,
			ProjectID:   &pID,
			ActorID:     userID,
			ActorName:   actorName,
			ActorEmail:  actorEmail,
			Action:      "api_key.create",
			TargetType:  "api_key",
			TargetID:    created.ID.String(),
			Metadata:    map[string]interface{}{"name": created.Name, "key_prefix": created.KeyPrefix, "environment": created.Environment},
			IPAddress:   c.IP(),
			UserAgent:   c.Get("User-Agent"),
		})
	}

	return response.Success(c, fiber.StatusCreated, fiber.Map{
		"id":          created.ID,
		"project_id":  created.ProjectID,
		"name":        created.Name,
		"key_prefix":  created.KeyPrefix,
		"environment": created.Environment,
		"created_at":  created.CreatedAt,
		"raw_key":     created.RawKey,
		"api_key":     created.RawKey,
		"key":         created,
	})
}

// RevokeAPIKey godoc
// @Summary Revoke a specific API key by ID
// @Tags projects
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param keyID path string true "Key UUID"
// @Success 200 {object} response.Response
// @Router /api/v1/projects/{projectID}/keys/{keyID} [delete]
func (h *Handler) RevokeAPIKey(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	keyID, err := uuid.Parse(c.Params("keyID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "API key not found")
	}

	if err := h.service.RevokeKey(c.UserContext(), userID, projectID, keyID); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
		}
		if errors.Is(err, ErrForbidden) {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "You do not have permission to revoke API keys for this project")
		}
		h.logger.Error("failed to revoke API key", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Unable to revoke API key")
	}

	if h.audit != nil {
		actorEmail, _ := c.Locals("userEmail").(string)
		actorName, _ := c.Locals("userName").(string)
		pID := projectID
		_ = h.audit.Record(c.UserContext(), compliance.RecordAuditParams{
			WorkspaceID: userID,
			ProjectID:   &pID,
			ActorID:     userID,
			ActorName:   actorName,
			ActorEmail:  actorEmail,
			Action:      "api_key.revoke",
			TargetType:  "api_key",
			TargetID:    keyID.String(),
			Metadata:    map[string]interface{}{"revoked": true},
			IPAddress:   c.IP(),
			UserAgent:   c.Get("User-Agent"),
		})
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{"revoked": true, "key_id": keyID.String()})
}
