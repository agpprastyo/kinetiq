package dlq

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/response"
)

type Handler struct {
	service Service
	logger  *slog.Logger
}

func NewHandler(service Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// ListDeadLetters returns recent dead letter queue items and count summaries
func (h *Handler) ListDeadLetters(c *fiber.Ctx) error {
	ctx := c.UserContext()

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	items, summary, err := h.service.List(ctx, limit, offset)
	if err != nil {
		if h.logger != nil {
			h.logger.ErrorContext(ctx, "failed to list dead letters", slog.String("error", err.Error()))
		}
		return response.Error(c, fiber.StatusInternalServerError, "DLQ_FETCH_FAILED", "Failed to retrieve dead-letter queue items")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"items":   items,
		"summary": summary,
	})
}

// ReplayMessage puts a failed dead letter message back onto the ingestion stream
func (h *Handler) ReplayMessage(c *fiber.Ctx) error {
	ctx := c.UserContext()
	idStr := c.Params("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_ID", "Invalid dead letter ID format")
	}

	err = h.service.Replay(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Dead letter record not found")
		}
		return response.Error(c, fiber.StatusInternalServerError, "REPLAY_FAILED", "Failed to replay message to stream")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"replayed": true,
		"id":       id,
		"message":  "Message successfully reenqueued to ingestion stream",
	})
}

// ReplayAllMessages batch reenqueues all failed dead letter items to stream
func (h *Handler) ReplayAllMessages(c *fiber.Ctx) error {
	ctx := c.UserContext()

	count, err := h.service.ReplayAll(ctx)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "REPLAY_ALL_FAILED", err.Error())
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"replayed_count": count,
		"message":        "Messages reenqueued to stream",
	})
}

// PurgeMessage permanently deletes a single dead letter record
func (h *Handler) PurgeMessage(c *fiber.Ctx) error {
	ctx := c.UserContext()
	idStr := c.Params("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "INVALID_ID", "Invalid dead letter ID format")
	}

	count, err := h.service.Purge(ctx, &id)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "PURGE_FAILED", "Failed to delete dead letter item")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"purged": count > 0,
		"id":     id,
	})
}

// PurgeAllMessages permanently clears all dead letter records
func (h *Handler) PurgeAllMessages(c *fiber.Ctx) error {
	ctx := c.UserContext()

	count, err := h.service.Purge(ctx, nil)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "PURGE_FAILED", "Failed to clear dead letter queue")
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"purged_count": count,
	})
}
