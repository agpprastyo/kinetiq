package compliance

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

// ListWorkspaceAuditLogs godoc
// @Summary List workspace audit trail logs
// @Description Retrieve append-only audit trail logs for workspace actions (owner/admin only)
// @Tags compliance
// @Produce json
// @Router /api/v1/workspace/audit-logs [get]
func (h *Handler) ListWorkspaceAuditLogs(c *fiber.Ctx) error {
	ctx := c.UserContext()
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	// Active workspace ID is identified by user's organization context or primary user ID
	workspaceID := strings.TrimSpace(c.Query("workspace_id"))
	if workspaceID == "" {
		workspaceID = userID
	}

	// Enforce RBAC: Caller must be owner/admin of the workspace
	userRole, _ := c.Locals("userRole").(string)
	isStaff, _ := c.Locals("isPlatformStaff").(bool)

	// If requesting another workspace's audit log, caller must be owner/admin or platform staff
	if workspaceID != userID {
		if !isStaff && userRole != "owner" && userRole != "admin" && userRole != "superadmin" {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Only workspace owner or admin can view audit logs")
		}
	} else {
		// Even for self workspace, role "user" or "member" cannot view enterprise compliance audit log if organization role restricts it
		if !isStaff && userRole != "owner" && userRole != "admin" && userRole != "superadmin" {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Only workspace owner or admin can view audit logs")
		}
	}

	params := response.ParseOffsetParams(c, false)

	var projectID *uuid.UUID
	if pIDStr := strings.TrimSpace(c.Query("project_id")); pIDStr != "" {
		if parsed, err := uuid.Parse(pIDStr); err == nil {
			projectID = &parsed
		}
	}

	action := strings.TrimSpace(c.Query("action"))
	targetType := strings.TrimSpace(c.Query("target_type"))
	search := strings.TrimSpace(c.Query("search"))

	res, err := h.service.List(ctx, ListAuditLogsParams{
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Action:      action,
		TargetType:  targetType,
		Search:      search,
		Limit:       params.Limit,
		Offset:      params.Offset,
	})
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(ctx, "failed to list workspace audit logs", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}

	return response.Success(c, fiber.StatusOK, response.Paginated(res.Items, params, res.Total))
}

// DeleteUserData godoc
// @Summary GDPR / Privacy Right-to-be-forgotten deletion
// @Description Purge or pseudonymize all events belonging to a user distinct_id
// @Tags compliance
// @Produce json
// @Router /api/v1/projects/{projectID}/compliance/users/{distinctID} [delete]
func (h *Handler) DeleteUserData(c *fiber.Ctx) error {
	ctx := c.UserContext()
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectIDStr := strings.TrimSpace(c.Params("projectID"))
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	distinctID := strings.TrimSpace(c.Params("distinctID"))
	if distinctID == "" {
		distinctID = strings.TrimSpace(c.Params("distinct_id"))
	}
	if distinctID == "" {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_DISTINCT_ID", "distinct_id is required")
	}

	userRole, _ := c.Locals("userRole").(string)
	isStaff, _ := c.Locals("isPlatformStaff").(bool)
	if !isStaff && userRole != "owner" && userRole != "admin" && userRole != "superadmin" {
		return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Only project owner or admin can execute GDPR deletion")
	}

	// Tenant isolation: caller must own / hold owner|admin membership on this project.
	role := ""
	if h.access != nil {
		role, err = h.access(ctx, userID, projectID)
	}
	if err != nil || role == "" {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	if role != "owner" && role != "admin" {
		return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Only project owner or admin can execute GDPR deletion")
	}

	mode := strings.ToLower(strings.TrimSpace(c.Query("mode", "delete")))
	if mode != "delete" && mode != "pseudonymize" {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_MODE", "mode must be 'delete' or 'pseudonymize'")
	}

	userName, _ := c.Locals("userName").(string)
	userEmail, _ := c.Locals("userEmail").(string)
	workspaceID, _ := c.Locals("organizationID").(string)
	if workspaceID == "" {
		workspaceID = userID
	}

	result, err := h.service.GDPRDelete(ctx, GDPRDeleteParams{
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		DistinctID:  distinctID,
		Mode:        mode,
		ActorID:     userID,
		ActorName:   userName,
		ActorEmail:  userEmail,
		IPAddress:   c.IP(),
		UserAgent:   c.Get("User-Agent"),
	})
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(ctx, "failed to execute GDPR user deletion", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to execute privacy deletion")
	}

	return response.Success(c, fiber.StatusOK, result)
}
