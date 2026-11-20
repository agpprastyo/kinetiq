package tests

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"kinetiq/internal/app"
)

func TestOrgRoleEnforcement(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	db := container.DBPool
	orgID := uuid.New()
	userIDViewer := "user_viewer_1"
	userIDOwner := "user_owner_1"

	// Insert test org & memberships
	_, err = db.Exec(ctx, "INSERT INTO organizations (id, name) VALUES ($1, $2)", orgID, "Acme Corp")
	if err != nil {
		t.Fatalf("failed to insert org: %v", err)
	}
	_, err = db.Exec(ctx, "INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, 'viewer'), ($1, $3, 'owner')", orgID, userIDViewer, userIDOwner)
	if err != nil {
		t.Fatalf("failed to insert members: %v", err)
	}

	svc := container.ProjectService

	// Viewer cannot perform manage actions
	canAdmin, err := svc.CanUserManageOrg(ctx, userIDViewer, orgID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if canAdmin {
		t.Errorf("expected viewer cannot manage org, got true")
	}

	// Owner can perform manage actions
	canAdminOwner, err := svc.CanUserManageOrg(ctx, userIDOwner, orgID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canAdminOwner {
		t.Errorf("expected owner can manage org, got false")
	}
}
