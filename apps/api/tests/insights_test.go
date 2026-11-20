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
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
	"kinetiq/internal/insight"
)

func TestProjectInsightsSeam(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(container)
	queries := db.New(container.DBPool)

	userCookie := "user1_session_cookie"
	projectID := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Insights Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Seed some sample events
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		_, err := container.DBPool.Exec(ctx, `
			INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
		`, projectID, uuid.New().String(), "checkout", fmt.Sprintf("user_%d", i%2), `{"amount": 100}`, now.Add(-time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("failed to seed event: %v", err)
		}
	}

	var createdInsightID string

	t.Run("POST /projects/:id/insights creates new custom insight", func(t *testing.T) {
		payload := map[string]interface{}{
			"title":        "Total Checkouts",
			"chart_type":   "metric",
			"event_name":   "checkout",
			"aggregation":  "count",
			"property_key": "amount",
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/api/v1/projects/%s/insights", projectID.String()),
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", userCookie)

		resp, err := server.Test(req, 5000)
		if err != nil {
			t.Fatalf("request error: %v", err)
		}
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 201/200, got %d", resp.StatusCode)
		}

		var wrapper struct {
			Success bool `json:"success"`
			Data    struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				EventName   string `json:"event_name"`
				Aggregation string `json:"aggregation"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if wrapper.Data.Title != "Total Checkouts" {
			t.Errorf("expected title 'Total Checkouts', got '%s'", wrapper.Data.Title)
		}
		createdInsightID = wrapper.Data.ID
	})

	t.Run("GET /projects/:id/insights lists insights with computed values", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("/api/v1/projects/%s/insights", projectID.String()),
			nil,
		)
		req.Header.Set("Cookie", userCookie)

		resp, err := server.Test(req, 5000)
		if err != nil {
			t.Fatalf("request error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var wrapper struct {
			Success bool `json:"success"`
			Data    []struct {
				ID            string  `json:"id"`
				Title         string  `json:"title"`
				ComputedValue float64 `json:"computed_value"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(wrapper.Data) == 0 {
			t.Fatalf("expected at least 1 insight in list")
		}

		// 5 checkouts seeded
		if wrapper.Data[0].ComputedValue != 5.0 {
			t.Errorf("expected computed count 5.0, got %.2f", wrapper.Data[0].ComputedValue)
		}
	})

	t.Run("PUT /projects/:id/insights/:insightId updates insight configuration", func(t *testing.T) {
		newTitle := "Total Checkout Amount (Updated)"
		newAgg := "sum_property"
		newChart := "bar"
		prop := "amount"
		payload := map[string]interface{}{
			"title":        newTitle,
			"aggregation":  newAgg,
			"chart_type":   newChart,
			"property_key": prop,
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(
			http.MethodPut,
			fmt.Sprintf("/api/v1/projects/%s/insights/%s", projectID.String(), createdInsightID),
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", userCookie)

		resp, err := server.Test(req, 5000)
		if err != nil {
			t.Fatalf("request error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK on update, got %d", resp.StatusCode)
		}

		var wrapper struct {
			Success bool `json:"success"`
			Data    struct {
				ID            string  `json:"id"`
				Title         string  `json:"title"`
				ChartType     string  `json:"chart_type"`
				Aggregation   string  `json:"aggregation"`
				ComputedValue float64 `json:"computed_value"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if wrapper.Data.Title != newTitle {
			t.Errorf("expected updated title %s, got %s", newTitle, wrapper.Data.Title)
		}
		// 5 checkouts with amount 100 = 500.0
		if wrapper.Data.ComputedValue != 500.0 {
			t.Errorf("expected computed sum 500.0, got %.2f", wrapper.Data.ComputedValue)
		}
	})

	t.Run("Insight validation errors and 404 paths", func(t *testing.T) {
		// Test invalid json body on create
		badReq := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/api/v1/projects/%s/insights", projectID.String()),
			bytes.NewReader([]byte("{invalid-json}")),
		)
		badReq.Header.Set("Content-Type", "application/json")
		badReq.Header.Set("Cookie", userCookie)
		resp, _ := server.Test(badReq, 5000)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for bad json, got %d", resp.StatusCode)
		}

		// Test validation errors on create
		invalidReq := httptest.NewRequest(
			http.MethodPost,
			fmt.Sprintf("/api/v1/projects/%s/insights", projectID.String()),
			bytes.NewReader([]byte(`{"title":"","chart_type":"unknown"}`)),
		)
		invalidReq.Header.Set("Content-Type", "application/json")
		invalidReq.Header.Set("Cookie", userCookie)
		resp, _ = server.Test(invalidReq, 5000)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for validation failure, got %d", resp.StatusCode)
		}

		// Test unauthorized request
		unauthReq := httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("/api/v1/projects/%s/insights", projectID.String()),
			nil,
		)
		resp, _ = server.Test(unauthReq, 5000)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for missing cookie, got %d", resp.StatusCode)
		}

		// Test update with invalid id
		invalidUpdate := httptest.NewRequest(
			http.MethodPut,
			fmt.Sprintf("/api/v1/projects/%s/insights/not-a-uuid", projectID.String()),
			bytes.NewReader([]byte(`{"title":"hello"}`)),
		)
		invalidUpdate.Header.Set("Content-Type", "application/json")
		invalidUpdate.Header.Set("Cookie", userCookie)
		resp, _ = server.Test(invalidUpdate, 5000)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for bad uuid, got %d", resp.StatusCode)
		}

		// Test delete with invalid id
		invalidDel := httptest.NewRequest(
			http.MethodDelete,
			fmt.Sprintf("/api/v1/projects/%s/insights/not-a-uuid", projectID.String()),
			nil,
		)
		invalidDel.Header.Set("Cookie", userCookie)
		resp, _ = server.Test(invalidDel, 5000)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for bad uuid, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /projects/:id/insights/:insightId deletes insight", func(t *testing.T) {
		if createdInsightID == "" {
			t.Skip("skipping delete: no insight created")
		}

		req := httptest.NewRequest(
			http.MethodDelete,
			fmt.Sprintf("/api/v1/projects/%s/insights/%s", projectID.String(), createdInsightID),
			nil,
		)
		req.Header.Set("Cookie", userCookie)

		resp, err := server.Test(req, 5000)
		if err != nil {
			t.Fatalf("request error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK on delete, got %d", resp.StatusCode)
		}
	})

	t.Run("ComputeValue safely handles malformed, non-numeric, or missing JSON properties without SQL error", func(t *testing.T) {
		repo := insight.NewRepository(container.DBPool)

		// Seed event with non-numeric string, boolean, object, and missing property
		badEvents := []string{
			`{"amount": "free"}`,
			`{"amount": "NaN"}`,
			`{"amount": true}`,
			`{"amount": {"sub": 10}}`,
			`{"other": 100}`,
			`{"amount": "12.50"}`,
		}

		for _, p := range badEvents {
			_, err := container.DBPool.Exec(ctx, `
				INSERT INTO events (project_id, event_id, event_name, distinct_id, properties, occurred_at, received_at)
				VALUES ($1, $2, 'donation', 'donator_1', $3, NOW(), NOW())
			`, projectID, uuid.New().String(), p)
			if err != nil {
				t.Fatalf("failed to insert test event: %v", err)
			}
		}

		prop := "amount"
		val, err := repo.ComputeValue(ctx, projectID, "donation", "sum_property", &prop)
		if err != nil {
			t.Fatalf("ComputeValue failed on non-numeric property: %v", err)
		}

		// Only "12.50" should be matched and summed by regex
		if val != 12.50 {
			t.Errorf("expected 12.50, got %.2f", val)
		}

		// Test non-existent property key
		missingProp := "non_existent_key"
		valMissing, err := repo.ComputeValue(ctx, projectID, "donation", "sum_property", &missingProp)
		if err != nil {
			t.Fatalf("ComputeValue failed on missing property: %v", err)
		}
		if valMissing != 0.0 {
			t.Errorf("expected 0.0 for missing property, got %.2f", valMissing)
		}
	})
}
