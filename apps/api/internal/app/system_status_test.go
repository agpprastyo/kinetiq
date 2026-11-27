package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestParseRedisUsedMemoryHuman(t *testing.T) {
	info := "# Memory\r\nused_memory:1048576\r\nused_memory_human:1.00M\r\nused_memory_rss:2097152\r\n"
	got := parseRedisUsedMemoryHuman(info)
	if got != "1.00M" {
		t.Fatalf("expected 1.00M, got %q", got)
	}

	if empty := parseRedisUsedMemoryHuman("# Memory\r\n"); empty != "" {
		t.Fatalf("expected empty string, got %q", empty)
	}
}

func TestCollectSystemStatus_NilDeps(t *testing.T) {
	res := collectSystemStatus(context.Background(), nil, nil)
	if res.Postgres.Status != "degraded" {
		t.Fatalf("expected postgres degraded, got %q", res.Postgres.Status)
	}
	if res.Redis.Status != "degraded" {
		t.Fatalf("expected redis degraded, got %q", res.Redis.Status)
	}
	if res.Redis.UsedMemoryHuman != "unknown" {
		t.Fatalf("expected used_memory_human unknown, got %q", res.Redis.UsedMemoryHuman)
	}
}

func TestSystemStatusHandler_PartialFailureReturns200(t *testing.T) {
	app := fiber.New()
	c := &Container{}
	app.Get("/api/system/status", systemStatusHandler(c))

	req := httptest.NewRequest(http.MethodGet, "/api/system/status", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body SystemStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if body.Postgres.Status != "degraded" || body.Redis.Status != "degraded" {
		t.Fatalf("expected degraded statuses, got %+v", body)
	}
}
