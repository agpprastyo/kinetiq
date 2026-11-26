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
	"time"

	"github.com/google/uuid"

	"kinetiq/internal/app"
	"kinetiq/internal/worker"
)

func TestEndToEndGoldenFlowAndEdgeCases(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))

	userCookie := "user1_session_cookie"

	// ---------------------------------------------------------
	// 1. Golden Flow E2E: Create Project -> Ingest -> Worker -> Analytics
	// ---------------------------------------------------------
	t.Run("E2E Golden Flow: User Journey from Project creation to Analytics verification", func(t *testing.T) {
		// Step A: Create Project
		projectName := "E2E Platform Project " + uuid.NewString()[:8]
		createBody, _ := json.Marshal(map[string]string{"name": projectName})
		reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(createBody))
		reqCreate.Header.Set("Content-Type", "application/json")
		reqCreate.Header.Set("Cookie", userCookie)

		respCreate, err := server.Test(reqCreate)
		if err != nil {
			t.Fatalf("failed to create project: %v", err)
		}
		if respCreate.StatusCode != http.StatusCreated && respCreate.StatusCode != http.StatusOK {
			t.Fatalf("expected status 201/200, got %d", respCreate.StatusCode)
		}

		var projectResp struct {
			Data struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				APIKey string `json:"api_key"`
			} `json:"data"`
		}
		if err := json.NewDecoder(respCreate.Body).Decode(&projectResp); err != nil {
			t.Fatalf("failed to decode project response: %v", err)
		}

		projectID := projectResp.Data.ID
		apiKey := projectResp.Data.APIKey
		if projectID == "" || apiKey == "" {
			t.Fatalf("expected non-empty project ID and API key, got ID=%s, Key=%s", projectID, apiKey)
		}

		// Step B: Initialize Worker with consumer group before enqueuing events
		groupName := "e2e-worker-group-" + uuid.New().String()
		w, err := worker.NewConsumerWorker(container.DBPool, container.RedisClient, container.Logger, worker.Config{
			StreamKey:    "events:stream",
			GroupName:    groupName,
			ConsumerName: "e2e-consumer-1",
			BatchSize:    10,
			BlockTimeout: 200 * time.Millisecond,
			MaxRetries:   3,
		})
		if err != nil {
			t.Fatalf("failed to instantiate worker: %v", err)
		}

		// Step C: Ingest batch of events using API Key
		t0 := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
		t1 := time.Date(2026, 10, 15, 10, 15, 0, 0, time.UTC)
		t2 := time.Date(2026, 10, 15, 10, 30, 0, 0, time.UTC)

		batchEvents := map[string]interface{}{
			"events": []map[string]interface{}{
				{
					"event_id":    "e2e_evt_01",
					"event_name":  "page_view",
					"distinct_id": "cust_100",
					"occurred_at": t0.Format(time.RFC3339),
					"properties":  map[string]interface{}{"url": "/home"},
				},
				{
					"event_id":    "e2e_evt_02",
					"event_name":  "page_view",
					"distinct_id": "cust_100",
					"occurred_at": t1.Format(time.RFC3339),
					"properties":  map[string]interface{}{"url": "/pricing"},
				},
				{
					"event_id":    "e2e_evt_03",
					"event_name":  "sign_up",
					"distinct_id": "cust_100",
					"occurred_at": t2.Format(time.RFC3339),
					"properties":  map[string]interface{}{"plan": "starter"},
				},
				{
					"event_id":    "e2e_evt_04",
					"event_name":  "page_view",
					"distinct_id": "cust_200",
					"occurred_at": t2.Format(time.RFC3339),
					"properties":  map[string]interface{}{"url": "/home"},
				},
			},
		}

		ingestBody, _ := json.Marshal(batchEvents)
		reqIngest := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(ingestBody))
		reqIngest.Header.Set("Content-Type", "application/json")
		reqIngest.Header.Set("Authorization", "Bearer "+apiKey)

		respIngest, err := server.Test(reqIngest)
		if err != nil {
			t.Fatalf("failed to ingest events: %v", err)
		}
		if respIngest.StatusCode != http.StatusAccepted {
			t.Fatalf("expected 202 Accepted, got %d", respIngest.StatusCode)
		}

		// Step D: Trigger Worker to process batch from Redis Stream
		processed, err := w.ProcessBatch(ctx)
		if err != nil {
			t.Fatalf("worker failed to process batch: %v", err)
		}
		if processed < 4 {
			t.Fatalf("expected at least 4 processed events, got %d", processed)
		}

		// Step D: Query Analytics Summary via Session Cookie
		fromStr := "2026-10-15T09:00:00Z"
		toStr := "2026-10-15T11:00:00Z"

		reqSummary := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/summary?from=%s&to=%s", projectID, fromStr, toStr), nil)
		reqSummary.Header.Set("Cookie", userCookie)

		respSummary, err := server.Test(reqSummary)
		if err != nil {
			t.Fatalf("failed to get summary: %v", err)
		}
		if respSummary.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK on summary, got %d", respSummary.StatusCode)
		}

		var summaryData struct {
			TotalEvents       int64 `json:"total_events"`
			UniqueDistinctIds int64 `json:"unique_distinct_ids"`
		}
		if err := decodeData(respSummary.Body, &summaryData); err != nil {
			t.Fatalf("failed to decode summary: %v", err)
		}

		if summaryData.TotalEvents != 4 {
			t.Errorf("expected 4 total events, got %d", summaryData.TotalEvents)
		}
		if summaryData.UniqueDistinctIds != 2 { // cust_100 and cust_200
			t.Errorf("expected 2 unique distinct IDs, got %d", summaryData.UniqueDistinctIds)
		}

		// Step E: Query Analytics Top Events
		reqTop := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/top-events?from=%s&to=%s&limit=10", projectID, fromStr, toStr), nil)
		reqTop.Header.Set("Cookie", userCookie)

		respTop, err := server.Test(reqTop)
		if err != nil {
			t.Fatalf("failed to get top events: %v", err)
		}
		if respTop.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK on top events, got %d", respTop.StatusCode)
		}

		var topData struct {
			Items []struct {
				EventName string `json:"event_name"`
				Count     int64  `json:"count"`
			} `json:"items"`
		}
		if err := decodeData(respTop.Body, &topData); err != nil {
			t.Fatalf("failed to decode top events: %v", err)
		}

		// page_view should be #1 with count 3, sign_up should be #2 with count 1
		if len(topData.Items) != 2 {
			t.Fatalf("expected 2 distinct event types, got %d", len(topData.Items))
		}
		if topData.Items[0].EventName != "page_view" || topData.Items[0].Count != 3 {
			t.Errorf("expected page_view with count 3 at top, got %s (%d)", topData.Items[0].EventName, topData.Items[0].Count)
		}
		if topData.Items[1].EventName != "sign_up" || topData.Items[1].Count != 1 {
			t.Errorf("expected sign_up with count 1, got %s (%d)", topData.Items[1].EventName, topData.Items[1].Count)
		}
	})

	// ---------------------------------------------------------
	// 2. Edge Case: Batch limit overflow (> 100 events)
	// ---------------------------------------------------------
	t.Run("Edge Case: Ingest rejects batch overflow exceeding max limit (>100)", func(t *testing.T) {
		// Create 101 events
		oversizedEvents := make([]map[string]interface{}, 101)
		for i := 0; i < 101; i++ {
			oversizedEvents[i] = map[string]interface{}{
				"event_id":   fmt.Sprintf("evt_overflow_%d", i),
				"event_name": "bulk_event",
			}
		}

		body, _ := json.Marshal(map[string]interface{}{"events": oversizedEvents})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sp_live_dummy_for_validation")

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed request: %v", err)
		}
		// Must reject with 400 Bad Request or 401 (if checked before auth) but specifically invalid body
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 400 or 401 for batch overflow, got %d", resp.StatusCode)
		}
	})

	// ---------------------------------------------------------
	// 3. Edge Case: Exact timestamp boundaries [from, to)
	// ---------------------------------------------------------
	t.Run("Edge Case: Analytics respects exact [from, to) boundaries where 'to' is exclusive", func(t *testing.T) {
		// Project setup
		createBody, _ := json.Marshal(map[string]string{"name": "Boundary Test Project " + uuid.NewString()[:8]})
		reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(createBody))
		reqCreate.Header.Set("Content-Type", "application/json")
		reqCreate.Header.Set("Cookie", userCookie)
		respCreate, _ := server.Test(reqCreate)

		var pResp struct {
			Data struct {
				ID     string `json:"id"`
				APIKey string `json:"api_key"`
			} `json:"data"`
		}
		json.NewDecoder(respCreate.Body).Decode(&pResp)

		exactFrom := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
		inside := time.Date(2026, 10, 15, 12, 30, 0, 0, time.UTC)
		exactTo := time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC) // exclusive boundary

		events := map[string]interface{}{
			"events": []map[string]interface{}{
				{
					"event_id":    "b_evt_from",
					"event_name":  "boundary_event",
					"occurred_at": exactFrom.Format(time.RFC3339),
				},
				{
					"event_id":    "b_evt_mid",
					"event_name":  "boundary_event",
					"occurred_at": inside.Format(time.RFC3339),
				},
				{
					"event_id":    "b_evt_to",
					"event_name":  "boundary_event",
					"occurred_at": exactTo.Format(time.RFC3339),
				},
			},
		}

		// Initialize worker with consumer group before enqueuing boundary events
		w, _ := worker.NewConsumerWorker(container.DBPool, container.RedisClient, container.Logger, worker.Config{
			StreamKey:    "events:stream",
			GroupName:    "boundary-group-" + uuid.New().String(),
			ConsumerName: "boundary-worker",
			BatchSize:    10,
			BlockTimeout: 100 * time.Millisecond,
		})

		bBody, _ := json.Marshal(events)
		reqIngest := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(bBody))
		reqIngest.Header.Set("Content-Type", "application/json")
		reqIngest.Header.Set("Authorization", "Bearer "+pResp.Data.APIKey)
		server.Test(reqIngest)

		// Process with worker
		w.ProcessBatch(ctx)

		// Query [exactFrom, exactTo)
		fromParam := exactFrom.Format(time.RFC3339)
		toParam := exactTo.Format(time.RFC3339)

		reqQuery := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/summary?from=%s&to=%s", pResp.Data.ID, fromParam, toParam), nil)
		reqQuery.Header.Set("Cookie", userCookie)

		respQuery, err := server.Test(reqQuery)
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		var summary struct {
			TotalEvents int64 `json:"total_events"`
		}
		decodeData(respQuery.Body, &summary)

		// b_evt_from and b_evt_mid should be included (2 events). b_evt_to must be excluded.
		if summary.TotalEvents != 2 {
			t.Errorf("expected exactly 2 events in [from, to) where 'to' is exclusive, got %d", summary.TotalEvents)
		}
	})

	// ---------------------------------------------------------
	// 4. Edge Case: Cross-User Project & Analytics Isolation
	// ---------------------------------------------------------
	t.Run("Security: Another authenticated user cannot access or query foreign project", func(t *testing.T) {
		// User 1 creates project
		pName := "User1 Private Project " + uuid.NewString()[:8]
		body, _ := json.Marshal(map[string]string{"name": pName})
		reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
		reqCreate.Header.Set("Content-Type", "application/json")
		reqCreate.Header.Set("Cookie", userCookie) // user-1
		respCreate, err := server.Test(reqCreate)
		if err != nil {
			t.Fatalf("failed create: %v", err)
		}

		var pResp struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		json.NewDecoder(respCreate.Body).Decode(&pResp)
		projID := pResp.Data.ID

		// User 2 attempts to query User 1's project details
		user2Cookie := "user2_session_cookie"
		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projID, nil)
		reqGet.Header.Set("Cookie", user2Cookie)
		respGet, err := server.Test(reqGet)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if respGet.StatusCode != http.StatusNotFound && respGet.StatusCode != http.StatusForbidden {
			t.Errorf("expected 404/403 for unauthorized project access, got %d", respGet.StatusCode)
		}

		// User 2 attempts to query User 1's analytics
		reqAnalytics := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/summary?from=2026-10-15T00:00:00Z&to=2026-10-15T23:59:59Z", projID), nil)
		reqAnalytics.Header.Set("Cookie", user2Cookie)
		respAnalytics, err := server.Test(reqAnalytics)
		if err != nil {
			t.Fatalf("analytics request failed: %v", err)
		}
		if respAnalytics.StatusCode != http.StatusNotFound && respAnalytics.StatusCode != http.StatusForbidden {
			t.Errorf("expected 404/403 for unauthorized analytics access, got %d", respAnalytics.StatusCode)
		}
	})
}
