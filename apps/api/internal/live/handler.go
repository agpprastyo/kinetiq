package live

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"

	"kinetiq/internal/project"
	"kinetiq/internal/response"
)

type Handler struct {
	rdb            *redis.Client
	projectService project.Service
	logger         *slog.Logger
	shutdown       <-chan struct{}
}

// SetShutdown makes open SSE streams close when done is closed (server shutdown).
func (h *Handler) SetShutdown(done <-chan struct{}) { h.shutdown = done }

func NewHandler(rdb *redis.Client, projectService project.Service, logger *slog.Logger) *Handler {
	return &Handler{
		rdb:            rdb,
		projectService: projectService,
		logger:         logger,
	}
}

func (h *Handler) StreamLiveEvents(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}
	pid, err := uuid.Parse(c.Params("projectID"))
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	if proj, err := h.projectService.Get(c.UserContext(), userID, pid); err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}
	projectID := pid.String()

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
			case <-h.shutdown:
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
