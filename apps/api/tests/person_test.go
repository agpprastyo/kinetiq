package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/analytics"
	"kinetiq/internal/app"
	"kinetiq/internal/ingestion"
	"kinetiq/internal/person"
	"kinetiq/internal/worker"
)

func personTestEnv(t *testing.T) (*app.Container, *person.Service, *db.Queries) {
	t.Helper()
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")
	c, err := app.NewContainer(context.Background())
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	return c, person.NewService(c.DBPool), db.New(c.DBPool)
}

func personTestProject(t *testing.T, q *db.Queries) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := q.CreateProject(context.Background(), db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		OwnerUserID: "person-test-user",
		Name:        "Person " + id.String()[:8],
		Status:      "active",
	}); err != nil {
		t.Fatalf("project: %v", err)
	}
	return id
}

func personTestEvent(t *testing.T, q *db.Queries, pid uuid.UUID, distinct string, at time.Time) {
	t.Helper()
	if err := q.InsertEvent(context.Background(), db.InsertEventParams{
		ProjectID:  pgtype.UUID{Bytes: pid, Valid: true},
		EventID:    uuid.NewString(),
		EventName:  "page_view",
		OccurredAt: pgtype.Timestamptz{Time: at, Valid: true},
		ReceivedAt: pgtype.Timestamptz{Time: at, Valid: true},
		DistinctID: pgtype.Text{String: distinct, Valid: true},
		Properties: []byte("{}"),
	}); err != nil {
		t.Fatalf("event: %v", err)
	}
}

func personCount(t *testing.T, pool *pgxpool.Pool, pid uuid.UUID) (persons, mappings, orphans int) {
	t.Helper()
	ctx := context.Background()
	pool.QueryRow(ctx, `SELECT count(*) FROM persons WHERE project_id=$1`, pid).Scan(&persons)
	pool.QueryRow(ctx, `SELECT count(*) FROM person_distinct_ids WHERE project_id=$1`, pid).Scan(&mappings)
	pool.QueryRow(ctx, `SELECT count(*) FROM persons p WHERE p.project_id=$1 AND NOT EXISTS (SELECT 1 FROM person_distinct_ids d WHERE d.person_id=p.id)`, pid).Scan(&orphans)
	return
}

func uniqueUsers(t *testing.T, c *app.Container, q *db.Queries, pid uuid.UUID, at time.Time) int64 {
	t.Helper()
	svc := analytics.NewAnalyticsService(c.DBPool, q, nil)
	s, err := svc.GetSummary(context.Background(), pid, at.Add(-time.Hour), at.Add(time.Hour), analytics.QueryOptions{})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	return s.UniqueDistinctIds
}

func TestPersonIdentity(t *testing.T) {
	c, svc, q := personTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("anonymous events attributed to identified user after merge", func(t *testing.T) {
		pid := personTestProject(t, q)
		personTestEvent(t, q, pid, "anon_1", now)
		personTestEvent(t, q, pid, "user_1", now)
		if got := uniqueUsers(t, c, q, pid, now); got != 2 {
			t.Fatalf("before identify unique=%d want 2", got)
		}
		if _, err := svc.Identify(ctx, pid, person.IdentifyInput{DistinctID: "user_1", AnonymousID: "anon_1"}); err != nil {
			t.Fatalf("identify: %v", err)
		}
		if got := uniqueUsers(t, c, q, pid, now); got != 1 {
			t.Fatalf("after identify unique=%d want 1", got)
		}
	})

	t.Run("identify is idempotent", func(t *testing.T) {
		pid := personTestProject(t, q)
		var first *person.Person
		for i := 0; i < 3; i++ {
			p, err := svc.Identify(ctx, pid, person.IdentifyInput{DistinctID: "user_2", AnonymousID: "anon_2"})
			if err != nil {
				t.Fatalf("identify #%d: %v", i, err)
			}
			if first == nil {
				first = p
			} else if p.ID != first.ID {
				t.Fatalf("person id changed: %s vs %s", p.ID, first.ID)
			}
		}
		if !first.IsIdentified {
			t.Error("expected identified")
		}
		if p, m, o := personCount(t, c.DBPool, pid); p != 1 || m != 2 || o != 0 {
			t.Fatalf("persons=%d mappings=%d orphans=%d want 1/2/0", p, m, o)
		}
	})

	t.Run("merging two identified users is rejected", func(t *testing.T) {
		pid := personTestProject(t, q)
		if _, err := svc.Identify(ctx, pid, person.IdentifyInput{DistinctID: "user_a"}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Identify(ctx, pid, person.IdentifyInput{DistinctID: "user_b"}); err != nil {
			t.Fatal(err)
		}
		_, err := svc.Identify(ctx, pid, person.IdentifyInput{DistinctID: "user_a", AnonymousID: "user_b"})
		if err != person.ErrAlreadyIdentified {
			t.Fatalf("identify err=%v want ErrAlreadyIdentified", err)
		}
		if _, err := svc.Alias(ctx, pid, "user_a", "user_b"); err != person.ErrAlreadyIdentified {
			t.Fatalf("alias err=%v want ErrAlreadyIdentified", err)
		}
		if p, _, _ := personCount(t, c.DBPool, pid); p != 2 {
			t.Fatalf("persons=%d want 2", p)
		}
	})

	t.Run("alias links two known ids without marking identified", func(t *testing.T) {
		pid := personTestProject(t, q)
		a, err := svc.Alias(ctx, pid, "dev_1", "dev_2")
		if err != nil {
			t.Fatal(err)
		}
		if a.IsIdentified {
			t.Error("alias must not mark identified")
		}
		b, err := svc.Alias(ctx, pid, "dev_2", "dev_3")
		if err != nil || b.ID != a.ID {
			t.Fatalf("alias chain: %v %v", b, err)
		}
	})

	t.Run("set and set_once semantics", func(t *testing.T) {
		pid := personTestProject(t, q)
		_, err := svc.Identify(ctx, pid, person.IdentifyInput{
			DistinctID: "user_3", Set: map[string]any{"plan": "free", "name": "A"}, SetOnce: map[string]any{"first_seen": "d1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		p, err := svc.Identify(ctx, pid, person.IdentifyInput{
			DistinctID: "user_3", Set: map[string]any{"plan": "pro"}, SetOnce: map[string]any{"first_seen": "d2", "src": "ads"},
		})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"plan": "pro", "name": "A", "first_seen": "d1", "src": "ads"}
		if fmt.Sprint(p.Properties) != fmt.Sprint(want) {
			t.Fatalf("props=%v want %v", p.Properties, want)
		}
	})

	t.Run("capture creates person and applies properties", func(t *testing.T) {
		pid := personTestProject(t, q)
		for i := 0; i < 2; i++ {
			if err := svc.Capture(ctx, pid, "visitor_1", map[string]any{"k": i}, map[string]any{"once": i}); err != nil {
				t.Fatal(err)
			}
		}
		var props []byte
		if err := c.DBPool.QueryRow(ctx, `SELECT p.properties FROM persons p JOIN person_distinct_ids d ON d.person_id=p.id WHERE d.project_id=$1 AND d.distinct_id='visitor_1'`, pid).Scan(&props); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		json.Unmarshal(props, &got)
		if got["k"] != float64(1) || got["once"] != float64(0) {
			t.Fatalf("props=%v", got)
		}
		if p, m, o := personCount(t, c.DBPool, pid); p != 1 || m != 1 || o != 0 {
			t.Fatalf("persons=%d mappings=%d orphans=%d", p, m, o)
		}
	})

	t.Run("invalid and reserved ids rejected", func(t *testing.T) {
		pid := personTestProject(t, q)
		if _, err := svc.Identify(ctx, pid, person.IdentifyInput{DistinctID: "null"}); err == nil {
			t.Error("reserved distinct_id accepted")
		}
		if _, err := svc.Identify(ctx, pid, person.IdentifyInput{DistinctID: "user_x", AnonymousID: "undefined"}); err == nil {
			t.Error("reserved anonymous_id accepted")
		}
		if _, err := svc.Alias(ctx, pid, "", "x"); err == nil {
			t.Error("empty alias accepted")
		}
		if p, _, _ := personCount(t, c.DBPool, pid); p != 0 {
			t.Fatalf("persons=%d want 0", p)
		}
	})

	t.Run("cross-project isolation", func(t *testing.T) {
		p1, p2 := personTestProject(t, q), personTestProject(t, q)
		personTestEvent(t, q, p2, "anon_x", now)
		personTestEvent(t, q, p2, "user_x", now)
		a, err := svc.Identify(ctx, p1, person.IdentifyInput{DistinctID: "user_x", AnonymousID: "anon_x"})
		if err != nil {
			t.Fatal(err)
		}
		if got := uniqueUsers(t, c, q, p2, now); got != 2 {
			t.Fatalf("project 2 unique=%d want 2 (merge leaked)", got)
		}
		b, err := svc.Identify(ctx, p2, person.IdentifyInput{DistinctID: "user_x"})
		if err != nil || b.ID == a.ID {
			t.Fatalf("persons shared across projects: %v %v", b, err)
		}
		if _, m, _ := personCount(t, c.DBPool, p1); m != 2 {
			t.Fatalf("project 1 mappings=%d want 2", m)
		}
	})

	t.Run("concurrent identify and capture leave no orphans or duplicates", func(t *testing.T) {
		pid := personTestProject(t, q)
		const n = 20
		var wg sync.WaitGroup
		errs := make(chan error, n*3)
		for i := 0; i < n; i++ {
			anon := fmt.Sprintf("anon_%d", i)
			wg.Add(3)
			go func() {
				defer wg.Done()
				if _, err := svc.Identify(ctx, pid, person.IdentifyInput{DistinctID: "user_c", AnonymousID: anon}); err != nil {
					errs <- err
				}
			}()
			go func() { defer wg.Done(); errs <- svc.Capture(ctx, pid, anon, nil, nil) }()
			go func() { defer wg.Done(); errs <- svc.Capture(ctx, pid, "user_c", nil, nil) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent op: %v", err)
			}
		}
		if p, m, o := personCount(t, c.DBPool, pid); p != 1 || m != n+1 || o != 0 {
			t.Fatalf("persons=%d mappings=%d orphans=%d want 1/%d/0", p, m, o, n+1)
		}
	})

	t.Run("backfill migration maps existing distinct ids idempotently", func(t *testing.T) {
		pid := personTestProject(t, q)
		personTestEvent(t, q, pid, "legacy_1", now)
		personTestEvent(t, q, pid, "legacy_2", now)
		sql, err := os.ReadFile("../../../migrations/app/000024_backfill_persons.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if _, err := c.DBPool.Exec(ctx, string(sql)); err != nil {
				t.Fatalf("backfill run %d: %v", i, err)
			}
		}
		if p, m, o := personCount(t, c.DBPool, pid); p != 2 || m != 2 || o != 0 {
			t.Fatalf("persons=%d mappings=%d orphans=%d want 2/2/0", p, m, o)
		}
	})
}

func TestPersonHTTP(t *testing.T) {
	c, _, q := personTestEnv(t)
	server := app.SetupWithContainer(withFakeAuth(c))
	_ = q

	projBody, _ := json.Marshal(map[string]string{"name": "Person HTTP " + uuid.NewString()[:8]})
	cr := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(projBody))
	cr.Header.Set("Content-Type", "application/json")
	cr.Header.Set("Cookie", "user1_session_cookie")
	cresp, err := server.Test(cr)
	if err != nil {
		t.Fatal(err)
	}
	var pw struct {
		Data struct {
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	json.NewDecoder(cresp.Body).Decode(&pw)
	key := pw.Data.APIKey

	post := func(path, auth string, body map[string]any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		resp, err := server.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	if code, _ := post("/api/v1/identify", "", map[string]any{"distinct_id": "u1"}); code != http.StatusUnauthorized {
		t.Errorf("no key: %d", code)
	}
	if code, _ := post("/api/v1/identify", key, map[string]any{"distinct_id": "null"}); code != http.StatusBadRequest {
		t.Errorf("reserved id: %d", code)
	}
	code, out := post("/api/v1/identify", key, map[string]any{"distinct_id": "u1", "anonymous_id": "a1", "$set": map[string]any{"plan": "pro"}})
	if code != http.StatusOK {
		t.Fatalf("identify: %d %v", code, out)
	}
	if code, _ := post("/api/v1/identify", key, map[string]any{"distinct_id": "u2"}); code != http.StatusOK {
		t.Fatalf("identify u2: %d", code)
	}
	if code, out := post("/api/v1/alias", key, map[string]any{"distinct_id": "u1", "alias": "u2"}); code != http.StatusConflict {
		t.Errorf("alias identified pair: %d %v", code, out)
	}
	if code, _ := post("/api/v1/alias", key, map[string]any{"distinct_id": "u1", "alias": "legacy"}); code != http.StatusOK {
		t.Errorf("alias: %d", code)
	}
}

func TestPersonWorkerCapture(t *testing.T) {
	c, _, q := personTestEnv(t)
	ctx := context.Background()
	pid := personTestProject(t, q)

	w, err := worker.NewConsumerWorker(c.DBPool, c.RedisClient, c.Logger, worker.Config{
		StreamKey: "events:stream", GroupName: "kinetiq-workers-person-" + uuid.NewString(),
		ConsumerName: "person-w", BatchSize: 10, BlockTimeout: 100 * time.Millisecond, MaxRetries: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	evts := []ingestion.EventPayload{
		{EventID: uuid.NewString(), EventName: "signup", OccurredAt: &now, DistinctID: "w_user",
			Properties: map[string]interface{}{"$set": map[string]any{"plan": "pro"}, "$set_once": map[string]any{"src": "ads"}}},
		{EventID: uuid.NewString(), EventName: "x", OccurredAt: &now, DistinctID: "w_user",
			Properties: map[string]interface{}{"$set": map[string]any{"plan": "team"}, "$set_once": map[string]any{"src": "organic"}}},
	}
	if _, err := c.IngestionService.EnqueueEvents(ctx, pid, evts); err != nil {
		t.Fatal(err)
	}
	if n, err := w.ProcessBatch(ctx); err != nil || n != 2 {
		t.Fatalf("processed=%d err=%v", n, err)
	}
	var props []byte
	if err := c.DBPool.QueryRow(ctx, `SELECT p.properties FROM persons p JOIN person_distinct_ids d ON d.person_id=p.id WHERE d.project_id=$1 AND d.distinct_id='w_user'`, pid).Scan(&props); err != nil {
		t.Fatalf("person not created by worker: %v", err)
	}
	var got map[string]any
	json.Unmarshal(props, &got)
	if got["plan"] != "team" || got["src"] != "ads" {
		t.Fatalf("props=%v", got)
	}
}
