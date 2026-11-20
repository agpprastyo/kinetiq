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
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
	"kinetiq/internal/export"
)

func TestCSVExportStream(t *testing.T) {
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
		Name:        "Export Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	now := time.Now().UTC()
	_ = queries.InsertEvent(ctx, db.InsertEventParams{
		ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
		EventID:    uuid.New().String(),
		EventName:  "purchase_completed",
		OccurredAt: pgtype.Timestamptz{Time: now, Valid: true},
		ReceivedAt: pgtype.Timestamptz{Time: now, Valid: true},
		DistinctID: pgtype.Text{String: "cust_42", Valid: true},
		Properties: []byte(`{"amount":99.5}`),
	})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/export?format=csv", projectID.String()), nil)
	req.Header.Set("Cookie", "user1_session_cookie")

	resp, err := server.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/csv") {
		t.Errorf("expected Content-Type text/csv, got %s", contentType)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	content := string(bodyBytes)
	if !strings.Contains(content, "event_id,event_name,timestamp,distinct_id,properties") {
		t.Errorf("expected header row, got %s", content)
	}
	if !strings.Contains(content, "purchase_completed") {
		t.Errorf("expected body to contain purchase_completed, got %s", content)
	}
	if !strings.Contains(content, "cust_42") {
		t.Errorf("expected body to contain cust_42, got %s", content)
	}

	// Test date filtering: matching range includes event
	fromValid := now.Add(-10 * time.Minute).Format(time.RFC3339)
	toValid := now.Add(10 * time.Minute).Format(time.RFC3339)
	reqFiltered := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/export?format=csv&from=%s&to=%s", projectID.String(), fromValid, toValid), nil)
	reqFiltered.Header.Set("Cookie", "user1_session_cookie")
	respFiltered, err := server.Test(reqFiltered)
	if err != nil || respFiltered.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on filtered export, got %d", respFiltered.StatusCode)
	}
	bodyFiltered, _ := io.ReadAll(respFiltered.Body)
	if !strings.Contains(string(bodyFiltered), "purchase_completed") {
		t.Errorf("expected filtered export to contain purchase_completed")
	}

	// Test date filtering: past range excludes event
	fromPast := now.Add(-2 * time.Hour).Format(time.RFC3339)
	toPast := now.Add(-1 * time.Hour).Format(time.RFC3339)
	reqPast := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/export?format=csv&from=%s&to=%s", projectID.String(), fromPast, toPast), nil)
	reqPast.Header.Set("Cookie", "user1_session_cookie")
	respPast, err := server.Test(reqPast)
	if err != nil || respPast.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on past filtered export, got %d", respPast.StatusCode)
	}
	bodyPast, _ := io.ReadAll(respPast.Body)
	if strings.Contains(string(bodyPast), "purchase_completed") {
		t.Errorf("expected past filtered export to exclude purchase_completed, got %s", string(bodyPast))
	}

	// Test GET /storage-sync initially empty
	reqGetSync := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/storage-sync", projectID.String()), nil)
	reqGetSync.Header.Set("Cookie", "user1_session_cookie")
	respGetSync, err := server.Test(reqGetSync)
	if err != nil || respGetSync.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on get storage-sync, got %d", respGetSync.StatusCode)
	}

	// Test PUT /storage-sync
	syncPayload := `{"provider":"s3","bucket_name":"my-test-lake","region":"us-east-1","access_key_id":"AKIA123","secret_access_key":"supersecret","prefix_path":"events","frequency":"hourly","is_enabled":true}`
	reqPutSync := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/projects/%s/storage-sync", projectID.String()), strings.NewReader(syncPayload))
	reqPutSync.Header.Set("Cookie", "user1_session_cookie")
	reqPutSync.Header.Set("Content-Type", "application/json")
	respPutSync, err := server.Test(reqPutSync)
	if err != nil || respPutSync.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on put storage-sync, got %d", respPutSync.StatusCode)
	}

	bodyPut, _ := io.ReadAll(respPutSync.Body)
	if strings.Contains(string(bodyPut), "supersecret") {
		t.Errorf("CRITICAL SECURITY: secret_access_key was not masked in response!")
	}
	if !strings.Contains(string(bodyPut), "********") {
		t.Errorf("expected masked secret in response, got %s", string(bodyPut))
	}

	// Test POST /storage-sync/test with invalid missing credentials
	reqTestSync := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/storage-sync/test", projectID.String()), strings.NewReader(`{"provider":"s3","bucket_name":""}`))
	reqTestSync.Header.Set("Cookie", "user1_session_cookie")
	reqTestSync.Header.Set("Content-Type", "application/json")
	respTestSync, err := server.Test(reqTestSync)
	if err != nil {
		t.Fatalf("failed to execute test sync request: %v", err)
	}
	if respTestSync.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on empty test config, got %d", respTestSync.StatusCode)
	}

	// Test SyncWorker.ExecuteProjectSync seam
	worker := export.NewSyncWorker(container.DBPool, nil, nil)
	syncCfg := export.ProjectStorageSync{
		ID:              uuid.New(),
		ProjectID:       projectID,
		Provider:        "s3",
		BucketName:      "my-test-lake",
		AccessKeyID:     "AKIA123",
		SecretAccessKey: "supersecret",
		PrefixPath:      "events",
		Frequency:       "daily",
		IsEnabled:       true,
	}
	_ = worker.ExecuteProjectSync(ctx, syncCfg, time.Now())
}
