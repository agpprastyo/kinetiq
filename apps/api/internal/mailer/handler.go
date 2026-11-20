package mailer

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/response"
)

type Handler struct {
	dispatcher *Dispatcher
	cleaner    *Cleaner
	logger     *slog.Logger
}

func NewHandler(dispatcher *Dispatcher, cleaner *Cleaner, logger *slog.Logger) *Handler {
	return &Handler{
		dispatcher: dispatcher,
		cleaner:    cleaner,
		logger:     logger,
	}
}

// GetMetrics returns outbox delivery counters for ops dashboard
func (h *Handler) GetMetrics(c *fiber.Ctx) error {
	metrics, err := h.dispatcher.GetMetrics(c.UserContext())
	if err != nil {
		h.logger.Error("failed to get mailer outbox metrics", slog.String("error", err.Error()))
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve mailer metrics")
	}

	return response.Success(c, fiber.StatusOK, metrics)
}
