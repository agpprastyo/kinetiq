package analytics

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// QueryOptions carries the shared filter DSL and breakdown_by accepted by analytics queries.
type QueryOptions struct {
	Filter         *Filter `json:"filter,omitempty"`
	BreakdownBy    string  `json:"breakdown_by,omitempty"`
	BreakdownScope string  `json:"breakdown_scope,omitempty"`
	BreakdownLimit int     `json:"breakdown_limit,omitempty"`
	// TZ is a validated IANA zone (see ParseTZ) used for day/hour/week bucketing. Empty = UTC.
	TZ string `json:"tz,omitempty"`
}

var tzPattern = regexp.MustCompile(`^[A-Za-z0-9_+\-]+(/[A-Za-z0-9_+\-]+){0,2}$`)

// ParseTZ validates an IANA zone name (empty = "UTC"). The strict pattern plus
// time.LoadLocation make the value safe to embed in SQL as a literal.
func ParseTZ(tz string) (string, error) {
	if tz == "" {
		return "UTC", nil
	}
	if !tzPattern.MatchString(tz) {
		return "", errors.New("tz must be an IANA time zone such as 'Asia/Jakarta'")
	}
	if _, err := time.LoadLocation(tz); err != nil || tz == "Local" {
		return "", errors.New("tz must be an IANA time zone such as 'Asia/Jakarta'")
	}
	return tz, nil
}

func (o QueryOptions) zone() string {
	if o.TZ == "" {
		return "UTC"
	}
	return o.TZ
}

func (o QueryOptions) Active() bool { return o.Filter != nil || o.BreakdownBy != "" }

// CacheKey is a stable string for cache hashing.
func (o QueryOptions) CacheKey() string {
	b, _ := json.Marshal(o)
	return string(b)
}

// ParseQueryOptions validates raw request inputs (filter JSON, breakdown_by, scope, limit).
func ParseQueryOptions(filter, breakdownBy, scope, limit string) (QueryOptions, error) {
	var o QueryOptions
	f, err := ParseFilter(filter)
	if err != nil {
		return o, err
	}
	o.Filter = f
	if breakdownBy == "" {
		return o, nil
	}
	if err := ValidatePropertyName(breakdownBy); err != nil {
		return o, err
	}
	if scope != "" && scope != "event" && scope != "person" {
		return o, errors.New("breakdown_scope must be 'event' or 'person'")
	}
	o.BreakdownBy, o.BreakdownScope, o.BreakdownLimit = breakdownBy, scope, 10
	if limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > 50 {
			return o, errors.New("breakdown_limit must be an integer between 1 and 50")
		}
		o.BreakdownLimit = n
	}
	return o, nil
}

// buildWhere returns the project/time predicate (+ filter) and its bind args.
func buildWhere(alias string, projectID uuid.UUID, from, to time.Time, f *Filter) (string, []any, error) {
	where := fmt.Sprintf("%[1]s.project_id = $1 AND %[1]s.occurred_at >= $2 AND %[1]s.occurred_at < $3", alias)
	args := []any{projectID, from, to}
	if f != nil {
		sql, fargs, err := f.Compile(alias, len(args))
		if err != nil {
			return "", nil, err
		}
		where += " AND " + sql
		args = append(args, fargs...)
	}
	return where, args, nil
}

// breakdownExpr binds the breakdown property name and returns the value expression.
func breakdownExpr(alias string, o QueryOptions, args *[]any) string {
	*args = append(*args, o.BreakdownBy)
	return fmt.Sprintf("COALESCE(%s ->> $%d::text, 'unknown')", scopedProps(alias, o.BreakdownScope), len(*args))
}

// TopNWithOther keeps the n largest buckets and folds the rest into "other".
func TopNWithOther(items []BreakdownItem, n int) []BreakdownItem {
	sorted := append([]BreakdownItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Count != sorted[j].Count {
			return sorted[i].Count > sorted[j].Count
		}
		return sorted[i].Value < sorted[j].Value
	})
	if n < 1 || len(sorted) <= n {
		return sorted
	}
	var rest int64
	for _, it := range sorted[n:] {
		rest += it.Count
	}
	return append(sorted[:n:n], BreakdownItem{Value: "other", Count: rest})
}

type SeriesRow struct {
	Bucket time.Time
	Value  string
	Count  int64
}

type VolumeSeries struct {
	Value  string        `json:"value"`
	Points []VolumePoint `json:"points"`
}

// SeriesTopN groups rows into per-value series, folding all but the top n values into "other".
func SeriesTopN(rows []SeriesRow, n int) []VolumeSeries {
	totals := map[string]int64{}
	for _, r := range rows {
		totals[r.Value] += r.Count
	}
	items := make([]BreakdownItem, 0, len(totals))
	for v, c := range totals {
		items = append(items, BreakdownItem{Value: v, Count: c})
	}
	top := TopNWithOther(items, n)
	keep := map[string]bool{}
	order := make([]string, 0, len(top))
	for _, it := range top {
		order = append(order, it.Value)
		if it.Value != "other" || len(items) <= n {
			keep[it.Value] = true
		}
	}
	buckets := map[string]map[time.Time]int64{}
	for _, r := range rows {
		k := r.Value
		if !keep[k] {
			k = "other"
		}
		if buckets[k] == nil {
			buckets[k] = map[time.Time]int64{}
		}
		buckets[k][r.Bucket] += r.Count
	}
	out := make([]VolumeSeries, 0, len(order))
	for _, v := range order {
		ts := make([]time.Time, 0, len(buckets[v]))
		for b := range buckets[v] {
			ts = append(ts, b)
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
		pts := make([]VolumePoint, len(ts))
		for i, b := range ts {
			pts[i] = VolumePoint{Timestamp: b.UTC().Format(time.RFC3339), Count: buckets[v][b]}
		}
		out = append(out, VolumeSeries{Value: v, Points: pts})
	}
	return out
}
