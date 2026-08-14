//go:build integration

package projectrepo_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projectrepo"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/repositorysync"
)

type snapshotProvider struct{ snapshot repositorysync.Snapshot }

func (p snapshotProvider) Fetch(context.Context, repositorysync.Reference) (repositorysync.Snapshot, error) {
	return p.snapshot, nil
}

func TestAttachMultipleRepositoriesAndKeepPrimaryRevisionProjection(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{ApplicationName: "pact-project-repository-integration-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	initialRevision := "0123456789abcdef0123456789abcdef01234567"
	projectService := projects.NewService(config.DefaultLocalOrganizationID, projects.NewPostgresRepository(pool))
	created, err := projectService.Create(ctx, "multi-repo-project-"+suffix, projects.CreateInput{
		Name: "Multi repository " + suffix, Slug: "multi-repo-" + suffix,
		CanonicalRevision: &initialRevision,
		RootRepository: &projects.SourceRepositoryInput{
			Slug: "backend", Name: "Backend", RemoteURL: "https://github.com/example/backend-" + suffix + ".git",
			DefaultBranch: "main", ObjectFormat: "sha1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	installationID := time.Now().UnixNano()
	backendID, frontendID := installationID+1, installationID+2
	_, err = pool.Exec(ctx, `
		INSERT INTO integrations.github_installations (
			organization_id, installation_id, account_id, account_login, account_type,
			repository_selection, permissions
		) VALUES ($1, $2, $3, 'example', 'Organization', 'selected', '{"contents":"read","metadata":"read"}')
	`, config.DefaultLocalOrganizationID, installationID, installationID+3)
	if err != nil {
		t.Fatal(err)
	}
	for _, repository := range []struct {
		id   int64
		name string
	}{
		{backendID, "backend-" + suffix}, {frontendID, "frontend-" + suffix},
	} {
		fullName := "example/" + repository.name
		_, err = pool.Exec(ctx, `
			INSERT INTO integrations.github_repositories (
				organization_id, github_repository_id, installation_id, owner_login,
				name, full_name, private, visibility, default_branch, html_url, clone_url
			) VALUES ($1, $2, $3, 'example', $4, $5, true, 'private', 'main', $6, $7)
		`, config.DefaultLocalOrganizationID, repository.id, installationID, repository.name,
			fullName, "https://github.com/"+fullName, "https://github.com/"+fullName+".git")
		if err != nil {
			t.Fatal(err)
		}
	}

	service := projectrepo.NewService(config.DefaultLocalOrganizationID, projectrepo.NewPostgresRepository(pool))
	backend, err := service.Attach(ctx, access.BootstrapPrincipalID, created.Project.ID, projectrepo.AttachInput{
		GitHubRepositoryID: backendID, Purpose: "backend", Primary: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if backend.ID != created.Project.RootRepository.ID || !backend.Primary {
		t.Fatalf("linked backend = %#v", backend)
	}
	frontend, err := service.Attach(ctx, access.BootstrapPrincipalID, created.Project.ID, projectrepo.AttachInput{
		GitHubRepositoryID: frontendID, Purpose: "frontend",
	})
	if err != nil {
		t.Fatal(err)
	}
	if frontend.Primary || frontend.Purpose != "frontend" {
		t.Fatalf("frontend = %#v", frontend)
	}
	repositories, err := service.List(ctx, created.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 2 {
		t.Fatalf("repository count = %d", len(repositories))
	}
	var attachedEvents int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM platform.events
		WHERE organization_id = $1 AND project_id = $2
		  AND event_type = 'pact.project.repository_attached.v1'
	`, config.DefaultLocalOrganizationID, created.Project.ID).Scan(&attachedEvents); err != nil {
		t.Fatal(err)
	}
	if attachedEvents != 2 {
		t.Fatalf("repository attached event count = %d", attachedEvents)
	}

	secondaryRevision := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	syncService := repositorysync.NewService(
		config.DefaultLocalOrganizationID, projectService, repositorysync.NewPostgresRepository(pool),
		snapshotProvider{snapshot: repositorysync.Snapshot{
			Provider: "github", RepositoryFullName: "example/frontend-" + suffix,
			DefaultBranch: "main", CanonicalRevision: secondaryRevision, Visibility: "private",
		}}, service,
	)
	if _, err := syncService.SyncRepository(ctx, access.BootstrapPrincipalID, created.Project.ID, frontend.ID, "secondary-sync-"+suffix); err != nil {
		t.Fatal(err)
	}
	project, err := projectService.Get(ctx, created.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if project.CanonicalRevision == nil || *project.CanonicalRevision != initialRevision {
		t.Fatalf("primary canonical revision changed after secondary sync: %#v", project.CanonicalRevision)
	}
	secondaryState, err := syncService.GetRepository(ctx, created.Project.ID, frontend.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondaryState.CanonicalRevision == nil || *secondaryState.CanonicalRevision != secondaryRevision {
		t.Fatalf("secondary state = %#v", secondaryState)
	}
}
