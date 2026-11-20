package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "kinetiq/db/sqlc"
)

type AnalyticsService struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	logger  *slog.Logger
}

func NewAnalyticsService(pool *pgxpool.Pool, queries *db.Queries, logger *slog.Logger) *AnalyticsService {
	return &AnalyticsService{
		pool:    pool,
		queries: queries,
		logger:  logger,
	}
}

func (s *AnalyticsService) GetSummary(ctx context.Context, projectID uuid.UUID, from, to time.Time) (*SummaryResponse, error) {
	row, err := s.queries.GetAnalyticsSummary(ctx, db.GetAnalyticsSummaryParams{
		ProjectID:    pgtype.UUID{Bytes: projectID, Valid: true},
		OccurredAt:   pgtype.Timestamptz{Time: from, Valid: true},
		OccurredAt_2: pgtype.Timestamptz{Time: to, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query analytics summary: %w", err)
	}

	return &SummaryResponse{
		From:              from.UTC().Format(time.RFC3339),
		To:                to.UTC().Format(time.RFC3339),
		TotalEvents:       row.TotalEvents,
		UniqueDistinctIds: row.UniqueDistinctIds,
	}, nil
}

func (s *AnalyticsService) GetTopEvents(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int) (*TopEventsResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	rows, err := s.queries.GetTopEvents(ctx, db.GetTopEventsParams{
		ProjectID:    pgtype.UUID{Bytes: projectID, Valid: true},
		OccurredAt:   pgtype.Timestamptz{Time: from, Valid: true},
		OccurredAt_2: pgtype.Timestamptz{Time: to, Valid: true},
		Limit:        int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query top events: %w", err)
	}

	items := make([]TopEventItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, TopEventItem{
			EventName: r.EventName,
			Count:     r.Count,
		})
	}

	return &TopEventsResponse{
		Items: items,
	}, nil
}

func (s *AnalyticsService) GetVolume(ctx context.Context, projectID uuid.UUID, from, to time.Time, interval string) (*VolumeResponse, error) {
	points := make([]VolumePoint, 0)

	if interval == "day" {
		rows, err := s.queries.GetDailyVolume(ctx, db.GetDailyVolumeParams{
			ProjectID:    pgtype.UUID{Bytes: projectID, Valid: true},
			OccurredAt:   pgtype.Timestamptz{Time: from, Valid: true},
			OccurredAt_2: pgtype.Timestamptz{Time: to, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query daily volume: %w", err)
		}
		for _, r := range rows {
			points = append(points, VolumePoint{
				Timestamp: r.Bucket.Time.UTC().Format(time.RFC3339),
				Count:     r.Count,
			})
		}
	} else if interval == "minute" {
		rows, err := s.queries.GetMinuteVolume(ctx, db.GetMinuteVolumeParams{
			ProjectID:    pgtype.UUID{Bytes: projectID, Valid: true},
			OccurredAt:   pgtype.Timestamptz{Time: from, Valid: true},
			OccurredAt_2: pgtype.Timestamptz{Time: to, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query minute volume: %w", err)
		}
		for _, r := range rows {
			points = append(points, VolumePoint{
				Timestamp: r.Bucket.Time.UTC().Format(time.RFC3339),
				Count:     r.Count,
			})
		}
	} else {
		// default / hour
		rows, err := s.queries.GetHourlyVolume(ctx, db.GetHourlyVolumeParams{
			ProjectID:    pgtype.UUID{Bytes: projectID, Valid: true},
			OccurredAt:   pgtype.Timestamptz{Time: from, Valid: true},
			OccurredAt_2: pgtype.Timestamptz{Time: to, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query hourly volume: %w", err)
		}
		for _, r := range rows {
			points = append(points, VolumePoint{
				Timestamp: r.Bucket.Time.UTC().Format(time.RFC3339),
				Count:     r.Count,
			})
		}
	}

	return &VolumeResponse{
		Interval: interval,
		Points:   points,
	}, nil
}

func (s *AnalyticsService) GetFunnel(ctx context.Context, projectID uuid.UUID, from, to time.Time, steps []string) (*FunnelResponse, error) {
	rows, err := s.queries.GetEventsForFunnel(ctx, db.GetEventsForFunnelParams{
		ProjectID:    pgtype.UUID{Bytes: projectID, Valid: true},
		OccurredAt:   pgtype.Timestamptz{Time: from, Valid: true},
		OccurredAt_2: pgtype.Timestamptz{Time: to, Valid: true},
		Column4:      steps,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query events for funnel: %w", err)
	}

	// Group chronologically sorted events per distinct_id
	userEvents := make(map[string][]string)
	for _, r := range rows {
		if r.DistinctID.Valid && r.DistinctID.String != "" {
			dID := r.DistinctID.String
			userEvents[dID] = append(userEvents[dID], r.EventName)
		}
	}

	stepCounts := make([]int64, len(steps))

	// For each distinct user, verify how far along the sequential steps they progressed
	for _, events := range userEvents {
		stepIdx := 0
		for _, evt := range events {
			if stepIdx < len(steps) && evt == steps[stepIdx] {
				stepIdx++
			}
		}
		// User completed up to stepIdx steps
		for i := 0; i < stepIdx; i++ {
			stepCounts[i]++
		}
	}

	stepResults := make([]FunnelStepResult, len(steps))
	baseCount := float64(stepCounts[0])

	for i, stepName := range steps {
		count := stepCounts[i]
		conversionRate := 0.0
		if i == 0 {
			if count > 0 {
				conversionRate = 100.0
			}
		} else if baseCount > 0 {
			conversionRate = math.Round((float64(count)/baseCount)*10000.0) / 100.0
		}

		stepResults[i] = FunnelStepResult{
			Step:           i + 1,
			EventName:      stepName,
			Count:          count,
			ConversionRate: conversionRate,
		}
	}

	return &FunnelResponse{
		From:  from.UTC().Format(time.RFC3339),
		To:    to.UTC().Format(time.RFC3339),
		Steps: stepResults,
	}, nil
}

func (s *AnalyticsService) GetPropertyBreakdown(ctx context.Context, projectID uuid.UUID, eventName, property string, from, to time.Time, limit int) ([]PropertyBreakdownItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	rows, err := s.queries.GetPropertyBreakdown(ctx, db.GetPropertyBreakdownParams{
		ProjectID:    pgtype.UUID{Bytes: projectID, Valid: true},
		EventName:    eventName,
		OccurredAt:   pgtype.Timestamptz{Time: from, Valid: true},
		OccurredAt_2: pgtype.Timestamptz{Time: to, Valid: true},
		Limit:        int32(limit),
		PropertyKey:  property,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query property breakdown: %w", err)
	}

	items := make([]PropertyBreakdownItem, len(rows))
	for i, r := range rows {
		val := "unknown"
		if r.PropertyValue != nil {
			val = fmt.Sprintf("%v", r.PropertyValue)
		}
		items[i] = PropertyBreakdownItem{
			Value: val,
			Count: r.Count,
		}
	}
	return items, nil
}

func (s *AnalyticsService) GetRetention(ctx context.Context, projectID uuid.UUID, from, to time.Time, firstEvent, returnEvent, granularity string) (*RetentionResponse, error) {
	if granularity != "week" {
		granularity = "day"
	}
	if firstEvent == "" {
		firstEvent = "signup"
	}
	if returnEvent == "" {
		returnEvent = "app_opened"
	}

	dateTrunc := "day"
	maxPeriods := 8
	if granularity == "week" {
		dateTrunc = "week"
		maxPeriods = 5
	}

	// 1. Query initial cohorts and total users
	cohortQuery := fmt.Sprintf(`
		WITH initial_cohort AS (
			SELECT 
				distinct_id,
				date_trunc('%s', MIN(occurred_at))::date AS cohort_date
			FROM events
			WHERE project_id = $1 
			  AND event_name = $2
			  AND occurred_at >= $3 
			  AND occurred_at <= $4
			GROUP BY distinct_id
		)
		SELECT 
			cohort_date,
			COUNT(DISTINCT distinct_id) AS total_users
		FROM initial_cohort
		GROUP BY cohort_date
		ORDER BY cohort_date ASC;
	`, dateTrunc)

	rows, err := s.pool.Query(ctx, cohortQuery, projectID, firstEvent, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query initial cohorts: %w", err)
	}
	defer rows.Close()

	type cohortMeta struct {
		date       time.Time
		totalUsers int64
	}
	var metaList []cohortMeta
	for rows.Next() {
		var m cohortMeta
		if err := rows.Scan(&m.date, &m.totalUsers); err != nil {
			return nil, fmt.Errorf("failed to scan cohort meta: %w", err)
		}
		metaList = append(metaList, m)
	}

	cohortResults := make([]CohortItem, 0, len(metaList))

	// 2. Query return event periods per cohort
	for _, m := range metaList {
		periods := make([]RetentionPeriod, maxPeriods)
		for p := 0; p < maxPeriods; p++ {
			periods[p] = RetentionPeriod{
				Period:     p,
				Count:      0,
				Percentage: 0.0,
			}
		}

		if m.totalUsers > 0 {
			periods[0].Count = m.totalUsers
			periods[0].Percentage = 100.0

			periodQuery := fmt.Sprintf(`
				WITH initial_cohort AS (
					SELECT 
						distinct_id,
						date_trunc('%s', MIN(occurred_at))::date AS cohort_date
					FROM events
					WHERE project_id = $1 
					  AND event_name = $2
					  AND occurred_at >= $3 
					  AND occurred_at <= $4
					GROUP BY distinct_id
					HAVING date_trunc('%s', MIN(occurred_at))::date = $5
				),
				activity AS (
					SELECT 
						e.distinct_id,
						FLOOR(EXTRACT(EPOCH FROM (date_trunc('%s', e.occurred_at) - c.cohort_date)) / %d)::int AS period
					FROM events e
					JOIN initial_cohort c ON c.distinct_id = e.distinct_id
					WHERE e.project_id = $1 
					  AND e.event_name = $6
					  AND e.occurred_at >= c.cohort_date
				)
				SELECT period, COUNT(DISTINCT distinct_id)
				FROM activity
				WHERE period > 0 AND period < %d
				GROUP BY period
				ORDER BY period ASC;
			`, dateTrunc, dateTrunc, dateTrunc, map[string]int{"day": 86400, "week": 604800}[granularity], maxPeriods)

			pRows, err := s.pool.Query(ctx, periodQuery, projectID, firstEvent, from, to, m.date, returnEvent)
			if err == nil {
				for pRows.Next() {
					var pIdx int
					var pCount int64
					if err := pRows.Scan(&pIdx, &pCount); err == nil && pIdx < maxPeriods {
						periods[pIdx].Count = pCount
						periods[pIdx].Percentage = math.Round((float64(pCount)/float64(m.totalUsers))*10000.0) / 100.0
					}
				}
				pRows.Close()
			}
		}

		cohortResults = append(cohortResults, CohortItem{
			CohortDate: m.date.Format("2006-01-02"),
			TotalUsers: m.totalUsers,
			Periods:    periods,
		})
	}

	return &RetentionResponse{
		Granularity: granularity,
		FirstEvent:  firstEvent,
		ReturnEvent: returnEvent,
		From:        from.UTC().Format(time.RFC3339),
		To:          to.UTC().Format(time.RFC3339),
		Cohorts:     cohortResults,
	}, nil
}

func (s *AnalyticsService) ListUsers(ctx context.Context, projectID uuid.UUID, search string, limit, offset int) ([]UserExplorerItem, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var query string
	var rows pgx.Rows
	var err error

	if search != "" {
		query = `
			SELECT distinct_id,
			       COUNT(*) AS total_events,
			       MIN(occurred_at) AS first_seen,
			       MAX(occurred_at) AS last_active
			FROM events
			WHERE project_id = $1
			  AND distinct_id IS NOT NULL
			  AND distinct_id ILIKE $2
			GROUP BY distinct_id
			ORDER BY last_active DESC
			LIMIT $3 OFFSET $4;
		`
		rows, err = s.pool.Query(ctx, query, projectID, "%"+search+"%", limit, offset)
	} else {
		query = `
			SELECT distinct_id,
			       COUNT(*) AS total_events,
			       MIN(occurred_at) AS first_seen,
			       MAX(occurred_at) AS last_active
			FROM events
			WHERE project_id = $1
			  AND distinct_id IS NOT NULL
			GROUP BY distinct_id
			ORDER BY last_active DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err = s.pool.Query(ctx, query, projectID, limit, offset)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []UserExplorerItem
	for rows.Next() {
		var u UserExplorerItem
		if err := rows.Scan(&u.DistinctID, &u.TotalEvents, &u.FirstSeen, &u.LastActive); err != nil {
			return nil, fmt.Errorf("failed to scan user item: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *AnalyticsService) GetUserTimeline(ctx context.Context, projectID uuid.UUID, distinctID string, limit int, before *time.Time) ([]UserTimelineItem, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	var query string
	var rows pgx.Rows
	var err error

	if before != nil && !before.IsZero() {
		query = `
			SELECT event_id, event_name, occurred_at, properties
			FROM events
			WHERE project_id = $1
			  AND distinct_id = $2
			  AND occurred_at < $3
			ORDER BY occurred_at DESC
			LIMIT $4;
		`
		rows, err = s.pool.Query(ctx, query, projectID, distinctID, *before, limit)
	} else {
		query = `
			SELECT event_id, event_name, occurred_at, properties
			FROM events
			WHERE project_id = $1
			  AND distinct_id = $2
			ORDER BY occurred_at DESC
			LIMIT $3;
		`
		rows, err = s.pool.Query(ctx, query, projectID, distinctID, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query user timeline: %w", err)
	}
	defer rows.Close()

	var timeline []UserTimelineItem
	for rows.Next() {
		var item UserTimelineItem
		if err := rows.Scan(&item.EventID, &item.EventName, &item.OccurredAt, &item.Properties); err != nil {
			return nil, fmt.Errorf("failed to scan timeline item: %w", err)
		}
		timeline = append(timeline, item)
	}
	return timeline, nil
}

func (s *AnalyticsService) GetDeviceBreakdown(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]DeviceBreakdownItem, error) {
	query := `
		SELECT 
			COALESCE(properties ->> '$device_type', 'unknown') AS device_type,
			COUNT(*)::bigint AS count
		FROM events
		WHERE project_id = $1
		  AND occurred_at >= $2
		  AND occurred_at < $3
		GROUP BY device_type
		ORDER BY count DESC, device_type ASC;
	`
	rows, err := s.pool.Query(ctx, query, projectID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query device breakdown: %w", err)
	}
	defer rows.Close()

	var items []DeviceBreakdownItem
	for rows.Next() {
		var item DeviceBreakdownItem
		if err := rows.Scan(&item.DeviceType, &item.Count); err != nil {
			return nil, fmt.Errorf("failed to scan device breakdown item: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *AnalyticsService) GetGeoBreakdown(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int) ([]GeoBreakdownItem, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	query := `
		SELECT 
			COALESCE(properties ->> '$geo_country', 'unknown') AS country_code,
			COUNT(*)::bigint AS count
		FROM events
		WHERE project_id = $1
		  AND occurred_at >= $2
		  AND occurred_at < $3
		GROUP BY country_code
		ORDER BY count DESC, country_code ASC
		LIMIT $4;
	`
	rows, err := s.pool.Query(ctx, query, projectID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query geo breakdown: %w", err)
	}
	defer rows.Close()

	var items []GeoBreakdownItem
	for rows.Next() {
		var item GeoBreakdownItem
		if err := rows.Scan(&item.CountryCode, &item.Count); err != nil {
			return nil, fmt.Errorf("failed to scan geo breakdown item: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}
