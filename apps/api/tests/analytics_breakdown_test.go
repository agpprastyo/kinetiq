package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

func TestPropertyBreakdown(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(container)
	queries := db.New(container.DBPool)

	projectID := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Breakdown Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	now := time.Now().UTC()
	// Insert 2 chrome, 1 safari
	_ = queries.InsertEvent(ctx, db.InsertEventParams{
		ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
		EventID:    uuid.New().String(),
		EventName:  "page_view",
		OccurredAt: pgtype.Timestamptz{Time: now.Add(-10 * time.Minute), Valid: true},
		ReceivedAt: pgtype.Timestamptz{Time: now.Add(-10 * time.Minute), Valid: true},
		Properties: []byte(`{"browser":"chrome"}`),
	})
	_ = queries.InsertEvent(ctx, db.InsertEventParams{
		ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
		EventID:    uuid.New().String(),
		EventName:  "page_view",
		OccurredAt: pgtype.Timestamptz{Time: now.Add(-5 * time.Minute), Valid: true},
		ReceivedAt: pgtype.Timestamptz{Time: now.Add(-5 * time.Minute), Valid: true},
		Properties: []byte(`{"browser":"chrome"}`),
	})
	_ = queries.InsertEvent(ctx, db.InsertEventParams{
		ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
		EventID:    uuid.New().String(),
		EventName:  "page_view",
		OccurredAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true},
		ReceivedAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true},
		Properties: []byte(`{"browser":"safari"}`),
	})

	from := url.QueryEscape(now.Add(-1 * time.Hour).Format(time.RFC3339))
	to := url.QueryEscape(now.Add(1 * time.Hour).Format(time.RFC3339))
	reqURL := fmt.Sprintf("/api/v1/projects/%s/analytics/breakdown?event_name=page_view&property=browser&from=%s&to=%s", projectID.String(), from, to)

	req := httptest.NewRequest(http.MethodGet, reqURL, nil)
	req.Header.Set("Cookie", "user1_session_cookie")

	resp, err := server.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	var res struct {
		Success bool `json:"success"`
		Data    []struct {
			Value string `json:"value"`
			Count int64  `json:"count"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(res.Data) != 2 {
		t.Fatalf("expected 2 breakdown items, got %d", len(res.Data))
	}
	if res.Data[0].Value != "chrome" || res.Data[0].Count != 2 {
		t.Errorf("expected chrome: 2, got %s: %d", res.Data[0].Value, res.Data[0].Count)
	}
	if res.Data[1].Value != "safari" || res.Data[1].Count != 1 {
		t.Errorf("expected safari: 1, got %s: %d", res.Data[1].Value, res.Data[1].Count)
	}

	// Test Device Breakdown & Geo Breakdown endpoints
	// Insert events with enriched properties
	_ = queries.InsertEvent(ctx, db.InsertEventParams{
		ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
		EventID:    uuid.New().String(),
		EventName:  "custom_click",
		OccurredAt: pgtype.Timestamptz{Time: now.Add(-5 * time.Minute), Valid: true},
		ReceivedAt: pgtype.Timestamptz{Time: now.Add(-5 * time.Minute), Valid: true},
		Properties: []byte(`{"$device_type":"desktop", "$geo_country":"ID"}`),
	})
	_ = queries.InsertEvent(ctx, db.InsertEventParams{
		ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
		EventID:    uuid.New().String(),
		EventName:  "custom_click",
		OccurredAt: pgtype.Timestamptz{Time: now.Add(-4 * time.Minute), Valid: true},
		ReceivedAt: pgtype.Timestamptz{Time: now.Add(-4 * time.Minute), Valid: true},
		Properties: []byte(`{"$device_type":"desktop", "$geo_country":"ID"}`),
	})
	_ = queries.InsertEvent(ctx, db.InsertEventParams{
		ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
		EventID:    uuid.New().String(),
		EventName:  "custom_click",
		OccurredAt: pgtype.Timestamptz{Time: now.Add(-3 * time.Minute), Valid: true},
		ReceivedAt: pgtype.Timestamptz{Time: now.Add(-3 * time.Minute), Valid: true},
		Properties: []byte(`{"$device_type":"mobile", "$geo_country":"SG"}`),
	})

	// 1. GET /analytics/breakdown/devices
	devReqURL := fmt.Sprintf("/api/v1/projects/%s/analytics/breakdown/devices?from=%s&to=%s", projectID.String(), from, to)
	devReq := httptest.NewRequest(http.MethodGet, devReqURL, nil)
	devReq.Header.Set("Cookie", "user1_session_cookie")
	devResp, err := server.Test(devReq)
	if err != nil {
		t.Fatalf("failed device breakdown request: %v", err)
	}
	if devResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", devResp.StatusCode)
	}
	var devResult struct {
		Success bool `json:"success"`
		Data    []struct {
			DeviceType string `json:"device_type"`
			Count      int64  `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(devResp.Body).Decode(&devResult)
	if len(devResult.Data) < 2 {
		t.Fatalf("expected at least 2 device types, got %d", len(devResult.Data))
	}

	// 2. GET /analytics/breakdown/geo
	geoReqURL := fmt.Sprintf("/api/v1/projects/%s/analytics/breakdown/geo?from=%s&to=%s&limit=5", projectID.String(), from, to)
	geoReq := httptest.NewRequest(http.MethodGet, geoReqURL, nil)
	geoReq.Header.Set("Cookie", "user1_session_cookie")
	geoResp, err := server.Test(geoReq)
	if err != nil {
		t.Fatalf("failed geo breakdown request: %v", err)
	}
	if geoResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", geoResp.StatusCode)
	}
	var geoResult struct {
		Success bool `json:"success"`
		Data    []struct {
			CountryCode string `json:"country_code"`
			Count       int64  `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(geoResp.Body).Decode(&geoResult)
	if len(geoResult.Data) < 2 {
		t.Fatalf("expected at least 2 geo countries, got %d", len(geoResult.Data))
	}
}
