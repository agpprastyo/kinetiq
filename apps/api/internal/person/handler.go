package person

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/auth"
	"kinetiq/internal/ingestion"
	"kinetiq/internal/response"
)

type Handler struct {
	svc     *Service
	keyAuth auth.APIKeyValidator
	limiter ingestion.Limiter
}

func (h *Handler) SetLimiter(l ingestion.Limiter) { h.limiter = l }

func NewHandler(svc *Service, keyAuth auth.APIKeyValidator) *Handler {
	return &Handler{svc: svc, keyAuth: keyAuth}
}

type identifyBody struct {
	DistinctID  string         `json:"distinct_id"`
	AnonymousID string         `json:"anonymous_id"`
	Set         map[string]any `json:"$set"`
	SetOnce     map[string]any `json:"$set_once"`
}

type aliasBody struct {
	DistinctID string `json:"distinct_id"`
	Alias      string `json:"alias"`
}

type personData struct {
	PersonID     string         `json:"person_id"`
	IsIdentified bool           `json:"is_identified"`
	Properties   map[string]any `json:"properties"`
}

// Identify godoc
// @Summary Link an anonymous distinct_id to an identified user
// @Tags identity
// @Security ApiKeyAuth
// @Router /api/v1/identify [post]
func (h *Handler) Identify(c *fiber.Ctx) error {
	var b identifyBody
	return h.run(c, &b, func(ctx context.Context, pid uuid.UUID) (*Person, error) {
		return h.svc.Identify(ctx, pid, IdentifyInput{DistinctID: b.DistinctID, AnonymousID: b.AnonymousID, Set: b.Set, SetOnce: b.SetOnce})
	})
}

// Alias godoc
// @Summary Link two known IDs to the same person
// @Tags identity
// @Security ApiKeyAuth
// @Router /api/v1/alias [post]
func (h *Handler) Alias(c *fiber.Ctx) error {
	var b aliasBody
	return h.run(c, &b, func(ctx context.Context, pid uuid.UUID) (*Person, error) {
		return h.svc.Alias(ctx, pid, b.DistinctID, b.Alias)
	})
}

func (h *Handler) run(c *fiber.Ctx, body any, fn func(context.Context, uuid.UUID) (*Person, error)) error {
	ctx := c.UserContext()
	pid, ok, err := ingestion.AuthenticateKey(c, h.keyAuth, h.limiter)
	if !ok {
		return err
	}
	if err := c.BodyParser(body); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload")
	}
	p, err := fn(ctx, pid)
	switch {
	case err == nil:
		return response.Success(c, fiber.StatusOK, personData{PersonID: p.ID.String(), IsIdentified: p.IsIdentified, Properties: p.Properties})
	case errors.Is(err, ErrInvalidID):
		return response.Error(c, fiber.StatusBadRequest, "INVALID_ID", err.Error())
	case errors.Is(err, ErrAlreadyIdentified):
		return response.Error(c, fiber.StatusConflict, "ALREADY_IDENTIFIED", err.Error())
	default:
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to resolve person identity")
	}
}
