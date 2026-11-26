package response_test

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/response"
)

func offsetOf(t *testing.T, query string, legacy bool) response.OffsetParams {
	t.Helper()
	var got response.OffsetParams
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		got = response.ParseOffsetParams(c, legacy)
		return nil
	})
	if _, err := app.Test(httptest.NewRequest("GET", "/"+query, nil)); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestParseOffsetParams(t *testing.T) {
	cases := []struct {
		name, q            string
		legacy             bool
		page, limit, offst int
	}{
		{"defaults", "", false, 1, 20, 0},
		{"explicit", "?page=3&limit=10", false, 3, 10, 20},
		{"clamp max", "?limit=1000", false, 1, 100, 0},
		{"page<1", "?page=0&limit=5", false, 1, 5, 0},
		{"negative page", "?page=-4", false, 1, 20, 0},
		{"non numeric", "?page=abc&limit=xyz", false, 1, 20, 0},
		{"limit<=0", "?limit=0", false, 1, 20, 0},
		{"legacy offset ignored when disabled", "?offset=40", false, 1, 20, 0},
		{"legacy offset", "?offset=40&limit=20", true, 3, 20, 40},
		{"legacy offset invalid", "?offset=-1", true, 1, 20, 0},
		{"page wins over legacy offset", "?page=2&offset=40&limit=20", true, 2, 20, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := offsetOf(t, tc.q, tc.legacy)
			if p.Page != tc.page || p.Limit != tc.limit || p.Offset != tc.offst {
				t.Fatalf("got %+v, want page=%d limit=%d offset=%d", p, tc.page, tc.limit, tc.offst)
			}
		})
	}
}

func TestPaginatedEnvelope(t *testing.T) {
	p := response.OffsetParams{Page: 2, Limit: 20, Offset: 20}
	env := response.Paginated([]int{1, 2}, p, 137)
	b, _ := json.Marshal(env)
	var out struct {
		Items      []int `json:"items"`
		Pagination struct {
			Page       int   `json:"page"`
			Limit      int   `json:"limit"`
			Total      int64 `json:"total"`
			TotalPages int   `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 || out.Pagination.Total != 137 || out.Pagination.TotalPages != 7 || out.Pagination.Page != 2 {
		t.Fatalf("unexpected envelope: %s", b)
	}
}

func TestPaginatedEmptySet(t *testing.T) {
	var nilItems []string
	env := response.Paginated(nilItems, response.OffsetParams{Page: 1, Limit: 20}, 0)
	b, _ := json.Marshal(env)
	var out map[string]json.RawMessage
	_ = json.Unmarshal(b, &out)
	if string(out["items"]) != "[]" {
		t.Fatalf("nil items must serialise as [], got %s", out["items"])
	}
	var pg struct {
		TotalPages int `json:"total_pages"`
	}
	_ = json.Unmarshal(out["pagination"], &pg)
	if pg.TotalPages != 0 {
		t.Fatalf("empty set total_pages = %d, want 0", pg.TotalPages)
	}
}

func TestWindow(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	if got := response.Window(items, response.OffsetParams{Page: 1, Limit: 2, Offset: 0}); len(got) != 2 || got[0] != 1 {
		t.Fatalf("page1 = %v", got)
	}
	if got := response.Window(items, response.OffsetParams{Page: 3, Limit: 2, Offset: 4}); len(got) != 1 || got[0] != 5 {
		t.Fatalf("last page = %v", got)
	}
	if got := response.Window(items, response.OffsetParams{Page: 9, Limit: 2, Offset: 16}); len(got) != 0 || got == nil {
		t.Fatalf("beyond range = %#v, want empty non-nil", got)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	in := response.Cursor{T: time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC), ID: "evt-1"}
	out, err := response.DecodeCursor(response.EncodeCursor(in))
	if err != nil {
		t.Fatal(err)
	}
	if !out.T.Equal(in.T) || out.ID != in.ID {
		t.Fatalf("round trip mismatch: %+v vs %+v", out, in)
	}
}

func TestDecodeCursorMalformed(t *testing.T) {
	for _, s := range []string{"!!!", "bm90LWpzb24", "e30" /* {} */} {
		if _, err := response.DecodeCursor(s); err == nil {
			t.Errorf("DecodeCursor(%q) should fail", s)
		}
	}
}

func cursorOf(t *testing.T, query string, def, max int) (response.CursorParams, error) {
	t.Helper()
	var got response.CursorParams
	var gotErr error
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		got, gotErr = response.ParseCursorParams(c, def, max)
		return nil
	})
	if _, err := app.Test(httptest.NewRequest("GET", "/"+query, nil)); err != nil {
		t.Fatal(err)
	}
	return got, gotErr
}

func TestParseCursorParams(t *testing.T) {
	p, err := cursorOf(t, "", 50, 200)
	if err != nil || p.Limit != 50 || p.Cursor != nil {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	if p, _ = cursorOf(t, "?limit=999", 50, 200); p.Limit != 200 {
		t.Fatalf("clamp: %d", p.Limit)
	}
	if p, _ = cursorOf(t, "?limit=zzz", 50, 200); p.Limit != 50 {
		t.Fatalf("non numeric: %d", p.Limit)
	}
	enc := response.EncodeCursor(response.Cursor{T: time.Now().UTC(), ID: "9"})
	if p, err = cursorOf(t, "?cursor="+enc, 50, 200); err != nil || p.Cursor == nil || p.Cursor.ID != "9" {
		t.Fatalf("valid cursor: %+v %v", p, err)
	}
	if _, err = cursorOf(t, "?cursor=%25%25bad", 50, 200); err != response.ErrInvalidCursor {
		t.Fatalf("want ErrInvalidCursor, got %v", err)
	}
}

func TestCursorPaginated(t *testing.T) {
	last := response.Cursor{T: time.Now().UTC(), ID: "x"}
	more := response.CursorPaginated([]int{1}, 50, &last, true)
	b, _ := json.Marshal(more)
	var m struct {
		Pagination struct {
			NextCursor *string `json:"next_cursor"`
			HasMore    bool    `json:"has_more"`
			Limit      int     `json:"limit"`
		} `json:"pagination"`
	}
	_ = json.Unmarshal(b, &m)
	if m.Pagination.NextCursor == nil || !m.Pagination.HasMore || m.Pagination.Limit != 50 {
		t.Fatalf("more page: %s", b)
	}

	done := response.CursorPaginated([]int{}, 50, &last, false)
	b, _ = json.Marshal(done)
	if !jsonHas(b, `"next_cursor":null`) || !jsonHas(b, `"has_more":false`) {
		t.Fatalf("last page must have null next_cursor: %s", b)
	}
}

func TestInvalidCursorHTTP(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		if _, err := response.ParseCursorParams(c, 50, 200); err != nil {
			return response.InvalidCursor(c)
		}
		return nil
	})
	resp, _ := app.Test(httptest.NewRequest("GET", "/?cursor=garbage!", nil))
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 400 || !jsonHas(body, `"INVALID_CURSOR"`) {
		t.Fatalf("got %d %s", resp.StatusCode, body)
	}
}

func jsonHas(b []byte, sub string) bool { return strings.Contains(string(b), sub) }
