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

	"kinetiq/internal/app"
)

func TestWorkspaceAudit_RecordAndQuery(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	container, err := app.NewContainer(context.Background())
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))

	t.Run("creating project and API key records audit logs", func(t *testing.T) {
		// 1. Create a project
		projName := "Compliance Proj " + uuid.NewString()[:8]
		projBody, _ := json.Marshal(map[string]string{"name": projName})
		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(projBody))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.Header.Set("Cookie", "user1_session_cookie")

		createResp, err := server.Test(createReq)
		if err != nil {
			t.Fatalf("failed to create project: %v", err)
		}
		if createResp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d", createResp.StatusCode)
		}

		var projResult struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		json.NewDecoder(createResp.Body).Decode(&projResult)
		projectID := projResult.Data.ID

		// 2. Create an additional API key
		keyBody, _ := json.Marshal(map[string]string{"name": "Audit Key Test", "environment": "live"})
		keyReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/keys", projectID), bytes.NewReader(keyBody))
		keyReq.Header.Set("Content-Type", "application/json")
		keyReq.Header.Set("Cookie", "user1_session_cookie")
		keyResp, err := server.Test(keyReq)
		if err != nil {
			t.Fatalf("failed to create key: %v", err)
		}
		if keyResp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d", keyResp.StatusCode)
		}

		var keyResult struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		json.NewDecoder(keyResp.Body).Decode(&keyResult)
		keyID := keyResult.Data.ID

		// 3. Revoke the API key
		delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s/keys/%s", projectID, keyID), nil)
		delReq.Header.Set("Cookie", "user1_session_cookie")
		delResp, err := server.Test(delReq)
		if err != nil {
			t.Fatalf("failed to revoke key: %v", err)
		}
		if delResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", delResp.StatusCode)
		}

		// 4. Query audit logs via GET /api/v1/workspace/audit-logs
		auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspace/audit-logs?workspace_id=user-1-id", nil)
		auditReq.Header.Set("Cookie", "user1_session_cookie")
		auditResp, err := server.Test(auditReq)
		if err != nil {
			t.Fatalf("failed to query audit logs: %v", err)
		}
		if auditResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", auditResp.StatusCode)
		}

		var auditResult struct {
			Success bool `json:"success"`
			Data    struct {
				Pagination struct {
					Total int64 `json:"total"`
				} `json:"pagination"`
				Items []struct {
					Action     string `json:"action"`
					TargetType string `json:"target_type"`
					TargetID   string `json:"target_id"`
				} `json:"items"`
			} `json:"data"`
		}
		json.NewDecoder(auditResp.Body).Decode(&auditResult)

		if auditResult.Data.Pagination.Total < 3 {
			t.Errorf("expected at least 3 audit log entries, got %d", auditResult.Data.Pagination.Total)
		}

		// Verify expected actions exist
		foundProjectCreate := false
		foundKeyCreate := false
		foundKeyRevoke := false

		for _, item := range auditResult.Data.Items {
			if item.Action == "project.create" {
				foundProjectCreate = true
			}
			if item.Action == "api_key.create" && item.TargetID == keyID {
				foundKeyCreate = true
			}
			if item.Action == "api_key.revoke" && item.TargetID == keyID {
				foundKeyRevoke = true
			}
		}

		if !foundProjectCreate || !foundKeyCreate || !foundKeyRevoke {
			t.Errorf("audit actions missing: project.create=%v, key.create=%v, key.revoke=%v", foundProjectCreate, foundKeyCreate, foundKeyRevoke)
		}
	})

	t.Run("cross-tenant / unauthorized audit access is forbidden (403)", func(t *testing.T) {
		// user2 attempting to query user1's workspace audit log with member role
		req := httptest.NewRequest(http.MethodGet, "/api/v1/workspace/audit-logs?workspace_id=user-1-id", nil)
		req.Header.Set("Cookie", "user2_session_cookie") // user2 has role "member"

		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for unauthorized audit view, got %d", resp.StatusCode)
		}
	})
}
