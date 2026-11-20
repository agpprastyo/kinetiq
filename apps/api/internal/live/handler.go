package live

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
)

type Handler struct {
	rdb    *redis.Client
	logger *slog.Logger
}

func NewHandler(rdb *redis.Client, logger *slog.Logger) *Handler {
	return &Handler{
		rdb:    rdb,
		logger: logger,
	}
}

func (h *Handler) StreamLiveEvents(c *fiber.Ctx) error {
	projectID := c.Params("projectID")
	if projectID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "projectID is required"})
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	once := c.QueryBool("once", false)
	limit := c.QueryInt("limit", 0)

	userCtx := c.UserContext()
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		ctx, cancel := context.WithCancel(userCtx)
		defer cancel()

		pubsub := h.rdb.Subscribe(ctx, fmt.Sprintf("events:pubsub:%s", projectID))
		defer pubsub.Close()

		ch := pubsub.Channel()

		// Initial connection acknowledgment comment
		fmt.Fprintf(w, ": connected\n\n")
		if err := w.Flush(); err != nil {
			return
		}

		if once {
			return
		}

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		sent := 0
		for {
			select {
			case <-userCtx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
				if err := w.Flush(); err != nil {
					return
				}
				sent++
				if limit > 0 && sent >= limit {
					return
				}
			case <-ticker.C:
				// Heartbeat comment to keep connection alive
				fmt.Fprintf(w, ": ping\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}
