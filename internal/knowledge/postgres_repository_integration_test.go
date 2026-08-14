//go:build integration

package knowledge_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

func TestWorkspaceKnowledgeLifecycleIsDurableAndIdempotent(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{
		ApplicationName: "pact-knowledge-integration-test",
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
	createdProject, err := projectService.Create(ctx, "knowledge-project-"+suffix, projects.CreateInput{
		Name: "Knowledge Project " + suffix, Slug: "knowledge-project-" + suffix,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workspaceService := workspaces.NewService(config.DefaultLocalOrganizationID, workspaces.NewPostgresRepository(pool))
	workspace, err := workspaceService.Get(ctx, createdProject.Project.Slug)
	if err != nil {
		t.Fatalf("get project workspace: %v", err)
	}
	service := knowledge.NewService(config.DefaultLocalOrganizationID, knowledge.NewPostgresRepository(pool))
	actorID := access.BootstrapPrincipalID

	resourceInput := knowledge.CreateResourceInput{
		Kind: "document", Title: "Architecture brief", Locator: "docs/architecture.md",
		Description: "Primary source for the platform boundary",
	}
	createdResource, err := service.CreateResource(ctx, actorID, workspace.ID, "knowledge-resource-"+suffix, resourceInput)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	replayedResource, err := service.CreateResource(ctx, actorID, workspace.ID, "knowledge-resource-"+suffix, resourceInput)
	if err != nil || !replayedResource.Replayed || replayedResource.Resource.ID != createdResource.Resource.ID {
		t.Fatalf("replay resource = %#v, error = %v", replayedResource, err)
	}

	recordInput := knowledge.CreateRecordInput{
		Type: "decision", Title: "Use PostgreSQL", Body: "Keep shared project truth in PostgreSQL.",
		Evidence: []knowledge.EvidenceInput{{
			ResourceID: createdResource.Resource.ID, Relation: "origin", Note: "Architecture source",
		}},
	}
	createdRecord, err := service.CreateRecord(ctx, actorID, workspace.ID, "knowledge-record-"+suffix, recordInput)
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	if len(createdRecord.Record.Evidence) != 1 || createdRecord.Record.Evidence[0].Resource.ID != createdResource.Resource.ID {
		t.Fatalf("record evidence = %#v", createdRecord.Record.Evidence)
	}
	beforeAcceptance, err := service.Context(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("compile proposed context: %v", err)
	}
	if len(beforeAcceptance.Decisions) != 0 {
		t.Fatalf("proposed decision leaked into accepted context: %#v", beforeAcceptance.Decisions)
	}

	accepted, err := service.UpdateRecordStatus(ctx, actorID, workspace.ID, createdRecord.Record.ID,
		"knowledge-accept-"+suffix, knowledge.RecordStatusInput{
			Status: "accepted", ExpectedVersion: createdRecord.Record.Version, Reason: "Architecture review passed",
		})
	if err != nil {
		t.Fatalf("accept record: %v", err)
	}
	if accepted.Record.Status != "accepted" || accepted.Record.Version != 2 {
		t.Fatalf("accepted record = %#v", accepted.Record)
	}
	replayedStatus, err := service.UpdateRecordStatus(ctx, actorID, workspace.ID, createdRecord.Record.ID,
		"knowledge-accept-"+suffix, knowledge.RecordStatusInput{
			Status: "accepted", ExpectedVersion: createdRecord.Record.Version, Reason: "Architecture review passed",
		})
	if err != nil || !replayedStatus.Replayed || replayedStatus.Record.Version != 2 {
		t.Fatalf("replay status = %#v, error = %v", replayedStatus, err)
	}
	_, err = service.UpdateRecordStatus(ctx, actorID, workspace.ID, createdRecord.Record.ID,
		"knowledge-stale-"+suffix, knowledge.RecordStatusInput{
			Status: "disputed", ExpectedVersion: 1,
		})
	if !errors.Is(err, knowledge.ErrVersionConflict) {
		t.Fatalf("stale update error = %v", err)
	}

	contextValue, err := service.Context(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("compile accepted context: %v", err)
	}
	if len(contextValue.Decisions) != 1 || contextValue.Decisions[0].ID != createdRecord.Record.ID || len(contextValue.Resources) != 1 {
		t.Fatalf("compiled context = %#v", contextValue)
	}
	searched, err := service.ListRecords(ctx, workspace.ID, knowledge.ListOptions{Query: "PostgreSQL", Limit: 10})
	if err != nil || len(searched) != 1 {
		t.Fatalf("search records = %#v, error = %v", searched, err)
	}

	var eventCount int
	var hashesValid bool
	if err := pool.QueryRow(ctx, `
		SELECT count(*), bool_and(payload_hash = sha256(convert_to(payload::text, 'UTF8')))
		FROM knowledge.events
		WHERE organization_id = $1 AND workspace_id = $2
	`, config.DefaultLocalOrganizationID, workspace.ID).Scan(&eventCount, &hashesValid); err != nil {
		t.Fatalf("inspect knowledge events: %v", err)
	}
	if eventCount != 3 || !hashesValid {
		t.Fatalf("knowledge events count=%d hashes_valid=%t", eventCount, hashesValid)
	}
}
