package project

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

type Service interface {
	Create(ctx context.Context, ownerUserID, name string) (*Project, error)
	List(ctx context.Context, ownerUserID string) ([]Project, error)
	Get(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*Project, error)
	Update(ctx context.Context, ownerUserID string, projectID uuid.UUID, input UpdateProjectInput) (*Project, error)
	Delete(ctx context.Context, ownerUserID string, projectID uuid.UUID) error
	PurgeEvents(ctx context.Context, ownerUserID string, projectID uuid.UUID) (int64, error)
	RotateKey(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*Project, string, error)
	ListKeys(ctx context.Context, ownerUserID string, projectID uuid.UUID) ([]APIKeyInfo, error)
	CreateKey(ctx context.Context, ownerUserID string, projectID uuid.UUID, name, env string) (*APIKeyInfo, error)
	RevokeKey(ctx context.Context, ownerUserID string, projectID, keyID uuid.UUID) error
	CanUserManageOrg(ctx context.Context, userID, orgID string) (bool, error)
}

type ProjectService struct {
	repo   Repository
	logger *slog.Logger
}

func NewProjectService(repo Repository, logger *slog.Logger) *ProjectService {
	return &ProjectService{
		repo:   repo,
		logger: logger,
	}
}

func (s *ProjectService) Create(ctx context.Context, ownerUserID, name string) (*Project, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "project creation rejected: empty name", slog.String("owner_user_id", ownerUserID))
		}
		return nil, errors.New("project name cannot be empty")
	}
	if len(trimmedName) > 100 {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "project creation rejected: name exceeds 100 characters",
				slog.String("owner_user_id", ownerUserID),
				slog.Int("length", len(trimmedName)),
			)
		}
		return nil, errors.New("project name exceeds maximum length of 100 characters")
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx, "creating new project in domain service",
			slog.String("owner_user_id", ownerUserID),
			slog.String("project_name", trimmedName),
		)
	}

	return s.repo.CreateProject(ctx, ownerUserID, trimmedName)
}

func (s *ProjectService) List(ctx context.Context, ownerUserID string) ([]Project, error) {
	return s.repo.ListProjects(ctx, ownerUserID)
}

func (s *ProjectService) Get(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*Project, error) {
	return s.repo.GetProject(ctx, ownerUserID, projectID)
}

func (s *ProjectService) Update(ctx context.Context, ownerUserID string, projectID uuid.UUID, input UpdateProjectInput) (*Project, error) {
	return s.repo.UpdateProject(ctx, ownerUserID, projectID, input)
}

func (s *ProjectService) Delete(ctx context.Context, ownerUserID string, projectID uuid.UUID) error {
	return s.repo.DeleteProject(ctx, ownerUserID, projectID)
}

func (s *ProjectService) PurgeEvents(ctx context.Context, ownerUserID string, projectID uuid.UUID) (int64, error) {
	return s.repo.PurgeEvents(ctx, ownerUserID, projectID)
}

func (s *ProjectService) RotateKey(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*Project, string, error) {
	if s.logger != nil {
		s.logger.InfoContext(ctx, "rotating project API key",
			slog.String("owner_user_id", ownerUserID),
			slog.String("project_id", projectID.String()),
		)
	}
	return s.repo.RotateAPIKey(ctx, ownerUserID, projectID)
}

func (s *ProjectService) ListKeys(ctx context.Context, ownerUserID string, projectID uuid.UUID) ([]APIKeyInfo, error) {
	return s.repo.ListAPIKeys(ctx, ownerUserID, projectID)
}

func (s *ProjectService) CreateKey(ctx context.Context, ownerUserID string, projectID uuid.UUID, name, env string) (*APIKeyInfo, error) {
	return s.repo.CreateAPIKey(ctx, ownerUserID, projectID, name, env)
}

func (s *ProjectService) RevokeKey(ctx context.Context, ownerUserID string, projectID, keyID uuid.UUID) error {
	return s.repo.RevokeAPIKey(ctx, ownerUserID, projectID, keyID)
}

func (s *ProjectService) CanUserManageOrg(ctx context.Context, userID, orgID string) (bool, error) {
	return s.repo.CanUserManageOrg(ctx, userID, orgID)
}
