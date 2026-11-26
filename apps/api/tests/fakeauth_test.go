package tests

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/app"
	"kinetiq/internal/auth"
)

// fakeAuthClient resolves test cookies in-process so tests never depend on the auth service.
type fakeAuthClient struct{}

func (fakeAuthClient) ValidateSession(cookie string) (*auth.IntrospectResponse, error) {
	resp := &auth.IntrospectResponse{Authenticated: true}
	staff := func(id, email, role, staffRole string) {
		resp.User.ID, resp.User.Email, resp.User.Role = id, email, role
		resp.User.IsPlatformStaff = true
		resp.User.StaffRole = &staffRole
	}
	switch {
	case strings.HasPrefix(cookie, "admin_session"):
		staff("admin-1-id", "admin@kinetiq.dev", "owner", "superadmin")
	case strings.HasPrefix(cookie, "ops_session"):
		staff("ops-1-id", "ops@kinetiq.dev", "user", "ops")
	case strings.HasPrefix(cookie, "user1_session"):
		resp.User.ID, resp.User.Email, resp.User.Role = "user-1-id", "user1@example.com", "owner"
	case strings.HasPrefix(cookie, "user2_session"):
		resp.User.ID, resp.User.Email, resp.User.Role = "user-2-id", "user2@example.com", "user"
	default:
		return &auth.IntrospectResponse{Authenticated: false}, nil
	}
	return resp, nil
}

func withFakeAuth(c *app.Container) *app.Container {
	c.AuthClient = fakeAuthClient{}
	return c
}

func setupTestServer() *fiber.App {
	c, err := app.NewContainer(context.Background())
	if err != nil {
		panic(err)
	}
	return app.SetupWithContainer(withFakeAuth(c))
}
