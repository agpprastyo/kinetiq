package observability

import (
	"context"
	"log/slog"
	"os"
)

// LogContextExtractor extracts trace_id and request_id from context and injects into log records
type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
			r.AddAttrs(slog.String("request_id", reqID))
		}
		if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
			r.AddAttrs(slog.String("trace_id", traceID))
		}
	}
	return h.Handler.Handle(ctx, r)
}

func NewLogger() *slog.Logger {
	baseHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	customHandler := &ContextHandler{Handler: baseHandler}
	logger := slog.New(customHandler)
	return logger
}
