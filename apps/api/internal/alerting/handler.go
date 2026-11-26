package alerting

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/project"
	"kinetiq/internal/response"
	"kinetiq/internal/webhook"
)

type CreateRuleRequest struct {
	Name            string `json:"name" validate:"required,min=1,max=100"`
	EventName       string `json:"event_name" validate:"required,min=1,max=100"`
	Condition       string `json:"condition" validate:"required,oneof=spike_gt drop_lt"`
	ThresholdCount  int64  `json:"threshold_count" validate:"required,min=1"`
	WindowMinutes   int    `json:"window_minutes" validate:"min=1,max=60"`
	WebhookURL      string `json:"webhook_url" validate:"required,url"`
	WebhookSecret   string `json:"webhook_secret"`
	ChannelType     string `json:"channel_type" validate:"required,oneof=slack discord telegram webhook"`
	TelegramChatID  string `json:"telegram_chat_id"`
	IsActive        *bool  `json:"is_active"`
	CooldownMinutes int    `json:"cooldown_minutes" validate:"min=1,max=1440"`
}

type SnoozeRuleRequest struct {
	DurationHours int `json:"duration_hours" validate:"min=0,max=720"` // 0 to un-snooze
}

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

// ListRules lists all alert rules configured for a project
func (h *Handler) ListRules(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	// Verify project access
	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	rules, err := h.service.ListRules(c.UserContext(), projectID)
	if err != nil {
		h.logger.Error("failed to list alert rules", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve alert rules")
	}

	return response.Success(c, fiber.StatusOK, response.PaginateSlice(rules, response.ParseOffsetParams(c, false)))
}

// CreateRule registers a new alert notification rule
func (h *Handler) CreateRule(c *fiber.Ctx) error {
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
	if strings.ToLower(proj.Role) == "viewer" {
		return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Viewers cannot create alert rules")
	}

	var req CreateRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_BODY", "Invalid alert rule format")
	}

	trimmedName := strings.TrimSpace(req.Name)
	if trimmedName == "" {
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Rule name is required")
	}

	channel := strings.ToLower(strings.TrimSpace(req.ChannelType))
	if channel == "" {
		channel = "webhook"
	}
	if !isValidChannel(channel) {
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Invalid channel type: must be slack, discord, telegram, or webhook")
	}

	window := req.WindowMinutes
	if window <= 0 {
		window = 5
	}
	cooldown := req.CooldownMinutes
	if cooldown <= 0 {
		cooldown = 15
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	if err := validateDestination(channel, req.WebhookURL); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "UNSAFE_WEBHOOK_URL", err.Error())
	}

	rule := AlertRule{
		ProjectID:       projectID.String(),
		Name:            trimmedName,
		EventName:       strings.TrimSpace(req.EventName),
		Condition:       req.Condition,
		ThresholdCount:  req.ThresholdCount,
		WindowMinutes:   window,
		WebhookURL:      strings.TrimSpace(req.WebhookURL),
		WebhookSecret:   strings.TrimSpace(req.WebhookSecret),
		ChannelType:     channel,
		TelegramChatID:  strings.TrimSpace(req.TelegramChatID),
		IsActive:        isActive,
		CooldownMinutes: cooldown,
	}

	created, err := h.service.CreateRule(c.UserContext(), rule)
	if err != nil {
		h.logger.Error("failed to create alert rule", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Failed to create alert rule")
	}

	return response.Success(c, fiber.StatusCreated, created)
}

// UpdateRule modifies an existing alert rule
func (h *Handler) UpdateRule(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	ruleID, err := uuid.Parse(c.Params("ruleID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Rule not found")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	if strings.ToLower(proj.Role) == "viewer" {
		return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Viewers cannot modify alert rules")
	}

	existing, err := h.service.GetRule(c.UserContext(), ruleID)
	if err != nil || existing == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Alert rule not found")
	}

	var req CreateRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_BODY", "Invalid alert rule format")
	}

	if req.Name != "" {
		existing.Name = strings.TrimSpace(req.Name)
	}
	if req.EventName != "" {
		existing.EventName = strings.TrimSpace(req.EventName)
	}
	if req.Condition != "" {
		existing.Condition = req.Condition
	}
	if req.ThresholdCount > 0 {
		existing.ThresholdCount = req.ThresholdCount
	}
	if req.WindowMinutes > 0 {
		existing.WindowMinutes = req.WindowMinutes
	}
	if req.WebhookURL != "" {
		existing.WebhookURL = strings.TrimSpace(req.WebhookURL)
	}
	if req.WebhookSecret != "" {
		existing.WebhookSecret = strings.TrimSpace(req.WebhookSecret)
	}
	if req.ChannelType != "" {
		existing.ChannelType = strings.ToLower(strings.TrimSpace(req.ChannelType))
	}
	if req.TelegramChatID != "" {
		existing.TelegramChatID = strings.TrimSpace(req.TelegramChatID)
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}
	if req.WebhookURL != "" || req.ChannelType != "" {
		if err := validateDestination(existing.ChannelType, existing.WebhookURL); err != nil {
			return response.Error(c, fiber.StatusBadRequest, "UNSAFE_WEBHOOK_URL", err.Error())
		}
	}
	if req.CooldownMinutes > 0 {
		existing.CooldownMinutes = req.CooldownMinutes
	}

	updated, err := h.service.UpdateRule(c.UserContext(), *existing)
	if err != nil {
		h.logger.Error("failed to update alert rule", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "Failed to update alert rule")
	}

	return response.Success(c, fiber.StatusOK, updated)
}

// DeleteRule removes an alert rule
func (h *Handler) DeleteRule(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	ruleID, err := uuid.Parse(c.Params("ruleID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Rule not found")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	if strings.ToLower(proj.Role) == "viewer" {
		return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Viewers cannot delete alert rules")
	}

	if err := h.service.DeleteRule(c.UserContext(), ruleID); err != nil {
		if errors.Is(err, ErrAlertRuleNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Alert rule not found")
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete alert rule")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{"deleted": true, "id": ruleID.String()})
}

// TestRule dispatches an immediate test ping to the rule's destination channel
func (h *Handler) TestRule(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	ruleID, err := uuid.Parse(c.Params("ruleID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Rule not found")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	if err := h.service.TestRule(c.UserContext(), ruleID); err != nil {
		h.logger.Error("failed to test alert rule", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusBadRequest, "DISPATCH_FAILED", "Unable to dispatch test ping: "+err.Error())
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{"tested": true, "id": ruleID.String()})
}

// SnoozeRule temporarily silences alerts for a given rule
func (h *Handler) SnoozeRule(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectID, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	ruleID, err := uuid.Parse(c.Params("ruleID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Rule not found")
	}

	proj, err := h.projectSvc.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	if strings.ToLower(proj.Role) == "viewer" {
		return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Viewers cannot snooze alert rules")
	}

	var req SnoozeRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "Invalid JSON body")
	}

	updatedRule, err := h.service.SnoozeRule(c.UserContext(), ruleID, req.DurationHours)
	if err != nil {
		if errors.Is(err, ErrAlertRuleNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Alert rule not found")
		}
		h.logger.Error("failed to snooze alert rule", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to snooze alert rule")
	}

	return response.Success(c, fiber.StatusOK, updatedRule)
}

func isValidChannel(channel string) bool {
	switch channel {
	case "slack", "discord", "telegram", "webhook":
		return true
	default:
		return false
	}
}

// validateDestination rejects webhook targets that resolve to private/metadata
// addresses. Telegram bot tokens (non-URL) are sent to api.telegram.org.
func validateDestination(channel, rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return errors.New("webhook_url is required")
	}
	if channel == "telegram" && !strings.HasPrefix(rawURL, "http") {
		return nil
	}
	if IsDryRunURL(rawURL) {
		return nil
	}
	if err := webhook.IsSafeTargetURL(rawURL); err != nil {
		return fmt.Errorf("webhook_url rejected: %w", err)
	}
	return nil
}
