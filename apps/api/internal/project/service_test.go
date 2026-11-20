package project_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"kinetiq/internal/project"
)

type mockRepo struct {
	createFunc func(ctx context.Context, ownerUserID, name string) (*project.Project, error)
	listFunc   func(ctx context.Context, ownerUserID string) ([]project.Project, error)
	getFunc    func(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*project.Project, error)
}

func TestProjectService_Unit(t *testing.T) {
	t.Run("Create rejects empty or whitespace-only names", func(t *testing.T) {
		service := project.NewProjectService(nil, nil)
		_, err := service.Create(context.Background(), "user-1", "   ")
		if err == nil {
			t.Errorf("expected error for empty name, got nil")
		}
	})

	t.Run("Create rejects names exceeding 100 characters", func(t *testing.T) {
		service := project.NewProjectService(nil, nil)
		longName := strings.Repeat("a", 101)
		_, err := service.Create(context.Background(), "user-1", longName)
		if err == nil {
			t.Errorf("expected error for overly long name, got nil")
		}
	})

	t.Run("GenerateAPIKey produces valid format and non-empty hash", func(t *testing.T) {
		rawKey, prefix, hash, err := project.GenerateAPIKey()
		if err != nil {
			t.Fatalf("failed to generate key: %v", err)
		}
		if !strings.HasPrefix(rawKey, "sp_live_") {
			t.Errorf("expected sp_live_ prefix, got %s", rawKey)
		}
		if prefix == "" || len(prefix) < 8 {
			t.Errorf("expected non-empty prefix, got %s", prefix)
		}
		if hash == "" || len(hash) != 64 {
			t.Errorf("expected 64-char sha256 hex hash, got %s", hash)
		}
	})
}
