package insight_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"kinetiq/internal/insight"
)

type mockInsightRepo struct {
	createFunc       func(ctx context.Context, i *insight.ProjectInsight) error
	listFunc         func(ctx context.Context, projectID uuid.UUID) ([]insight.ProjectInsight, error)
	getByIDFunc      func(ctx context.Context, projectID, insightID uuid.UUID) (*insight.ProjectInsight, error)
	updateFunc       func(ctx context.Context, i *insight.ProjectInsight) error
	deleteFunc       func(ctx context.Context, projectID, insightID uuid.UUID) error
	computeValueFunc func(ctx context.Context, projectID uuid.UUID, eventName, aggregation string, propertyKey *string) (float64, error)
}

func (m *mockInsightRepo) Create(ctx context.Context, i *insight.ProjectInsight) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, i)
	}
	return nil
}

func (m *mockInsightRepo) List(ctx context.Context, projectID uuid.UUID) ([]insight.ProjectInsight, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, projectID)
	}
	return nil, nil
}

func (m *mockInsightRepo) GetByID(ctx context.Context, projectID, insightID uuid.UUID) (*insight.ProjectInsight, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, projectID, insightID)
	}
	return nil, nil
}

func (m *mockInsightRepo) Update(ctx context.Context, i *insight.ProjectInsight) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, i)
	}
	return nil
}

func (m *mockInsightRepo) Delete(ctx context.Context, projectID, insightID uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, projectID, insightID)
	}
	return nil
}

func (m *mockInsightRepo) ComputeValue(ctx context.Context, projectID uuid.UUID, eventName, aggregation string, propertyKey *string) (float64, error) {
	if m.computeValueFunc != nil {
		return m.computeValueFunc(ctx, projectID, eventName, aggregation, propertyKey)
	}
	return 0, nil
}

func TestComputeValue_AdvancedAggregations(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	testCases := []struct {
		name        string
		aggregation string
		propKey     string
		mockVal     float64
	}{
		{
			name:        "Average property aggregation",
			aggregation: "avg_property",
			propKey:     "duration_ms",
			mockVal:     142.55,
		},
		{
			name:        "Min property aggregation",
			aggregation: "min_property",
			propKey:     "response_time",
			mockVal:     12.0,
		},
		{
			name:        "Max property aggregation",
			aggregation: "max_property",
			propKey:     "latency",
			mockVal:     1250.8,
		},
		{
			name:        "Sum property aggregation",
			aggregation: "sum_property",
			propKey:     "price",
			mockVal:     9999.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prop := tc.propKey
			repo := &mockInsightRepo{
				createFunc: func(ctx context.Context, i *insight.ProjectInsight) error {
					return nil
				},
				computeValueFunc: func(ctx context.Context, pID uuid.UUID, eventName, agg string, pKey *string) (float64, error) {
					if pID != projectID || eventName != "api_call" || agg != tc.aggregation || pKey == nil || *pKey != prop {
						return 0, fmt.Errorf("unexpected arguments")
					}
					return tc.mockVal, nil
				},
			}

			svc := insight.NewService(repo, nil)

			req := &insight.CreateInsightRequest{
				Title:       fmt.Sprintf("Insight %s", tc.aggregation),
				ChartType:   "metric",
				EventName:   "api_call",
				Aggregation: tc.aggregation,
				PropertyKey: &prop,
			}

			result, err := svc.CreateInsight(ctx, projectID, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatalf("expected non-nil insight result")
			}
			if result.ComputedValue != tc.mockVal {
				t.Errorf("expected computed value %f, got %f", tc.mockVal, result.ComputedValue)
			}
			if result.Aggregation != tc.aggregation {
				t.Errorf("expected aggregation %s, got %s", tc.aggregation, result.Aggregation)
			}
		})
	}
}
