package analytics

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestQueryTimeoutMapsTo504(t *testing.T) {
	h := &Handler{}
	app := fiber.New()
	app.Get("/ctx", QueryTimeout(10*time.Millisecond), func(c *fiber.Ctx) error {
		<-c.UserContext().Done()
		return h.fail(c, "slow", c.UserContext().Err())
	})
	app.Get("/pg", func(c *fiber.Ctx) error {
		return h.fail(c, "slow", &pgconn.PgError{Code: "57014"})
	})
	app.Get("/boom", func(c *fiber.Ctx) error {
		return h.fail(c, "boom", context.Canceled)
	})
	for path, want := range map[string]int{"/ctx": 504, "/pg": 504, "/boom": 500} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil), 2000)
		if err != nil || resp.StatusCode != want {
			t.Errorf("%s: want %d got %v err %v", path, want, resp, err)
		}
	}
}
