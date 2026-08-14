//go:build integration

package repositorysync_test

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
	"github.com/jorgenuanzs/the-pact/internal/repositorysync"
)

type mutableProvider struct {
	snapshot repositorysync.Snapshot
	err      error
}

func (p *mutableProvider) Fetch(context.Context, repositorysync.Reference) (repositorysync.Snapshot, error) {
	return p.snapshot, p.err
}

func TestCanonicalRepositorySyncIsDurableIdempotentAndRecoverable(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{
		ApplicationName: "pact-repository-sync-integration-test",
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	initialRevision := "0123456789abcdef0123456789abcdef01234567"
	canonicalRevision := "37ab373144cb17d18e77c52c03f5f6e18e1fb3c5"
	projectService := projects.NewService(config.DefaultLocalOrganizationID, projects.NewPostgresRepository(pool))
	created, err := projectService.Create(ctx, "repository-sync-project-"+suffix, projects.CreateInput{
		Name: "Repository Sync " + suffix, Slug: "repository-sync-" + suffix,
		CanonicalRevision: &initialRevision,
		RootRepository: &projects.SourceRepositoryInput{
			Slug: "primary", Name: "Primary",
			RemoteURL:     "https://github.com/example/private-repository.git",
			DefaultBranch: "main", ObjectFormat: "sha1",
		},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	providerUpdatedAt := time.Now().UTC().Truncate(time.Microsecond)
	provider := &mutableProvider{snapshot: repositorysync.Snapshot{
		Provider: "github", RepositoryFullName: "example/private-repository",
		DefaultBranch: "trunk", CanonicalRevision: canonicalRevision,
		Visibility: "private", ProviderUpdatedAt: &providerUpdatedAt,
	}}
	service := repositorysync.NewService(
		config.DefaultLocalOrganizationID, projectService,
		repositorysync.NewPostgresRepository(pool), provider,
	)

	result, err := service.Sync(ctx, access.BootstrapPrincipalID, created.Project.ID, "canonical-sync-"+suffix)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !result.Changed || result.Replayed || result.EventID == nil ||
		result.State.Status != repositorysync.StatusSynced || result.State.Version != 1 {
		t.Fatalf("first sync = %#v", result)
	}
	replay, err := service.Sync(ctx, access.BootstrapPrincipalID, created.Project.ID, "canonical-sync-"+suffix)
	if err != nil {
		t.Fatalf("replay Sync() error = %v", err)
	}
	if !replay.Replayed || replay.EventID == nil || *replay.EventID != *result.EventID {
		t.Fatalf("replay = %#v", replay)
	}
	unchanged, err := service.Sync(ctx, access.BootstrapPrincipalID, created.Project.ID, "canonical-sync-unchanged-"+suffix)
	if err != nil {
		t.Fatalf("unchanged Sync() error = %v", err)
	}
	if unchanged.Changed || unchanged.EventID != nil || unchanged.State.Version != 1 {
		t.Fatalf("unchanged sync = %#v", unchanged)
	}
	project, err := projectService.Get(ctx, created.Project.ID)
	if err != nil {
		t.Fatalf("get synchronized project: %v", err)
	}
	if project.CanonicalRevision == nil || *project.CanonicalRevision != canonicalRevision ||
		project.RootRepository == nil || project.RootRepository.DefaultBranch != "trunk" {
		t.Fatalf("synchronized project = %#v", project)
	}

	provider.err = &repositorysync.ProviderError{Code: "authentication_required", Err: errors.New("provider detail")}
	if _, err := service.Sync(ctx, access.BootstrapPrincipalID, created.Project.ID, "canonical-sync-failed-"+suffix); err == nil {
		t.Fatal("failed provider sync succeeded")
	}
	failed, err := service.Get(ctx, created.Project.ID)
	if err != nil {
		t.Fatalf("get failed state: %v", err)
	}
	if failed.Status != repositorysync.StatusFailed || failed.LastErrorCode == nil ||
		*failed.LastErrorCode != "authentication_required" || failed.Version != 2 ||
		failed.CanonicalRevision == nil || *failed.CanonicalRevision != canonicalRevision {
		t.Fatalf("failed state = %#v", failed)
	}

	provider.err = nil
	recovered, err := service.Sync(ctx, access.BootstrapPrincipalID, created.Project.ID, "canonical-sync-recovered-"+suffix)
	if err != nil {
		t.Fatalf("recovery Sync() error = %v", err)
	}
	if !recovered.Changed || recovered.State.Status != repositorysync.StatusSynced || recovered.State.Version != 3 {
		t.Fatalf("recovered sync = %#v", recovered)
	}

	var syncEvents int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM platform.events
		WHERE organization_id = $1 AND project_id = $2
		  AND event_type IN ($3, $4)
	`, config.DefaultLocalOrganizationID, created.Project.ID,
		"pact.repository.canonical_synced.v1", "pact.repository.sync_failed.v1").Scan(&syncEvents); err != nil {
		t.Fatalf("count repository sync events: %v", err)
	}
	if syncEvents != 3 {
		t.Fatalf("repository sync event count = %d, want 3", syncEvents)
	}
}
