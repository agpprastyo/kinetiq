package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// TracerName is the instrumentation scope for HTTP spans.
var TracerName = "kinetiq-api"

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	TraceIDKey   contextKey = "trace_id"
)

var Logger *slog.Logger

func InitLogger() {
	Logger = NewLogger()
	slog.SetDefault(Logger)
}

// GenerateTraceID generates a 16-byte random hex string (W3C standard trace-id)
func GenerateTraceID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return uuid.New().String()
	}
	return hex.EncodeToString(bytes)
}

// TraceMiddleware extracts or generates RequestID & TraceID (W3C traceparent compatible)
func TraceMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. Check for incoming X-Request-ID or generate new UUID
		reqID := c.Get("X-Request-ID")
		if strings.TrimSpace(reqID) == "" {
			reqID = uuid.New().String()
		}

		// 2. Check for W3C traceparent: version-trace_id-parent_id-flags
		// Format: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
		traceParent := c.Get("traceparent")
		var traceID string
		if traceParent != "" {
			parts := strings.Split(traceParent, "-")
			if len(parts) >= 2 && len(parts[1]) == 32 {
				traceID = parts[1]
			}
		}
		if traceID == "" {
			traceID = GenerateTraceID()
		}

		// Set header in response so Frontend can track it
		c.Set("X-Request-ID", reqID)
		c.Set("traceparent", fmt.Sprintf("00-%s-0000000000000000-01", traceID))

		// Set to Fiber locals
		c.Locals("requestID", reqID)
		c.Locals("traceID", traceID)

		// Start OTel span (no-op unless InitTracer installed a real provider);
		// incoming W3C traceparent is extracted as the remote parent.
		parent := otel.GetTextMapPropagator().Extract(c.UserContext(), propagation.MapCarrier{"traceparent": traceParent})
		parent, span := otel.Tracer(TracerName).Start(parent, c.Method()+" "+c.Path(),
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithAttributes(
				attribute.String("http.request.method", c.Method()),
				attribute.String("url.path", c.Path()),
				attribute.String("request.id", reqID),
			))
		defer span.End()
		if sc := span.SpanContext(); sc.IsValid() {
			traceID = sc.TraceID().String()
			c.Set("traceparent", fmt.Sprintf("00-%s-%s-%s", traceID, sc.SpanID(), sc.TraceFlags()))
			c.Locals("traceID", traceID)
		}

		// Set to context.Context
		ctx := context.WithValue(parent, RequestIDKey, reqID)
		ctx = context.WithValue(ctx, TraceIDKey, traceID)
		c.SetUserContext(ctx)

		// Log incoming request
		Logger.InfoContext(ctx, "HTTP request started",
			slog.String("request_id", reqID),
			slog.String("trace_id", traceID),
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.String("ip", c.IP()),
		)

		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		routePath := c.Route().Path
		if routePath == "" {
			routePath = c.Path()
		}
		status := c.Response().StatusCode()
		span.SetName(c.Method() + " " + routePath)
		span.SetAttributes(attribute.String("http.route", routePath), attribute.Int("http.response.status_code", status))
		if err != nil || status >= 500 {
			span.SetStatus(codes.Error, "request failed")
			if err != nil {
				span.RecordError(err)
			}
		}
		DefaultMetrics.RecordHTTPRequest(c.Method(), routePath, status, duration)

		// Log completed request
		Logger.InfoContext(ctx, "HTTP request completed",
			slog.String("request_id", reqID),
			slog.String("trace_id", traceID),
			slog.Int("status", status),
			slog.String("method", c.Method()),
			slog.String("route", routePath),
			slog.Duration("duration", duration),
		)

		return err
	}
}
