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

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

func TestAnalyticsCatalogAndFilters(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))
	queries := db.New(container.DBPool)

	newProject := func(owner string) uuid.UUID {
		id := uuid.New()
		if _, err := queries.CreateProject(ctx, db.CreateProjectParams{
			ID:          pgtype.UUID{Bytes: id, Valid: true},
			OwnerUserID: owner,
			Name:        "Catalog " + id.String()[:8],
			Status:      "active",
		}); err != nil {
			t.Fatalf("create project: %v", err)
		}
		return id
	}
	pid := newProject("user-1-id")
	other := newProject("user-1-id")

	now := time.Now().UTC().Truncate(time.Second)
	insert := func(project uuid.UUID, distinct, name string, props string) {
		if err := queries.InsertEvent(ctx, db.InsertEventParams{
			ProjectID:  pgtype.UUID{Bytes: project, Valid: true},
			EventID:    uuid.New().String(),
			EventName:  name,
			DistinctID: pgtype.Text{String: distinct, Valid: true},
			OccurredAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true},
			ReceivedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Properties: []byte(props),
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	insert(pid, "u1", "page_view", `{"browser":"chrome","amount":5,"url":"/pricing"}`)
	insert(pid, "u1", "page_view", `{"browser":"chrome","amount":15,"url":"/docs"}`)
	insert(pid, "u2", "page_view", `{"browser":"safari","amount":"n/a"}`)
	insert(pid, "u3", "page_view", `{"browser":"firefox"}`)
	insert(pid, "u4", "signup", `{"plan":"pro","paid":true}`)
	insert(other, "x", "secret_event", `{"secret_prop":"leak"}`)

	// person with properties for person-scope filters
	var personID uuid.UUID
	if err := container.DBPool.QueryRow(ctx, `INSERT INTO persons (project_id, properties) VALUES ($1, '{"tier":"gold"}') RETURNING id`, pid).Scan(&personID); err != nil {
		t.Fatalf("person: %v", err)
	}
	if _, err := container.DBPool.Exec(ctx, `INSERT INTO person_distinct_ids (project_id, distinct_id, person_id) VALUES ($1, 'u1', $2)`, pid, personID); err != nil {
		t.Fatalf("person map: %v", err)
	}

	get := func(path string, q url.Values) (int, json.RawMessage) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s%s?%s", pid, path, q.Encode()), nil)
		req.Header.Set("Cookie", "user1_session_cookie")
		resp, err := server.Test(req, -1)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		var body struct {
			Data json.RawMessage `json:"data"`
		}
		raw := json.RawMessage{}
		_ = json.NewDecoder(resp.Body).Decode(&raw)
		if json.Unmarshal(raw, &body) == nil && body.Data != nil {
			return resp.StatusCode, body.Data
		}
		return resp.StatusCode, raw
	}
	rng := func(extra map[string]string) url.Values {
		q := url.Values{"from": {now.Add(-time.Hour).Format(time.RFC3339)}, "to": {now.Add(time.Hour).Format(time.RFC3339)}}
		for k, v := range extra {
			q.Set(k, v)
		}
		return q
	}

	t.Run("event catalog is project scoped", func(t *testing.T) {
		code, data := get("/catalog/events", nil)
		if code != fiber.StatusOK {
			t.Fatalf("status %d %s", code, data)
		}
		var items []struct {
			EventName string `json:"event_name"`
			Count     int64  `json:"count"`
			LastSeen  string `json:"last_seen"`
		}
		_ = json.Unmarshal(data, &items)
		if len(items) != 2 || items[0].EventName != "page_view" || items[0].Count != 4 || items[0].LastSeen == "" {
			t.Fatalf("got %+v", items)
		}
	})

	t.Run("property catalog infers types and is scoped", func(t *testing.T) {
		code, data := get("/catalog/properties", nil)
		if code != fiber.StatusOK {
			t.Fatalf("status %d", code)
		}
		var items []struct{ Name, Type, Scope string }
		_ = json.Unmarshal(data, &items)
		types := map[string]string{}
		for _, it := range items {
			types[it.Name] = it.Type
		}
		want := map[string]string{"browser": "string", "amount": "mixed", "paid": "boolean", "plan": "string", "url": "string"}
		for k, v := range want {
			if types[k] != v {
				t.Errorf("%s: got %q want %q", k, types[k], v)
			}
		}
		if _, leaked := types["secret_prop"]; leaked {
			t.Error("property from another project leaked")
		}

		_, data = get("/catalog/properties", url.Values{"scope": {"person"}})
		_ = json.Unmarshal(data, &items)
		if len(items) != 1 || items[0].Name != "tier" || items[0].Scope != "person" {
			t.Errorf("person props %+v", items)
		}
	})

	t.Run("property values", func(t *testing.T) {
		code, data := get("/catalog/property-values", url.Values{"property": {"browser"}})
		if code != fiber.StatusOK {
			t.Fatalf("status %d", code)
		}
		var items []struct {
			Value string
			Count int64
		}
		_ = json.Unmarshal(data, &items)
		if len(items) != 3 || items[0].Value != "chrome" || items[0].Count != 2 {
			t.Fatalf("got %+v", items)
		}
		if code, _ := get("/catalog/property-values", nil); code != fiber.StatusBadRequest {
			t.Errorf("missing property: %d", code)
		}
	})

	summaryTotal := func(filter string) int64 {
		t.Helper()
		code, data := get("/analytics/summary", rng(map[string]string{"filter": filter}))
		if code != fiber.StatusOK {
			t.Fatalf("filter %s: status %d %s", filter, code, data)
		}
		var s struct {
			TotalEvents int64 `json:"total_events"`
		}
		_ = json.Unmarshal(data, &s)
		return s.TotalEvents
	}

	t.Run("filter operators compose with AND/OR", func(t *testing.T) {
		cases := map[string]int64{
			`{"property":"browser","op":"eq","value":"chrome"}`:             2,
			`{"property":"browser","op":"neq","value":"chrome"}`:            3,
			`{"property":"url","op":"contains","value":"PRIC"}`:             1,
			`{"property":"amount","op":"gt","value":10}`:                    1,
			`{"property":"amount","op":"lt","value":10}`:                    1,
			`{"property":"browser","op":"in","value":["safari","firefox"]}`: 2,
			`{"property":"browser","op":"is_set"}`:                          4,
			`{"property":"browser","op":"is_not_set"}`:                      1,
			`{"property":"paid","op":"eq","value":true}`:                    1,
			`{"property":"tier","op":"eq","value":"gold","scope":"person"}`: 2,
			`{"group":"and","filters":[{"property":"browser","op":"eq","value":"chrome"},{"property":"amount","op":"gt","value":10}]}`: 1,
			`{"group":"or","filters":[{"property":"browser","op":"eq","value":"safari"},{"property":"plan","op":"is_set"}]}`:           2,
		}
		for f, want := range cases {
			if got := summaryTotal(f); got != want {
				t.Errorf("%s: got %d want %d", f, got, want)
			}
		}
	})

	t.Run("filters accepted by other analytics queries", func(t *testing.T) {
		f := `{"property":"browser","op":"eq","value":"chrome"}`
		for path, extra := range map[string]map[string]string{
			"/analytics/top-events": {"filter": f},
			"/analytics/volume":     {"filter": f},
			"/analytics/funnel":     {"filter": f, "steps": "page_view,signup"},
			"/analytics/breakdown":  {"filter": f, "event_name": "page_view", "property": "browser"},
			"/analytics/retention":  {"filter": f, "first_event": "page_view", "return_event": "page_view"},
		} {
			if code, data := get(path, rng(extra)); code != fiber.StatusOK {
				t.Errorf("%s: status %d %s", path, code, data)
			}
		}
		_, data := get("/analytics/top-events", rng(map[string]string{"filter": f}))
		var top struct {
			Items []struct {
				EventName string `json:"event_name"`
				Count     int64  `json:"count"`
			} `json:"items"`
		}
		_ = json.Unmarshal(data, &top)
		if len(top.Items) != 1 || top.Items[0].Count != 2 {
			t.Errorf("filtered top events %+v", top.Items)
		}
	})

	t.Run("breakdown top N with other", func(t *testing.T) {
		_, data := get("/analytics/summary", rng(map[string]string{"breakdown_by": "browser", "breakdown_limit": "1"}))
		var s struct {
			Breakdown []struct {
				Value string
				Count int64
			} `json:"breakdown"`
		}
		_ = json.Unmarshal(data, &s)
		// chrome 2, then firefox 1, safari 1, unknown 1 -> other 3
		if len(s.Breakdown) != 2 || s.Breakdown[0].Value != "chrome" || s.Breakdown[0].Count != 2 || s.Breakdown[1].Value != "other" || s.Breakdown[1].Count != 3 {
			t.Fatalf("got %+v", s.Breakdown)
		}

		_, data = get("/analytics/volume", rng(map[string]string{"breakdown_by": "browser", "breakdown_limit": "2", "interval": "day"}))
		var v struct {
			Series []struct {
				Value string `json:"value"`
			} `json:"series"`
		}
		_ = json.Unmarshal(data, &v)
		if len(v.Series) != 3 || v.Series[2].Value != "other" {
			t.Fatalf("volume series %+v", v.Series)
		}
	})

	t.Run("malicious names and values cannot alter SQL", func(t *testing.T) {
		evil := `browser' OR '1'='1`
		drop := `x'); DROP TABLE events; --`
		for _, f := range []string{
			`{"property":"` + evil + `","op":"is_set"}`,
			`{"property":"browser","op":"eq","value":"` + evil + `"}`,
			`{"property":"` + drop + `","op":"eq","value":"` + drop + `","scope":"person"}`,
			`{"property":"browser","op":"in","value":["` + drop + `"]}`,
			`{"property":"browser","op":"contains","value":"%' OR 1=1 --"}`,
		} {
			if got := summaryTotal(f); got != 0 {
				t.Errorf("%s matched %d events", f, got)
			}
		}
		code, _ := get("/analytics/summary", rng(map[string]string{"breakdown_by": drop}))
		if code != fiber.StatusOK {
			t.Errorf("breakdown_by evil: %d", code)
		}
		if code, _ := get("/catalog/property-values", url.Values{"property": {drop}}); code != fiber.StatusOK {
			t.Errorf("property-values evil: %d", code)
		}
		var n int
		if err := container.DBPool.QueryRow(ctx, `SELECT count(*) FROM events WHERE project_id = $1`, pid).Scan(&n); err != nil || n != 5 {
			t.Fatalf("events table damaged: n=%d err=%v", n, err)
		}
	})

	t.Run("invalid filter rejected", func(t *testing.T) {
		for _, f := range []string{`{`, `{"property":"a","op":"like","value":"x"}`, `{"property":"a","op":"gt","value":"x"}`} {
			if code, _ := get("/analytics/summary", rng(map[string]string{"filter": f})); code != fiber.StatusBadRequest {
				t.Errorf("%s: status %d", f, code)
			}
		}
	})
}
