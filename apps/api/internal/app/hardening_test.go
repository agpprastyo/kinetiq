package app

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"kinetiq/internal/config"
)

func clientIP(t *testing.T, cfg *config.Config, xff string) string {
	t.Helper()
	a := fiber.New(newFiberConfig(cfg))
	a.Get("/ip", func(c *fiber.Ctx) error { return c.SendString(c.IP()) })
	req := httptest.NewRequest("GET", "/ip", nil)
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	resp, err := a.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 64)
	n, _ := resp.Body.Read(b)
	return string(b[:n])
}

func TestUntrustedForwardedForIsIgnored(t *testing.T) {
	got := clientIP(t, &config.Config{ProxyHeader: "X-Forwarded-For"}, "6.6.6.6")
	if got == "6.6.6.6" {
		t.Fatalf("spoofed XFF must be ignored, got %s", got)
	}
}

func TestTrustedProxyForwardedForIsHonoured(t *testing.T) {
	// fiber's in-memory test conn peers as 0.0.0.0
	cfg := &config.Config{ProxyHeader: "X-Forwarded-For", TrustedProxies: []string{"0.0.0.0"}}
	if got := clientIP(t, cfg, "6.6.6.6"); got != "6.6.6.6" {
		t.Fatalf("trusted proxy XFF must be honoured, got %s", got)
	}
}

func preflight(t *testing.T, cfg *config.Config, path, origin string) *httptestResp {
	t.Helper()
	a := fiber.New()
	registerCORS(a, cfg)
	a.Post(path, func(c *fiber.Ctx) error { return c.SendStatus(204) })
	req := httptest.NewRequest("OPTIONS", path, nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	resp, err := a.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return &httptestResp{
		allowOrigin: resp.Header.Get("Access-Control-Allow-Origin"),
		creds:       resp.Header.Get("Access-Control-Allow-Credentials"),
	}
}

type httptestResp struct{ allowOrigin, creds string }

var _ = cors.ConfigDefault

func TestDashboardCORSUsesConfiguredOrigins(t *testing.T) {
	cfg := &config.Config{WebOrigin: "https://app.example.org, https://admin.example.org"}
	r := preflight(t, cfg, "/api/v1/projects", "https://admin.example.org")
	if r.allowOrigin != "https://admin.example.org" || r.creds != "true" {
		t.Fatalf("configured origin should be allowed with credentials: %+v", r)
	}
	r = preflight(t, cfg, "/api/v1/projects", "http://localhost:5173")
	if r.allowOrigin != "" {
		t.Fatalf("hardcoded localhost must not be allowed in prod config: %+v", r)
	}
	r = preflight(t, cfg, "/api/v1/projects", "https://evil.example.net")
	if r.allowOrigin != "" {
		t.Fatalf("foreign origin must be rejected: %+v", r)
	}
}

func TestPublicIngestCORSAllowsAnyOriginWithoutCredentials(t *testing.T) {
	cfg := &config.Config{WebOrigin: "https://app.example.org"}
	for _, p := range []string{"/api/v1/ingest", "/api/v1/identify", "/api/v1/alias"} {
		r := preflight(t, cfg, p, "https://customer-site.example.net")
		if r.allowOrigin != "*" {
			t.Errorf("%s: customer preflight must succeed, got %+v", p, r)
		}
		if r.creds == "true" {
			t.Errorf("%s: must not allow credentials", p)
		}
	}
}
