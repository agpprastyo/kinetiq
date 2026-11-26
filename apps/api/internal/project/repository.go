package project

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "kinetiq/db/sqlc"
)

var (
	ErrDuplicateProjectName = errors.New("project with this name already exists")
	ErrProjectNotFound      = errors.New("project not found or unauthorized")
	ErrForbidden            = errors.New("forbidden: insufficient project permissions")
)

type Project struct {
	ID              uuid.UUID `json:"id"`
	OwnerUserID     string    `json:"owner_user_id,omitempty"`
	Name            string    `json:"name"`
	Role            string    `json:"role,omitempty"` // owner, admin, viewer
	Status          string    `json:"status"`
	RetentionDays   int       `json:"retention_days"`
	AllowedOrigins  string    `json:"allowed_origins"`
	RateLimitPerSec int       `json:"rate_limit_per_sec"`
	AnonymizeIP     bool      `json:"anonymize_ip"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	APIKey          string    `json:"api_key,omitempty"`
}

type APIKeyInfo struct {
	ID          uuid.UUID  `json:"id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	Name        string     `json:"name"`
	KeyPrefix   string     `json:"key_prefix"`
	Environment string     `json:"environment"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RawKey      string     `json:"raw_key,omitempty"`
}

type UpdateProjectInput struct {
	Name            *string `json:"name,omitempty"`
	Status          *string `json:"status,omitempty"`
	RetentionDays   *int    `json:"retention_days,omitempty"`
	AllowedOrigins  *string `json:"allowed_origins,omitempty"`
	RateLimitPerSec *int    `json:"rate_limit_per_sec,omitempty"`
	AnonymizeIP     *bool   `json:"anonymize_ip,omitempty"`
}

type Repository interface {
	CreateProject(ctx context.Context, ownerUserID, name string) (*Project, error)
	ListProjects(ctx context.Context, ownerUserID string) ([]Project, error)
	GetProject(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*Project, error)
	UpdateProject(ctx context.Context, ownerUserID string, projectID uuid.UUID, input UpdateProjectInput) (*Project, error)
	DeleteProject(ctx context.Context, ownerUserID string, projectID uuid.UUID) error
	PurgeEvents(ctx context.Context, ownerUserID string, projectID uuid.UUID) (int64, error)
	RotateAPIKey(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*Project, string, error)
	ListAPIKeys(ctx context.Context, ownerUserID string, projectID uuid.UUID) ([]APIKeyInfo, error)
	CreateAPIKey(ctx context.Context, ownerUserID string, projectID uuid.UUID, name, env string) (*APIKeyInfo, error)
	RevokeAPIKey(ctx context.Context, ownerUserID string, projectID, keyID uuid.UUID) error
	CanUserManageOrg(ctx context.Context, userID, orgID string) (bool, error)
}

type SQLCRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	logger  *slog.Logger
}

func NewSQLCRepository(pool *pgxpool.Pool, logger *slog.Logger) *SQLCRepository {
	return &SQLCRepository{
		pool:    pool,
		queries: db.New(pool),
		logger:  logger,
	}
}

// GenerateAPIKey creates a raw key formatted as sp_live_<prefix>_<secret>
// and returns (rawKey, prefix, hash).
func GenerateAPIKey() (rawKey string, prefix string, hash string, err error) {
	return GenerateAPIKeyWithEnv("sp", "live")
}

// GenerateAPIKeyWithEnv creates a raw key formatted as <brand>_<env>_<prefix>_<secret>
func GenerateAPIKeyWithEnv(brandPrefix, env string) (rawKey string, prefix string, hash string, err error) {
	prefixBytes := make([]byte, 6)
	if _, err := rand.Read(prefixBytes); err != nil {
		return "", "", "", err
	}
	prefix = hex.EncodeToString(prefixBytes)

	secretBytes := make([]byte, 16)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", "", err
	}
	secret := hex.EncodeToString(secretBytes)

	if env != "test" {
		env = "live"
	}
	if brandPrefix == "" {
		brandPrefix = "sp"
	}

	rawKey = fmt.Sprintf("%s_%s_%s_%s", brandPrefix, env, prefix, secret)
	h := sha256.Sum256([]byte(rawKey))
	hash = hex.EncodeToString(h[:])

	return rawKey, prefix, hash, nil
}

func toUUID(pgUUID pgtype.UUID) uuid.UUID {
	return uuid.UUID(pgUUID.Bytes)
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{
		Bytes: [16]byte(id),
		Valid: true,
	}
}

func (r *SQLCRepository) CreateProject(ctx context.Context, ownerUserID, name string) (*Project, error) {
	trimmedName := strings.TrimSpace(name)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Check case-insensitive duplicate name for this owner
	var existingCount int64
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM projects
		WHERE owner_user_id = $1 AND LOWER(TRIM(name)) = LOWER(TRIM($2))
	`, ownerUserID, trimmedName).Scan(&existingCount)
	if err != nil {
		return nil, err
	}
	if existingCount > 0 {
		return nil, ErrDuplicateProjectName
	}

	qtx := r.queries.WithTx(tx)

	projectID := uuid.New()
	pRow, err := qtx.CreateProject(ctx, db.CreateProjectParams{
		ID:          toPgUUID(projectID),
		OwnerUserID: ownerUserID,
		Name:        trimmedName,
		Status:      "active",
	})
	if err != nil {
		if strings.Contains(err.Error(), "idx_projects_owner_unique_name") || strings.Contains(err.Error(), "23505") {
			return nil, ErrDuplicateProjectName
		}
		return nil, err
	}

	rawKey, prefix, keyHash, err := GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	keyID := uuid.New()
	_, err = qtx.CreateProjectAPIKey(ctx, db.CreateProjectAPIKeyParams{
		ID:        toPgUUID(keyID),
		ProjectID: pRow.ID,
		KeyPrefix: prefix,
		KeyHash:   keyHash,
	})
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &Project{
		ID:              toUUID(pRow.ID),
		OwnerUserID:     pRow.OwnerUserID,
		Name:            pRow.Name,
		Status:          pRow.Status,
		RetentionDays:   90,
		AllowedOrigins:  "*",
		RateLimitPerSec: 500,
		AnonymizeIP:     true,
		CreatedAt:       pRow.CreatedAt.Time,
		UpdatedAt:       pRow.UpdatedAt.Time,
		APIKey:          rawKey,
	}, nil
}

func (r *SQLCRepository) ListProjects(ctx context.Context, ownerUserID string) ([]Project, error) {
	// Query projects owned directly OR shared via Better-Auth "member" table
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.owner_user_id, p.name, p.status,
		       COALESCE(p.retention_days, 90),
		       COALESCE(p.allowed_origins, '*'),
		       COALESCE(p.rate_limit_per_sec, 500),
		       COALESCE(p.anonymize_ip, true),
		       p.created_at, p.updated_at,
		       COALESCE(
		           CASE WHEN p.owner_user_id = $1 THEN 'owner' ELSE NULL END,
		           m.role,
		           'viewer'
		       ) AS user_role
		FROM projects p
		LEFT JOIN "member" m ON (
		    m."userId" = $1 AND (
		        (m."scopeType" = 'project' AND m."projectId" = p.id) OR
		        (m."scopeType" = 'workspace' AND m."organizationId" IN (
		            SELECT "id" FROM "organization" WHERE "metadata" ILIKE '%' || p.owner_user_id || '%' OR "id" = m."organizationId"
		        ))
		    )
		)
		WHERE p.owner_user_id = $1 OR m."id" IS NOT NULL
		ORDER BY p.created_at DESC, p.id DESC
	`, ownerUserID)
	if err != nil {
		// Fallback for test databases or before member table queries
		fallbackRows, err2 := r.pool.Query(ctx, `
			SELECT id, owner_user_id, name, status,
			       COALESCE(retention_days, 90),
			       COALESCE(allowed_origins, '*'),
			       COALESCE(rate_limit_per_sec, 500),
			       COALESCE(anonymize_ip, true),
			       created_at, updated_at
			FROM projects
			WHERE owner_user_id = $1
			ORDER BY created_at DESC, id DESC
		`, ownerUserID)
		if err2 != nil {
			sqlcRows, fallbackErr := r.queries.ListProjectsByOwner(ctx, ownerUserID)
			if fallbackErr != nil {
				return nil, err
			}
			projects := make([]Project, 0, len(sqlcRows))
			for _, row := range sqlcRows {
				projects = append(projects, Project{
					ID:              toUUID(row.ID),
					OwnerUserID:     row.OwnerUserID,
					Name:            row.Name,
					Role:            "owner",
					Status:          row.Status,
					RetentionDays:   90,
					AllowedOrigins:  "*",
					RateLimitPerSec: 500,
					AnonymizeIP:     true,
					CreatedAt:       row.CreatedAt.Time,
					UpdatedAt:       row.UpdatedAt.Time,
				})
			}
			return projects, nil
		}
		defer fallbackRows.Close()
		projects := make([]Project, 0)
		for fallbackRows.Next() {
			var p Project
			if err := fallbackRows.Scan(
				&p.ID, &p.OwnerUserID, &p.Name, &p.Status,
				&p.RetentionDays, &p.AllowedOrigins, &p.RateLimitPerSec, &p.AnonymizeIP,
				&p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return nil, err
			}
			p.Role = "owner"
			projects = append(projects, p)
		}
		return projects, nil
	}
	defer rows.Close()

	projects := make([]Project, 0)
	for rows.Next() {
		var p Project
		if err := rows.Scan(
			&p.ID,
			&p.OwnerUserID,
			&p.Name,
			&p.Status,
			&p.RetentionDays,
			&p.AllowedOrigins,
			&p.RateLimitPerSec,
			&p.AnonymizeIP,
			&p.CreatedAt,
			&p.UpdatedAt,
			&p.Role,
		); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (r *SQLCRepository) GetProject(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*Project, error) {
	var p Project
	err := r.pool.QueryRow(ctx, `
		SELECT p.id, p.owner_user_id, p.name, p.status,
		       COALESCE(p.retention_days, 90),
		       COALESCE(p.allowed_origins, '*'),
		       COALESCE(p.rate_limit_per_sec, 500),
		       COALESCE(p.anonymize_ip, true),
		       p.created_at, p.updated_at,
		       COALESCE(
		           CASE WHEN p.owner_user_id = $2 THEN 'owner' ELSE NULL END,
		           m.role,
		           'viewer'
		       ) AS user_role
		FROM projects p
		LEFT JOIN "member" m ON (
		    m."userId" = $2 AND (
		        (m."scopeType" = 'project' AND m."projectId" = p.id) OR
		        (m."scopeType" = 'workspace' AND m."organizationId" IN (
		            SELECT "id" FROM "organization" WHERE "metadata" ILIKE '%' || p.owner_user_id || '%' OR "id" = m."organizationId"
		        ))
		    )
		)
		WHERE p.id = $1 AND (p.owner_user_id = $2 OR m."id" IS NOT NULL)
	`, projectID, ownerUserID).Scan(
		&p.ID,
		&p.OwnerUserID,
		&p.Name,
		&p.Status,
		&p.RetentionDays,
		&p.AllowedOrigins,
		&p.RateLimitPerSec,
		&p.AnonymizeIP,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Role,
	)
	if err != nil {
		// Fallback for simple owner check
		fallbackErr := r.pool.QueryRow(ctx, `
			SELECT id, owner_user_id, name, status,
			       COALESCE(retention_days, 90),
			       COALESCE(allowed_origins, '*'),
			       COALESCE(rate_limit_per_sec, 500),
			       COALESCE(anonymize_ip, true),
			       created_at, updated_at
			FROM projects
			WHERE id = $1 AND owner_user_id = $2
		`, projectID, ownerUserID).Scan(
			&p.ID, &p.OwnerUserID, &p.Name, &p.Status,
			&p.RetentionDays, &p.AllowedOrigins, &p.RateLimitPerSec, &p.AnonymizeIP,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if fallbackErr == nil {
			p.Role = "owner"
			return &p, nil
		}

		if errors.Is(err, pgx.ErrNoRows) || errors.Is(fallbackErr, pgx.ErrNoRows) {
			// Fallback to sqlc query
			row, sqlcErr := r.queries.GetProjectByIDAndOwner(ctx, db.GetProjectByIDAndOwnerParams{
				ID:          toPgUUID(projectID),
				OwnerUserID: ownerUserID,
			})
			if sqlcErr != nil {
				return nil, nil
			}
			return &Project{
				ID:              toUUID(row.ID),
				OwnerUserID:     row.OwnerUserID,
				Name:            row.Name,
				Role:            "owner",
				Status:          row.Status,
				RetentionDays:   90,
				AllowedOrigins:  "*",
				RateLimitPerSec: 500,
				AnonymizeIP:     true,
				CreatedAt:       row.CreatedAt.Time,
				UpdatedAt:       row.UpdatedAt.Time,
			}, nil
		}
		return nil, err
	}

	return &p, nil
}

func canManageProject(p *Project) bool {
	if p == nil {
		return false
	}
	r := strings.ToLower(p.Role)
	return r == "owner" || r == "admin"
}

func (r *SQLCRepository) UpdateProject(ctx context.Context, ownerUserID string, projectID uuid.UUID, input UpdateProjectInput) (*Project, error) {
	existing, err := r.GetProject(ctx, ownerUserID, projectID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrProjectNotFound
	}
	if !canManageProject(existing) {
		return nil, ErrForbidden
	}

	newName := existing.Name
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" {
			return nil, errors.New("project name cannot be empty")
		}
		if len(trimmed) > 100 {
			return nil, errors.New("project name exceeds maximum length of 100 characters")
		}
		// Check duplicate name against other projects of this owner
		var dupCount int64
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM projects
			WHERE owner_user_id = $1 AND id <> $2 AND LOWER(TRIM(name)) = LOWER(TRIM($3))
		`, existing.OwnerUserID, projectID, trimmed).Scan(&dupCount)
		if err != nil {
			return nil, err
		}
		if dupCount > 0 {
			return nil, ErrDuplicateProjectName
		}
		newName = trimmed
	}

	newStatus := existing.Status
	if input.Status != nil && (*input.Status == "active" || *input.Status == "paused" || *input.Status == "archived") {
		newStatus = *input.Status
	}

	newRetention := existing.RetentionDays
	if input.RetentionDays != nil && *input.RetentionDays >= 1 && *input.RetentionDays <= 3650 {
		newRetention = *input.RetentionDays
	}

	newOrigins := existing.AllowedOrigins
	if input.AllowedOrigins != nil {
		newOrigins = strings.TrimSpace(*input.AllowedOrigins)
		if newOrigins == "" {
			newOrigins = "*"
		}
	}

	newRateLimit := existing.RateLimitPerSec
	if input.RateLimitPerSec != nil && *input.RateLimitPerSec >= 10 && *input.RateLimitPerSec <= 100000 {
		newRateLimit = *input.RateLimitPerSec
	}

	newAnonymize := existing.AnonymizeIP
	if input.AnonymizeIP != nil {
		newAnonymize = *input.AnonymizeIP
	}

	var updated Project
	err = r.pool.QueryRow(ctx, `
		UPDATE projects
		SET name = $2,
		    status = $3,
		    retention_days = $4,
		    allowed_origins = $5,
		    rate_limit_per_sec = $6,
		    anonymize_ip = $7,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, owner_user_id, name, status, retention_days, allowed_origins, rate_limit_per_sec, anonymize_ip, created_at, updated_at
	`, projectID, newName, newStatus, newRetention, newOrigins, newRateLimit, newAnonymize).Scan(
		&updated.ID,
		&updated.OwnerUserID,
		&updated.Name,
		&updated.Status,
		&updated.RetentionDays,
		&updated.AllowedOrigins,
		&updated.RateLimitPerSec,
		&updated.AnonymizeIP,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return nil, ErrDuplicateProjectName
		}
		return nil, err
	}
	updated.Role = existing.Role

	return &updated, nil
}

func (r *SQLCRepository) DeleteProject(ctx context.Context, ownerUserID string, projectID uuid.UUID) error {
	existing, err := r.GetProject(ctx, ownerUserID, projectID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrProjectNotFound
	}
	// Only direct project owner can delete project permanently
	if strings.ToLower(existing.Role) != "owner" && existing.OwnerUserID != ownerUserID {
		return ErrForbidden
	}

	tag, err := r.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

func (r *SQLCRepository) PurgeEvents(ctx context.Context, ownerUserID string, projectID uuid.UUID) (int64, error) {
	existing, err := r.GetProject(ctx, ownerUserID, projectID)
	if err != nil {
		return 0, err
	}
	if existing == nil {
		return 0, ErrProjectNotFound
	}
	if !canManageProject(existing) {
		return 0, ErrForbidden
	}

	tag, err := r.pool.Exec(ctx, `DELETE FROM events WHERE project_id = $1`, projectID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *SQLCRepository) RotateAPIKey(ctx context.Context, ownerUserID string, projectID uuid.UUID) (*Project, string, error) {
	existing, err := r.GetProject(ctx, ownerUserID, projectID)
	if err != nil {
		return nil, "", err
	}
	if existing == nil {
		return nil, "", ErrProjectNotFound
	}
	if !canManageProject(existing) {
		return nil, "", ErrForbidden
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	// Revoke old active keys
	if err := qtx.RevokeActiveKeysForProject(ctx, toPgUUID(projectID)); err != nil {
		return nil, "", err
	}

	// Issue new key
	rawKey, prefix, keyHash, err := GenerateAPIKey()
	if err != nil {
		return nil, "", err
	}

	keyID := uuid.New()
	_, err = qtx.CreateProjectAPIKey(ctx, db.CreateProjectAPIKeyParams{
		ID:        toPgUUID(keyID),
		ProjectID: toPgUUID(projectID),
		KeyPrefix: prefix,
		KeyHash:   keyHash,
	})
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}

	return &Project{
		ID:          projectID,
		OwnerUserID: existing.OwnerUserID,
		Name:        existing.Name,
		Role:        existing.Role,
		Status:      existing.Status,
		CreatedAt:   existing.CreatedAt,
		UpdatedAt:   time.Now(),
		APIKey:      rawKey,
	}, rawKey, nil
}

func (r *SQLCRepository) ListAPIKeys(ctx context.Context, ownerUserID string, projectID uuid.UUID) ([]APIKeyInfo, error) {
	existing, err := r.GetProject(ctx, ownerUserID, projectID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrProjectNotFound
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id,
		       COALESCE(name, 'Default Live Key'),
		       key_prefix,
		       COALESCE(environment, 'live'),
		       created_at, revoked_at, last_used_at, expires_at
		FROM project_api_keys
		WHERE project_id = $1
		ORDER BY revoked_at IS NOT NULL ASC, created_at DESC, id DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]APIKeyInfo, 0)
	for rows.Next() {
		var k APIKeyInfo
		if err := rows.Scan(
			&k.ID,
			&k.ProjectID,
			&k.Name,
			&k.KeyPrefix,
			&k.Environment,
			&k.CreatedAt,
			&k.RevokedAt,
			&k.LastUsedAt,
			&k.ExpiresAt,
		); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (r *SQLCRepository) CreateAPIKey(ctx context.Context, ownerUserID string, projectID uuid.UUID, name, env string) (*APIKeyInfo, error) {
	existing, err := r.GetProject(ctx, ownerUserID, projectID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrProjectNotFound
	}
	if !canManageProject(existing) {
		return nil, ErrForbidden
	}

	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		trimmedName = "Ingestion Key"
	}
	if env != "test" {
		env = "live"
	}

	rawKey, prefix, keyHash, err := GenerateAPIKeyWithEnv("tlk", env)
	if err != nil {
		return nil, err
	}

	keyID := uuid.New()
	var created APIKeyInfo
	err = r.pool.QueryRow(ctx, `
		INSERT INTO project_api_keys (id, project_id, name, key_prefix, key_hash, environment)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, project_id, name, key_prefix, environment, created_at, revoked_at, last_used_at, expires_at
	`, keyID, projectID, trimmedName, prefix, keyHash, env).Scan(
		&created.ID,
		&created.ProjectID,
		&created.Name,
		&created.KeyPrefix,
		&created.Environment,
		&created.CreatedAt,
		&created.RevokedAt,
		&created.LastUsedAt,
		&created.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	created.RawKey = rawKey
	return &created, nil
}

func (r *SQLCRepository) RevokeAPIKey(ctx context.Context, ownerUserID string, projectID, keyID uuid.UUID) error {
	existing, err := r.GetProject(ctx, ownerUserID, projectID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrProjectNotFound
	}
	if !canManageProject(existing) {
		return ErrForbidden
	}

	tag, err := r.pool.Exec(ctx, `
		UPDATE project_api_keys
		SET revoked_at = NOW()
		WHERE id = $1 AND project_id = $2 AND revoked_at IS NULL
	`, keyID, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("api key not found or already revoked")
	}
	return nil
}

func (r *SQLCRepository) CanUserManageOrg(ctx context.Context, userID, orgID string) (bool, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM organization_members
		WHERE organization_id = $1
		  AND user_id = $2
		  AND role IN ('owner', 'admin')
	`, orgID, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
