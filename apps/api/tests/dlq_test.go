package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"kinetiq/internal/config"
	"kinetiq/internal/dlq"
)

func TestDLQEndpointsAndReplay(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	server := setupTestServer()
	userCookie := "admin_session_cookie"

	// 1. Create a test project
	body, _ := json.Marshal(map[string]string{"name": "DLQ Test Project " + uuid.NewString()[:8]})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", userCookie)
	resp, err := server.Test(req)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	var createResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&createResp)
	projectID := createResp.Data.ID
	if projectID == "" {
		t.Fatalf("expected project ID to be non-empty")
	}

	// 2. Connect directly to Postgres to insert mock dead letter records
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	rdbOpts, _ := redis.ParseURL(cfg.RedisURL)
	rdb := redis.NewClient(rdbOpts)
	defer rdb.Close()

	var insertedID int64
	poisonPayload := `{"event_id":"evt_dlq_poison","event_name":"poison_pill","invalid_field":123}`
	err = pool.QueryRow(context.Background(),
		`INSERT INTO event_dead_letters (event_id, project_id, payload, reason, attempts, status, failed_at)
		 VALUES ($1, $2, $3::jsonb, $4, $5, 'failed', NOW())
		 RETURNING id`,
		"evt_dlq_poison",
		projectID,
		poisonPayload,
		"validation failed: unexpected schema",
		3,
	).Scan(&insertedID)
	if err != nil {
		t.Fatalf("failed to seed test dead letter record: %v", err)
	}

	t.Run("GET /api/v1/ops/dlq returns list and summary", func(t *testing.T) {
		listReq := httptest.NewRequest(http.MethodGet, "/api/v1/ops/dlq?limit=10", nil)
		listReq.Header.Set("Cookie", userCookie)

		listResp, err := server.Test(listReq)
		if err != nil {
			t.Fatalf("failed to list dead letters: %v", err)
		}
		if listResp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", listResp.StatusCode)
		}

		var result struct {
			Data struct {
				Items      []dlq.DeadLetterItem `json:"items"`
				Summary    dlq.DLQSummary       `json:"summary"`
				Pagination struct {
					Limit   int  `json:"limit"`
					HasMore bool `json:"has_more"`
				} `json:"pagination"`
			} `json:"data"`
		}
		if err := json.NewDecoder(listResp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if result.Data.Summary.TotalItems == 0 {
			t.Errorf("expected summary total items > 0, got %d", result.Data.Summary.TotalItems)
		}
		found := false
		for _, itm := range result.Data.Items {
			if itm.ID == insertedID {
				found = true
				if itm.Status != "failed" {
					t.Errorf("expected item status 'failed', got '%s'", itm.Status)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected to find inserted dead letter record #%d in list", insertedID)
		}
	})

	t.Run("POST /api/v1/ops/dlq/replay/:id reenqueues to stream", func(t *testing.T) {
		replayReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/ops/dlq/replay/%d", insertedID), nil)
		replayReq.Header.Set("Cookie", userCookie)

		replayResp, err := server.Test(replayReq)
		if err != nil {
			t.Fatalf("failed to replay message: %v", err)
		}
		if replayResp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", replayResp.StatusCode)
		}

		// Verify record is marked as replayed
		var status string
		err = pool.QueryRow(context.Background(), "SELECT status FROM event_dead_letters WHERE id = $1", insertedID).Scan(&status)
		if err != nil {
			t.Fatalf("failed to query dead letter status: %v", err)
		}
		if status != "replayed" {
			t.Errorf("expected status 'replayed', got '%s'", status)
		}
	})

	t.Run("POST /api/v1/ops/dlq/replay-all reenqueues remaining messages", func(t *testing.T) {
		// Insert another failed record
		var batchID int64
		_ = pool.QueryRow(context.Background(),
			`INSERT INTO event_dead_letters (event_id, project_id, payload, reason, attempts, status, failed_at)
			 VALUES ($1, $2, $3::jsonb, $4, $5, 'failed', NOW())
			 RETURNING id`,
			"evt_dlq_poison_2",
			projectID,
			poisonPayload,
			"database timeout",
			3,
		).Scan(&batchID)

		replayAllReq := httptest.NewRequest(http.MethodPost, "/api/v1/ops/dlq/replay-all", nil)
		replayAllReq.Header.Set("Cookie", userCookie)

		replayAllResp, err := server.Test(replayAllReq)
		if err != nil {
			t.Fatalf("failed to call replay-all: %v", err)
		}
		if replayAllResp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", replayAllResp.StatusCode)
		}
	})

	t.Run("DELETE /api/v1/ops/dlq/:id purges record", func(t *testing.T) {
		deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/ops/dlq/%d", insertedID), nil)
		deleteReq.Header.Set("Cookie", userCookie)

		deleteResp, err := server.Test(deleteReq)
		if err != nil {
			t.Fatalf("failed to delete dead letter: %v", err)
		}
		if deleteResp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", deleteResp.StatusCode)
		}

		var count int
		_ = pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM event_dead_letters WHERE id = $1", insertedID).Scan(&count)
		if count != 0 {
			t.Errorf("expected record #%d to be purged, found count %d", insertedID, count)
		}
	})

	t.Run("DELETE /api/v1/ops/dlq purges all records", func(t *testing.T) {
		deleteAllReq := httptest.NewRequest(http.MethodDelete, "/api/v1/ops/dlq", nil)
		deleteAllReq.Header.Set("Cookie", userCookie)

		deleteAllResp, err := server.Test(deleteAllReq)
		if err != nil {
			t.Fatalf("failed to delete all dead letters: %v", err)
		}
		if deleteAllResp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", deleteAllResp.StatusCode)
		}

		var count int
		_ = pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM event_dead_letters").Scan(&count)
		if count != 0 {
			t.Errorf("expected all DLQ records to be purged, found count %d", count)
		}
	})
}
