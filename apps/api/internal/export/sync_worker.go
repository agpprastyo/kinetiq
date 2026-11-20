package export

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectStorageSync struct {
	ID               uuid.UUID  `json:"id"`
	ProjectID        uuid.UUID  `json:"project_id"`
	Provider         string     `json:"provider"`
	BucketName       string     `json:"bucket_name"`
	EndpointURL      string     `json:"endpoint_url"`
	Region           string     `json:"region"`
	AccessKeyID      string     `json:"access_key_id"`
	SecretAccessKey  string     `json:"secret_access_key,omitempty"`
	PrefixPath       string     `json:"prefix_path"`
	Frequency        string     `json:"frequency"` // "hourly", "daily"
	IsEnabled        bool       `json:"is_enabled"`
	LastSyncAt       *time.Time `json:"last_sync_at"`
	LastSyncStatus   string     `json:"last_sync_status"` // "idle", "syncing", "success", "failed"
	LastErrorMessage *string    `json:"last_error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type SyncWorker struct {
	pool   *pgxpool.Pool
	driver StorageDriver
	logger *slog.Logger
}

func NewSyncWorker(pool *pgxpool.Pool, driver StorageDriver, logger *slog.Logger) *SyncWorker {
	if driver == nil {
		driver = NewS3StorageDriver()
	}
	return &SyncWorker{
		pool:   pool,
		driver: driver,
		logger: logger,
	}
}

// EventStreamer defines the interface for reading events for a project and time window
type EventStreamer interface {
	StreamEvents(ctx context.Context, projectID uuid.UUID, from, to time.Time) (<-chan []byte, <-chan error)
}

func (w *SyncWorker) BuildPartitionKey(projectID uuid.UUID, syncTime time.Time) string {
	syncTime = syncTime.UTC()
	return fmt.Sprintf("kinetiq/project_id=%s/year=%04d/month=%02d/day=%02d/events-%02d00.jsonl.gz",
		projectID.String(),
		syncTime.Year(),
		syncTime.Month(),
		syncTime.Day(),
		syncTime.Hour(),
	)
}

// CompressAndUpload streams items, compresses them with gzip, and uploads via the storage driver
func (w *SyncWorker) CompressAndUpload(ctx context.Context, cfg StorageConfig, key string, eventLines [][]byte) error {
	if len(eventLines) == 0 {
		return nil
	}

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	for _, line := range eventLines {
		if _, err := gzWriter.Write(line); err != nil {
			return fmt.Errorf("failed to write line to gzip: %w", err)
		}
		if _, err := gzWriter.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write newline to gzip: %w", err)
		}
	}

	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return w.driver.UploadGzip(ctx, cfg, key, &buf)
}

// ExecuteProjectSync executes the sync for a single project configuration
func (w *SyncWorker) ExecuteProjectSync(ctx context.Context, syncCfg ProjectStorageSync, now time.Time) error {
	now = now.UTC()
	fromTime := now.Add(-24 * time.Hour)
	if syncCfg.Frequency == "hourly" {
		fromTime = now.Add(-1 * time.Hour)
	}
	if syncCfg.LastSyncAt != nil && !syncCfg.LastSyncAt.IsZero() {
		fromTime = *syncCfg.LastSyncAt
	}

	// Update status to syncing if DB pool is present
	if w.pool != nil {
		_, _ = w.pool.Exec(ctx, `UPDATE project_storage_sync SET last_sync_status = 'syncing', updated_at = NOW() WHERE id = $1`, syncCfg.ID)
	}

	var eventLines [][]byte
	if w.pool != nil {
		rows, err := w.pool.Query(ctx, `
			SELECT json_build_object(
				'event_id', event_id,
				'event_name', event_name,
				'occurred_at', occurred_at,
				'distinct_id', distinct_id,
				'properties', properties
			)::text
			FROM events
			WHERE project_id = $1 AND occurred_at >= $2 AND occurred_at < $3
			ORDER BY occurred_at ASC
		`, syncCfg.ProjectID, fromTime, now)
		if err != nil {
			errMsg := err.Error()
			if w.pool != nil {
				_, _ = w.pool.Exec(ctx, `UPDATE project_storage_sync SET last_sync_status = 'failed', last_error_message = $2, updated_at = NOW() WHERE id = $1`, syncCfg.ID, errMsg)
			}
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				continue
			}
			eventLines = append(eventLines, []byte(line))
		}
	}

	// Edge Case: If zero events, skip upload and just update last_sync_at to now
	if len(eventLines) == 0 {
		if w.pool != nil {
			_, _ = w.pool.Exec(ctx, `
				UPDATE project_storage_sync
				SET last_sync_at = $2, last_sync_status = 'success', last_error_message = NULL, updated_at = NOW()
				WHERE id = $1
			`, syncCfg.ID, now)
		}
		return nil
	}

	key := w.BuildPartitionKey(syncCfg.ProjectID, now)
	storageCfg := StorageConfig{
		Provider:        syncCfg.Provider,
		BucketName:      syncCfg.BucketName,
		EndpointURL:     syncCfg.EndpointURL,
		Region:          syncCfg.Region,
		AccessKeyID:     syncCfg.AccessKeyID,
		SecretAccessKey: syncCfg.SecretAccessKey,
		PrefixPath:      syncCfg.PrefixPath,
	}

	if err := w.CompressAndUpload(ctx, storageCfg, key, eventLines); err != nil {
		errMsg := err.Error()
		if w.pool != nil {
			_, _ = w.pool.Exec(ctx, `UPDATE project_storage_sync SET last_sync_status = 'failed', last_error_message = $2, updated_at = NOW() WHERE id = $1`, syncCfg.ID, errMsg)
		}
		return err
	}

	if w.pool != nil {
		_, _ = w.pool.Exec(ctx, `
			UPDATE project_storage_sync
			SET last_sync_at = $2, last_sync_status = 'success', last_error_message = NULL, updated_at = NOW()
			WHERE id = $1
		`, syncCfg.ID, now)
	}

	return nil
}

// MaskSecret masks secret access key leaving plain text hidden
func MaskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	return "********"
}

// SerializeSyncResponse masks secret access key in the response payload
func SerializeSyncResponse(syncCfg ProjectStorageSync) ProjectStorageSync {
	res := syncCfg
	res.SecretAccessKey = MaskSecret(syncCfg.SecretAccessKey)
	return res
}
