//go:build integration

package coordination_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

const testPrincipalID = "00000000-0000-4000-8000-000000000002"

func TestCoordinatedWorkLifecycleAndScopeExclusion(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{ApplicationName: "pact-coordination-integration-test"})
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
	).Create(ctx, "coordination-project-"+suffix, projects.CreateInput{
		Name: "Coordination project " + suffix,
		Slug: "coordination-project-" + suffix,
		RootRepository: &projects.SourceRepositoryInput{
			Slug: "primary", Name: "Primary repository",
			RemoteURL:     "https://example.com/coordination-" + suffix + ".git",
			DefaultBranch: "main", ObjectFormat: "sha1",
		},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	sessions := agentsession.NewService(config.DefaultLocalOrganizationID, agentsession.NewPostgresRepository(pool))
	firstSession := startAgent(t, ctx, sessions, projectResult.Project.ID, "first-"+suffix, "Codex")
	secondSession := startAgent(t, ctx, sessions, projectResult.Project.ID, "second-"+suffix, "Kimi")
	service := coordination.NewService(config.DefaultLocalOrganizationID, coordination.NewPostgresRepository(pool))
	base := strings.Repeat("a", 40)

	started, err := service.Start(ctx, testPrincipalID, false, projectResult.Project.ID, "start-first-"+suffix, coordination.StartInput{
		SessionID: firstSession.ID, Title: "Change API", Goal: "Make the API safer",
		SuccessCriteria: []string{"Tests pass"}, BaseRevision: base,
		Scopes: []coordination.ScopeInput{{Kind: "path", Locator: "internal/api"}},
	})
	if err != nil {
		t.Fatalf("start first work: %v", err)
	}
	if started.Intent.Status != "active" || len(started.Claims) != 1 || started.Claims[0].Mode != coordination.ClaimModeExclusive {
		t.Fatalf("started work = %#v", started)
	}

	check, err := service.CheckScopes(ctx, projectResult.Project.ID, []coordination.ScopeInput{{Kind: "file", Locator: "internal/api/router.go"}})
	if err != nil {
		t.Fatalf("check scopes: %v", err)
	}
	if !check.Blocked || len(check.Overlaps) != 1 || check.Overlaps[0].ExistingActor != "Codex" {
		t.Fatalf("scope check = %#v", check)
	}

	_, err = service.Start(ctx, testPrincipalID, false, projectResult.Project.ID, "start-conflict-"+suffix, coordination.StartInput{
		SessionID: secondSession.ID, Title: "Change router", Goal: "Edit the same API area",
		BaseRevision: base,
		Scopes:       []coordination.ScopeInput{{Kind: "file", Locator: "internal/api/router.go"}},
	})
	var conflict *coordination.ScopeConflictError
	if !errors.As(err, &conflict) || len(conflict.Overlaps) != 1 {
		t.Fatalf("conflicting start error = %#v", err)
	}

	workspace, err := service.AttachWorkspace(ctx, testPrincipalID, false, started.Intent.ID, "workspace-"+suffix, coordination.WorkspaceInput{
		SessionID: firstSession.ID, BaseRevision: base,
		PathRef:   ".pact/worktrees/" + started.Intent.ID,
		GitBranch: "pact/" + started.Intent.ID[:12] + "-change-api",
	})
	if err != nil {
		t.Fatalf("attach workspace: %v", err)
	}
	workspaceID := workspace.Workspace.ID
	observation, err := sessions.Observe(ctx, testPrincipalID, firstSession.ID, "observe-workspace-"+suffix, agentsession.ObservationInput{
		WorkspaceID: &workspaceID, Dirty: true, ChangedPaths: 1,
		DiffFingerprint: strings.Repeat("1", 64), HeadRevision: base,
		Branch: workspace.Workspace.GitBranch,
	})
	if err != nil {
		t.Fatalf("observe workspace: %v", err)
	}
	if observation.Observation.IntentID == nil || *observation.Observation.IntentID != started.Intent.ID {
		t.Fatalf("workspace observation = %#v", observation)
	}
	advanced, err := sessions.Observe(ctx, testPrincipalID, firstSession.ID, "observe-workspace-head-"+suffix, agentsession.ObservationInput{
		WorkspaceID: &workspaceID, DiffFingerprint: strings.Repeat("0", 64),
		HeadRevision: strings.Repeat("b", 40), Branch: workspace.Workspace.GitBranch,
	})
	if err != nil {
		t.Fatalf("observe workspace head: %v", err)
	}
	if advanced.EventType == nil || *advanced.EventType != "pact.workspace.head_updated.v1" {
		t.Fatalf("workspace head observation = %#v", advanced)
	}

	items, err := service.List(ctx, projectResult.Project.ID)
	if err != nil {
		t.Fatalf("list work: %v", err)
	}
	if len(items) != 1 || items[0].Workspace == nil || !items[0].SessionLive || items[0].ResponsibleName != "Codex" {
		t.Fatalf("work items = %#v", items)
	}

	submitted, err := service.UpdateStatus(ctx, testPrincipalID, false, started.Intent.ID, "submit-"+suffix, coordination.StatusInput{
		SessionID: firstSession.ID, Status: "submitted", ExpectedVersion: started.Intent.Version,
		Summary: "Ready for validation",
	})
	if err != nil {
		t.Fatalf("submit work: %v", err)
	}
	completed, err := service.UpdateStatus(ctx, testPrincipalID, false, started.Intent.ID, "complete-"+suffix, coordination.StatusInput{
		SessionID: firstSession.ID, Status: "completed", ExpectedVersion: submitted.Intent.Version,
		Summary: "Validated and complete",
	})
	if err != nil {
		t.Fatalf("complete work: %v", err)
	}
	replayed, err := service.UpdateStatus(ctx, testPrincipalID, false, started.Intent.ID, "complete-"+suffix, coordination.StatusInput{
		SessionID: firstSession.ID, Status: "completed", ExpectedVersion: submitted.Intent.Version,
		Summary: "Validated and complete",
	})
	if err != nil || !replayed.Replayed || replayed.Intent.Version != completed.Intent.Version {
		t.Fatalf("replayed completion = %#v, %v", replayed, err)
	}

	second, err := service.Start(ctx, testPrincipalID, false, projectResult.Project.ID, "start-after-release-"+suffix, coordination.StartInput{
		SessionID: secondSession.ID, Title: "Change router", Goal: "Edit after release",
		BaseRevision: base,
		Scopes:       []coordination.ScopeInput{{Kind: "file", Locator: "internal/api/router.go"}},
	})
	if err != nil {
		t.Fatalf("start after release: %v", err)
	}
	if len(second.Overlaps) != 0 {
		t.Fatalf("released claim still overlaps: %#v", second.Overlaps)
	}
}

func startAgent(
	t *testing.T,
	ctx context.Context,
	service *agentsession.Service,
	projectID, nodeKey, name string,
) agentsession.Session {
	t.Helper()
	session, err := service.Start(ctx, testPrincipalID, projectID, agentsession.StartInput{
		NodeKey: nodeKey, NodeName: name + " computer", AgentName: name,
		AgentType: strings.ToLower(name), ClientType: strings.ToLower(name) + "-cli",
		ObserveGit: true,
	})
	if err != nil {
		t.Fatalf("start %s session: %v", name, err)
	}
	return session
}
