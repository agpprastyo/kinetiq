package importer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Importer struct {
	pool *pgxpool.Pool
}

func NewImporter(pool *pgxpool.Pool) *Importer {
	return &Importer{pool: pool}
}

func (imp *Importer) ImportStream(ctx context.Context, source string, r io.Reader, opts ImportOptions, progressFn func(read int64)) (*ImportStats, error) {
	start := time.Now()
	source = strings.ToLower(strings.TrimSpace(source))
	if source != "posthog" && source != "mixpanel" {
		return nil, fmt.Errorf("unsupported source: %s (supported: 'posthog', 'mixpanel')", source)
	}

	if opts.BatchSize <= 0 {
		opts.BatchSize = 500
	}

	stats := &ImportStats{
		Source: source,
		DryRun: opts.DryRun,
	}

	scanner := bufio.NewScanner(r)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024) // up to 10MB per line

	var batch []StandardEvent

	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}

		if opts.DryRun || imp.pool == nil {
			stats.Imported += int64(len(batch))
			batch = batch[:0]
			return nil
		}

		// Insert batch into PostgreSQL with ON CONFLICT DO NOTHING
		b := &pgx.Batch{}
		for _, evt := range batch {
			propsJSON, err := json.Marshal(evt.Properties)
			if err != nil {
				propsJSON = []byte("{}")
			}
			query := `
				INSERT INTO events (
					project_id, event_id, event_name, occurred_at, received_at, distinct_id, properties
				) VALUES (
					$1, $2, $3, $4, NOW(), $5, $6
				) ON CONFLICT (project_id, event_id) DO NOTHING;
			`
			var did *string
			if evt.DistinctID != "" {
				did = &evt.DistinctID
			}
			b.Queue(query, opts.ProjectID, evt.EventID, evt.EventName, evt.OccurredAt, did, propsJSON)
		}

		br := imp.pool.SendBatch(ctx, b)
		defer br.Close()

		for i := 0; i < len(batch); i++ {
			cmdTag, err := br.Exec()
			if err != nil {
				stats.ErrorCount++
			} else if cmdTag.RowsAffected() > 0 {
				stats.Imported++
			} else {
				stats.Skipped++ // deduplicated
			}
		}

		batch = batch[:0]
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		stats.TotalRead++
		if progressFn != nil && stats.TotalRead%500 == 0 {
			progressFn(stats.TotalRead)
		}

		var evt *StandardEvent
		var err error

		if source == "posthog" {
			evt, err = ParsePostHog([]byte(line))
		} else {
			evt, err = ParseMixpanel([]byte(line))
		}

		if err != nil {
			stats.ErrorCount++
			continue
		}

		batch = append(batch, *evt)
		if len(batch) >= opts.BatchSize {
			if err := flushBatch(); err != nil {
				return nil, err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error during import: %w", err)
	}

	if err := flushBatch(); err != nil {
		return nil, err
	}

	if progressFn != nil {
		progressFn(stats.TotalRead)
	}

	stats.DurationMs = time.Since(start).Milliseconds()
	return stats, nil
}

// NormalizeProjectID parses a string or generates a default UUID if empty (for dry-run)
func NormalizeProjectID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, fmt.Errorf("project-id is required")
	}
	return uuid.Parse(s)
}
