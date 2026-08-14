//go:build integration

package contextpack_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/contextpack"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

func TestHandoffAndContextPackLifecycle(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{
		ApplicationName: "pact-context-pack-integration-test",
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := migrations.Verify(ctx, pool); err != nil {
		t.Fatalf("verify migrations: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	projectService := projects.NewService(config.DefaultLocalOrganizationID, projects.NewPostgresRepository(pool))
	created, err := projectService.Create(ctx, "context-project-"+suffix, projects.CreateInput{
		Name: "Context Project " + suffix, Slug: "context-project-" + suffix,
		RootRepository: &projects.SourceRepositoryInput{
			Slug: "primary", Name: "Primary repository",
			RemoteURL:     "https://example.com/context-" + suffix + ".git",
			DefaultBranch: "main", ObjectFormat: "sha1",
		},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workspaceService := workspaces.NewService(config.DefaultLocalOrganizationID, workspaces.NewPostgresRepository(pool))
	workspace, err := workspaceService.Get(ctx, created.Project.Slug)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}

	sessionService := agentsession.NewService(config.DefaultLocalOrganizationID, agentsession.NewPostgresRepository(pool))
	first := startContextAgent(t, ctx, sessionService, created.Project.ID, "codex-"+suffix, "Codex")
	second := startContextAgent(t, ctx, sessionService, created.Project.ID, "kimi-"+suffix, "Kimi")
	coordinationService := coordination.NewService(config.DefaultLocalOrganizationID, coordination.NewPostgresRepository(pool))
	baseRevision := strings.Repeat("a", 40)
	started, err := coordinationService.Start(
		ctx, access.BootstrapPrincipalID, false, created.Project.ID, "context-work-"+suffix,
		coordination.StartInput{
			SessionID: first.ID, Title: "Build context packs", Goal: "Create a verifiable project snapshot",
			SuccessCriteria: []string{"Handoff survives another client"}, BaseRevision: baseRevision,
			Scopes: []coordination.ScopeInput{{Kind: "path", Locator: "internal/contextpack"}},
		},
	)
	if err != nil {
		t.Fatalf("start work: %v", err)
	}

	knowledgeService := knowledge.NewService(config.DefaultLocalOrganizationID, knowledge.NewPostgresRepository(pool))
	record, err := knowledgeService.CreateRecord(
		ctx, access.BootstrapPrincipalID, workspace.ID, "context-record-"+suffix,
		knowledge.CreateRecordInput{
			Type: "decision", Title: "Context packs are snapshots",
			Body: "Compile project truth by intent instead of copying private conversations.",
		},
	)
	if err != nil {
		t.Fatalf("create knowledge record: %v", err)
	}
	acceptedRecord, err := knowledgeService.UpdateRecordStatus(
		ctx, access.BootstrapPrincipalID, workspace.ID, record.Record.ID,
		"context-record-accept-"+suffix, knowledge.RecordStatusInput{
			Status: "accepted", ExpectedVersion: record.Record.Version,
		},
	)
	if err != nil {
		t.Fatalf("accept knowledge record: %v", err)
	}

	offer, err := coordinationService.OfferHandoff(
		ctx, access.BootstrapPrincipalID, false, created.Project.ID, started.Intent.ID,
		"handoff-offer-"+suffix, coordination.OfferHandoffInput{
			SessionID: first.ID, Summary: "Core implementation is ready for review.",
			Completed:     []string{"Persisted the immutable snapshot"},
			RemainingWork: []string{"Review API semantics"}, NextSteps: []string{"Compile review context"},
			Validations:     []coordination.HandoffValidation{{Name: "go test", Status: "passed"}},
			LinkedRecordIDs: []string{acceptedRecord.Record.ID},
		},
	)
	if err != nil {
		t.Fatalf("offer handoff: %v", err)
	}
	if offer.Handoff.Status != "offered" || len(offer.Handoff.LinkedRecordIDs) != 1 {
		t.Fatalf("offered handoff = %#v", offer.Handoff)
	}

	service := contextpack.NewService(
		config.DefaultLocalOrganizationID, contextpack.NewPostgresRepository(pool),
		projectService, workspaceService, coordinationService, knowledgeService,
	)
	input := contextpack.CompileInput{SessionID: first.ID, Type: "review", TTLMinutes: 10}
	compiled, err := service.Compile(
		ctx, access.BootstrapPrincipalID, false, created.Project.ID, started.Intent.ID,
		"context-compile-"+suffix, input,
	)
	if err != nil {
		t.Fatalf("compile context pack: %v", err)
	}
	pack := compiled.Pack
	if pack.Type != "review" || pack.Snapshot.Consistency != "eventual" || len(pack.Snapshot.SourceFingerprint) != 64 {
		t.Fatalf("compiled context pack = %#v", pack)
	}
	if pack.Snapshot.GitRevision == nil || *pack.Snapshot.GitRevision != baseRevision {
		t.Fatalf("context pack git revision = %v; want intent base revision %s", pack.Snapshot.GitRevision, baseRevision)
	}
	if len(pack.Handoffs) != 1 || len(pack.Knowledge.Decisions) != 1 || pack.Knowledge.Decisions[0].ID != acceptedRecord.Record.ID {
		t.Fatalf("compiled context sources = handoffs %#v, knowledge %#v", pack.Handoffs, pack.Knowledge)
	}
	encoded := fmt.Sprintf("%#v", pack)
	if strings.Contains(encoded, "/Users/") || strings.Contains(encoded, ".pact/worktrees/") {
		t.Fatalf("context pack leaked a local path: %s", encoded)
	}
	replayed, err := service.Compile(
		ctx, access.BootstrapPrincipalID, false, created.Project.ID, started.Intent.ID,
		"context-compile-"+suffix, input,
	)
	if err != nil || !replayed.Replayed || replayed.Pack.ID != pack.ID {
		t.Fatalf("replayed context pack = %#v, error = %v", replayed, err)
	}
	recompiled, err := service.Compile(
		ctx, access.BootstrapPrincipalID, false, created.Project.ID, started.Intent.ID,
		"context-recompile-"+suffix, input,
	)
	if err != nil || recompiled.Pack.ID == pack.ID || recompiled.Pack.Snapshot.SourceFingerprint != pack.Snapshot.SourceFingerprint {
		t.Fatalf("recompiled context pack = %#v, error = %v", recompiled, err)
	}
	loaded, err := service.Get(ctx, created.Project.ID, pack.ID)
	if err != nil || loaded.Snapshot.SourceFingerprint != pack.Snapshot.SourceFingerprint {
		t.Fatalf("loaded context pack = %#v, error = %v", loaded, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE knowledge.context_packs
		SET expires_at = expires_at + interval '1 minute'
		WHERE organization_id = $1 AND project_id = $2 AND id = $3
	`, config.DefaultLocalOrganizationID, created.Project.ID, pack.ID); err == nil {
		t.Fatal("immutable context pack accepted an update")
	}

	acceptedHandoff, err := coordinationService.UpdateHandoffStatus(
		ctx, access.BootstrapPrincipalID, false, created.Project.ID, started.Intent.ID,
		offer.Handoff.ID, "handoff-accept-"+suffix, coordination.HandoffStatusInput{
			SessionID: second.ID, Status: "accepted", ExpectedVersion: offer.Handoff.Version,
		},
	)
	if err != nil {
		t.Fatalf("accept handoff: %v", err)
	}
	if acceptedHandoff.Handoff.Status != "accepted" || acceptedHandoff.Handoff.ToActorName == nil || *acceptedHandoff.Handoff.ToActorName != "Kimi" {
		t.Fatalf("accepted handoff = %#v", acceptedHandoff.Handoff)
	}

	var packCount, eventCount int
	var hashesValid bool
	if err := pool.QueryRow(ctx, `
		SELECT count(*), bool_and(payload_hash = sha256(convert_to(payload::text, 'UTF8')))
		FROM knowledge.context_packs
		WHERE organization_id = $1 AND project_id = $2
	`, config.DefaultLocalOrganizationID, created.Project.ID).Scan(&packCount, &hashesValid); err != nil {
		t.Fatalf("inspect context pack persistence: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM platform.events
		WHERE organization_id = $1 AND project_id = $2
		  AND event_type IN ('pact.handoff.offered.v1', 'pact.handoff.accepted.v1', 'pact.context.compiled.v1')
	`, config.DefaultLocalOrganizationID, created.Project.ID).Scan(&eventCount); err != nil {
		t.Fatalf("inspect context events: %v", err)
	}
	if packCount != 2 || !hashesValid || eventCount != 4 {
		t.Fatalf("context durability pack_count=%d hashes_valid=%t event_count=%d", packCount, hashesValid, eventCount)
	}
}

func startContextAgent(
	t *testing.T, ctx context.Context, service *agentsession.Service,
	projectID, nodeKey, name string,
) agentsession.Session {
	t.Helper()
	session, err := service.Start(ctx, access.BootstrapPrincipalID, projectID, agentsession.StartInput{
		NodeKey: nodeKey, NodeName: name + " computer", AgentName: name,
		AgentType: strings.ToLower(name), ClientType: strings.ToLower(name) + "-mcp", ObserveGit: true,
	})
	if err != nil {
		t.Fatalf("start %s session: %v", name, err)
	}
	return session
}
