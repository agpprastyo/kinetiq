package response

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

var ErrInvalidCursor = errors.New("invalid cursor")

type OffsetParams struct {
	Page   int
	Limit  int
	Offset int
}

type OffsetMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type OffsetPage[T any] struct {
	Items      []T        `json:"items"`
	Pagination OffsetMeta `json:"pagination"`
}

// ParseOffsetParams reads ?page&limit. Invalid values fall back to defaults; limit is clamped.
// allowLegacyOffset maps a deprecated ?offset onto the page when ?page is absent.
func ParseOffsetParams(c *fiber.Ctx, allowLegacyOffset bool) OffsetParams {
	limit := DefaultPageLimit
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 {
		limit = n
	}
	if limit > MaxPageLimit {
		limit = MaxPageLimit
	}

	if allowLegacyOffset && c.Query("page") == "" {
		if n, err := strconv.Atoi(c.Query("offset")); err == nil && n > 0 {
			slog.Warn("deprecated query param 'offset'; use 'page'", slog.String("path", c.Path()))
			return OffsetParams{Page: n/limit + 1, Limit: limit, Offset: n}
		}
	}

	page := 1
	if n, err := strconv.Atoi(c.Query("page")); err == nil && n > 1 {
		page = n
	}
	return OffsetParams{Page: page, Limit: limit, Offset: (page - 1) * limit}
}

func NewOffsetMeta(p OffsetParams, total int64) OffsetMeta {
	pages := 0
	if p.Limit > 0 {
		pages = int((total + int64(p.Limit) - 1) / int64(p.Limit))
	}
	return OffsetMeta{Page: p.Page, Limit: p.Limit, Total: total, TotalPages: pages}
}

func Paginated[T any](items []T, p OffsetParams, total int64) OffsetPage[T] {
	if items == nil {
		items = []T{}
	}
	return OffsetPage[T]{Items: items, Pagination: NewOffsetMeta(p, total)}
}

// Window slices an in-memory list for the requested page (for small per-project lists).
func Window[T any](items []T, p OffsetParams) []T {
	if p.Offset >= len(items) {
		return []T{}
	}
	end := p.Offset + p.Limit
	if end > len(items) {
		end = len(items)
	}
	return items[p.Offset:end]
}

// PaginateSlice windows an in-memory list and wraps it in the offset envelope.
func PaginateSlice[T any](items []T, p OffsetParams) OffsetPage[T] {
	return Paginated(Window(items, p), p, int64(len(items)))
}

type Cursor struct {
	T  time.Time `json:"t"`
	ID string    `json:"id"`
}

func EncodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeCursor(s string) (Cursor, error) {
	var c Cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || json.Unmarshal(b, &c) != nil || c.T.IsZero() || c.ID == "" {
		return Cursor{}, ErrInvalidCursor
	}
	return c, nil
}

type CursorParams struct {
	Cursor *Cursor
	Limit  int
}

// ParseCursorParams reads ?cursor&limit. A malformed cursor returns ErrInvalidCursor.
func ParseCursorParams(c *fiber.Ctx, defLimit, maxLimit int) (CursorParams, error) {
	p := CursorParams{Limit: defLimit}
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 {
		p.Limit = n
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	if s := c.Query("cursor"); s != "" {
		cur, err := DecodeCursor(s)
		if err != nil {
			return p, err
		}
		p.Cursor = &cur
	}
	return p, nil
}

type CursorMeta struct {
	Limit      int     `json:"limit"`
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

type CursorPage[T any] struct {
	Items      []T        `json:"items"`
	Pagination CursorMeta `json:"pagination"`
}

// CursorPaginated builds the envelope; last is the final item's keyset, used only when hasMore.
func CursorPaginated[T any](items []T, limit int, last *Cursor, hasMore bool) CursorPage[T] {
	if items == nil {
		items = []T{}
	}
	m := CursorMeta{Limit: limit, HasMore: hasMore}
	if hasMore && last != nil {
		s := EncodeCursor(*last)
		m.NextCursor = &s
	}
	return CursorPage[T]{Items: items, Pagination: m}
}

func InvalidCursor(c *fiber.Ctx) error {
	return Error(c, fiber.StatusBadRequest, "INVALID_CURSOR", "Malformed cursor")
}
