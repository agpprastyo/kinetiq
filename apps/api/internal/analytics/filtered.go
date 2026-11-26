package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Filter/breakdown variants of analytics queries. Hand-built SQL because the predicate is
// dynamic; every user-supplied token is a bind parameter (see Filter.Compile).

var volumeTrunc = map[string]string{"minute": "minute", "hour": "hour", "day": "day"}

func (s *AnalyticsService) getSummaryFiltered(ctx context.Context, projectID uuid.UUID, from, to time.Time, opts QueryOptions) (*SummaryResponse, error) {
	where, args, err := buildWhere("e", projectID, from, to, opts.Filter)
	if err != nil {
		return nil, err
	}
	resp := &SummaryResponse{From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339)}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::bigint, COUNT(DISTINCT e.person_key)::bigint FROM resolved_events e WHERE `+where, args...).
		Scan(&resp.TotalEvents, &resp.UniqueDistinctIds); err != nil {
		return nil, fmt.Errorf("failed to query filtered summary: %w", err)
	}
	if opts.BreakdownBy != "" {
		if resp.Breakdown, err = s.breakdownCounts(ctx, projectID, from, to, "", opts.Filter, opts); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (s *AnalyticsService) getTopEventsFiltered(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int, opts QueryOptions) (*TopEventsResponse, error) {
	where, args, err := buildWhere("e", projectID, from, to, opts.Filter)
	if err != nil {
		return nil, err
	}
	args = append(args, limit)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT e.event_name, COUNT(*)::bigint AS count FROM events e WHERE %s
		GROUP BY e.event_name ORDER BY count DESC, e.event_name ASC LIMIT $%d`, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query filtered top events: %w", err)
	}
	defer rows.Close()
	items := []TopEventItem{}
	for rows.Next() {
		var it TopEventItem
		if err := rows.Scan(&it.EventName, &it.Count); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return &TopEventsResponse{Items: items}, rows.Err()
}

func (s *AnalyticsService) getVolumeFiltered(ctx context.Context, projectID uuid.UUID, from, to time.Time, interval string, opts QueryOptions) (*VolumeResponse, error) {
	trunc, ok := volumeTrunc[interval]
	if !ok {
		trunc = "hour"
	}
	where, args, err := buildWhere("e", projectID, from, to, opts.Filter)
	if err != nil {
		return nil, err
	}
	value := "''"
	if opts.BreakdownBy != "" {
		value = breakdownExpr("e", opts, &args)
	}
	// Buckets are cut on the wall clock of the requested zone, then mapped back to an instant.
	args = append(args, opts.zone())
	tzArg := len(args)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT (date_trunc('%s', e.occurred_at AT TIME ZONE $%d::text) AT TIME ZONE $%d::text)::timestamptz AS bucket, %s AS value, COUNT(*)::bigint
		FROM events e WHERE %s GROUP BY bucket, value ORDER BY bucket ASC`, trunc, tzArg, tzArg, value, where), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query filtered volume: %w", err)
	}
	defer rows.Close()
	var series []SeriesRow
	totals := map[time.Time]int64{}
	var order []time.Time
	for rows.Next() {
		var r SeriesRow
		if err := rows.Scan(&r.Bucket, &r.Value, &r.Count); err != nil {
			return nil, err
		}
		if _, seen := totals[r.Bucket]; !seen {
			order = append(order, r.Bucket)
		}
		totals[r.Bucket] += r.Count
		series = append(series, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	resp := &VolumeResponse{Interval: interval, Points: make([]VolumePoint, 0, len(order))}
	for _, b := range order {
		resp.Points = append(resp.Points, VolumePoint{Timestamp: b.UTC().Format(time.RFC3339), Count: totals[b]})
	}
	if opts.BreakdownBy != "" {
		resp.Series = SeriesTopN(series, opts.BreakdownLimit)
	}
	return resp, nil
}

// breakdownCounts returns top-N values of opts.BreakdownBy plus an "other" remainder bucket.
func (s *AnalyticsService) breakdownCounts(ctx context.Context, projectID uuid.UUID, from, to time.Time, eventName string, f *Filter, opts QueryOptions) ([]PropertyBreakdownItem, error) {
	where, args, err := buildWhere("e", projectID, from, to, f)
	if err != nil {
		return nil, err
	}
	if eventName != "" {
		args = append(args, eventName)
		where += fmt.Sprintf(" AND e.event_name = $%d", len(args))
	}
	value := breakdownExpr("e", opts, &args)
	args = append(args, opts.BreakdownLimit)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`WITH counts AS (
			SELECT %s AS value, COUNT(*)::bigint AS count FROM events e WHERE %s GROUP BY 1
		), ranked AS (
			SELECT value, count, ROW_NUMBER() OVER (ORDER BY count DESC, value ASC) AS rn FROM counts
		)
		SELECT CASE WHEN rn <= $%[3]d THEN value ELSE 'other' END AS bucket, SUM(count)::bigint AS total, MIN(rn) AS ord
		FROM ranked GROUP BY bucket ORDER BY ord ASC`, value, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query breakdown: %w", err)
	}
	defer rows.Close()
	items := []PropertyBreakdownItem{}
	for rows.Next() {
		var it PropertyBreakdownItem
		var ord int64
		if err := rows.Scan(&it.Value, &it.Count, &ord); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// retentionFilter compiles an optional filter for the retention queries (alias "e").
func retentionFilter(f *Filter, offset int) (string, []any, error) {
	if f == nil {
		return "TRUE", nil, nil
	}
	return f.Compile("e", offset)
}
