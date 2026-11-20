package insight

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

type Service interface {
	CreateInsight(ctx context.Context, projectID uuid.UUID, req *CreateInsightRequest) (*ProjectInsight, error)
	ListInsights(ctx context.Context, projectID uuid.UUID) ([]ProjectInsight, error)
	UpdateInsight(ctx context.Context, projectID, insightID uuid.UUID, req *UpdateInsightRequest) (*ProjectInsight, error)
	DeleteInsight(ctx context.Context, projectID, insightID uuid.UUID) error
}

type insightService struct {
	repo   Repository
	logger *slog.Logger
}

func NewService(repo Repository, logger *slog.Logger) Service {
	return &insightService{repo: repo, logger: logger}
}

func (s *insightService) CreateInsight(ctx context.Context, projectID uuid.UUID, req *CreateInsightRequest) (*ProjectInsight, error) {
	insight := &ProjectInsight{
		ProjectID:   projectID,
		Title:       req.Title,
		ChartType:   req.ChartType,
		EventName:   req.EventName,
		Aggregation: req.Aggregation,
		PropertyKey: req.PropertyKey,
	}

	if err := s.repo.Create(ctx, insight); err != nil {
		return nil, fmt.Errorf("failed to create insight: %w", err)
	}

	val, _ := s.repo.ComputeValue(ctx, projectID, insight.EventName, insight.Aggregation, insight.PropertyKey)
	insight.ComputedValue = val
	return insight, nil
}

func (s *insightService) ListInsights(ctx context.Context, projectID uuid.UUID) ([]ProjectInsight, error) {
	insights, err := s.repo.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list insights: %w", err)
	}

	for i := range insights {
		val, _ := s.repo.ComputeValue(ctx, projectID, insights[i].EventName, insights[i].Aggregation, insights[i].PropertyKey)
		insights[i].ComputedValue = val
	}
	return insights, nil
}

func (s *insightService) UpdateInsight(ctx context.Context, projectID, insightID uuid.UUID, req *UpdateInsightRequest) (*ProjectInsight, error) {
	insight, err := s.repo.GetByID(ctx, projectID, insightID)
	if err != nil {
		return nil, fmt.Errorf("insight not found: %w", err)
	}

	if req.Title != nil {
		insight.Title = *req.Title
	}
	if req.ChartType != nil {
		insight.ChartType = *req.ChartType
	}
	if req.EventName != nil {
		insight.EventName = *req.EventName
	}
	if req.Aggregation != nil {
		insight.Aggregation = *req.Aggregation
	}
	if req.PropertyKey != nil {
		insight.PropertyKey = req.PropertyKey
	}

	if err := s.repo.Update(ctx, insight); err != nil {
		return nil, fmt.Errorf("failed to update insight: %w", err)
	}

	val, _ := s.repo.ComputeValue(ctx, projectID, insight.EventName, insight.Aggregation, insight.PropertyKey)
	insight.ComputedValue = val
	return insight, nil
}

func (s *insightService) DeleteInsight(ctx context.Context, projectID, insightID uuid.UUID) error {
	return s.repo.Delete(ctx, projectID, insightID)
}
