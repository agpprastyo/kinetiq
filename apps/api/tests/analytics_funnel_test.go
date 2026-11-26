package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

func TestAnalyticsFunnelV2(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))
	queries := db.New(container.DBPool)

	pid := uuid.New()
	if _, err := queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: pid, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Funnel v2 " + pid.String()[:8],
		Status:      "active",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	insert := func(distinct, name string, at time.Duration, plan string) {
		ts := base.Add(at)
		if err := queries.InsertEvent(ctx, db.InsertEventParams{
			ProjectID:  pgtype.UUID{Bytes: pid, Valid: true},
			EventID:    uuid.New().String(),
			EventName:  name,
			DistinctID: pgtype.Text{String: distinct, Valid: true},
			OccurredAt: pgtype.Timestamptz{Time: ts, Valid: true},
			ReceivedAt: pgtype.Timestamptz{Time: ts, Valid: true},
			Properties: []byte(`{"plan":"` + plan + `"}`),
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	day := 24 * time.Hour
	// u1: full funnel within 2h.
	insert("u1", "signup", 0, "pro")
	insert("u1", "activate", time.Hour, "pro")
	insert("u1", "purchase", 2*time.Hour, "pro")
	// u2: purchase after 10 days -> converts only with a window >= 10d.
	insert("u2", "signup", 0, "free")
	insert("u2", "activate", day, "free")
	insert("u2", "purchase", 10*day, "free")
	// u3: intervening event between signup and activate -> strict stops at step 1.
	insert("u3", "signup", 0, "free")
	insert("u3", "page_view", time.Minute, "free")
	insert("u3", "activate", 2*time.Minute, "free")
	// u4: signup only.
	insert("u4", "signup", 0, "free")
	// u5a + u5b resolve to one person.
	insert("u5a", "signup", 0, "free")
	insert("u5b", "activate", 30*time.Minute, "free")
	// u6: never entered the funnel.
	insert("u6", "activate", 0, "free")

	var personID uuid.UUID
	if err := container.DBPool.QueryRow(ctx, `INSERT INTO persons (project_id, properties) VALUES ($1, '{}') RETURNING id`, pid).Scan(&personID); err != nil {
		t.Fatalf("person: %v", err)
	}
	if _, err := container.DBPool.Exec(ctx, `INSERT INTO person_distinct_ids (project_id, distinct_id, person_id) VALUES ($1, 'u5a', $2), ($1, 'u5b', $2)`, pid, personID); err != nil {
		t.Fatalf("person map: %v", err)
	}

	get := func(path string, extra map[string]string) (int, []byte) {
		t.Helper()
		q := url.Values{
			"from":  {base.Add(-time.Hour).Format(time.RFC3339)},
			"to":    {base.Add(30 * day).Format(time.RFC3339)},
			"steps": {"signup,activate,purchase"},
		}
		for k, v := range extra {
			q.Set(k, v)
		}
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s%s?%s", pid, path, q.Encode()), nil)
		req.Header.Set("Cookie", "user1_session_cookie")
		resp, err := server.Test(req, -1)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		var raw json.RawMessage
		_ = json.NewDecoder(resp.Body).Decode(&raw)
		return resp.StatusCode, raw
	}

	type step struct {
		Count             int64    `json:"count"`
		ConversionRate    float64  `json:"conversion_rate"`
		MedianTimeSeconds *float64 `json:"median_time_to_convert_seconds"`
	}
	type funnelBody struct {
		Steps     []step `json:"steps"`
		Breakdown []struct {
			Value string `json:"value"`
			Steps []step `json:"steps"`
		} `json:"breakdown"`
	}
	funnel := func(extra map[string]string) funnelBody {
		t.Helper()
		code, raw := get("/analytics/funnel", extra)
		if code != http.StatusOK {
			t.Fatalf("status %d: %s", code, raw)
		}
		var env struct {
			Data funnelBody `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return env.Data
	}
	counts := func(steps []step) []int64 {
		out := make([]int64, len(steps))
		for i, s := range steps {
			out[i] = s.Count
		}
		return out
	}
	want := func(t *testing.T, got []int64, exp ...int64) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(exp) {
			t.Fatalf("counts %v, want %v", got, exp)
		}
	}

	t.Run("conversion window excludes late completions", func(t *testing.T) {
		want(t, counts(funnel(map[string]string{"window": "7d"}).Steps), 5, 4, 1)
		want(t, counts(funnel(map[string]string{"window": "14d"}).Steps), 5, 4, 2)
	})

	t.Run("strict order excludes intervening events", func(t *testing.T) {
		want(t, counts(funnel(map[string]string{"window": "14d", "order": "strict"}).Steps), 5, 3, 2)
	})

	t.Run("conversion rate and median time to convert", func(t *testing.T) {
		b := funnel(map[string]string{"window": "14d"})
		if b.Steps[0].MedianTimeSeconds != nil || b.Steps[2].ConversionRate != 40 {
			t.Fatalf("got %+v", b.Steps)
		}
		// step2 deltas: 2m, 30m, 1h, 1d -> 45m; step3 deltas: 1h, 9d.
		if m := b.Steps[1].MedianTimeSeconds; m == nil || *m != 2700 {
			t.Fatalf("median step2 %v", m)
		}
		if m := b.Steps[2].MedianTimeSeconds; m == nil || *m != (3600+9*86400)/2.0 {
			t.Fatalf("median step3 %v", m)
		}
	})

	t.Run("filters and breakdowns", func(t *testing.T) {
		want(t, counts(funnel(map[string]string{"window": "14d", "filter": `{"property":"plan","op":"eq","value":"pro"}`}).Steps), 1, 1, 1)
		b := funnel(map[string]string{"window": "14d", "breakdown_by": "plan"})
		want(t, counts(b.Steps), 5, 4, 2)
		if len(b.Breakdown) != 2 || b.Breakdown[0].Value != "free" || b.Breakdown[1].Value != "pro" {
			t.Fatalf("breakdown %+v", b.Breakdown)
		}
		want(t, counts(b.Breakdown[0].Steps), 4, 3, 1)
		want(t, counts(b.Breakdown[1].Steps), 1, 1, 1)
	})

	type dropoff struct {
		Data struct {
			Total int64 `json:"total"`
			Items []struct {
				PersonKey  string `json:"person_key"`
				DistinctID string `json:"distinct_id"`
			} `json:"items"`
		} `json:"data"`
	}
	drop := func(extra map[string]string) dropoff {
		t.Helper()
		extra["window"] = "14d"
		code, raw := get("/analytics/funnel/dropoff", extra)
		if code != http.StatusOK {
			t.Fatalf("status %d: %s", code, raw)
		}
		var d dropoff
		_ = json.Unmarshal(raw, &d)
		return d
	}

	t.Run("drop-off users are exactly those stopping at a step, paginated", func(t *testing.T) {
		d := drop(map[string]string{"step": "1"})
		if d.Data.Total != 1 || len(d.Data.Items) != 1 || d.Data.Items[0].PersonKey != "d:u4" || d.Data.Items[0].DistinctID != "u4" {
			t.Fatalf("step1 %+v", d.Data)
		}
		p1 := drop(map[string]string{"step": "2", "limit": "1"})
		p2 := drop(map[string]string{"step": "2", "limit": "1", "offset": "1"})
		if p1.Data.Total != 2 || len(p1.Data.Items) != 1 || len(p2.Data.Items) != 1 {
			t.Fatalf("pages %+v %+v", p1.Data, p2.Data)
		}
		if p1.Data.Items[0].PersonKey != "d:u3" || p2.Data.Items[0].PersonKey != "p:"+personID.String() || p2.Data.Items[0].DistinctID != "u5a" {
			t.Fatalf("pages %+v %+v", p1.Data.Items, p2.Data.Items)
		}
	})

	t.Run("validation", func(t *testing.T) {
		for _, extra := range []map[string]string{
			{"window": "0d"}, {"window": "91d"}, {"window": "abc"}, {"order": "random"},
			{"steps": "signup"},
		} {
			if code, raw := get("/analytics/funnel", extra); code != http.StatusBadRequest {
				t.Errorf("%v: status %d %s", extra, code, raw)
			}
		}
		for _, extra := range []map[string]string{{"step": "3"}, {"step": "0"}, {"step": "x"}, {"step": "1", "limit": "1000"}} {
			if code, raw := get("/analytics/funnel/dropoff", extra); code != http.StatusBadRequest {
				t.Errorf("%v: status %d %s", extra, code, raw)
			}
		}
	})
}
