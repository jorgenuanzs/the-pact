//go:build integration

package projects_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

func TestProjectCreateWithRootRepository(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{
		ApplicationName: "pact-project-root-repository-integration-test",
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	revision := "0123456789abcdef0123456789abcdef01234567"
	remoteURL := "https://github.com/example/integration-" + suffix
	service := projects.NewService(config.DefaultLocalOrganizationID, projects.NewPostgresRepository(pool))
	result, err := service.Create(ctx, "root-repository-"+suffix, projects.CreateInput{
		Name:              "Root Repository " + suffix,
		Slug:              "root-repository-" + suffix,
		CanonicalRevision: &revision,
		RootRepository: &projects.SourceRepositoryInput{
			Slug:          "primary",
			Name:          "Primary",
			RemoteURL:     remoteURL,
			DefaultBranch: "main",
			ObjectFormat:  "sha1",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Project.Status != "active" ||
		result.Project.RootRepository == nil ||
		result.Project.RootRepository.RemoteURL == nil ||
		*result.Project.RootRepository.RemoteURL != remoteURL {
		t.Fatalf("created project = %#v", result.Project)
	}

	loaded, err := service.Get(ctx, result.Project.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if loaded.RootRepository == nil || loaded.RootRepository.ID != result.Project.RootRepository.ID {
		t.Fatalf("loaded project = %#v", loaded)
	}
	listed, err := service.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	found := false
	for _, project := range listed {
		if project.ID == result.Project.ID {
			found = project.RootRepository != nil && project.RootRepository.RemoteURL != nil
			break
		}
	}
	if !found {
		t.Fatal("project list did not include the root repository")
	}
}

func TestProjectCreateIsAtomicAndIdempotent(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{
		ApplicationName: "pact-projects-integration-test",
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
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	key := "integration-create-" + suffix
	input := projects.CreateInput{Name: "Integration " + suffix, Slug: "integration-" + suffix}
	service := projects.NewService(config.DefaultLocalOrganizationID, projects.NewPostgresRepository(pool))

	const workers = 8
	results := make(chan projects.CreateResult, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, createErr := service.Create(ctx, key, input)
			if createErr != nil {
				errs <- createErr
				return
			}
			results <- result
		}()
	}
	group.Wait()
	close(results)
	close(errs)

	for createErr := range errs {
		t.Errorf("concurrent Create() error = %v", createErr)
	}

	var (
		projectID string
		created   int
		replayed  int
	)
	for result := range results {
		if projectID == "" {
			projectID = result.Project.ID
		} else if result.Project.ID != projectID {
			t.Errorf("project ID = %q, want %q", result.Project.ID, projectID)
		}
		if result.Project.Status != "initializing" {
			t.Errorf("project status = %q, want initializing", result.Project.Status)
		}
		if result.Replayed {
			replayed++
		} else {
			created++
		}
	}
	if created != 1 || replayed != workers-1 {
		t.Fatalf("created=%d replayed=%d", created, replayed)
	}

	_, err = service.Create(ctx, key, projects.CreateInput{Name: input.Name, Slug: input.Slug + "-different"})
	if !errors.Is(err, projects.ErrIdempotencyConflict) {
		t.Fatalf("different payload error = %v", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET response_body = response_body - 'status'
		WHERE organization_id = $1
		  AND command_type = 'project.create'
		  AND idempotency_key = $2
	`, config.DefaultLocalOrganizationID, key); err != nil {
		t.Fatalf("prepare legacy idempotency response: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE identity.projects
		SET status = 'active'
		WHERE organization_id = $1
		  AND id = $2
	`, config.DefaultLocalOrganizationID, projectID); err != nil {
		t.Fatalf("advance project after stored response: %v", err)
	}
	legacyReplay, err := service.Create(ctx, key, input)
	if err != nil {
		t.Fatalf("replay legacy idempotency response: %v", err)
	}
	if !legacyReplay.Replayed || legacyReplay.Project.Status != "initializing" {
		t.Fatalf("legacy replay = %#v", legacyReplay)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE identity.projects
		SET status = 'initializing'
		WHERE organization_id = $1
		  AND id = $2
	`, config.DefaultLocalOrganizationID, projectID); err != nil {
		t.Fatalf("restore project status: %v", err)
	}

	var (
		projectCount     int
		eventCount       int
		outboxCount      int
		commandCount     int
		payloadHashValid bool
		sequenceValid    bool
	)
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM identity.projects WHERE id = $1),
			(SELECT count(*) FROM platform.events WHERE project_id = $1),
			(SELECT count(*) FROM platform.outbox WHERE project_id = $1),
			(
				SELECT count(*)
				FROM platform.idempotency_records
				WHERE organization_id = $2
				  AND command_type = 'project.create'
				  AND idempotency_key = $3
			),
			(
				SELECT bool_and(
					payload_hash = sha256(convert_to(payload::text, 'UTF8'))
				)
				FROM platform.events
				WHERE project_id = $1
			),
			(
				SELECT
					project.event_sequence = counter.last_sequence
					AND counter.last_sequence = (
						SELECT max(event.project_sequence)
						FROM platform.events AS event
						WHERE event.project_id = $1
					)
				FROM identity.projects AS project
				JOIN platform.project_event_counters AS counter
				  ON counter.organization_id = project.organization_id
				 AND counter.project_id = project.id
				WHERE project.id = $1
			)
	`, projectID, config.DefaultLocalOrganizationID, key).Scan(
		&projectCount,
		&eventCount,
		&outboxCount,
		&commandCount,
		&payloadHashValid,
		&sequenceValid,
	); err != nil {
		t.Fatalf("count durable records: %v", err)
	}
	if projectCount != 1 ||
		eventCount != 1 ||
		outboxCount != 1 ||
		commandCount != 1 ||
		!payloadHashValid ||
		!sequenceValid {
		t.Fatalf(
			"durable records project=%d event=%d outbox=%d command=%d payload_hash_valid=%t sequence_valid=%t",
			projectCount,
			eventCount,
			outboxCount,
			commandCount,
			payloadHashValid,
			sequenceValid,
		)
	}

	events, err := eventlog.NewPostgresReader(pool).List(
		ctx,
		config.DefaultLocalOrganizationID,
		projectID,
		0,
		10,
	)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].ProjectSequence != 1 || events[0].Type != "pact.project.created.v1" {
		t.Fatalf("events = %#v", events)
	}

	projectList, err := service.List(ctx)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	foundProject := false
	for _, listedProject := range projectList {
		if listedProject.ID == projectID {
			foundProject = true
			if listedProject.Status != "initializing" {
				t.Fatalf("listed project status = %q", listedProject.Status)
			}
			break
		}
	}
	if !foundProject {
		t.Fatalf("project %s is missing from list", projectID)
	}

	overview, err := backoffice.NewPostgresReader(pool).Get(
		ctx,
		config.DefaultLocalOrganizationID,
		projectID,
	)
	if err != nil {
		t.Fatalf("get project overview: %v", err)
	}
	if overview.Counts.Events != 1 ||
		len(overview.RecentEvents) != 1 ||
		overview.CodeActivity.State != backoffice.CodeActivityUnobserved {
		t.Fatalf("overview = %#v", overview)
	}

	var actorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO identity.actors (
			organization_id,
			kind,
			display_name
		)
		VALUES ($1, 'agent', $2)
		RETURNING id
	`, config.DefaultLocalOrganizationID, "Integration agent "+suffix).Scan(&actorID); err != nil {
		t.Fatalf("create active session actor: %v", err)
	}
	var nodeID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO identity.actors (
			organization_id,
			kind,
			display_name
		)
		VALUES ($1, 'node', $2)
		RETURNING id
	`, config.DefaultLocalOrganizationID, "Integration node "+suffix).Scan(&nodeID); err != nil {
		t.Fatalf("create active node actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO identity.nodes (
			id,
			organization_id,
			node_key,
			name,
			lifecycle_status,
			last_seen_at
		)
		VALUES ($1, $2, $3, $4, 'active', transaction_timestamp())
	`, nodeID, config.DefaultLocalOrganizationID, "integration-node-"+suffix, "Integration node "+suffix); err != nil {
		t.Fatalf("create active node: %v", err)
	}
	var sessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO identity.sessions (
			organization_id,
			project_id,
			actor_id,
			node_id,
			status,
			client_type,
			protocol_version,
			announced_capabilities,
			expires_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			'active',
			'integration-test',
			'0.1',
			'{"workspace.diff.observe.v1": true}'::jsonb,
			transaction_timestamp() + interval '5 minutes'
		)
		RETURNING id
	`, config.DefaultLocalOrganizationID, projectID, actorID, nodeID).Scan(&sessionID); err != nil {
		t.Fatalf("create active session: %v", err)
	}

	overview, err = backoffice.NewPostgresReader(pool).Get(
		ctx,
		config.DefaultLocalOrganizationID,
		projectID,
	)
	if err != nil {
		t.Fatalf("get overview with active observer: %v", err)
	}
	if overview.Counts.LiveSessions != 1 ||
		overview.Counts.ConnectedNodes != 1 ||
		overview.Counts.ConnectedObservers != 1 ||
		len(overview.ActiveWork) != 1 ||
		overview.ActiveWork[0].WorkspaceID != nil ||
		overview.CodeActivity.State != backoffice.CodeActivityIdle {
		t.Fatalf("overview with active observer = %#v", overview)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE identity.sessions
		SET status = 'stale',
		    version = version + 1
		WHERE organization_id = $1
		  AND project_id = $2
		  AND id = $3
	`, config.DefaultLocalOrganizationID, projectID, sessionID); err != nil {
		t.Fatalf("mark session stale: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE identity.nodes
		SET lifecycle_status = 'offline',
		    version = version + 1
		WHERE organization_id = $1
		  AND id = $2
	`, config.DefaultLocalOrganizationID, nodeID); err != nil {
		t.Fatalf("mark node offline: %v", err)
	}
	overview, err = backoffice.NewPostgresReader(pool).Get(
		ctx,
		config.DefaultLocalOrganizationID,
		projectID,
	)
	if err != nil {
		t.Fatalf("get overview with stale session: %v", err)
	}
	if overview.Counts.LiveSessions != 0 ||
		overview.Counts.ConnectedNodes != 0 ||
		overview.Counts.ConnectedObservers != 0 ||
		len(overview.ActiveWork) != 0 ||
		overview.CodeActivity.State != backoffice.CodeActivityUnobserved {
		t.Fatalf("overview with stale session = %#v", overview)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO public.pact_schema_migrations (version, checksum)
		VALUES ('999999_unknown', 'test-only')
	`); err != nil {
		t.Fatalf("insert unknown migration: %v", err)
	}
	if err := migrations.Verify(ctx, pool); err == nil {
		t.Fatal("Verify() accepted an unknown database migration")
	}
	if err := migrations.Up(ctx, pool); err == nil {
		t.Fatal("Up() accepted an unknown database migration")
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM public.pact_schema_migrations
		WHERE version = '999999_unknown'
	`); err != nil {
		t.Fatalf("remove unknown migration fixture: %v", err)
	}
}
