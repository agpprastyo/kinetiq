package analytics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MinFunnelSteps       = 2
	MaxFunnelSteps       = 5
	MinFunnelWindow      = time.Second
	MaxFunnelWindow      = 90 * 24 * time.Hour
	DefaultFunnelWindow  = 14 * 24 * time.Hour
	FunnelOrderOrdered   = "ordered"
	FunnelOrderStrict    = "strict"
	MaxDropoffPageSize   = 200
	DefaultDropoffPageSz = 50
)

var windowRe = regexp.MustCompile(`^(\d{1,7})([smhd])$`)

// FunnelParams is the validated funnel definition shared by the funnel and drop-off queries.
type FunnelParams struct {
	Steps  []string
	Window time.Duration
	Order  string
}

// CacheKey is a stable string for cache hashing.
func (p FunnelParams) CacheKey() string {
	return fmt.Sprintf("steps=%q&window=%d&order=%s", p.Steps, int64(p.Window/time.Second), p.Order)
}

// ParseFunnelWindow parses "<n><s|m|h|d>"; empty means DefaultFunnelWindow.
func ParseFunnelWindow(raw string) (time.Duration, error) {
	if raw == "" {
		return DefaultFunnelWindow, nil
	}
	m := windowRe.FindStringSubmatch(raw)
	if m == nil {
		return 0, errors.New("window must look like 30m, 12h or 7d")
	}
	n, _ := strconv.Atoi(m[1])
	unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
	d := time.Duration(n) * unit
	if d < MinFunnelWindow || d > MaxFunnelWindow {
		return 0, errors.New("window must be between 1s and 90d")
	}
	return d, nil
}

// ParseFunnelParams validates raw request inputs (comma-separated steps, window, order).
func ParseFunnelParams(steps, window, order string) (FunnelParams, error) {
	p := FunnelParams{Order: order}
	for _, s := range strings.Split(steps, ",") {
		if s = strings.TrimSpace(s); s != "" {
			p.Steps = append(p.Steps, s)
		}
	}
	if len(p.Steps) < MinFunnelSteps || len(p.Steps) > MaxFunnelSteps {
		return p, fmt.Errorf("Funnel steps must contain between %d and %d comma-separated event names", MinFunnelSteps, MaxFunnelSteps)
	}
	var err error
	if p.Window, err = ParseFunnelWindow(window); err != nil {
		return p, err
	}
	switch p.Order {
	case "":
		p.Order = FunnelOrderOrdered
	case FunnelOrderOrdered, FunnelOrderStrict:
	default:
		return p, errors.New("order must be 'ordered' or 'strict'")
	}
	return p, nil
}

type FunnelStepResult struct {
	Step                       int      `json:"step"`
	EventName                  string   `json:"event_name"`
	Count                      int64    `json:"count"`
	ConversionRate             float64  `json:"conversion_rate"`
	MedianTimeToConvertSeconds *float64 `json:"median_time_to_convert_seconds"`
}

type FunnelBreakdown struct {
	Value string             `json:"value"`
	Steps []FunnelStepResult `json:"steps"`
}

type FunnelResponse struct {
	From          string             `json:"from"`
	To            string             `json:"to"`
	WindowSeconds int64              `json:"window_seconds"`
	Order         string             `json:"order"`
	Steps         []FunnelStepResult `json:"steps"`
	Breakdown     []FunnelBreakdown  `json:"breakdown,omitempty"`
}

type FunnelDropoffUser struct {
	PersonKey  string    `json:"person_key"`
	DistinctID string    `json:"distinct_id"`
	EnteredAt  time.Time `json:"entered_at"`
	ReachedAt  time.Time `json:"reached_at"`
}

type FunnelDropoffResponse struct {
	Step      int                 `json:"step"`
	EventName string              `json:"event_name"`
	NextStep  string              `json:"next_event_name"`
	Total     int64               `json:"total"`
	Limit     int                 `json:"limit"`
	Offset    int                 `json:"offset"`
	Items     []FunnelDropoffUser `json:"items"`
}

// funnelCTEs renders the shared funnel pipeline ending in an `fp` CTE: one row per
// resolved person with the best chain found (deepest, then fastest) and its step times.
//
// A chain starts at every step-1 event; each next step is the earliest later event inside the
// conversion window (ordered) or the very next event of the person, which must be the step
// (strict). Joins are hash joins on person_key, so cost is O(events) plus the pairs per person.
// ponytail: a person with thousands of step-1 events costs O(k^2); cap/sample start events if needed.
func funnelCTEs(projectID uuid.UUID, from, to time.Time, p FunnelParams, opts QueryOptions) (string, []any, error) {
	where, args, err := buildWhere("e", projectID, from, to, opts.Filter)
	if err != nil {
		return "", nil, err
	}
	bind := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	strict := p.Order == FunnelOrderStrict
	n := len(p.Steps)
	names := make([]string, n)
	for i, s := range p.Steps {
		names[i] = bind(s) + "::text"
	}
	win := "make_interval(secs => " + bind(p.Window.Seconds()) + "::float8)"
	bv := "''::text"
	if opts.BreakdownBy != "" {
		bv = breakdownExpr("e", opts, &args)
	}
	// strict: id is the person's event sequence number, so "next event" is a hash-join on id+1.
	idExpr := "e.id"
	if strict {
		idExpr = "ROW_NUMBER() OVER (PARTITION BY e.person_key ORDER BY e.occurred_at, e.id)"
	}
	evWhere := where + " AND e.person_key IS NOT NULL"
	if !strict {
		evWhere += " AND e.event_name = ANY(" + bind(p.Steps) + "::text[])"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `WITH ev AS (
		SELECT e.person_key, e.event_name, e.occurred_at, %s AS id, e.distinct_id, %s AS bv
		FROM resolved_events e WHERE %s
	), c1 AS (
		SELECT person_key, id AS sid, bv, distinct_id AS did, occurred_at AS t1, id AS id1 FROM ev WHERE event_name = %s
	)`, idExpr, bv, evWhere, names[0])

	for k := 1; k < n; k++ {
		carry := make([]string, 0, k)
		for i := 1; i <= k; i++ {
			carry = append(carry, fmt.Sprintf("c.t%d", i))
		}
		after := fmt.Sprintf("(n.occurred_at, n.id) > (c.t%d, c.id%d)", k, k)
		if strict {
			after = fmt.Sprintf("n.id = c.id%d + 1", k)
		}
		cond := fmt.Sprintf(" AND n.event_name = %s", names[k])
		fmt.Fprintf(&b, `, c%[1]d AS (
		SELECT DISTINCT ON (c.person_key, c.sid) c.person_key, c.sid, c.bv, c.did, %[2]s, n.occurred_at AS t%[1]d, n.id AS id%[1]d
		FROM c%[3]d c LEFT JOIN ev n ON n.person_key = c.person_key
			AND %[4]s
			AND n.occurred_at <= c.t1 + %[5]s%[6]s
		ORDER BY c.person_key, c.sid, n.occurred_at, n.id
	)`, k+1, strings.Join(carry, ", "), k, after, win, cond)
	}
	// c{k+1} above selects t1..tk then t{k+1}; the last CTE has every t column.
	var ts, depth []string
	for i := 1; i <= n; i++ {
		ts = append(ts, fmt.Sprintf("t%d", i))
		depth = append(depth, fmt.Sprintf("(t%d IS NOT NULL)::int", i))
	}
	fmt.Fprintf(&b, `, fp AS (
		SELECT DISTINCT ON (person_key) person_key, bv, did, %[1]s, depth FROM (
			SELECT *, %[2]s AS depth FROM c%[3]d
		) x ORDER BY person_key, depth DESC, GREATEST(%[1]s) - t1 ASC, sid
	)`, strings.Join(ts, ", "), strings.Join(depth, " + "), n)
	return b.String(), args, nil
}

func (s *AnalyticsService) GetFunnel(ctx context.Context, projectID uuid.UUID, from, to time.Time, p FunnelParams, opts QueryOptions) (*FunnelResponse, error) {
	ctes, args, err := funnelCTEs(projectID, from, to, p, opts)
	if err != nil {
		return nil, err
	}
	n := len(p.Steps)
	aggs := make([]string, 0, 2*n)
	for k := 1; k <= n; k++ {
		aggs = append(aggs, fmt.Sprintf("count(*) FILTER (WHERE depth >= %d)::bigint", k))
	}
	for k := 2; k <= n; k++ {
		aggs = append(aggs, fmt.Sprintf("(percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM t%d - t%d)) FILTER (WHERE depth >= %d))::float8", k, k-1, k))
	}

	source, group, order := "fp", "GROUP BY ()", "g DESC"
	selectG := "1 AS g, ''::text AS bucket"
	if opts.BreakdownBy != "" {
		args = append(args, opts.BreakdownLimit)
		source = fmt.Sprintf(`(SELECT p.*, CASE WHEN r.rn <= $%d THEN p.bv ELSE 'other' END AS bucket FROM fp p
			JOIN (SELECT bv, ROW_NUMBER() OVER (ORDER BY count(*) DESC, bv ASC) AS rn FROM fp GROUP BY bv) r USING (bv)) pb`, len(args))
		group = "GROUP BY GROUPING SETS ((), (bucket))"
		selectG = "GROUPING(bucket) AS g, COALESCE(bucket, '') AS bucket"
		order = "g DESC, (bucket = 'other'), 3 DESC, bucket ASC"
	}
	q := fmt.Sprintf("%s SELECT %s, %s FROM %s %s ORDER BY %s", ctes, selectG, strings.Join(aggs, ", "), source, group, order)
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query funnel: %w", err)
	}
	defer rows.Close()

	resp := &FunnelResponse{
		From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339),
		WindowSeconds: int64(p.Window / time.Second), Order: p.Order,
	}
	for rows.Next() {
		var g int
		var bucket string
		counts := make([]int64, n)
		medians := make([]*float64, n-1)
		dest := []any{&g, &bucket}
		for i := range counts {
			dest = append(dest, &counts[i])
		}
		for i := range medians {
			dest = append(dest, &medians[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		steps := buildFunnelSteps(p.Steps, counts, medians)
		if g == 1 {
			resp.Steps = steps
		} else {
			resp.Breakdown = append(resp.Breakdown, FunnelBreakdown{Value: bucket, Steps: steps})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

func buildFunnelSteps(names []string, counts []int64, medians []*float64) []FunnelStepResult {
	out := make([]FunnelStepResult, len(names))
	for i, name := range names {
		rate := 0.0
		if i == 0 {
			if counts[0] > 0 {
				rate = 100
			}
		} else if counts[0] > 0 {
			rate = math.Round(float64(counts[i])/float64(counts[0])*10000) / 100
		}
		out[i] = FunnelStepResult{Step: i + 1, EventName: name, Count: counts[i], ConversionRate: rate}
		if i > 0 {
			out[i].MedianTimeToConvertSeconds = medians[i-1]
		}
	}
	return out
}

// GetFunnelDropoff lists the persons whose deepest step is `step` (1-based): they entered it
// but never reached the next one. Ordered by person_key for stable pagination.
func (s *AnalyticsService) GetFunnelDropoff(ctx context.Context, projectID uuid.UUID, from, to time.Time, p FunnelParams, step, limit, offset int, opts QueryOptions) (*FunnelDropoffResponse, error) {
	if step < 1 || step >= len(p.Steps) {
		return nil, fmt.Errorf("step must be between 1 and %d", len(p.Steps)-1)
	}
	ctes, args, err := funnelCTEs(projectID, from, to, p, opts)
	if err != nil {
		return nil, err
	}
	args = append(args, step, limit, offset)
	st, lim, off := len(args)-2, len(args)-1, len(args)
	q := fmt.Sprintf(`%[1]s, sel AS (SELECT * FROM fp WHERE depth = $%[2]d)
		SELECT s.total, pg.person_key, pg.did, pg.t1, pg.reached FROM (SELECT count(*)::bigint AS total FROM sel) s
		LEFT JOIN (SELECT person_key, did, t1, %[5]s AS reached FROM sel ORDER BY person_key LIMIT $%[3]d OFFSET $%[4]d) pg ON true
		ORDER BY pg.person_key`, ctes, st, lim, off, fmt.Sprintf("(ARRAY[%s])[$%d]", tsList(len(p.Steps)), st))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query funnel drop-off: %w", err)
	}
	defer rows.Close()
	resp := &FunnelDropoffResponse{Step: step, EventName: p.Steps[step-1], NextStep: p.Steps[step], Limit: limit, Offset: offset, Items: []FunnelDropoffUser{}}
	for rows.Next() {
		var key, did *string
		var entered, reached *time.Time
		if err := rows.Scan(&resp.Total, &key, &did, &entered, &reached); err != nil {
			return nil, err
		}
		if key == nil {
			continue
		}
		u := FunnelDropoffUser{PersonKey: *key, EnteredAt: *entered, ReachedAt: *reached}
		if did != nil {
			u.DistinctID = *did
		}
		resp.Items = append(resp.Items, u)
	}
	return resp, rows.Err()
}

func tsList(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("t%d", i+1)
	}
	return strings.Join(parts, ", ")
}
