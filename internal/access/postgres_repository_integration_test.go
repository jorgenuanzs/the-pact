//go:build integration

package access_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

func TestInvitationPersonalTokenAndRevocationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{ApplicationName: "pact-access-integration-test"})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	projectResult, err := projects.NewService(
		config.DefaultLocalOrganizationID,
		projects.NewPostgresRepository(pool),
	).Create(ctx, "access-project-"+suffix, projects.CreateInput{
		Name: "Access project " + suffix,
		Slug: "access-project-" + suffix,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	const bootstrapToken = "integration-bootstrap-token-long-enough"
	service := access.NewService(
		config.DefaultLocalOrganizationID,
		bootstrapToken,
		access.NewPostgresRepository(pool),
	)
	administrator, err := service.Authenticate(ctx, bootstrapToken)
	if err != nil {
		t.Fatalf("authenticate bootstrap: %v", err)
	}
	created, err := service.CreateInvitation(ctx, administrator, projectResult.Project.ID, access.CreateInvitationInput{
		Email: "collaborator-" + suffix + "@example.com", Role: "contributor", ExpiresAfter: time.Hour,
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if created.Secret == "" {
		t.Fatal("invitation secret is empty")
	}
	accepted, err := service.AcceptInvitation(ctx, access.AcceptInvitationInput{
		Secret: created.Secret, DisplayName: "Integration collaborator", TokenName: "Integration computer",
	})
	if err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	if accepted.AccessToken == "" || accepted.ProjectRole != "contributor" {
		t.Fatalf("accepted invitation = %#v", accepted)
	}
	if _, err := service.AcceptInvitation(ctx, access.AcceptInvitationInput{
		Secret: created.Secret, DisplayName: "Replay", TokenName: "Replay",
	}); !errors.Is(err, access.ErrInvitationInvalid) {
		t.Fatalf("replayed invitation error = %v", err)
	}
	principal, err := service.Authenticate(ctx, accepted.AccessToken)
	if err != nil {
		t.Fatalf("authenticate personal token: %v", err)
	}
	if principal.ID != accepted.Principal.ID || principal.Bootstrap {
		t.Fatalf("personal principal = %#v", principal)
	}
	if err := service.RequireProjectRole(ctx, principal, projectResult.Project.ID, "contributor"); err != nil {
		t.Fatalf("contributor authorization: %v", err)
	}
	if _, err := service.CreateInvitation(ctx, principal, projectResult.Project.ID, access.CreateInvitationInput{
		Email: "forbidden@example.com", Role: "viewer",
	}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("contributor invitation error = %v", err)
	}
	if err := service.RevokeCurrentToken(ctx, principal); err != nil {
		t.Fatalf("revoke personal token: %v", err)
	}
	if _, err := service.Authenticate(ctx, accepted.AccessToken); !errors.Is(err, access.ErrUnauthorized) {
		t.Fatalf("authentication after revocation error = %v", err)
	}
}
