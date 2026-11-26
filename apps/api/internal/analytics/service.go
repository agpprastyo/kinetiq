package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/response"
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

func (s *AnalyticsService) GetSummary(ctx context.Context, projectID uuid.UUID, from, to time.Time, opts QueryOptions) (*SummaryResponse, error) {
	if opts.Active() {
		return s.getSummaryFiltered(ctx, projectID, from, to, opts)
	}
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

func (s *AnalyticsService) GetTopEvents(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int, opts QueryOptions) (*TopEventsResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if opts.Filter != nil {
		return s.getTopEventsFiltered(ctx, projectID, from, to, limit, opts)
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

func (s *AnalyticsService) GetVolume(ctx context.Context, projectID uuid.UUID, from, to time.Time, interval string, opts QueryOptions) (*VolumeResponse, error) {
	return s.getVolumeFiltered(ctx, projectID, from, to, interval, opts)
}

func (s *AnalyticsService) GetPropertyBreakdown(ctx context.Context, projectID uuid.UUID, eventName, property string, from, to time.Time, limit int, opts QueryOptions) ([]PropertyBreakdownItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	return s.breakdownCounts(ctx, projectID, from, to, eventName, opts.Filter, QueryOptions{BreakdownBy: property, BreakdownScope: opts.BreakdownScope, BreakdownLimit: limit})
}

func (s *AnalyticsService) GetRetention(ctx context.Context, projectID uuid.UUID, from, to time.Time, firstEvent, returnEvent, granularity string, opts QueryOptions) (*RetentionResponse, error) {
	if granularity != "week" {
		granularity = "day"
	}
	if firstEvent == "" {
		firstEvent = "signup"
	}
	if returnEvent == "" {
		returnEvent = "app_opened"
	}

	// tz is validated by ParseTZ at the handler (strict pattern + LoadLocation), so inlining is safe.
	tz := opts.zone()
	dateTrunc := "day"
	maxPeriods := 8
	if granularity == "week" {
		dateTrunc = "week"
		maxPeriods = 5
	}

	cohortFilter, cohortArgs, err := retentionFilter(opts.Filter, 4)
	if err != nil {
		return nil, err
	}
	periodFilter, periodArgs, err := retentionFilter(opts.Filter, 6)
	if err != nil {
		return nil, err
	}

	// 1. Query initial cohorts and total users
	cohortQuery := fmt.Sprintf(`
		WITH initial_cohort AS (
			SELECT 
				person_key,
				date_trunc('%s', MIN(occurred_at) AT TIME ZONE '%s')::date AS cohort_date
			FROM resolved_events e
			WHERE project_id = $1 
			  AND event_name = $2
			  AND occurred_at >= $3 
			  AND occurred_at <= $4
			  AND %s
			GROUP BY person_key
		)
		SELECT 
			cohort_date,
			COUNT(DISTINCT person_key) AS total_users
		FROM initial_cohort
		GROUP BY cohort_date
		ORDER BY cohort_date ASC;
	`, dateTrunc, tz, cohortFilter)

	rows, err := s.pool.Query(ctx, cohortQuery, append([]any{projectID, firstEvent, from, to}, cohortArgs...)...)
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
						person_key,
						date_trunc('%s', MIN(occurred_at) AT TIME ZONE '%s')::date AS cohort_date
					FROM resolved_events e
					WHERE project_id = $1 
					  AND event_name = $2
					  AND occurred_at >= $3 
					  AND occurred_at <= $4
					  AND %s
					GROUP BY person_key
					HAVING date_trunc('%s', MIN(occurred_at) AT TIME ZONE '%s')::date = $5
				),
				activity AS (
					SELECT 
						e.person_key,
						FLOOR(EXTRACT(EPOCH FROM (date_trunc('%s', e.occurred_at AT TIME ZONE '%s') - c.cohort_date)) / %d)::int AS period
					FROM resolved_events e
					JOIN initial_cohort c ON c.person_key = e.person_key
					WHERE e.project_id = $1 
					  AND e.event_name = $6
					  AND e.occurred_at >= (c.cohort_date::timestamp AT TIME ZONE '%s')
					  AND %s
				)
				SELECT period, COUNT(DISTINCT person_key)
				FROM activity
				WHERE period > 0 AND period < %d
				GROUP BY period
				ORDER BY period ASC;
			`, dateTrunc, tz, periodFilter, dateTrunc, tz, dateTrunc, tz, map[string]int{"day": 86400, "week": 604800}[granularity], tz, periodFilter, maxPeriods)

			pRows, err := s.pool.Query(ctx, periodQuery, append([]any{projectID, firstEvent, from, to, m.date, returnEvent}, periodArgs...)...)
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

func (s *AnalyticsService) ListUsers(ctx context.Context, projectID uuid.UUID, search string, limit, offset int) ([]UserExplorerItem, int64, error) {
	if limit <= 0 {
		limit = response.DefaultPageLimit
	}
	if limit > response.MaxPageLimit {
		limit = response.MaxPageLimit
	}
	if offset < 0 {
		offset = 0
	}

	pattern := "%"
	if search != "" {
		pattern = "%" + search + "%"
	}

	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT distinct_id)
		FROM events
		WHERE project_id = $1
		  AND distinct_id IS NOT NULL
		  AND ($2 = '%' OR distinct_id ILIKE $2)
	`, projectID, pattern).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT distinct_id,
		       COUNT(*) AS total_events,
		       MIN(occurred_at) AS first_seen,
		       MAX(occurred_at) AS last_active
		FROM events
		WHERE project_id = $1
		  AND distinct_id IS NOT NULL
		  AND ($2 = '%' OR distinct_id ILIKE $2)
		GROUP BY distinct_id
		ORDER BY last_active DESC, distinct_id DESC
		LIMIT $3 OFFSET $4;
	`, projectID, pattern, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	users := make([]UserExplorerItem, 0)
	for rows.Next() {
		var u UserExplorerItem
		if err := rows.Scan(&u.DistinctID, &u.TotalEvents, &u.FirstSeen, &u.LastActive); err != nil {
			return nil, 0, fmt.Errorf("failed to scan user item: %w", err)
		}
		users = append(users, u)
	}
	return users, total, nil
}

// GetUserTimeline pages newest-first by keyset (occurred_at, event_id). The bool reports whether more rows exist.
// A legacy `before` timestamp is honoured only when cursor is nil.
func (s *AnalyticsService) GetUserTimeline(ctx context.Context, projectID uuid.UUID, distinctID string, limit int, cursor *response.Cursor, before *time.Time) ([]UserTimelineItem, bool, error) {
	if limit <= 0 {
		limit = timelineDefaultLimit
	}
	if limit > timelineMaxLimit {
		limit = timelineMaxLimit
	}

	var rows pgx.Rows
	var err error
	const sel = `SELECT event_id, event_name, occurred_at, properties FROM events WHERE project_id = $1 AND distinct_id = $2`
	switch {
	case cursor != nil:
		rows, err = s.pool.Query(ctx, sel+` AND (occurred_at, event_id) < ($3, $4) ORDER BY occurred_at DESC, event_id DESC LIMIT $5`,
			projectID, distinctID, cursor.T, cursor.ID, limit+1)
	case before != nil && !before.IsZero():
		rows, err = s.pool.Query(ctx, sel+` AND occurred_at < $3 ORDER BY occurred_at DESC, event_id DESC LIMIT $4`,
			projectID, distinctID, *before, limit+1)
	default:
		rows, err = s.pool.Query(ctx, sel+` ORDER BY occurred_at DESC, event_id DESC LIMIT $3`,
			projectID, distinctID, limit+1)
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to query user timeline: %w", err)
	}
	defer rows.Close()

	timeline := make([]UserTimelineItem, 0, limit+1)
	for rows.Next() {
		var item UserTimelineItem
		if err := rows.Scan(&item.EventID, &item.EventName, &item.OccurredAt, &item.Properties); err != nil {
			return nil, false, fmt.Errorf("failed to scan timeline item: %w", err)
		}
		timeline = append(timeline, item)
	}
	hasMore := len(timeline) > limit
	if hasMore {
		timeline = timeline[:limit]
	}
	return timeline, hasMore, nil
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

// GetSessions derives sessions on read from raw events (no session table), entirely in SQL so
// Go memory is O(limit), not O(events). Events carrying a client `$session_id` are grouped by it;
// the rest are split per distinct_id whenever the gap to the previous event exceeds timeout.
// Attribution (referrer/UTM) is first-touch. Events with neither a session id nor a distinct_id are ignored.
// ponytail: still scans the range server-side (bounded by statement_timeout); add a materialized sessions table if ranges get heavy.
func (s *AnalyticsService) GetSessions(ctx context.Context, projectID uuid.UUID, from, to time.Time, timeout time.Duration, limit int) (*SessionsResponse, error) {
	if timeout <= 0 {
		timeout = DefaultSessionTimeout
	}
	const q = `
WITH ev AS (
	SELECT id, COALESCE(distinct_id, '') AS did, event_name, occurred_at,
	       COALESCE(properties->>'$session_id', '') AS sid,
	       COALESCE(properties->>'$pathname', '') AS path,
	       COALESCE(properties->>'$referrer', '') AS referrer,
	       COALESCE(properties->>'$referring_domain', '') AS ref_domain,
	       COALESCE(properties->>'utm_source', '') AS utm_source,
	       COALESCE(properties->>'utm_medium', '') AS utm_medium,
	       COALESCE(properties->>'utm_campaign', '') AS utm_campaign,
	       COALESCE(properties->>'utm_term', '') AS utm_term,
	       COALESCE(properties->>'utm_content', '') AS utm_content
	FROM events
	WHERE project_id = $1 AND occurred_at >= $2 AND occurred_at < $3
	  AND (COALESCE(properties->>'$session_id', '') <> '' OR COALESCE(distinct_id, '') <> '')
), gapped AS (
	SELECT *, CASE WHEN sid <> '' THEN 0
	       WHEN lag(occurred_at) OVER w IS NULL OR occurred_at - lag(occurred_at) OVER w > make_interval(secs => $4) THEN 1
	       ELSE 0 END AS is_new
	FROM ev
	WINDOW w AS (PARTITION BY did, (sid <> '') ORDER BY occurred_at, id)
), keyed AS (
	SELECT *, CASE WHEN sid <> '' THEN 'c:' || sid
	       ELSE 'g:' || did || '#' || sum(is_new) OVER (PARTITION BY did, (sid <> '') ORDER BY occurred_at, id) END AS skey
	FROM gapped
), sess AS (
	SELECT skey,
	       CASE WHEN min(sid) <> '' THEN min(sid)
	            ELSE left(encode(sha256(convert_to(min(did) || '|' || min(extract(epoch FROM occurred_at))::text, 'UTF8')), 'hex'), 16) END AS session_id,
	       (array_agg(did ORDER BY occurred_at, id))[1] AS distinct_id,
	       min(occurred_at) AS started_at, max(occurred_at) AS ended_at,
	       floor(extract(epoch FROM max(occurred_at) - min(occurred_at)))::bigint AS duration_seconds,
	       count(*)::int AS event_count,
	       (count(*) FILTER (WHERE event_name = '$pageview'))::int AS pageviews,
	       COALESCE((array_agg(path ORDER BY occurred_at, id) FILTER (WHERE path <> ''))[1], '') AS entry_page,
	       COALESCE((array_agg(path ORDER BY occurred_at DESC, id DESC) FILTER (WHERE path <> ''))[1], '') AS exit_page,
	       COALESCE((array_agg(referrer ORDER BY occurred_at, id) FILTER (WHERE referrer <> ''))[1], '') AS referrer,
	       COALESCE((array_agg(ref_domain ORDER BY occurred_at, id) FILTER (WHERE ref_domain <> ''))[1], '') AS referring_domain,
	       COALESCE((array_agg(utm_source ORDER BY occurred_at, id) FILTER (WHERE utm_source <> ''))[1], '') AS utm_source,
	       COALESCE((array_agg(utm_medium ORDER BY occurred_at, id) FILTER (WHERE utm_medium <> ''))[1], '') AS utm_medium,
	       COALESCE((array_agg(utm_campaign ORDER BY occurred_at, id) FILTER (WHERE utm_campaign <> ''))[1], '') AS utm_campaign,
	       COALESCE((array_agg(utm_term ORDER BY occurred_at, id) FILTER (WHERE utm_term <> ''))[1], '') AS utm_term,
	       COALESCE((array_agg(utm_content ORDER BY occurred_at, id) FILTER (WHERE utm_content <> ''))[1], '') AS utm_content
	FROM keyed GROUP BY skey
)
SELECT count(*),
       COALESCE(sum(duration_seconds) / NULLIF(count(*), 0), 0)::bigint,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('value', v, 'count', c)) FROM (
           SELECT entry_page AS v, count(*) AS c FROM sess WHERE entry_page <> '' GROUP BY 1 ORDER BY c DESC, v LIMIT 10) t), '[]'::jsonb),
       COALESCE((SELECT jsonb_agg(jsonb_build_object('value', v, 'count', c)) FROM (
           SELECT referring_domain AS v, count(*) AS c FROM sess WHERE referring_domain <> '' GROUP BY 1 ORDER BY c DESC, v LIMIT 10) t), '[]'::jsonb),
       COALESCE((SELECT jsonb_agg(to_jsonb(r) - 'skey') FROM (
           SELECT * FROM sess ORDER BY started_at DESC, session_id LIMIT $5) r), '[]'::jsonb)
FROM sess`
	res := &SessionsResponse{}
	var entries, refs, list []byte
	if err := s.pool.QueryRow(ctx, q, projectID, from, to, timeout.Seconds(), limit).
		Scan(&res.TotalSessions, &res.AvgDurationSeconds, &entries, &refs, &list); err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	for dst, src := range map[any][]byte{&res.TopEntryPages: entries, &res.TopReferringDomains: refs, &res.Sessions: list} {
		if err := json.Unmarshal(src, dst); err != nil {
			return nil, fmt.Errorf("failed to decode sessions: %w", err)
		}
	}
	return res, nil
}
