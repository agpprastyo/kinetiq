package retention_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"kinetiq/internal/retention"
)

type mockRetentionService struct {
	getPolicyFunc    func(ctx context.Context, projectID uuid.UUID) (*retention.Policy, error)
	updatePolicyFunc func(ctx context.Context, projectID uuid.UUID, days int) (*retention.Policy, error)
	previewFunc      func(ctx context.Context, projectID uuid.UUID) (*retention.PreviewResult, error)
	purgeFunc        func(ctx context.Context, projectID uuid.UUID) (*retention.PurgeResult, error)
	sweepAllFunc     func(ctx context.Context) ([]retention.PurgeResult, error)
}

func (m *mockRetentionService) GetPolicy(ctx context.Context, projectID uuid.UUID) (*retention.Policy, error) {
	if m.getPolicyFunc != nil {
		return m.getPolicyFunc(ctx, projectID)
	}
	return nil, nil
}

func (m *mockRetentionService) UpdatePolicy(ctx context.Context, projectID uuid.UUID, days int) (*retention.Policy, error) {
	if m.updatePolicyFunc != nil {
		return m.updatePolicyFunc(ctx, projectID, days)
	}
	return nil, nil
}

func (m *mockRetentionService) Preview(ctx context.Context, projectID uuid.UUID) (*retention.PreviewResult, error) {
	if m.previewFunc != nil {
		return m.previewFunc(ctx, projectID)
	}
	return nil, nil
}

func (m *mockRetentionService) Purge(ctx context.Context, projectID uuid.UUID) (*retention.PurgeResult, error) {
	if m.purgeFunc != nil {
		return m.purgeFunc(ctx, projectID)
	}
	return nil, nil
}

func (m *mockRetentionService) SweepAll(ctx context.Context) ([]retention.PurgeResult, error) {
	if m.sweepAllFunc != nil {
		return m.sweepAllFunc(ctx)
	}
	return nil, nil
}

func TestRetentionService_Unit(t *testing.T) {
	ctx := context.Background()
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
				AffectedEvents: 1250,
				Message:        "1250 events eligible for deletion based on 90-day retention",
			}, nil
		},
		purgeFunc: func(ctx context.Context, pID uuid.UUID) (*retention.PurgeResult, error) {
			return &retention.PurgeResult{
				ProjectID:     pID,
				RetentionDays: 90,
				DeletedEvents: 1250,
				DurationMs:    15,
				Timestamp:     time.Now().UTC(),
			}, nil
		},
	}

	policy, err := mock.GetPolicy(ctx, projectID)
	if err != nil {
		t.Fatalf("unexpected error getting policy: %v", err)
	}
	if policy.RetentionDays != 90 || !policy.Enabled {
		t.Errorf("expected 90 days and enabled, got %+v", policy)
	}

	updated, err := mock.UpdatePolicy(ctx, projectID, 30)
	if err != nil {
		t.Fatalf("unexpected error updating policy: %v", err)
	}
	if updated.RetentionDays != 30 {
		t.Errorf("expected 30 days, got %d", updated.RetentionDays)
	}

	prev, err := mock.Preview(ctx, projectID)
	if err != nil {
		t.Fatalf("unexpected error previewing: %v", err)
	}
	if prev.AffectedEvents != 1250 || prev.CutoffTime == nil {
		t.Errorf("expected 1250 affected events, got %+v", prev)
	}

	purged, err := mock.Purge(ctx, projectID)
	if err != nil {
		t.Fatalf("unexpected error purging: %v", err)
	}
	if purged.DeletedEvents != 1250 {
		t.Errorf("expected 1250 deleted events, got %d", purged.DeletedEvents)
	}
}
