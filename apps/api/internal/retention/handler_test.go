package retention_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"kinetiq/internal/retention"
)

func TestRetentionHandler_Endpoints(t *testing.T) {
	projectID := uuid.New()
	mock := &mockRetentionService{
		getPolicyFunc: func(ctx context.Context, pID uuid.UUID) (*retention.Policy, error) {
			return &retention.Policy{
				ProjectID:     pID,
				RetentionDays: 90,
				Enabled:       true,
			}, nil
		},
		updatePolicyFunc: func(ctx context.Context, pID uuid.UUID, days int) (*retention.Policy, error) {
			return &retention.Policy{
				ProjectID:     pID,
				RetentionDays: days,
				Enabled:       days > 0,
			}, nil
		},
		previewFunc: func(ctx context.Context, pID uuid.UUID) (*retention.PreviewResult, error) {
			cutoff := time.Now().AddDate(0, 0, -90)
			return &retention.PreviewResult{
				ProjectID:      pID,
				RetentionDays:  90,
				CutoffTime:     &cutoff,
				AffectedEvents: 100,
				Message:        "100 events eligible",
			}, nil
		},
		purgeFunc: func(ctx context.Context, pID uuid.UUID) (*retention.PurgeResult, error) {
			return &retention.PurgeResult{
				ProjectID:     pID,
				RetentionDays: 90,
				DeletedEvents: 100,
				DurationMs:    10,
				Timestamp:     time.Now().UTC(),
			}, nil
		},
	}

	handler := retention.NewHandler(mock, func(context.Context, string, uuid.UUID) (string, error) { return "owner", nil }, nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("userID", "owner-id"); return c.Next() })
	app.Get("/api/v1/projects/:projectID/retention", handler.GetPolicy)
	app.Put("/api/v1/projects/:projectID/retention", handler.UpdatePolicy)
	app.Get("/api/v1/projects/:projectID/retention/preview", handler.Preview)
	app.Post("/api/v1/projects/:projectID/retention/run", handler.RunPurge)

	// 1. GET Policy
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/retention", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// 2. PUT Policy
	body, _ := json.Marshal(map[string]interface{}{"retention_days": 60})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID.String()+"/retention", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// 3. GET Preview
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/retention/preview", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	respBytes, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(respBytes, []byte("affected_events")) {
		t.Errorf("expected response to contain affected_events, got %s", string(respBytes))
	}

	// 4. POST Run
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/retention/run", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRetentionHandler_CrossTenantDenied(t *testing.T) {
	called := false
	mock := &mockRetentionService{
		getPolicyFunc:    func(context.Context, uuid.UUID) (*retention.Policy, error) { called = true; return nil, nil },
		updatePolicyFunc: func(context.Context, uuid.UUID, int) (*retention.Policy, error) { called = true; return nil, nil },
		previewFunc:      func(context.Context, uuid.UUID) (*retention.PreviewResult, error) { called = true; return nil, nil },
		purgeFunc:        func(context.Context, uuid.UUID) (*retention.PurgeResult, error) { called = true; return nil, nil },
	}
	// Caller has no access to any project.
	noAccess := func(context.Context, string, uuid.UUID) (string, error) { return "", errors.New("not found") }
	viewer := func(context.Context, string, uuid.UUID) (string, error) { return "viewer", nil }

	for _, tc := range []struct {
		name   string
		access retention.ProjectAccessFunc
		method string
		suffix string
		want   int
	}{
		{"foreign GET", noAccess, http.MethodGet, "", http.StatusNotFound},
		{"foreign PUT", noAccess, http.MethodPut, "", http.StatusNotFound},
		{"foreign preview", noAccess, http.MethodGet, "/preview", http.StatusNotFound},
		{"foreign run", noAccess, http.MethodPost, "/run", http.StatusNotFound},
		{"viewer PUT", viewer, http.MethodPut, "", http.StatusForbidden},
		{"viewer run", viewer, http.MethodPost, "/run", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := retention.NewHandler(mock, tc.access, nil)
			app := fiber.New()
			app.Use(func(c *fiber.Ctx) error { c.Locals("userID", "attacker"); return c.Next() })
			app.Get("/p/:projectID/retention", h.GetPolicy)
			app.Put("/p/:projectID/retention", h.UpdatePolicy)
			app.Get("/p/:projectID/retention/preview", h.Preview)
			app.Post("/p/:projectID/retention/run", h.RunPurge)
			req := httptest.NewRequest(tc.method, "/p/"+uuid.NewString()+"/retention"+tc.suffix, bytes.NewReader([]byte(`{"retention_days":1}`)))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.want {
				t.Errorf("expected %d, got %d", tc.want, resp.StatusCode)
			}
		})
	}
	if called {
		t.Error("service must not be invoked for unauthorized callers")
	}
}
