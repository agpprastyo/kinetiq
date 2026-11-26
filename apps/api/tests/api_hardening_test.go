package tests

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valyala/fasthttp"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func doReq(t *testing.T, server *fiber.App, method, path string, body []byte, hdr map[string]string) (*http.Response, envelope, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Cookie", "user1_session_cookie")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := server.Test(req, 10000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	_ = json.Unmarshal(raw, &env)
	return resp, env, string(raw)
}

func wantErr(t *testing.T, resp *http.Response, env envelope, raw string, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("want %d, got %d: %s", status, resp.StatusCode, raw)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") || env.Success || env.Error == nil {
		t.Fatalf("want JSON error envelope, got %q: %s", resp.Header.Get("Content-Type"), raw)
	}
	if code != "" && env.Error.Code != code {
		t.Fatalf("want code %s, got %s", code, env.Error.Code)
	}
}

func TestAPIHardening(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))
	server.Get("/__panic", func(*fiber.Ctx) error { panic("boom") })
	queries := db.New(container.DBPool)

	projectID := uuid.New()
	if _, err := queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Hardening " + projectID.String()[:8],
		Status:      "active",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	base := "/api/v1/projects/" + projectID.String()

	t.Run("panic recovered as 500 envelope, process alive", func(t *testing.T) {
		resp, env, raw := doReq(t, server, http.MethodGet, "/__panic", nil, nil)
		wantErr(t, resp, env, raw, 500, "INTERNAL_ERROR")
		if resp, _, _ := doReq(t, server, http.MethodGet, "/healthz", nil, nil); resp.StatusCode != 200 {
			t.Fatalf("server not alive after panic: %d", resp.StatusCode)
		}
	})

	t.Run("framework 404 uses envelope", func(t *testing.T) {
		resp, env, raw := doReq(t, server, http.MethodGet, "/does-not-exist", nil, nil)
		wantErr(t, resp, env, raw, 404, "NOT_FOUND")
	})

	t.Run("body over limit is 413 envelope", func(t *testing.T) {
		// server.Test surfaces fasthttp's body-limit error instead of the response, so use a real socket.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = server.Listener(ln) }()
		defer func() { _ = server.Shutdown() }()
		// Declared Content-Length over the limit is rejected before the body is read.
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		fmt.Fprintf(conn, "POST /api/v1/ingest HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n", container.Config.IngestionMaxBodyBytes+1)
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		rawb, _ := io.ReadAll(resp.Body)
		var env envelope
		_ = json.Unmarshal(rawb, &env)
		wantErr(t, resp, env, string(rawb), 413, "PAYLOAD_TOO_LARGE")
	})

	t.Run("oversized properties rejected", func(t *testing.T) {
		resp, env, raw := doReq(t, server, http.MethodPost, base+"/keys", []byte(`{"name":"k","environment":"live"}`), map[string]string{"Content-Type": "application/json"})
		if resp.StatusCode != 201 {
			t.Fatalf("create key: %d %s", resp.StatusCode, raw)
		}
		var key struct {
			APIKey string `json:"api_key"`
		}
		_ = json.Unmarshal(env.Data, &key)
		apiKey := key.APIKey
		props := strings.Repeat("x", int(container.Config.IngestionMaxPropertyBytes))
		body, _ := json.Marshal(map[string]any{"event_id": "big1", "event_name": "e", "properties": map[string]string{"p": props}})
		resp, env, raw = doReq(t, server, http.MethodPost, "/api/v1/ingest", body, map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + apiKey})
		wantErr(t, resp, env, raw, 400, "VALIDATION_ERROR")
	})

	t.Run("readyz exposes component status only; legacy health routes removed", func(t *testing.T) {
		_, _, raw := doReq(t, server, http.MethodGet, "/readyz", nil, nil)
		if strings.Contains(raw, `"error"`) || !strings.Contains(raw, `"postgres"`) {
			t.Fatalf("unexpected readyz body: %s", raw)
		}
		for _, p := range []string{"/api/v1/health", "/api/v1/healthz", "/api/v1/readyz"} {
			if resp, _, _ := doReq(t, server, http.MethodGet, p, nil, nil); resp.StatusCode != 404 {
				t.Errorf("%s: want 404, got %d", p, resp.StatusCode)
			}
		}
	})

	t.Run("invalid project UUID is 404 everywhere", func(t *testing.T) {
		for _, p := range []string{
			"/api/v1/projects/not-a-uuid/analytics/summary",
			"/api/v1/projects/not-a-uuid/intelligence/digest",
			"/api/v1/projects/not-a-uuid/quota",
			"/api/v1/projects/not-a-uuid/retention",
		} {
			resp, env, raw := doReq(t, server, http.MethodGet, p, nil, nil)
			wantErr(t, resp, env, raw, 404, "NOT_FOUND")
		}
	})

	t.Run("analytics success uses envelope", func(t *testing.T) {
		rng := fmt.Sprintf("from=%s&to=%s", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
		for _, p := range []string{"/analytics/summary?", "/analytics/top-events?", "/analytics/volume?", "/analytics/funnel?steps=a,b&"} {
			resp, env, raw := doReq(t, server, http.MethodGet, base+p+rng, nil, nil)
			if resp.StatusCode != 200 || !env.Success || len(env.Data) == 0 {
				t.Errorf("%s: want success envelope, got %d %s", p, resp.StatusCode, raw)
			}
		}
	})

	t.Run("export validation, range cap and truncation signal", func(t *testing.T) {
		resp, env, raw := doReq(t, server, http.MethodGet, base+"/export?format=json", nil, nil)
		wantErr(t, resp, env, raw, 400, "VALIDATION_ERROR")

		old := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
		resp, env, raw = doReq(t, server, http.MethodGet, base+"/export?from="+old, nil, nil)
		wantErr(t, resp, env, raw, 400, "INVALID_DATE_RANGE")

		if _, err := container.DBPool.Exec(ctx, `
			INSERT INTO events (project_id, event_id, event_name, occurred_at)
			SELECT $1, 'bulk-' || g, 'bulk', now() - g * interval '1 second' FROM generate_series(1, 10001) g`, projectID); err != nil {
			t.Fatalf("seed: %v", err)
		}
		resp, _, raw = doReq(t, server, http.MethodGet, base+"/export", nil, nil)
		if resp.StatusCode != 200 || resp.Header.Get("X-Kinetiq-Truncated") != "true" {
			t.Fatalf("want 200 + X-Kinetiq-Truncated, got %d %v", resp.StatusCode, resp.Header)
		}
		if n := strings.Count(raw, "\n"); n != 10001 {
			t.Fatalf("want header + 10000 rows, got %d lines", n)
		}
	})

	t.Run("server limits and timeouts configured; SSE exempt from write timeout", func(t *testing.T) {
		srv := server.Server()
		if srv.MaxRequestBodySize != int(container.Config.IngestionMaxBodyBytes) {
			t.Errorf("body limit %d", srv.MaxRequestBodySize)
		}
		if srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
			t.Errorf("timeouts not set: %v %v %v", srv.ReadTimeout, srv.WriteTimeout, srv.IdleTimeout)
		}
		var h fasthttp.RequestHeader
		h.SetRequestURI(base + "/live?once=true")
		if got := srv.HeaderReceived(&h).WriteTimeout; got <= srv.WriteTimeout {
			t.Errorf("SSE write timeout %v must exceed %v", got, srv.WriteTimeout)
		}
		h.SetRequestURI(base + "/analytics/summary")
		if got := srv.HeaderReceived(&h).WriteTimeout; got != 0 {
			t.Errorf("non-SSE route must keep server default, got %v", got)
		}
	})

	t.Run("sessions: SQL-side derivation honors client session id, gap and limit", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		ins := func(did, name string, ago time.Duration, props string) {
			at := now.Add(-ago)
			if _, err := container.DBPool.Exec(ctx, `INSERT INTO events (project_id, event_id, event_name, occurred_at, distinct_id, properties) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6::jsonb)`,
				projectID, uuid.NewString(), name, at, did, props); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
		ins("s1", "$pageview", 3*time.Hour, `{"$pathname":"/a","utm_source":"g"}`)
		ins("s1", "$pageview", 3*time.Hour-5*time.Minute, `{"$pathname":"/b"}`)
		ins("s1", "$pageview", time.Hour, `{"$pathname":"/c"}`) // gap > 30m: new session
		ins("s2", "x", 2*time.Hour, `{"$session_id":"cs"}`)
		ins("s2", "x", time.Hour, `{"$session_id":"cs"}`) // client session spans 1h despite gap
		ins("", "x", time.Hour, `{}`)                     // anonymous w/o id: ignored
		q := fmt.Sprintf("/analytics/sessions?from=%s&to=%s&limit=2",
			now.Add(-4*time.Hour).Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339))
		resp, env, raw := doReq(t, server, http.MethodGet, base+q, nil, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("%d %s", resp.StatusCode, raw)
		}
		var res struct {
			TotalSessions int64 `json:"total_sessions"`
			Sessions      []struct {
				SessionID       string `json:"session_id"`
				DistinctID      string `json:"distinct_id"`
				DurationSeconds int64  `json:"duration_seconds"`
				EntryPage       string `json:"entry_page"`
				ExitPage        string `json:"exit_page"`
				UTMSource       string `json:"utm_source"`
				EventCount      int    `json:"event_count"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal(env.Data, &res); err != nil {
			t.Fatal(err)
		}
		if res.TotalSessions != 3 || len(res.Sessions) != 2 {
			t.Fatalf("want 3 total / 2 listed, got %d / %d: %s", res.TotalSessions, len(res.Sessions), raw)
		}
		// newest first: s1/c and s2 (both start...); find client session
		var found bool
		for _, s := range res.Sessions {
			if s.SessionID == "cs" {
				found = true
				if s.DurationSeconds != 3600 || s.EventCount != 2 || s.DistinctID != "s2" {
					t.Errorf("client session wrong: %+v", s)
				}
			}
			if s.SessionID == "" {
				t.Errorf("missing session id: %+v", s)
			}
		}
		if !found {
			t.Errorf("client session not listed: %s", raw)
		}
	})
}

// decodeData decodes the {"success":true,"data":...} envelope payload into v.
func decodeData(r io.Reader, v any) error {
	var env envelope
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, v)
}
