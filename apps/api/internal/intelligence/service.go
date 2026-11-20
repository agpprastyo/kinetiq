package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"kinetiq/internal/alerting"
)

type Service interface {
	GenerateProjectWeeklyDigest(ctx context.Context, projectID uuid.UUID) (*WeeklyDigestReport, error)
	DispatchWeeklyDigests(ctx context.Context) (int, error)
	CheckTrafficAnomaly(ctx context.Context, projectID uuid.UUID, eventName string) (*AnomalyResult, error)
	RunAnomalySweep(ctx context.Context) ([]AnomalyResult, error)
}

type intelligenceService struct {
	pool        *pgxpool.Pool
	redis       *redis.Client
	alertingSvc alerting.Service
	logger      *slog.Logger
}

func NewService(pool *pgxpool.Pool, redisClient *redis.Client, alertingSvc alerting.Service, logger *slog.Logger) Service {
	return &intelligenceService{
		pool:        pool,
		redis:       redisClient,
		alertingSvc: alertingSvc,
		logger:      logger,
	}
}

// GenerateProjectWeeklyDigest calculates 7-day stats for a project
func (s *intelligenceService) GenerateProjectWeeklyDigest(ctx context.Context, projectID uuid.UUID) (*WeeklyDigestReport, error) {
	now := time.Now().UTC()
	sevenDaysAgo := now.AddDate(0, 0, -7)

	var projectName string
	err := s.pool.QueryRow(ctx, `SELECT name FROM projects WHERE id = $1`, projectID).Scan(&projectName)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	report := &WeeklyDigestReport{
		ProjectID:   projectID,
		ProjectName: projectName,
		PeriodStart: sevenDaysAgo,
		PeriodEnd:   now,
	}

	// 1. Total events and unique users
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(COUNT(DISTINCT distinct_id), 0)
		FROM events
		WHERE project_id = $1 AND occurred_at >= $2 AND occurred_at <= $3
	`, projectID, sevenDaysAgo, now).Scan(&report.TotalEvents, &report.UniqueUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate events: %w", err)
	}

	// 2. Top events
	rows, err := s.pool.Query(ctx, `
		SELECT event_name, COUNT(*) as cnt
		FROM events
		WHERE project_id = $1 AND occurred_at >= $2 AND occurred_at <= $3
		GROUP BY event_name
		ORDER BY cnt DESC
		LIMIT 5
	`, projectID, sevenDaysAgo, now)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var te DigestTopEvent
			if err := rows.Scan(&te.EventName, &te.Count); err == nil {
				report.TopEvents = append(report.TopEvents, te)
			}
		}
	}

	// 3. Conversion calculation (if at least 2 distinct events exist)
	if len(report.TopEvents) >= 2 {
		first := float64(report.TopEvents[0].Count)
		second := float64(report.TopEvents[1].Count)
		if first > 0 {
			report.ConversionRate = math.Round((second/first)*1000) / 10
		}
	}

	return report, nil
}

// DispatchWeeklyDigests compiles reports for all active projects and pushes HTML emails to auth_email_outbox
func (s *intelligenceService) DispatchWeeklyDigests(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.owner_user_id, u.email
		FROM projects p
		JOIN "user" u ON p.owner_user_id = u.id
		WHERE p.status = 'active'
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to query active projects: %w", err)
	}
	defer rows.Close()

	type projectOwner struct {
		id    uuid.UUID
		name  string
		email string
	}
	var targets []projectOwner

	for rows.Next() {
		var po projectOwner
		var rawID string
		var ownerID string
		if err := rows.Scan(&rawID, &po.name, &ownerID, &po.email); err == nil {
			po.id = uuid.MustParse(rawID)
			targets = append(targets, po)
		}
	}

	dispatched := 0
	for _, target := range targets {
		report, err := s.GenerateProjectWeeklyDigest(ctx, target.id)
		if err != nil {
			s.logger.Warn("skipping digest generation for project", slog.String("project_id", target.id.String()), slog.String("error", err.Error()))
			continue
		}

		subject := fmt.Sprintf("📊 Weekly Performance Digest: %s", target.name)
		htmlBody := fmt.Sprintf(`
			<div style="font-family: sans-serif; max-width: 600px; margin: 0 auto; padding: 24px; border: 1px solid #e2e8f0; border-radius: 12px;">
				<h2 style="color: #059669; margin-bottom: 8px;">Kinetiq Weekly Analytics Digest</h2>
				<p style="color: #64748b; font-size: 14px;">Ringkasan performa event 7 hari terakhir untuk proyek <strong>%s</strong>.</p>
				<hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;" />
				<div style="display: flex; gap: 16px; margin-bottom: 24px;">
					<div style="flex: 1; background: #f8fafc; padding: 16px; border-radius: 8px;">
						<div style="font-size: 12px; color: #64748b;">Total Events</div>
						<div style="font-size: 24px; font-weight: bold; color: #0f172a;">%d</div>
					</div>
					<div style="flex: 1; background: #f8fafc; padding: 16px; border-radius: 8px;">
						<div style="font-size: 12px; color: #64748b;">Active Users</div>
						<div style="font-size: 24px; font-weight: bold; color: #0f172a;">%d</div>
					</div>
				</div>
				<p style="font-size: 13px; color: #64748b;">Buka dashboard Anda untuk inspeksi mendalam: <a href="http://localhost:5173/app/projects/%s" style="color: #059669;">Buka Dashboard Kinetiq</a></p>
			</div>
		`, target.name, report.TotalEvents, report.UniqueUsers, target.id.String())

		outboxID := fmt.Sprintf("digest_%s_%s", target.id.String()[:8], time.Now().Format("20060102"))
		_, err = s.pool.Exec(ctx, `
			INSERT INTO auth_email_outbox (id, type, recipient, subject, url, provider, delivery_status, error_message, created_at)
			VALUES ($1, 'weekly_digest', $2, $3, $4, 'outbox', 'stored', NULL, NOW())
			ON CONFLICT (id) DO UPDATE SET created_at = NOW();
		`, outboxID, target.email, subject, htmlBody)
		if err == nil {
			dispatched++
		}
	}

	return dispatched, nil
}

// CheckTrafficAnomaly checks if event traffic in the current hour has dropped >50% compared to average of past 3 days at the exact same hour
func (s *intelligenceService) CheckTrafficAnomaly(ctx context.Context, projectID uuid.UUID, eventName string) (*AnomalyResult, error) {
	now := time.Now().UTC()
	currentHourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
	currentHourEnd := currentHourStart.Add(1 * time.Hour)

	// 1. Count events in the current hour
	var currentCount int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM events
		WHERE project_id = $1 AND event_name = $2 AND occurred_at >= $3 AND occurred_at < $4
	`, projectID, eventName, currentHourStart, currentHourEnd).Scan(&currentCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count current hourly events: %w", err)
	}

	// 2. Count events in the exact same hour across past 3 days
	var baselineTotal int64
	for day := 1; day <= 3; day++ {
		bStart := currentHourStart.AddDate(0, 0, -day)
		bEnd := currentHourEnd.AddDate(0, 0, -day)
		var c int64
		_ = s.pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM events
			WHERE project_id = $1 AND event_name = $2 AND occurred_at >= $3 AND occurred_at < $4
		`, projectID, eventName, bStart, bEnd).Scan(&c)
		baselineTotal += c
	}

	baselineHourly := float64(baselineTotal) / 3.0
	result := &AnomalyResult{
		ProjectID:      projectID,
		EventName:      eventName,
		CurrentHourly:  currentCount,
		BaselineHourly: baselineHourly,
		EvaluatedAt:    now,
	}

	// Only evaluate anomaly if baseline has meaningful traffic (e.g. at least 5 events average)
	if baselineHourly >= 5.0 {
		drop := (baselineHourly - float64(currentCount)) / baselineHourly
		result.DropPercentage = math.Round(drop*1000) / 10
		if drop >= 0.50 {
			result.IsAnomaly = true

			// Trigger registered alert rule if alerting service is available
			if s.alertingSvc != nil {
				rules, err := s.alertingSvc.ListRules(ctx, projectID)
				if err == nil {
					for _, r := range rules {
						if (r.EventName == "*" || r.EventName == eventName) && r.Condition == "drop_lt" {
							s.alertingSvc.EvaluateAndTrigger(ctx, r, currentCount, now)
						}
					}
				}
			}
		}
	}

	return result, nil
}

// RunAnomalySweep runs anomaly detection for all distinct events configured in active alert rules across all active projects
func (s *intelligenceService) RunAnomalySweep(ctx context.Context) ([]AnomalyResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT project_id, event_name
		FROM alert_rules
		WHERE is_active = true
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query active alert rules: %w", err)
	}
	defer rows.Close()

	var pairs []struct {
		projectID uuid.UUID
		eventName string
	}
	for rows.Next() {
		var pID string
		var eName string
		if err := rows.Scan(&pID, &eName); err == nil {
			if id, err := uuid.Parse(pID); err == nil {
				pairs = append(pairs, struct {
					projectID uuid.UUID
					eventName string
				}{projectID: id, eventName: eName})
			}
		}
	}

	var anomalies []AnomalyResult
	for _, pair := range pairs {
		res, err := s.CheckTrafficAnomaly(ctx, pair.projectID, pair.eventName)
		if err == nil && res.IsAnomaly {
			anomalies = append(anomalies, *res)
			s.logger.WarnContext(ctx, "traffic anomaly detected",
				slog.String("project_id", pair.projectID.String()),
				slog.String("event_name", pair.eventName),
				slog.Float64("baseline", res.BaselineHourly),
				slog.Int64("current", res.CurrentHourly),
				slog.Float64("drop_pct", res.DropPercentage),
			)
		}
	}

	return anomalies, nil
}
