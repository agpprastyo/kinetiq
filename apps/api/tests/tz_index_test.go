package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/app"
)

func setupContainer(t *testing.T) *app.Container {
	t.Helper()
	c, err := app.NewContainer(context.Background())
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	return withFakeAuth(c)
}

func TestAlertRulesProjectIDIndexExists(t *testing.T) {
	c := setupContainer(t)
	var n int
	if err := c.DBPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'alert_rules' AND indexname = 'idx_alert_rules_project_id'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("idx_alert_rules_project_id missing (n=%d err=%v)", n, err)
	}
}

func TestVolumeDayBucketsRespectTZ(t *testing.T) {
	ctx := context.Background()
	c := setupContainer(t)
	server := app.SetupWithContainer(c)
	pid := uuid.New()
	q := db.New(c.DBPool)
	if _, err := q.CreateProject(ctx, db.CreateProjectParams{ID: pgtype.UUID{Bytes: pid, Valid: true}, OwnerUserID: "user-1-id", Name: "TZ " + pid.String()[:8], Status: "active"}); err != nil {
		t.Fatal(err)
	}
	// 2026-01-01T20:00Z == 2026-01-02 03:00 in Asia/Jakarta (UTC+7)
	at := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	if err := q.InsertEvent(ctx, db.InsertEventParams{ProjectID: pgtype.UUID{Bytes: pid, Valid: true}, EventID: uuid.NewString(), EventName: "page_view",
		OccurredAt: pgtype.Timestamptz{Time: at, Valid: true}, ReceivedAt: pgtype.Timestamptz{Time: at, Valid: true}, Properties: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}

	get := func(tz string) (int, []string) {
		qs := url.Values{"from": {"2025-12-30T00:00:00Z"}, "to": {"2026-01-04T00:00:00Z"}, "interval": {"day"}}
		if tz != "" {
			qs.Set("tz", tz)
		}
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/analytics/volume?%s", pid, qs.Encode()), nil)
		req.Header.Set("Cookie", "user1_session_cookie")
		resp, err := server.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Data struct {
				Points []struct {
					Timestamp string `json:"timestamp"`
				} `json:"points"`
			} `json:"data"`
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(raw, &body)
		var ts []string
		for _, p := range body.Data.Points {
			ts = append(ts, p.Timestamp)
		}
		return resp.StatusCode, ts
	}

	if code, ts := get(""); code != 200 || len(ts) != 1 || ts[0] != "2026-01-01T00:00:00Z" {
		t.Errorf("default must bucket in explicit UTC, got %d %v", code, ts)
	}
	if code, ts := get("Asia/Jakarta"); code != 200 || len(ts) != 1 || ts[0] != "2026-01-01T17:00:00Z" {
		t.Errorf("Jakarta day bucket starts 2026-01-01T17:00Z, got %d %v", code, ts)
	}
	if code, _ := get("Mars/Phobos"); code != http.StatusBadRequest {
		t.Errorf("invalid tz must be 400, got %d", code)
	}
}
