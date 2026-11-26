package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

func TestLiveSSEStream(t *testing.T) {
	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))

	projectID := uuid.New()
	if _, err := db.New(container.DBPool).CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Live Project " + projectID.String()[:8],
		Status:      "active",
	}); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	get := func(path, cookie string) *http.Response {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Cookie", cookie)
		req.Header.Set("Accept", "text/event-stream")
		rctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		t.Cleanup(cancel)
		resp, err := server.Test(req.WithContext(rctx), 1000)
		if err != nil {
			t.Fatalf("failed to execute request: %v", err)
		}
		return resp
	}

	t.Run("owner can stream", func(t *testing.T) {
		resp := get(fmt.Sprintf("/api/v1/projects/%s/live?once=true", projectID), "user1_session_cookie")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
			t.Errorf("expected text/event-stream, got %s", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), ": connected") {
			t.Errorf("expected ': connected', got %q", body)
		}
	})

	t.Run("non-member gets 404", func(t *testing.T) {
		resp := get(fmt.Sprintf("/api/v1/projects/%s/live?once=true", projectID), "user2_session_cookie")
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant live stream: expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid UUID gets 404", func(t *testing.T) {
		resp := get("/api/v1/projects/not-a-uuid/live?once=true", "user1_session_cookie")
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}
