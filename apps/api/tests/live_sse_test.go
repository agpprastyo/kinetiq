package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"kinetiq/internal/app"
)

func TestLiveSSEStream(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	server := app.Setup()
	projectID := uuid.New().String()

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/live?once=true", projectID), nil)
	req.Header.Set("Cookie", "user1_session_cookie")
	req.Header.Set("Accept", "text/event-stream")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := server.Test(req, 1000)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if !strings.Contains(string(body), ": connected") {
		t.Errorf("expected body to contain ': connected', got %q", string(body))
	}
}
