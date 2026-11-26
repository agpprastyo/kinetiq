package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"kinetiq/internal/app"
)

func TestIngestSeam(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	container, err := app.NewContainer(context.Background())
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))

	t.Run("missing API key returns 401 Unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"event_id":   "evt_01",
			"event_name": "page_view",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed request: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid or malformed API key returns 401 Unauthorized", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"event_id":   "evt_01",
			"event_name": "page_view",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sp_live_invalid_secret")

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed request: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("valid API key with valid single event returns 202 Accepted and enqueues to Redis", func(t *testing.T) {
		// 1. Create a project to obtain valid API key
		projBody, _ := json.Marshal(map[string]string{"name": "Ingestion Test Project " + uuid.NewString()[:8]})
		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(projBody))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.Header.Set("Cookie", "user1_session_cookie")

		createResp, err := server.Test(createReq)
		if err != nil {
			t.Fatalf("failed to create project: %v", err)
		}

		var projectWrapper struct {
			Data struct {
				ID     string `json:"id"`
				APIKey string `json:"api_key"`
			} `json:"data"`
		}
		json.NewDecoder(createResp.Body).Decode(&projectWrapper)
		validAPIKey := projectWrapper.Data.APIKey
		if validAPIKey == "" {
			t.Fatalf("failed to obtain raw API key")
		}

		// 2. Reject event with missing event_name
		invalidEvent, _ := json.Marshal(map[string]interface{}{
			"event_id": "evt_test_01",
			// event_name is missing
			"properties": map[string]interface{}{"path": "/home"},
		})
		reqInvalid := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(invalidEvent))
		reqInvalid.Header.Set("Content-Type", "application/json")
		reqInvalid.Header.Set("Authorization", "Bearer "+validAPIKey)

		respInvalid, _ := server.Test(reqInvalid)
		if respInvalid.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400 for invalid event, got %d", respInvalid.StatusCode)
		}

		// 3. Send valid single event
		eventID := "evt_test_success_01"
		validEvent, _ := json.Marshal(map[string]interface{}{
			"event_id":    eventID,
			"event_name":  "page_view",
			"distinct_id": "visitor_99",
			"occurred_at": time.Now().UTC().Format(time.RFC3339),
			"properties":  map[string]interface{}{"url": "https://example.com/pricing"},
		})
		reqValid := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(validEvent))
		reqValid.Header.Set("Content-Type", "application/json")
		reqValid.Header.Set("Authorization", "Bearer "+validAPIKey)

		respValid, err := server.Test(reqValid)
		if err != nil {
			t.Fatalf("failed valid ingest request: %v", err)
		}
		if respValid.StatusCode != http.StatusAccepted {
			t.Fatalf("expected status 202 Accepted, got %d", respValid.StatusCode)
		}

		var ingestResp struct {
			Success bool `json:"success"`
			Data    struct {
				Status   string `json:"status"`
				Accepted int    `json:"accepted"`
				Rejected int    `json:"rejected"`
			} `json:"data"`
		}
		json.NewDecoder(respValid.Body).Decode(&ingestResp)
		if !ingestResp.Success || ingestResp.Data.Accepted != 1 {
			t.Errorf("expected 1 accepted event, got %+v", ingestResp)
		}
	})

	t.Run("server-side metadata enrichment injects client device, browser, os, geo properties", func(t *testing.T) {
		projBody, _ := json.Marshal(map[string]string{"name": "Enrichment Project " + uuid.NewString()[:8]})
		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(projBody))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.Header.Set("Cookie", "user1_session_cookie")
		createResp, _ := server.Test(createReq)

		var projectWrapper struct {
			Data struct {
				APIKey string `json:"api_key"`
			} `json:"data"`
		}
		json.NewDecoder(createResp.Body).Decode(&projectWrapper)
		apiKey := projectWrapper.Data.APIKey

		eventID := "evt_enrich_test_" + uuid.NewString()[:8]
		validEvent, _ := json.Marshal(map[string]interface{}{
			"event_id":    eventID,
			"event_name":  "page_view",
			"distinct_id": "visitor_enriched",
			"properties":  map[string]interface{}{"custom_prop": "hello"},
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(validEvent))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("CF-IPCountry", "ID")

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
		}
	})

	t.Run("atomic batch validation: any invalid event rejects the whole batch", func(t *testing.T) {
		// Use a valid API key
		projBody, _ := json.Marshal(map[string]string{"name": "Batch Test Project " + uuid.NewString()[:8]})
		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(projBody))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.Header.Set("Cookie", "user1_session_cookie")
		createResp, _ := server.Test(createReq)

		var projectWrapper struct {
			Data struct {
				APIKey string `json:"api_key"`
			} `json:"data"`
		}
		json.NewDecoder(createResp.Body).Decode(&projectWrapper)
		validAPIKey := projectWrapper.Data.APIKey

		// Batch where second event has empty event_id
		batchPayload, _ := json.Marshal(map[string]interface{}{
			"events": []map[string]interface{}{
				{
					"event_id":   "evt_batch_01",
					"event_name": "sign_up",
				},
				{
					"event_id":   "", // INVALID
					"event_name": "checkout",
				},
			},
		})
		reqBatch := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(batchPayload))
		reqBatch.Header.Set("Content-Type", "application/json")
		reqBatch.Header.Set("Authorization", "Bearer "+validAPIKey)

		respBatch, _ := server.Test(reqBatch)
		if respBatch.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400 for batch with invalid event, got %d", respBatch.StatusCode)
		}
	})
}
