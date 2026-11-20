package observability

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInitTracerDisabledIsNoop(t *testing.T) {
	shutdown, err := InitTracer(context.Background(), "svc", "http://localhost:4317", 1, false)
	if err != nil || shutdown == nil {
		t.Fatalf("want noop shutdown, got err=%v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("x").Start(context.Background(), "s")
	if span.SpanContext().IsValid() {
		t.Fatal("noop provider must not produce valid spans")
	}
}

func TestInitTracerEnabledShutdown(t *testing.T) {
	shutdown, err := InitTracer(context.Background(), "svc", "localhost:4317", 0.5, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = shutdown(context.Background())
}

func TestTraceMiddlewareRecordsSpan(t *testing.T) {
	InitLogger()
	rec := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	app := fiber.New()
	app.Use(TraceMiddleware())
	app.Get("/ping", func(c *fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest("GET", "/ping", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Name() != "GET /ping" {
		t.Fatalf("want 1 span 'GET /ping', got %d", len(spans))
	}
	if resp.Header.Get("traceparent") == "" {
		t.Fatal("missing traceparent header")
	}
}
