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

	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

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
