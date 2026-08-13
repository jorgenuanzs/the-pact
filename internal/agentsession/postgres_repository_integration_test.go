//go:build integration

package agentsession_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

func TestAgentSessionLifecycleAppearsInBackoffice(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{
		ApplicationName: "pact-agent-session-integration-test",
	})
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
	).Create(ctx, "agent-project-"+suffix, projects.CreateInput{
		Name: "Agent project " + suffix,
		Slug: "agent-project-" + suffix,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	service := agentsession.NewService(
		config.DefaultLocalOrganizationID,
		agentsession.NewPostgresRepository(pool),
	)
	session, err := service.Start(ctx, "00000000-0000-4000-8000-000000000002", projectResult.Project.ID, agentsession.StartInput{
		NodeKey:    "node-" + suffix,
		NodeName:   "Integration computer",
		AgentName:  "Kimi",
		AgentType:  "kimi",
		ClientType: "kimi-cli",
		ObserveGit: true,
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if session.Status != "active" || session.ActorName != "Kimi" {
		t.Fatalf("session = %#v", session)
	}
	if _, err := service.Heartbeat(ctx, "00000000-0000-4000-8000-000000000002", true, session.ID); err != nil {
		t.Fatalf("heartbeat session: %v", err)
	}
	clean, err := service.Observe(
		ctx,
		"00000000-0000-4000-8000-000000000002",
		session.ID,
		"clean-"+suffix,
		agentsession.ObservationInput{
			DiffFingerprint: strings.Repeat("0", 64),
			HeadRevision:    strings.Repeat("a", 40),
			Branch:          "main",
		},
	)
	if err != nil {
		t.Fatalf("observe clean repository: %v", err)
	}
	if clean.EventID != nil || clean.Observation.Dirty || clean.Observation.Version != 1 {
		t.Fatalf("clean observation = %#v", clean)
	}
	headChanged, err := service.Observe(
		ctx,
		"00000000-0000-4000-8000-000000000002",
		session.ID,
		"head-"+suffix,
		agentsession.ObservationInput{
			DiffFingerprint: strings.Repeat("0", 64),
			HeadRevision:    strings.Repeat("b", 40),
			Branch:          "main",
		},
	)
	if err != nil {
		t.Fatalf("observe changed Git HEAD: %v", err)
	}
	if headChanged.EventType == nil || *headChanged.EventType != "pact.git.external_change_detected.v1" || headChanged.Observation.Version != 2 {
		t.Fatalf("HEAD observation = %#v", headChanged)
	}
	dirtyInput := agentsession.ObservationInput{
		Dirty: true, DiffFingerprint: strings.Repeat("1", 64), ChangedPaths: 2,
		HeadRevision: strings.Repeat("b", 40), Branch: "main",
	}
	dirty, err := service.Observe(
		ctx,
		"00000000-0000-4000-8000-000000000002",
		session.ID,
		"dirty-"+suffix,
		dirtyInput,
	)
	if err != nil {
		t.Fatalf("observe dirty repository: %v", err)
	}
	if dirty.EventType == nil || *dirty.EventType != "pact.workspace.diff_updated.v1" || dirty.Observation.Version != 3 {
		t.Fatalf("dirty observation = %#v", dirty)
	}
	replayed, err := service.Observe(
		ctx,
		"00000000-0000-4000-8000-000000000002",
		session.ID,
		"dirty-"+suffix,
		dirtyInput,
	)
	if err != nil || !replayed.Replayed || replayed.Observation.Version != dirty.Observation.Version {
		t.Fatalf("replayed observation = %#v, %v", replayed, err)
	}
	conflictingInput := dirtyInput
	conflictingInput.ChangedPaths = 3
	_, err = service.Observe(
		ctx,
		"00000000-0000-4000-8000-000000000002",
		session.ID,
		"dirty-"+suffix,
		conflictingInput,
	)
	if !errors.Is(err, agentsession.ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict error = %v", err)
	}
	overview, err := backoffice.NewPostgresReader(pool).Get(
		ctx,
		config.DefaultLocalOrganizationID,
		projectResult.Project.ID,
	)
	if err != nil {
		t.Fatalf("read backoffice overview: %v", err)
	}
	if overview.Counts.LiveSessions != 1 || overview.Counts.ConnectedNodes != 1 || len(overview.ActiveWork) != 1 {
		t.Fatalf("active overview = %#v", overview)
	}
	if overview.Counts.ConnectedObservers != 1 || overview.CodeActivity.State != backoffice.CodeActivityEditing {
		t.Fatalf("observed overview = %#v", overview)
	}
	if overview.ActiveWork[0].ActorName != "Kimi" || overview.ActiveWork[0].ClientType != "kimi-cli" {
		t.Fatalf("active work = %#v", overview.ActiveWork[0])
	}
	if err := service.Close(ctx, "00000000-0000-4000-8000-000000000002", true, session.ID); err != nil {
		t.Fatalf("close session: %v", err)
	}
	overview, err = backoffice.NewPostgresReader(pool).Get(
		ctx,
		config.DefaultLocalOrganizationID,
		projectResult.Project.ID,
	)
	if err != nil {
		t.Fatalf("read closed overview: %v", err)
	}
	if overview.Counts.LiveSessions != 0 || overview.Counts.ConnectedNodes != 0 || len(overview.ActiveWork) != 0 {
		t.Fatalf("closed overview = %#v", overview)
	}
}
