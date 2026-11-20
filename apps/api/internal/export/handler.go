package export

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/valyala/fasthttp"

	"kinetiq/internal/project"
	"kinetiq/internal/response"
)

type Handler struct {
	pool           *pgxpool.Pool
	projectService project.Service
	storageDriver  StorageDriver
	logger         *slog.Logger
}

func NewHandler(pool *pgxpool.Pool, projectService project.Service, logger *slog.Logger) *Handler {
	return &Handler{
		pool:           pool,
		projectService: projectService,
		storageDriver:  NewS3StorageDriver(),
		logger:         logger,
	}
}

func (h *Handler) SetStorageDriver(driver StorageDriver) {
	h.storageDriver = driver
}

func (h *Handler) ExportEvents(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectIDStr := c.Params("projectID")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	// Verify project ownership
	proj, err := h.projectService.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	format := c.Query("format", "csv")
	if format != "csv" && format != "json" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Format must be 'csv' or 'json'")
	}

	var fromTime, toTime *time.Time
	if fromStr := strings.TrimSpace(c.Query("from")); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			tUTC := t.UTC()
			fromTime = &tUTC
		} else {
			return response.Error(c, fiber.StatusBadRequest, "INVALID_DATE_RANGE", "Invalid 'from' timestamp format. Must be RFC3339")
		}
	}
	if toStr := strings.TrimSpace(c.Query("to")); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			tUTC := t.UTC()
			toTime = &tUTC
		} else {
			return response.Error(c, fiber.StatusBadRequest, "INVALID_DATE_RANGE", "Invalid 'to' timestamp format. Must be RFC3339")
		}
	}

	if format == "csv" {
		c.Set("Content-Type", "text/csv")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"events-%s.csv\"", projectID.String()))

		userCtx := c.UserContext()
		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			csvWriter := csv.NewWriter(w)
			_ = csvWriter.Write([]string{"event_id", "event_name", "timestamp", "distinct_id", "properties"})
			csvWriter.Flush()

			rows, err := h.pool.Query(userCtx, `
				SELECT event_id, event_name, occurred_at, COALESCE(distinct_id, ''), properties::text
				FROM events
				WHERE project_id = $1
				  AND ($2::timestamptz IS NULL OR occurred_at >= $2)
				  AND ($3::timestamptz IS NULL OR occurred_at <= $3)
				ORDER BY occurred_at DESC
				LIMIT 10000
			`, projectID, fromTime, toTime)
			if err != nil {
				return
			}
			defer rows.Close()

			for rows.Next() {
				var eventID, eventName, distinctID, props string
				var occurredAt time.Time
				if err := rows.Scan(&eventID, &eventName, &occurredAt, &distinctID, &props); err == nil {
					_ = csvWriter.Write([]string{
						eventID,
						eventName,
						occurredAt.UTC().Format(time.RFC3339),
						distinctID,
						props,
					})
					csvWriter.Flush()
				}
			}
		}))
		return nil
	}

	return response.Error(c, fiber.StatusBadRequest, "INVALID_FORMAT", "Unsupported format")
}

// GetStorageSync retrieves project cloud storage sync configuration with secret masked
func (h *Handler) GetStorageSync(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectIDStr := c.Params("projectID")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	proj, err := h.projectService.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	var syncCfg ProjectStorageSync
	row := h.pool.QueryRow(c.UserContext(), `
		SELECT id, project_id, provider, bucket_name, endpoint_url, region,
		       access_key_id, secret_access_key, prefix_path, frequency,
		       is_enabled, last_sync_at, last_sync_status, last_error_message,
		       created_at, updated_at
		FROM project_storage_sync
		WHERE project_id = $1
	`, projectID)

	err = row.Scan(
		&syncCfg.ID, &syncCfg.ProjectID, &syncCfg.Provider, &syncCfg.BucketName, &syncCfg.EndpointURL, &syncCfg.Region,
		&syncCfg.AccessKeyID, &syncCfg.SecretAccessKey, &syncCfg.PrefixPath, &syncCfg.Frequency,
		&syncCfg.IsEnabled, &syncCfg.LastSyncAt, &syncCfg.LastSyncStatus, &syncCfg.LastErrorMessage,
		&syncCfg.CreatedAt, &syncCfg.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return response.Success(c, fiber.StatusOK, nil)
		}
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to query storage sync")
	}

	// Always mask secret_access_key
	return response.Success(c, fiber.StatusOK, SerializeSyncResponse(syncCfg))
}

type UpdateStorageSyncRequest struct {
	Provider        string `json:"provider"`
	BucketName      string `json:"bucket_name"`
	EndpointURL     string `json:"endpoint_url"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	PrefixPath      string `json:"prefix_path"`
	Frequency       string `json:"frequency"`
	IsEnabled       bool   `json:"is_enabled"`
}

// PutStorageSync creates or updates storage sync configuration
func (h *Handler) PutStorageSync(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectIDStr := c.Params("projectID")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	proj, err := h.projectService.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	var req UpdateStorageSyncRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
	}

	if req.Provider == "" {
		req.Provider = "s3"
	}
	if req.Region == "" {
		req.Region = "us-east-1"
	}
	if req.Frequency == "" {
		req.Frequency = "daily"
	}
	if req.PrefixPath == "" {
		req.PrefixPath = "kinetiq-events"
	}
	if req.BucketName == "" || req.AccessKeyID == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "bucket_name and access_key_id are required")
	}

	// Fetch existing to preserve secret if user didn't change it (e.g. sent masked or empty)
	var existingSecret string
	_ = h.pool.QueryRow(c.UserContext(), `SELECT secret_access_key FROM project_storage_sync WHERE project_id = $1`, projectID).Scan(&existingSecret)

	secretToSave := req.SecretAccessKey
	if secretToSave == "" || secretToSave == "********" {
		secretToSave = existingSecret
	}
	if secretToSave == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "secret_access_key is required")
	}

	var syncCfg ProjectStorageSync
	row := h.pool.QueryRow(c.UserContext(), `
		INSERT INTO project_storage_sync (
			project_id, provider, bucket_name, endpoint_url, region,
			access_key_id, secret_access_key, prefix_path, frequency, is_enabled, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (project_id) DO UPDATE SET
			provider = EXCLUDED.provider,
			bucket_name = EXCLUDED.bucket_name,
			endpoint_url = EXCLUDED.endpoint_url,
			region = EXCLUDED.region,
			access_key_id = EXCLUDED.access_key_id,
			secret_access_key = EXCLUDED.secret_access_key,
			prefix_path = EXCLUDED.prefix_path,
			frequency = EXCLUDED.frequency,
			is_enabled = EXCLUDED.is_enabled,
			updated_at = NOW()
		RETURNING id, project_id, provider, bucket_name, endpoint_url, region,
		          access_key_id, secret_access_key, prefix_path, frequency,
		          is_enabled, last_sync_at, last_sync_status, last_error_message,
		          created_at, updated_at
	`, projectID, req.Provider, req.BucketName, req.EndpointURL, req.Region, req.AccessKeyID, secretToSave, req.PrefixPath, req.Frequency, req.IsEnabled)

	err = row.Scan(
		&syncCfg.ID, &syncCfg.ProjectID, &syncCfg.Provider, &syncCfg.BucketName, &syncCfg.EndpointURL, &syncCfg.Region,
		&syncCfg.AccessKeyID, &syncCfg.SecretAccessKey, &syncCfg.PrefixPath, &syncCfg.Frequency,
		&syncCfg.IsEnabled, &syncCfg.LastSyncAt, &syncCfg.LastSyncStatus, &syncCfg.LastErrorMessage,
		&syncCfg.CreatedAt, &syncCfg.UpdatedAt,
	)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to save storage sync")
	}

	return response.Success(c, fiber.StatusOK, SerializeSyncResponse(syncCfg))
}

// TestStorageSync tests the provided credentials or existing credentials
func (h *Handler) TestStorageSync(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(string)
	if !ok || userID == "" {
		return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
	}

	projectIDStr := c.Params("projectID")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	proj, err := h.projectService.Get(c.UserContext(), userID, projectID)
	if err != nil || proj == nil {
		return response.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Project not found")
	}

	var req UpdateStorageSyncRequest
	_ = c.BodyParser(&req)

	secret := req.SecretAccessKey
	if secret == "" || secret == "********" {
		_ = h.pool.QueryRow(c.UserContext(), `SELECT secret_access_key FROM project_storage_sync WHERE project_id = $1`, projectID).Scan(&secret)
	}

	if req.BucketName == "" || req.AccessKeyID == "" || secret == "" {
		return response.Error(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "bucket_name, access_key_id, and secret_access_key are required to test connection")
	}

	cfg := StorageConfig{
		Provider:        req.Provider,
		BucketName:      req.BucketName,
		EndpointURL:     req.EndpointURL,
		Region:          req.Region,
		AccessKeyID:     req.AccessKeyID,
		SecretAccessKey: secret,
		PrefixPath:      req.PrefixPath,
	}

	if err := h.storageDriver.TestConnection(c.UserContext(), cfg); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "STORAGE_CONNECTION_FAILED", err.Error())
	}

	return response.Success(c, fiber.StatusOK, fiber.Map{
		"success": true,
		"message": "Connection to cloud storage bucket verified successfully",
	})
}
