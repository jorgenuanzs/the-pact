//go:build integration

package workspaces_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

func TestWorkspaceFoundationBackfillsCreatesAndMovesProjects(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{
		ApplicationName: "pact-workspaces-integration-test",
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	projectService := projects.NewService(
		config.DefaultLocalOrganizationID,
		projects.NewPostgresRepository(pool),
	)
	first, err := projectService.Create(ctx, "workspace-project-a-"+suffix, projects.CreateInput{
		Name: "Workspace Project A " + suffix,
		Slug: "workspace-project-a-" + suffix,
	})
	if err != nil {
		t.Fatalf("create first project: %v", err)
	}
	second, err := projectService.Create(ctx, "workspace-project-b-"+suffix, projects.CreateInput{
		Name: "Workspace Project B " + suffix,
		Slug: "workspace-project-b-" + suffix,
	})
	if err != nil {
		t.Fatalf("create second project: %v", err)
	}

	workspaceService := workspaces.NewService(
		config.DefaultLocalOrganizationID,
		workspaces.NewPostgresRepository(pool),
	)
	defaultWorkspace, err := workspaceService.Get(ctx, first.Project.Slug)
	if err != nil {
		t.Fatalf("get default workspace: %v", err)
	}
	if len(defaultWorkspace.Projects) != 1 || defaultWorkspace.Projects[0].ID != first.Project.ID {
		t.Fatalf("default workspace projects = %#v", defaultWorkspace.Projects)
	}

	input := workspaces.CreateInput{
		Name:       "Shared Workspace " + suffix,
		Slug:       "shared-workspace-" + suffix,
		ProjectIDs: []string{first.Project.ID, second.Project.ID},
	}
	created, err := workspaceService.Create(ctx, "workspace-create-"+suffix, input)
	if err != nil {
		t.Fatalf("create shared workspace: %v", err)
	}
	if len(created.Workspace.Projects) != 2 {
		t.Fatalf("shared workspace projects = %#v", created.Workspace.Projects)
	}
	replayed, err := workspaceService.Create(ctx, "workspace-create-"+suffix, input)
	if err != nil || !replayed.Replayed || replayed.Workspace.ID != created.Workspace.ID {
		t.Fatalf("replayed result = %#v, error = %v", replayed, err)
	}

	beforeVersion := created.Workspace.Version
	unchanged, err := workspaceService.AttachProject(ctx, created.Workspace.ID, first.Project.ID)
	if err != nil {
		t.Fatalf("repeat project attachment: %v", err)
	}
	if unchanged.Version != beforeVersion {
		t.Fatalf("idempotent attachment changed version from %d to %d", beforeVersion, unchanged.Version)
	}
}
