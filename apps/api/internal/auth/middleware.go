package auth

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"kinetiq/internal/response"
)

type IntrospectResponse struct {
	Authenticated bool `json:"authenticated"`
	User          struct {
		ID                      string   `json:"id"`
		Email                   string   `json:"email"`
		Role                    string   `json:"role"`
		IsPlatformStaff         bool     `json:"isPlatformStaff"`
		StaffRole               *string  `json:"staffRole"`
		PlatformPermissions     []string `json:"platformPermissions"`
		OrganizationPermissions []string `json:"organizationPermissions"`
	} `json:"user"`
	Session struct {
		ExpiresAt            string  `json:"expiresAt"`
		ActiveOrganizationID *string `json:"activeOrganizationId"`
	} `json:"session"`
}

type Client interface {
	ValidateSession(cookie string) (*IntrospectResponse, error)
}

type HTTPClient struct {
	baseURL        string
	internalSecret string
	httpClient     *http.Client
}

func NewHTTPClient(baseURL, internalSecret string) *HTTPClient {
	return &HTTPClient{
		baseURL:        strings.TrimRight(baseURL, "/"),
		internalSecret: internalSecret,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

func (c *HTTPClient) ValidateSession(cookie string) (*IntrospectResponse, error) {
	payload, err := json.Marshal(map[string]string{"cookie": cookie})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/internal/session/introspect", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Auth", c.internalSecret)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return &IntrospectResponse{Authenticated: false}, nil
	}

	var introspectResp IntrospectResponse
	if err := json.NewDecoder(res.Body).Decode(&introspectResp); err != nil {
		return nil, err
	}

	return &introspectResp, nil
}

func SessionMiddleware(authClient Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		cookie := c.Get("Cookie")
		if strings.TrimSpace(cookie) == "" {
			return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Missing authentication cookie")
		}

		res, err := authClient.ValidateSession(cookie)
		if err != nil {
			// Auth service error/timeout maps to 503 Service Unavailable
			return response.Error(c, fiber.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Authentication service unavailable")
		}

		if !res.Authenticated || res.User.ID == "" {
			return response.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired session")
		}

		c.Locals("userID", res.User.ID)
		c.Locals("userEmail", res.User.Email)
		c.Locals("userRole", res.User.Role)
		c.Locals("isPlatformStaff", res.User.IsPlatformStaff)
		if res.User.StaffRole != nil {
			c.Locals("staffRole", *res.User.StaffRole)
		} else {
			c.Locals("staffRole", "")
		}
		c.Locals("platformPermissions", res.User.PlatformPermissions)
		c.Locals("organizationPermissions", res.User.OrganizationPermissions)
		if res.Session.ActiveOrganizationID != nil {
			c.Locals("activeOrganizationID", *res.Session.ActiveOrganizationID)
		} else {
			c.Locals("activeOrganizationID", "")
		}
		return c.Next()
	}
}

// RequirePlatformStaff ensures the authenticated user is authorized as internal platform operations staff
func RequirePlatformStaff(allowedStaffRoles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		isStaff, _ := c.Locals("isPlatformStaff").(bool)
		if !isStaff {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Kinetiq platform staff access required")
		}

		if len(allowedStaffRoles) > 0 {
			userStaffRole, _ := c.Locals("staffRole").(string)
			// 'superadmin' staff role always bypasses granular role restrictions
			if userStaffRole == "superadmin" {
				return c.Next()
			}
			matched := false
			for _, r := range allowedStaffRoles {
				if r == userStaffRole {
					matched = true
					break
				}
			}
			if !matched {
				return response.Error(c, fiber.StatusForbidden, "INSUFFICIENT_STAFF_PERMISSIONS", "Action not permitted for staff role: "+userStaffRole)
			}
		}

		return c.Next()
	}
}

// RequirePlatformPermission checks whether the operator has a specific granular permission
func RequirePlatformPermission(resource, action string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		isStaff, _ := c.Locals("isPlatformStaff").(bool)
		if !isStaff {
			return response.Error(c, fiber.StatusForbidden, "FORBIDDEN", "Kinetiq platform staff access required")
		}

		userStaffRole, _ := c.Locals("staffRole").(string)
		if userStaffRole == "superadmin" {
			return c.Next()
		}

		perms, _ := c.Locals("platformPermissions").([]string)
		target := resource + ":" + action
		for _, p := range perms {
			if p == target || p == "*:*" || p == resource+":*" {
				return c.Next()
			}
		}

		return response.Error(c, fiber.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Missing permission: "+target)
	}
}

// RequireOrganizationPermission checks whether the tenant user has a specific granular permission in their workspace
func RequireOrganizationPermission(resource, action string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole, _ := c.Locals("userRole").(string)
		if userRole == "owner" {
			return c.Next()
		}

		perms, _ := c.Locals("organizationPermissions").([]string)
		target := resource + ":" + action
		for _, p := range perms {
			if p == target || p == "*:*" || p == resource+":*" {
				return c.Next()
			}
		}

		return response.Error(c, fiber.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Missing workspace permission: "+target)
	}
}

// RequirePlatformAdmin ensures the authenticated user possesses owner or admin privileges across the platform (deprecated: use RequirePlatformStaff)
func RequirePlatformAdmin() fiber.Handler {
	return RequirePlatformStaff()
}

func VerifyInternalSecret(headerValue, expectedSecret string) bool {
	if headerValue == "" || expectedSecret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(headerValue), []byte(expectedSecret)) == 1
}
