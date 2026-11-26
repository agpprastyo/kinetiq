package dlq_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/dlq"
	"kinetiq/internal/response"
)

type fakeDLQ struct {
	dlq.Service
	got   dlq.ListQuery
	items []dlq.DeadLetterItem
	more  bool
}

func (f *fakeDLQ) List(_ context.Context, q dlq.ListQuery) ([]dlq.DeadLetterItem, *dlq.DLQSummary, bool, error) {
	f.got = q
	return f.items, &dlq.DLQSummary{TotalItems: 3}, f.more, nil
}

func call(t *testing.T, svc dlq.Service, url string) (int, map[string]any) {
	t.Helper()
	app := fiber.New()
	app.Get("/ops/dlq", dlq.NewHandler(svc, nil).ListDeadLetters)
	resp, err := app.Test(httptest.NewRequest("GET", url, nil))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func TestListDeadLetters_DefaultsAndClamp(t *testing.T) {
	f := &fakeDLQ{}
	call(t, f, "/ops/dlq")
	if f.got.Limit != 50 || f.got.Cursor != nil || f.got.Offset != 0 {
		t.Fatalf("defaults: %+v", f.got)
	}
	call(t, f, "/ops/dlq?limit=9999")
	if f.got.Limit != 200 {
		t.Fatalf("clamp: %d", f.got.Limit)
	}
	call(t, f, "/ops/dlq?limit=abc")
	if f.got.Limit != 50 {
		t.Fatalf("non-numeric: %d", f.got.Limit)
	}
}

func TestListDeadLetters_EmptyAndLastPage(t *testing.T) {
	status, body := call(t, &fakeDLQ{items: []dlq.DeadLetterItem{}}, "/ops/dlq")
	data := body["data"].(map[string]any)
	pg := data["pagination"].(map[string]any)
	if status != 200 || len(data["items"].([]any)) != 0 || pg["next_cursor"] != nil || pg["has_more"] != false {
		t.Fatalf("empty: %d %v", status, body)
	}
	if data["summary"] == nil {
		t.Fatal("summary must stay alongside items")
	}
}

func TestListDeadLetters_NextCursor(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeDLQ{items: []dlq.DeadLetterItem{{ID: 9, FailedAt: ts}, {ID: 8, FailedAt: ts}}, more: true}
	_, body := call(t, f, "/ops/dlq?limit=2")
	pg := body["data"].(map[string]any)["pagination"].(map[string]any)
	next, _ := pg["next_cursor"].(string)
	cur, err := response.DecodeCursor(next)
	if err != nil || cur.ID != "8" || !cur.T.Equal(ts) || pg["has_more"] != true {
		t.Fatalf("next cursor: %v %v", pg, err)
	}

	call(t, f, "/ops/dlq?limit=2&cursor="+next)
	if f.got.Cursor == nil || f.got.Cursor.ID != "8" {
		t.Fatalf("cursor not forwarded: %+v", f.got)
	}
}

func TestListDeadLetters_InvalidCursor(t *testing.T) {
	status, body := call(t, &fakeDLQ{}, "/ops/dlq?cursor=not-a-cursor!")
	if status != 400 || body["error"].(map[string]any)["code"] != "INVALID_CURSOR" {
		t.Fatalf("got %d %v", status, body)
	}
}

func TestListDeadLetters_LegacyOffset(t *testing.T) {
	f := &fakeDLQ{}
	call(t, f, "/ops/dlq?offset=30")
	if f.got.Offset != 30 {
		t.Fatalf("legacy offset lost: %+v", f.got)
	}
	cur := response.EncodeCursor(response.Cursor{T: time.Now().UTC(), ID: "1"})
	call(t, f, "/ops/dlq?offset=30&cursor="+cur)
	if f.got.Offset != 0 || f.got.Cursor == nil {
		t.Fatalf("cursor must win over offset: %+v", f.got)
	}
}
