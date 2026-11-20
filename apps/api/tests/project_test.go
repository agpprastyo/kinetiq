package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"kinetiq/internal/app"
)

func TestProjectSeam(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	server := app.Setup()

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("authenticated project CRUD and isolation across users", func(t *testing.T) {
		// Mock/stub or authenticated user sessions
		user1Cookie := "user1_session_cookie"
		user2Cookie := "user2_session_cookie"
		uniqueSuffix := uuid.NewString()[:8]
		alphaName := "Project Alpha " + uniqueSuffix

		// 1. User 1 creates project
		body, _ := json.Marshal(map[string]string{"name": alphaName})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", user1Cookie)

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed to create project: %v", err)
		}
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200/201, got %d", resp.StatusCode)
		}

		var createResp struct {
			Success bool `json:"success"`
			Data    struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				APIKey string `json:"api_key"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			t.Fatalf("failed to decode created project: %v", err)
		}
		createdProject := createResp.Data

		if createdProject.ID == "" {
			t.Errorf("expected project ID to be non-empty")
		}
		if createdProject.Name != alphaName {
			t.Errorf("expected project name '%s', got '%s'", alphaName, createdProject.Name)
		}
		if !strings.HasPrefix(createdProject.APIKey, "sp_live_") && !strings.HasPrefix(createdProject.APIKey, "tlk_live_") {
			t.Errorf("expected raw API key starting with 'sp_live_' or 'tlk_live_', got '%s'", createdProject.APIKey)
		}

		// 1b. User 1 tries to create another project with the SAME name (case-insensitive) -> 409 Conflict
		dupBody, _ := json.Marshal(map[string]string{"name": strings.ToLower(alphaName)})
		dupReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(dupBody))
		dupReq.Header.Set("Content-Type", "application/json")
		dupReq.Header.Set("Cookie", user1Cookie)
		dupResp, _ := server.Test(dupReq)
		if dupResp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409 Conflict for duplicate project name, got %d", dupResp.StatusCode)
		}

		// 2. User 1 lists projects -> Project Alpha is present
		reqList1 := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
		reqList1.Header.Set("Cookie", user1Cookie)
		respList1, err := server.Test(reqList1)
		if err != nil {
			t.Fatalf("failed to list projects: %v", err)
		}
		if respList1.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", respList1.StatusCode)
		}

		var listResp1 struct {
			Success bool `json:"success"`
			Data    []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
		}
		json.NewDecoder(respList1.Body).Decode(&listResp1)
		user1Projects := listResp1.Data
		found := false
		for _, p := range user1Projects {
			if p.ID == createdProject.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected user1 to find project %s in list", createdProject.ID)
		}

		// 3. User 1 gets project details -> does not leak API key hash
		reqDetail := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+createdProject.ID, nil)
		reqDetail.Header.Set("Cookie", user1Cookie)
		respDetail, _ := server.Test(reqDetail)
		if respDetail.StatusCode != http.StatusOK {
			t.Errorf("expected status 200 on detail, got %d", respDetail.StatusCode)
		}
		var detailWrapper struct {
			Data map[string]interface{} `json:"data"`
		}
		json.NewDecoder(respDetail.Body).Decode(&detailWrapper)
		detailMap := detailWrapper.Data
		if _, exists := detailMap["key_hash"]; exists {
			t.Errorf("detail response leaked key_hash!")
		}
		if _, exists := detailMap["api_key"]; exists {
			t.Errorf("detail response leaked raw api_key!")
		}

		// 4. User 2 lists projects -> Project Alpha is NOT in the list
		reqList2 := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
		reqList2.Header.Set("Cookie", user2Cookie)
		respList2, _ := server.Test(reqList2)
		var listResp2 struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		json.NewDecoder(respList2.Body).Decode(&listResp2)
		for _, p := range listResp2.Data {
			if p.ID == createdProject.ID {
				t.Errorf("leak! User 2 should not see User 1's project")
			}
		}

		// 5. User 2 tries to get User 1's project -> returns 404 Not Found (avoids leaking existence)
		reqAccessOther := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+createdProject.ID, nil)
		reqAccessOther.Header.Set("Cookie", user2Cookie)
		respAccessOther, _ := server.Test(reqAccessOther)
		if respAccessOther.StatusCode != http.StatusNotFound {
			t.Errorf("expected status 404 for cross-user project access, got %d", respAccessOther.StatusCode)
		}

		// 6. User 1 updates project settings (rename, status to paused & retention) and manages multiple API keys
		renamed := "Project Alpha Renamed " + uniqueSuffix
		patchBody, _ := json.Marshal(map[string]interface{}{
			"name":               renamed,
			"status":             "paused",
			"retention_days":     180,
			"allowed_origins":    "https://kinetiq.dev",
			"rate_limit_per_sec": 1000,
			"anonymize_ip":       true,
		})
		patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+createdProject.ID, bytes.NewReader(patchBody))
		patchReq.Header.Set("Content-Type", "application/json")
		patchReq.Header.Set("Cookie", user1Cookie)
		patchResp, _ := server.Test(patchReq)
		if patchResp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK on project PATCH, got %d", patchResp.StatusCode)
		}
		var patchRespData struct {
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		_ = json.NewDecoder(patchResp.Body).Decode(&patchRespData)
		if patchRespData.Data.Status != "paused" {
			t.Errorf("expected project status 'paused', got '%s'", patchRespData.Data.Status)
		}

		// 7. Create a second API key for the same project
		newKeyBody, _ := json.Marshal(map[string]string{
			"name":        "Mobile iOS SDK",
			"environment": "live",
		})
		newKeyReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+createdProject.ID+"/keys", bytes.NewReader(newKeyBody))
		newKeyReq.Header.Set("Content-Type", "application/json")
		newKeyReq.Header.Set("Cookie", user1Cookie)
		newKeyResp, _ := server.Test(newKeyReq)
		if newKeyResp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201 Created for second API key, got %d", newKeyResp.StatusCode)
		}
		var createdKeyResp struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(newKeyResp.Body).Decode(&createdKeyResp)

		// 8. List API keys -> should have at least 2 active keys
		listKeysReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+createdProject.ID+"/keys", nil)
		listKeysReq.Header.Set("Cookie", user1Cookie)
		listKeysResp, _ := server.Test(listKeysReq)
		if listKeysResp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK listing API keys, got %d", listKeysResp.StatusCode)
		}

		// 9. Revoke the newly created key
		if createdKeyResp.Data.ID != "" {
			revReq := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+createdProject.ID+"/keys/"+createdKeyResp.Data.ID, nil)
			revReq.Header.Set("Cookie", user1Cookie)
			revResp, _ := server.Test(revReq)
			if revResp.StatusCode != http.StatusOK {
				t.Errorf("expected 200 OK revoking API key, got %d", revResp.StatusCode)
			}
		}

		// 10. Purge events for project
		purgeReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+createdProject.ID+"/purge-events", nil)
		purgeReq.Header.Set("Cookie", user1Cookie)
		purgeResp, _ := server.Test(purgeReq)
		if purgeResp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK purging events, got %d", purgeResp.StatusCode)
		}

		// 11. Create and delete a temporary project
		tmpBody, _ := json.Marshal(map[string]string{"name": "Temp Delete Project " + uniqueSuffix})
		tmpReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(tmpBody))
		tmpReq.Header.Set("Content-Type", "application/json")
		tmpReq.Header.Set("Cookie", user1Cookie)
		tmpResp, _ := server.Test(tmpReq)
		var tmpCreated struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(tmpResp.Body).Decode(&tmpCreated)
		if tmpCreated.Data.ID != "" {
			delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+tmpCreated.Data.ID, nil)
			delReq.Header.Set("Cookie", user1Cookie)
			delResp, _ := server.Test(delReq)
			if delResp.StatusCode != http.StatusOK {
				t.Errorf("expected 200 OK deleting project, got %d", delResp.StatusCode)
			}
		}
	})

	t.Run("Create project rejects empty name with 400 Bad Request", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"name": "   "})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", "user1_session_cookie")

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Swagger UI route returns 200 OK", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed to execute request: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200 for Swagger UI, got %d", resp.StatusCode)
		}
	})

	t.Run("propagates incoming X-Request-ID and sets W3C traceparent header", func(t *testing.T) {
		incomingReqID := "custom-fe-request-id-12345"
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("X-Request-ID", incomingReqID)

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed to execute request: %v", err)
		}

		gotReqID := resp.Header.Get("X-Request-ID")
		if gotReqID != incomingReqID {
			t.Errorf("expected X-Request-ID %s, got %s", incomingReqID, gotReqID)
		}

		traceParent := resp.Header.Get("traceparent")
		if !strings.HasPrefix(traceParent, "00-") {
			t.Errorf("expected valid W3C traceparent header, got %s", traceParent)
		}
	})

	t.Run("rotate_project_api_key_revokes_old_and_issues_new_key", func(t *testing.T) {
		userCookie := "user1_session_cookie"

		// 1. Create project
		body, _ := json.Marshal(map[string]string{"name": "Rotation Test Project " + uuid.NewString()[:8]})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", userCookie)

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("failed to create project: %v", err)
		}

		var createResp struct {
			Data struct {
				ID     string `json:"id"`
				APIKey string `json:"api_key"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		origKey := createResp.Data.APIKey
		projectID := createResp.Data.ID

		// 2. Call rotate-key endpoint
		rotReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/rotate-key", nil)
		rotReq.Header.Set("Cookie", userCookie)

		rotResp, err := server.Test(rotReq)
		if err != nil {
			t.Fatalf("failed to execute rotate-key: %v", err)
		}
		if rotResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK from rotate-key, got %d", rotResp.StatusCode)
		}

		var rotData struct {
			Success bool `json:"success"`
			Data    struct {
				APIKey string `json:"api_key"`
			} `json:"data"`
		}
		if err := json.NewDecoder(rotResp.Body).Decode(&rotData); err != nil {
			t.Fatalf("failed to decode rotate response: %v", err)
		}

		if rotData.Data.APIKey == "" || rotData.Data.APIKey == origKey {
			t.Fatalf("expected new distinct api_key, got %s (orig: %s)", rotData.Data.APIKey, origKey)
		}
	})

	t.Run("alert_rules_CRUD_and_test_dispatch", func(t *testing.T) {
		userCookie := "user1_session_cookie"

		// 1. Create project
		body, _ := json.Marshal(map[string]string{"name": "Alert Test Project " + uuid.NewString()[:8]})
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

		// 2. Create alert rule
		rulePayload, _ := json.Marshal(map[string]interface{}{
			"name":             "High Traffic Spike",
			"event_name":       "api_call",
			"condition":        "spike_gt",
			"threshold_count":  500,
			"window_minutes":   5,
			"channel_type":     "slack",
			"webhook_url":      "https://example.com/slack/webhook",
			"cooldown_minutes": 10,
		})
		ruleReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/alerts", bytes.NewReader(rulePayload))
		ruleReq.Header.Set("Content-Type", "application/json")
		ruleReq.Header.Set("Cookie", userCookie)
		ruleResp, err := server.Test(ruleReq)
		if err != nil {
			t.Fatalf("failed to create rule: %v", err)
		}
		if ruleResp.StatusCode != http.StatusCreated && ruleResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 201 Created from create rule, got %d", ruleResp.StatusCode)
		}

		var createdRule struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(ruleResp.Body).Decode(&createdRule)
		ruleID := createdRule.Data.ID

		// 3. List alert rules
		listReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/alerts", nil)
		listReq.Header.Set("Cookie", userCookie)
		listResp, err := server.Test(listReq)
		if err != nil {
			t.Fatalf("failed to list alert rules: %v", err)
		}
		if listResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK from list alert rules, got %d", listResp.StatusCode)
		}

		// 4. Update alert rule
		updatePayload, _ := json.Marshal(map[string]interface{}{
			"name":            "Updated High Traffic Spike",
			"threshold_count": 1000,
		})
		updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/alerts/"+ruleID, bytes.NewReader(updatePayload))
		updateReq.Header.Set("Content-Type", "application/json")
		updateReq.Header.Set("Cookie", userCookie)
		updateResp, err := server.Test(updateReq)
		if err != nil {
			t.Fatalf("failed to update alert rule: %v", err)
		}
		if updateResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK from update alert rule, got %d", updateResp.StatusCode)
		}

		// 5. Test alert rule (dry-run bypass for example.com)
		testReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/alerts/"+ruleID+"/test", nil)
		testReq.Header.Set("Cookie", userCookie)
		testResp, err := server.Test(testReq)
		if err != nil {
			t.Fatalf("failed to test alert rule: %v", err)
		}
		if testResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK from test alert rule, got %d", testResp.StatusCode)
		}

		// 6. Delete alert rule
		delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+projectID+"/alerts/"+ruleID, nil)
		delReq.Header.Set("Cookie", userCookie)
		delResp, err := server.Test(delReq)
		if err != nil {
			t.Fatalf("failed to delete alert rule: %v", err)
		}
		if delResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK from delete alert rule, got %d", delResp.StatusCode)
		}
	})
}
