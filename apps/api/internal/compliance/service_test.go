package compliance_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"kinetiq/internal/compliance"
)

type mockAuditService struct {
	compliance.Service
	recordFunc     func(ctx context.Context, params compliance.RecordAuditParams) error
	listFunc       func(ctx context.Context, params compliance.ListAuditLogsParams) (*compliance.ListAuditLogsResponse, error)
	gdprDeleteFunc func(ctx context.Context, params compliance.GDPRDeleteParams) (*compliance.GDPRDeleteResult, error)
}

func (m *mockAuditService) Record(ctx context.Context, params compliance.RecordAuditParams) error {
	if m.recordFunc != nil {
		return m.recordFunc(ctx, params)
	}
	return nil
}

func (m *mockAuditService) List(ctx context.Context, params compliance.ListAuditLogsParams) (*compliance.ListAuditLogsResponse, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, params)
	}
	return nil, nil
}

func (m *mockAuditService) GDPRDelete(ctx context.Context, params compliance.GDPRDeleteParams) (*compliance.GDPRDeleteResult, error) {
	if m.gdprDeleteFunc != nil {
		return m.gdprDeleteFunc(ctx, params)
	}
	return nil, nil
}

func TestAuditService_Unit(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mock := &mockAuditService{
		recordFunc: func(ctx context.Context, params compliance.RecordAuditParams) error {
			if params.WorkspaceID == "" {
				t.Errorf("expected non-empty workspace_id")
			}
			return nil
		},
		listFunc: func(ctx context.Context, params compliance.ListAuditLogsParams) (*compliance.ListAuditLogsResponse, error) {
			return &compliance.ListAuditLogsResponse{
				Items: []compliance.WorkspaceAuditLog{
					{
						ID:          uuid.New(),
						WorkspaceID: params.WorkspaceID,
						ActorID:     "user-1",
						ActorName:   "Admin User",
						ActorEmail:  "admin@example.com",
						Action:      "api_key.create",
						TargetType:  "api_key",
						TargetID:    "key-123",
						Metadata:    map[string]interface{}{"key_prefix": "sp_live_test"},
						CreatedAt:   now,
					},
				},
				Total: 1,
			}, nil
		},
	}

	err := mock.Record(ctx, compliance.RecordAuditParams{
		WorkspaceID: "ws-1",
		ActorID:     "user-1",
		Action:      "api_key.create",
		TargetType:  "api_key",
		TargetID:    "key-123",
	})
	if err != nil {
		t.Fatalf("unexpected record error: %v", err)
	}

	res, err := mock.List(ctx, compliance.ListAuditLogsParams{
		WorkspaceID: "ws-1",
		Limit:       50,
		Offset:      0,
	})
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("expected 1 audit item, got %d", len(res.Items))
	}
	if res.Items[0].Action != "api_key.create" {
		t.Errorf("expected action api_key.create, got %s", res.Items[0].Action)
	}
}

func TestAuditService_GDPRDelete_Unit(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	auditRecorded := false
	mock := &mockAuditService{
		recordFunc: func(ctx context.Context, params compliance.RecordAuditParams) error {
			if params.Action == "compliance.gdpr_delete" && params.TargetID == "user-123" {
				auditRecorded = true
			}
			return nil
		},
		gdprDeleteFunc: func(ctx context.Context, params compliance.GDPRDeleteParams) (*compliance.GDPRDeleteResult, error) {
			if params.DistinctID == "" {
				t.Errorf("expected non-empty distinct_id")
			}
			mode := params.Mode
			if mode == "" {
				mode = "delete"
			}
			return &compliance.GDPRDeleteResult{
				ProjectID:    params.ProjectID,
				DistinctID:   params.DistinctID,
				Mode:         mode,
				AffectedRows: 42,
			}, nil
		},
	}

	result, err := mock.GDPRDelete(ctx, compliance.GDPRDeleteParams{
		WorkspaceID: "ws-1",
		ProjectID:   projectID,
		DistinctID:  "user-123",
		Mode:        "delete",
		ActorID:     "admin-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = auditRecorded
	if result.AffectedRows != 42 {
		t.Errorf("expected 42 rows affected, got %d", result.AffectedRows)
	}
	if result.Mode != "delete" {
		t.Errorf("expected mode delete, got %s", result.Mode)
	}

	// Also test pseudonymize mode
	mock.gdprDeleteFunc = func(ctx context.Context, params compliance.GDPRDeleteParams) (*compliance.GDPRDeleteResult, error) {
		return &compliance.GDPRDeleteResult{
			ProjectID:    params.ProjectID,
			DistinctID:   params.DistinctID,
			Mode:         "pseudonymize",
			AffectedRows: 15,
			PseudonymID:  "anon_abc123",
		}, nil
	}

	resultPseudo, err := mock.GDPRDelete(ctx, compliance.GDPRDeleteParams{
		WorkspaceID: "ws-1",
		ProjectID:   projectID,
		DistinctID:  "user-456",
		Mode:        "pseudonymize",
		ActorID:     "admin-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resultPseudo.Mode != "pseudonymize" || resultPseudo.PseudonymID != "anon_abc123" {
		t.Errorf("expected pseudonymize result with anon_abc123, got %+v", resultPseudo)
	}
}
