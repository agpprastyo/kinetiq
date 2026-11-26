package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"kinetiq/internal/analytics"
	"kinetiq/internal/response"
)

type mockAnalyticsService struct {
	analytics.Service
	listUsersFunc       func(ctx context.Context, projectID uuid.UUID, search string, limit, offset int) ([]analytics.UserExplorerItem, int64, error)
	getUserTimelineFunc func(ctx context.Context, projectID uuid.UUID, distinctID string, limit int, cursor *response.Cursor, before *time.Time) ([]analytics.UserTimelineItem, bool, error)
	getDeviceBreakFunc  func(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]analytics.DeviceBreakdownItem, error)
	getGeoBreakFunc     func(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int) ([]analytics.GeoBreakdownItem, error)
}

func (m *mockAnalyticsService) GetDeviceBreakdown(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]analytics.DeviceBreakdownItem, error) {
	if m.getDeviceBreakFunc != nil {
		return m.getDeviceBreakFunc(ctx, projectID, from, to)
	}
	return nil, nil
}

func (m *mockAnalyticsService) GetGeoBreakdown(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int) ([]analytics.GeoBreakdownItem, error) {
	if m.getGeoBreakFunc != nil {
		return m.getGeoBreakFunc(ctx, projectID, from, to, limit)
	}
	return nil, nil
}

func (m *mockAnalyticsService) ListUsers(ctx context.Context, projectID uuid.UUID, search string, limit, offset int) ([]analytics.UserExplorerItem, int64, error) {
	if m.listUsersFunc != nil {
		return m.listUsersFunc(ctx, projectID, search, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockAnalyticsService) GetUserTimeline(ctx context.Context, projectID uuid.UUID, distinctID string, limit int, cursor *response.Cursor, before *time.Time) ([]analytics.UserTimelineItem, bool, error) {
	if m.getUserTimelineFunc != nil {
		return m.getUserTimelineFunc(ctx, projectID, distinctID, limit, cursor, before)
	}
	return nil, false, nil
}

func TestAnalytics_ListUsers_Unit(t *testing.T) {
	mockSvc := &mockAnalyticsService{
		listUsersFunc: func(ctx context.Context, pID uuid.UUID, search string, limit, offset int) ([]analytics.UserExplorerItem, int64, error) {
			if search == "user-123" {
				return []analytics.UserExplorerItem{
					{
						DistinctID:  "user-123",
						TotalEvents: 42,
						FirstSeen:   time.Now().Add(-48 * time.Hour),
						LastActive:  time.Now(),
					},
				}, 1, nil
			}
			return []analytics.UserExplorerItem{}, 0, nil
		},
	}

	cached := analytics.NewCachedAnalyticsService(mockSvc, nil, nil)
	projectID := uuid.New()

	users, _, err := cached.ListUsers(context.Background(), projectID, "user-123", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].DistinctID != "user-123" {
		t.Errorf("expected distinct_id user-123, got %s", users[0].DistinctID)
	}
	if users[0].TotalEvents != 42 {
		t.Errorf("expected 42 events, got %d", users[0].TotalEvents)
	}
}

func TestAnalytics_UserTimeline_Unit(t *testing.T) {
	now := time.Now().UTC()
	mockSvc := &mockAnalyticsService{
		getUserTimelineFunc: func(ctx context.Context, pID uuid.UUID, distinctID string, limit int, cursor *response.Cursor, before *time.Time) ([]analytics.UserTimelineItem, bool, error) {
			return []analytics.UserTimelineItem{
				{
					EventID:    "evt-1",
					EventName:  "checkout_completed",
					OccurredAt: now,
					Properties: map[string]interface{}{"amount": 250000},
				},
				{
					EventID:    "evt-2",
					EventName:  "view_item",
					OccurredAt: now.Add(-10 * time.Minute),
					Properties: map[string]interface{}{"item_id": "item-99"},
				},
			}, false, nil
		},
	}

	cached := analytics.NewCachedAnalyticsService(mockSvc, nil, nil)
	projectID := uuid.New()

	timeline, _, err := cached.GetUserTimeline(context.Background(), projectID, "user-123", 50, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 timeline items, got %d", len(timeline))
	}
	if timeline[0].EventName != "checkout_completed" {
		t.Errorf("expected event_name checkout_completed, got %s", timeline[0].EventName)
	}
	if timeline[0].Properties["amount"] != 250000 {
		t.Errorf("expected property amount 250000, got %v", timeline[0].Properties["amount"])
	}
}

func TestAnalytics_DeviceAndGeoBreakdown_Unit(t *testing.T) {
	mockSvc := &mockAnalyticsService{
		getDeviceBreakFunc: func(ctx context.Context, pID uuid.UUID, from, to time.Time) ([]analytics.DeviceBreakdownItem, error) {
			return []analytics.DeviceBreakdownItem{
				{DeviceType: "desktop", Count: 1420},
				{DeviceType: "mobile", Count: 890},
			}, nil
		},
		getGeoBreakFunc: func(ctx context.Context, pID uuid.UUID, from, to time.Time, limit int) ([]analytics.GeoBreakdownItem, error) {
			return []analytics.GeoBreakdownItem{
				{CountryCode: "ID", Count: 2100},
				{CountryCode: "SG", Count: 210},
			}, nil
		},
	}

	cached := analytics.NewCachedAnalyticsService(mockSvc, nil, nil)
	projectID := uuid.New()
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	devices, err := cached.GetDeviceBreakdown(context.Background(), projectID, from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 || devices[0].DeviceType != "desktop" || devices[0].Count != 1420 {
		t.Errorf("unexpected device breakdown response: %+v", devices)
	}

	geo, err := cached.GetGeoBreakdown(context.Background(), projectID, from, to, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(geo) != 2 || geo[0].CountryCode != "ID" || geo[0].Count != 2100 {
		t.Errorf("unexpected geo breakdown response: %+v", geo)
	}
}
