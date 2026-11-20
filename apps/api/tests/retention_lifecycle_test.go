package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"kinetiq/internal/compliance"
	"kinetiq/internal/importer"
	"kinetiq/internal/retention"
)

func TestDataLifecycle_Retention_GDPR_Importer(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(container)
	queries := db.New(container.DBPool)

	// Setup a test user and project
	userID := "user-1-id" // matches user1_session_cookie mock identity
	_, err = container.DBPool.Exec(ctx, `
		INSERT INTO "user" (id, name, email, "emailVerified", role, "twoFactorEnabled", banned, "isPlatformStaff", "createdAt", "updatedAt")
		VALUES ($1, 'Lifecycle Owner', $2, true, 'owner', false, false, false, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING;
	`, userID, "lifecycle-"+uuid.New().String()[:8]+"@example.com")
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	projectID := uuid.New()
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: userID,
		Name:        "Data Lifecycle Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// 1. Test Retention Policy GET & PUT
	t.Run("Retention Policy GET and PUT endpoints", func(t *testing.T) {
		// GET default retention (default is 90 days)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/retention", projectID), nil)
		req.Header.Set("Cookie", "user1_session_cookie")
		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("GET /retention failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var polResp struct {
			Success bool             `json:"success"`
			Data    retention.Policy `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&polResp)
		if polResp.Data.RetentionDays != 90 {
			t.Errorf("expected 90 days default, got %d", polResp.Data.RetentionDays)
		}

		// PUT to update retention to 30 days
		putBody, _ := json.Marshal(map[string]int{"retention_days": 30})
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/projects/%s/retention", projectID), bytes.NewReader(putBody))
		putReq.Header.Set("Content-Type", "application/json")
		putReq.Header.Set("Cookie", "user1_session_cookie")
		putResp, err := server.Test(putReq)
		if err != nil {
			t.Fatalf("PUT /retention failed: %v", err)
		}
		if putResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", putResp.StatusCode)
		}

		json.NewDecoder(putResp.Body).Decode(&polResp)
		if polResp.Data.RetentionDays != 30 {
			t.Errorf("expected 30 days after update, got %d", polResp.Data.RetentionDays)
		}

		// Invalid project ID & invalid body validation
		badGetReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/not-a-uuid/retention", nil)
		badGetReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(badGetReq); err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid project ID on GET, got status=%v err=%v", resp, err)
		}

		badPutIDReq := httptest.NewRequest(http.MethodPut, "/api/v1/projects/not-a-uuid/retention", bytes.NewReader(putBody))
		badPutIDReq.Header.Set("Content-Type", "application/json")
		badPutIDReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(badPutIDReq); err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid project ID on PUT, got status=%v err=%v", resp, err)
		}

		badPutBodyReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/projects/%s/retention", projectID), strings.NewReader("{bad-json"))
		badPutBodyReq.Header.Set("Content-Type", "application/json")
		badPutBodyReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(badPutBodyReq); err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid JSON body on PUT, got status=%v err=%v", resp, err)
		}

		// Service validation for uuid.Nil and non-existent project
		if _, err := container.RetentionService.GetPolicy(ctx, uuid.Nil); err == nil {
			t.Errorf("expected error for nil project ID in GetPolicy")
		}
		if _, err := container.RetentionService.UpdatePolicy(ctx, uuid.Nil, 10); err == nil {
			t.Errorf("expected error for nil project ID in UpdatePolicy")
		}
		missingID := uuid.New()
		missingGetReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/retention", missingID), nil)
		missingGetReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(missingGetReq); err != nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for non-existent project on GET, got status=%v err=%v", resp, err)
		}
		missingPutReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/projects/%s/retention", missingID), bytes.NewReader(putBody))
		missingPutReq.Header.Set("Content-Type", "application/json")
		missingPutReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(missingPutReq); err != nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for non-existent project on PUT, got status=%v err=%v", resp, err)
		}
	})

	// 2. Seed events for retention preview and purge test
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		occurred := now.AddDate(0, 0, -45) // 45 days ago -> expired under 30 days policy
		_, err := container.DBPool.Exec(ctx, `
			INSERT INTO events (id, project_id, event_id, event_name, occurred_at, distinct_id, properties)
			VALUES ($1, $2, $3, 'old_event', $4, 'user_old_1', '{"test":true}'::jsonb)
		`, uuid.New(), projectID, fmt.Sprintf("evt-old-%d", i), occurred)
		if err != nil {
			t.Fatalf("failed to insert expired event: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		occurred := now.AddDate(0, 0, -5) // 5 days ago -> fresh
		_, err := container.DBPool.Exec(ctx, `
			INSERT INTO events (id, project_id, event_id, event_name, occurred_at, distinct_id, properties)
			VALUES ($1, $2, $3, 'fresh_event', $4, 'user_fresh_1', '{"test":true}'::jsonb)
		`, uuid.New(), projectID, fmt.Sprintf("evt-fresh-%d", i), occurred)
		if err != nil {
			t.Fatalf("failed to insert fresh event: %v", err)
		}
	}

	t.Run("Retention Preview and Purge endpoints", func(t *testing.T) {
		// Preview
		prevReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/retention/preview", projectID), nil)
		prevReq.Header.Set("Cookie", "user1_session_cookie")
		prevResp, err := server.Test(prevReq)
		if err != nil {
			t.Fatalf("GET /retention/preview failed: %v", err)
		}
		if prevResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", prevResp.StatusCode)
		}

		var prevResult struct {
			Data retention.PreviewResult `json:"data"`
		}
		json.NewDecoder(prevResp.Body).Decode(&prevResult)
		if prevResult.Data.AffectedEvents != 5 {
			t.Errorf("expected 5 affected events in preview, got %d", prevResult.Data.AffectedEvents)
		}

		// Run Purge
		runReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/retention/run", projectID), nil)
		runReq.Header.Set("Cookie", "user1_session_cookie")
		runResp, err := server.Test(runReq)
		if err != nil {
			t.Fatalf("POST /retention/run failed: %v", err)
		}
		if runResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", runResp.StatusCode)
		}

		var purgeResult struct {
			Data retention.PurgeResult `json:"data"`
		}
		json.NewDecoder(runResp.Body).Decode(&purgeResult)
		if purgeResult.Data.DeletedEvents != 5 {
			t.Errorf("expected 5 deleted events, got %d", purgeResult.Data.DeletedEvents)
		}

		// Verify sweep all runs without error
		swept, err := container.RetentionService.SweepAll(ctx)
		if err != nil {
			t.Fatalf("SweepAll failed: %v", err)
		}
		_ = swept

		// Invalid project IDs and unlimited retention branch (-5 normalizes to 0)
		badPrevReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/bad-uuid/retention/preview", nil)
		badPrevReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(badPrevReq); err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid project ID on preview, got status=%v err=%v", resp, err)
		}
		badRunReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/bad-uuid/retention/run", nil)
		badRunReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(badRunReq); err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid project ID on run, got status=%v err=%v", resp, err)
		}

		missingID := uuid.New()
		missPrevReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/retention/preview", missingID), nil)
		missPrevReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(missPrevReq); err != nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for missing project on preview, got status=%v err=%v", resp, err)
		}
		missRunReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/retention/run", missingID), nil)
		missRunReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(missRunReq); err != nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for missing project on run, got status=%v err=%v", resp, err)
		}

		// Set unlimited retention (negative normalizes to 0) and verify Preview & Purge return 0
		unlimPol, err := container.RetentionService.UpdatePolicy(ctx, projectID, -5)
		if err != nil || unlimPol.RetentionDays != 0 || unlimPol.Enabled {
			t.Errorf("expected normalized unlimited policy (0, enabled=false), got %+v err=%v", unlimPol, err)
		}
		unlimPrev, err := container.RetentionService.Preview(ctx, projectID)
		if err != nil || unlimPrev.AffectedEvents != 0 {
			t.Errorf("expected 0 affected events for unlimited retention preview, got %+v err=%v", unlimPrev, err)
		}
		unlimPurge, err := container.RetentionService.Purge(ctx, projectID)
		if err != nil || unlimPurge.DeletedEvents != 0 {
			t.Errorf("expected 0 deleted events for unlimited retention purge, got %+v err=%v", unlimPurge, err)
		}
	})

	// 3. Test GDPR User Deletion & Pseudonymization
	t.Run("GDPR User Deletion & Pseudonymization", func(t *testing.T) {
		// Seed user data to delete
		for i := 0; i < 4; i++ {
			_, err := container.DBPool.Exec(ctx, `
				INSERT INTO events (id, project_id, event_id, event_name, occurred_at, distinct_id, properties)
				VALUES ($1, $2, $3, 'user_action', NOW(), 'gdpr_target_user', '{"email":"gdpr@target.com","name":"Target"}'::jsonb)
			`, uuid.New(), projectID, fmt.Sprintf("gdpr-evt-%d", i))
			if err != nil {
				t.Fatalf("failed to insert target event: %v", err)
			}
		}

		// Pseudonymize 2 of them or test pseudonymize mode first
		pseudoReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s/compliance/users/gdpr_target_user?mode=pseudonymize", projectID), nil)
		pseudoReq.Header.Set("Cookie", "user1_session_cookie")
		pseudoResp, err := server.Test(pseudoReq)
		if err != nil {
			t.Fatalf("pseudonymize request failed: %v", err)
		}
		if pseudoResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", pseudoResp.StatusCode)
		}

		var pseudoRes struct {
			Data struct {
				AffectedRows int64  `json:"affected_rows"`
				Mode         string `json:"mode"`
				PseudonymID  string `json:"pseudonym_id"`
			} `json:"data"`
		}
		json.NewDecoder(pseudoResp.Body).Decode(&pseudoRes)
		if pseudoRes.Data.AffectedRows != 4 || pseudoRes.Data.PseudonymID == "" {
			t.Errorf("expected 4 pseudonymized rows with pseudonym ID, got %+v", pseudoRes)
		}

		// Hard delete remaining
		delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s/compliance/users/%s?mode=delete", projectID, pseudoRes.Data.PseudonymID), nil)
		delReq.Header.Set("Cookie", "user1_session_cookie")
		delResp, err := server.Test(delReq)
		if err != nil {
			t.Fatalf("delete request failed: %v", err)
		}
		if delResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", delResp.StatusCode)
		}

		// Validation & RBAC error paths for GDPR endpoint
		badProjReq := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/not-a-uuid/compliance/users/u1", nil)
		badProjReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(badProjReq); err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid project ID, got status=%v err=%v", resp, err)
		}

		forbiddenReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s/compliance/users/u1", projectID), nil)
		forbiddenReq.Header.Set("Cookie", "user2_session_cookie")
		if resp, err := server.Test(forbiddenReq); err != nil || resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 for non-admin member, got status=%v err=%v", resp, err)
		}

		badModeReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s/compliance/users/u1?mode=invalid", projectID), nil)
		badModeReq.Header.Set("Cookie", "user1_session_cookie")
		if resp, err := server.Test(badModeReq); err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid mode, got status=%v err=%v", resp, err)
		}

		// Direct service validation paths
		if _, err := container.AuditService.GDPRDelete(ctx, compliance.GDPRDeleteParams{ProjectID: uuid.Nil, DistinctID: "u1"}); err == nil {
			t.Errorf("expected error for nil project ID")
		}
		if _, err := container.AuditService.GDPRDelete(ctx, compliance.GDPRDeleteParams{ProjectID: projectID, DistinctID: ""}); err == nil {
			t.Errorf("expected error for empty distinct ID")
		}
		if _, err := container.AuditService.GDPRDelete(ctx, compliance.GDPRDeleteParams{ProjectID: projectID, DistinctID: "u1", Mode: "invalid"}); err == nil {
			t.Errorf("expected error for unsupported mode")
		}
	})

	// 4. Test Importer Engine with PostHog and Mixpanel streams
	t.Run("Importer Engine with PostHog and Mixpanel", func(t *testing.T) {
		phData := `
{"event":"$pageview","distinct_id":"imp_u1","timestamp":"2026-10-25T12:00:00Z","uuid":"ph-imp-1","properties":{"url":"/docs"}}
{"event":"click","distinct_id":"imp_u1","timestamp":"2026-10-25T12:01:00Z","uuid":"ph-imp-2","properties":{"cta":"trial"}}
`
		imp := importer.NewImporter(container.DBPool)
		phStats, err := imp.ImportStream(ctx, "posthog", strings.NewReader(phData), importer.ImportOptions{
			ProjectID: projectID,
			DryRun:    false,
			BatchSize: 10,
		}, nil)
		if err != nil {
			t.Fatalf("PostHog import failed: %v", err)
		}
		if phStats.Imported != 2 {
			t.Errorf("expected 2 imported PostHog events, got %d", phStats.Imported)
		}

		// Re-importing same PostHog events should be deduplicated (idempotent)
		phStatsDedup, err := imp.ImportStream(ctx, "posthog", strings.NewReader(phData), importer.ImportOptions{
			ProjectID: projectID,
			DryRun:    false,
			BatchSize: 10,
		}, nil)
		if err != nil {
			t.Fatalf("PostHog re-import failed: %v", err)
		}
		if phStatsDedup.Skipped != 2 {
			t.Errorf("expected 2 skipped (deduplicated) events, got %d", phStatsDedup.Skipped)
		}

		// Mixpanel stream
		mpData := `
{"event":"Sign Up","properties":{"time":1729857600,"distinct_id":"mp_imp_1","$insert_id":"mp-imp-1","plan":"team"}}
{"event":"Upgrade","properties":{"time":1729858000,"distinct_id":"mp_imp_1","$insert_id":"mp-imp-2","plan":"enterprise"}}
`
		mpStats, err := imp.ImportStream(ctx, "mixpanel", strings.NewReader(mpData), importer.ImportOptions{
			ProjectID: projectID,
			DryRun:    false,
			BatchSize: 10,
		}, nil)
		if err != nil {
			t.Fatalf("Mixpanel import failed: %v", err)
		}
		if mpStats.Imported != 2 {
			t.Errorf("expected 2 imported Mixpanel events, got %d", mpStats.Imported)
		}

		// Edge cases: unsupported source, dry-run with progress callback, malformed lines, fallback IDs & timestamps
		if _, err := imp.ImportStream(ctx, "amplitude", strings.NewReader(""), importer.ImportOptions{ProjectID: projectID}, nil); err == nil {
			t.Errorf("expected error for unsupported source")
		}

		edgePH := `
not-json
{"event":"","distinct_id":"u1"}
{"event":"no_uuid_no_ts","distinct_id":"u2","id":"fallback-id-1"}
{"event":"hash_id_event","distinct_id":"u3","timestamp":"1729857600000"}
{"event":"float_ms_ts","distinct_id":"u4","timestamp":1729857600123}
{"event":"rfc_nano","distinct_id":"u5","timestamp":"2026-10-25T12:00:00.123456Z"}
`
		var progressCalled bool
		dryStats, err := imp.ImportStream(ctx, "posthog", strings.NewReader(edgePH), importer.ImportOptions{
			ProjectID: projectID,
			DryRun:    true,
			BatchSize: 2,
		}, func(read int64) {
			progressCalled = true
		})
		if err != nil {
			t.Fatalf("PostHog dry-run edge case import failed: %v", err)
		}
		if dryStats.Imported != 4 || dryStats.ErrorCount != 2 || !progressCalled {
			t.Errorf("unexpected dry-run stats: %+v progressCalled=%v", dryStats, progressCalled)
		}

		edgeMP := `
not-json
{"event":""}
{"event":"NoProps"}
{"event":"BadTime","properties":{"time":"not-a-time","distinct_id":"mp_u2"}}
`
		mpEdgeStats, err := imp.ImportStream(ctx, "mixpanel", strings.NewReader(edgeMP), importer.ImportOptions{
			ProjectID: projectID,
			DryRun:    true,
			BatchSize: 0, // tests default batch size
		}, nil)
		if err != nil {
			t.Fatalf("Mixpanel dry-run edge case import failed: %v", err)
		}
		if mpEdgeStats.Imported != 2 || mpEdgeStats.ErrorCount != 2 {
			t.Errorf("unexpected Mixpanel edge stats: %+v", mpEdgeStats)
		}

		if _, err := importer.NormalizeProjectID(""); err == nil {
			t.Errorf("expected error for empty project ID")
		}
		if parsed, err := importer.NormalizeProjectID(projectID.String()); err != nil || parsed != projectID {
			t.Errorf("expected valid UUID from NormalizeProjectID, got %v err=%v", parsed, err)
		}
	})
}

// Regression for #159: cross-tenant IDOR on retention + GDPR deletion endpoints.
func TestRetentionCompliance_CrossTenantIDOR(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(container)
	queries := db.New(container.DBPool)

	// Tenant B (user-2-id) owns the victim project; Tenant A (user-1-id, role "owner") is the attacker.
	victimID := uuid.New()
	if _, err := queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: victimID, Valid: true},
		OwnerUserID: "user-2-id",
		Name:        "IDOR Victim " + victimID.String()[:8],
		Status:      "active",
	}); err != nil {
		t.Fatalf("failed to create victim project: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := container.DBPool.Exec(ctx, `
			INSERT INTO events (id, project_id, event_id, event_name, occurred_at, distinct_id, properties)
			VALUES ($1, $2, $3, 'old_event', NOW() - INTERVAL '400 days', 'victim_user', '{}'::jsonb)
		`, uuid.New(), victimID, fmt.Sprintf("idor-%s-%d", victimID.String()[:8], i)); err != nil {
			t.Fatalf("failed to seed event: %v", err)
		}
	}
	// Victim policy: 30 days
	if _, err := container.DBPool.Exec(ctx, `UPDATE projects SET retention_days = 30 WHERE id = $1`, victimID); err != nil {
		t.Fatalf("failed to set retention: %v", err)
	}

	do := func(method, path, cookie string, body []byte) int {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", method, path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	denied := func(code int) bool { return code == http.StatusForbidden || code == http.StatusNotFound }

	base := fmt.Sprintf("/api/v1/projects/%s", victimID)
	putBody, _ := json.Marshal(map[string]int{"retention_days": 1})

	cases := []struct {
		name, method, path string
		body               []byte
	}{
		{"GET retention", http.MethodGet, base + "/retention", nil},
		{"PUT retention", http.MethodPut, base + "/retention", putBody},
		{"GET preview", http.MethodGet, base + "/retention/preview", nil},
		{"POST run", http.MethodPost, base + "/retention/run", nil},
		{"DELETE GDPR delete", http.MethodDelete, base + "/compliance/users/victim_user?mode=delete", nil},
		{"DELETE GDPR pseudonymize", http.MethodDelete, base + "/compliance/users/victim_user?mode=pseudonymize", nil},
	}
	for _, tc := range cases {
		t.Run("tenant A cannot "+tc.name, func(t *testing.T) {
			if code := do(tc.method, tc.path, "user1_session_cookie", tc.body); !denied(code) {
				t.Errorf("expected 403/404 for cross-tenant %s, got %d", tc.name, code)
			}
		})
	}

	t.Run("victim data untouched", func(t *testing.T) {
		var events, days int
		if err := container.DBPool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE project_id = $1 AND distinct_id = 'victim_user'`, victimID).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if events != 3 {
			t.Errorf("expected 3 victim events intact, got %d", events)
		}
		if err := container.DBPool.QueryRow(ctx, `SELECT retention_days FROM projects WHERE id = $1`, victimID).Scan(&days); err != nil {
			t.Fatal(err)
		}
		if days != 30 {
			t.Errorf("expected victim retention_days 30 unchanged, got %d", days)
		}
	})

	t.Run("owner still allowed", func(t *testing.T) {
		if code := do(http.MethodGet, base+"/retention", "user2_session_cookie", nil); code != http.StatusOK {
			t.Errorf("expected 200 for owner GET retention, got %d", code)
		}
	})
}
