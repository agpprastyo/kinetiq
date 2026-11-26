package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

type cursorPage struct {
	Data struct {
		Items []struct {
			ID      int64  `json:"id"`
			EventID string `json:"event_id"`
		} `json:"items"`
		Pagination struct {
			Limit      int     `json:"limit"`
			NextCursor *string `json:"next_cursor"`
			HasMore    bool    `json:"has_more"`
		} `json:"pagination"`
	} `json:"data"`
}

func fetchCursorPage(t *testing.T, server interface {
	Test(*http.Request, ...int) (*http.Response, error)
}, url, cookie string) cursorPage {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Cookie", cookie)
	resp, err := server.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s failed: %v status=%d", url, err, resp.StatusCode)
	}
	var p cursorPage
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return p
}

// Keyset pagination must return every row exactly once, even when timestamps collide,
// and must stay tenant-scoped.
func TestTimelineCursorDuplicateTimestampsAndIDOR(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))
	queries := db.New(container.DBPool)

	projectID := uuid.New()
	if _, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "user-1-id",
		Name:        "Pagination Timeline " + projectID.String()[:8],
		Status:      "active",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	same := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	want := map[string]bool{}
	for i := 0; i < 7; i++ {
		id := fmt.Sprintf("evt_pg_%02d", i)
		want[id] = true
		ts := same
		if i >= 5 {
			ts = same.Add(time.Minute)
		}
		if err := queries.InsertEvent(ctx, db.InsertEventParams{
			ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
			EventID:    id,
			EventName:  "click",
			OccurredAt: pgtype.Timestamptz{Time: ts, Valid: true},
			ReceivedAt: pgtype.Timestamptz{Time: ts, Valid: true},
			DistinctID: pgtype.Text{String: "pg_user", Valid: true},
			Properties: []byte("{}"),
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	base := fmt.Sprintf("/api/v1/projects/%s/analytics/users/pg_user/timeline?limit=2", projectID)
	seen := map[string]int{}
	url := base
	for i := 0; i < 10; i++ {
		p := fetchCursorPage(t, server, url, "user1_session_cookie")
		for _, it := range p.Data.Items {
			seen[it.EventID]++
		}
		if !p.Data.Pagination.HasMore {
			if p.Data.Pagination.NextCursor != nil {
				t.Fatalf("last page must have null next_cursor")
			}
			break
		}
		if p.Data.Pagination.NextCursor == nil {
			t.Fatalf("has_more without next_cursor")
		}
		url = base + "&cursor=" + *p.Data.Pagination.NextCursor
	}
	for id := range want {
		if seen[id] != 1 {
			t.Errorf("event %s seen %d times, want exactly 1", id, seen[id])
		}
	}

	// IDOR: another user cannot page this timeline, with or without a cursor.
	req := httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Cookie", "user2_session_cookie")
	resp, _ := server.Test(req)
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-tenant timeline: got %d, want 403/404", resp.StatusCode)
	}

	// Legacy `before` still works.
	legacy := fetchCursorPage(t, server, base+"&before="+same.Add(30*time.Second).Format(time.RFC3339), "user1_session_cookie")
	if len(legacy.Data.Items) == 0 {
		t.Errorf("legacy before= returned nothing")
	}
}

// Rows removed between page fetches (replay/purge) must not cause later rows to be skipped.
func TestDLQCursorStableUnderMutation(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	server := app.SetupWithContainer(withFakeAuth(container))
	pool := container.DBPool

	_, _ = pool.Exec(ctx, `DELETE FROM event_dead_letters`)
	ts := time.Now().UTC().Truncate(time.Second)
	var ids []int64
	for i := 0; i < 6; i++ {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO event_dead_letters (event_id, payload, reason, attempts, status, failed_at)
			 VALUES ($1, '{}'::jsonb, 'pg test', 1, 'failed', $2) RETURNING id`,
			fmt.Sprintf("evt_dlq_pg_%d", i), ts).Scan(&id); err != nil {
			t.Fatalf("seed: %v", err)
		}
		ids = append(ids, id)
	}
	defer pool.Exec(ctx, `DELETE FROM event_dead_letters`)

	cookie := "admin_session_cookie"
	first := fetchCursorPage(t, server, "/api/v1/ops/dlq?limit=2", cookie)
	if len(first.Data.Items) != 2 || !first.Data.Pagination.HasMore || first.Data.Pagination.NextCursor == nil {
		t.Fatalf("page1: %+v", first.Data)
	}
	seen := map[int64]bool{}
	for _, it := range first.Data.Items {
		seen[it.ID] = true
		// simulate replay removing the already-served rows
		_, _ = pool.Exec(ctx, `DELETE FROM event_dead_letters WHERE id = $1`, it.ID)
	}

	next := *first.Data.Pagination.NextCursor
	for i := 0; i < 5; i++ {
		p := fetchCursorPage(t, server, "/api/v1/ops/dlq?limit=2&cursor="+next, cookie)
		for _, it := range p.Data.Items {
			if seen[it.ID] {
				t.Errorf("duplicate id %d", it.ID)
			}
			seen[it.ID] = true
		}
		if !p.Data.Pagination.HasMore {
			break
		}
		next = *p.Data.Pagination.NextCursor
	}
	for _, id := range ids {
		if !seen[id] {
			t.Errorf("dead letter %d skipped after mutation", id)
		}
	}
}
