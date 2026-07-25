package projects

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const projectCreatedEventType = "pact.project.created.v1"

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	organizationID string,
	idempotencyKey string,
	requestHash [sha256.Size]byte,
	input CreateInput,
) (CreateResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CreateResult{}, fmt.Errorf("begin project.create: %w", err)
	}
	defer rollbackTransaction(tx)

	var commandID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.idempotency_records (
			organization_id,
			project_id,
			command_type,
			idempotency_key,
			request_hash
		)
		VALUES ($1, NULL, 'project.create', $2, $3)
		ON CONFLICT ON CONSTRAINT idempotency_records_scope_key_uq DO NOTHING
		RETURNING command_id
	`, organizationID, idempotencyKey, requestHash[:]).Scan(&commandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return loadStoredCreateResult(ctx, tx, organizationID, idempotencyKey, requestHash)
	}
	if err != nil {
		return CreateResult{}, fmt.Errorf("reserve project.create idempotency key: %w", err)
	}

	project, err := insertProject(ctx, tx, organizationID, input)
	if err != nil {
		return CreateResult{}, mapProjectWriteError(err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO platform.project_event_counters (
			organization_id,
			project_id,
			last_sequence
		)
		VALUES ($1, $2, 0)
	`, organizationID, project.ID); err != nil {
		return CreateResult{}, fmt.Errorf("initialize project event counter: %w", err)
	}
	projectSequence, err := nextProjectEventSequence(ctx, tx, organizationID, project.ID)
	if err != nil {
		return CreateResult{}, err
	}

	payload, err := json.Marshal(struct {
		Name              string  `json:"name"`
		Slug              string  `json:"slug"`
		CanonicalRevision *string `json:"canonical_revision"`
	}{
		Name:              project.Name,
		Slug:              project.Slug,
		CanonicalRevision: project.CanonicalRevision,
	})
	if err != nil {
		return CreateResult{}, fmt.Errorf("encode project.created payload: %w", err)
	}

	var eventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.events (
			organization_id,
			project_id,
			project_sequence,
			event_type,
			event_version,
			aggregate_type,
			aggregate_id,
			aggregate_version,
			command_id,
			correlation_id,
			payload,
			payload_hash
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			1,
			'project',
			$2,
			1,
			$5,
			$5,
			$6,
			sha256(convert_to(($6::jsonb)::text, 'UTF8'))
		)
		RETURNING id
	`,
		organizationID,
		project.ID,
		projectSequence,
		projectCreatedEventType,
		commandID,
		payload,
	).Scan(&eventID)
	if err != nil {
		return CreateResult{}, fmt.Errorf("append project.created event: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO platform.outbox (
			organization_id,
			project_id,
			event_id,
			channel
		)
		VALUES ($1, $2, $3, 'project-events')
	`, organizationID, project.ID, eventID); err != nil {
		return CreateResult{}, fmt.Errorf("enqueue project.created event: %w", err)
	}

	responseBody, err := json.Marshal(project)
	if err != nil {
		return CreateResult{}, fmt.Errorf("encode project.create response: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET outcome = 'succeeded',
		    response_status = 201,
		    response_body = $3,
		    event_id = $4,
		    aggregate_id = $5,
		    completed_at = transaction_timestamp()
		WHERE organization_id = $1
		  AND command_type = 'project.create'
		  AND idempotency_key = $2
		  AND project_id IS NULL
	`, organizationID, idempotencyKey, responseBody, eventID, project.ID)
	if err != nil {
		return CreateResult{}, fmt.Errorf("store project.create result: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return CreateResult{}, errors.New("store project.create result: idempotency reservation disappeared")
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, fmt.Errorf("commit project.create: %w", err)
	}
	return CreateResult{Project: project}, nil
}

func (r *PostgresRepository) Get(ctx context.Context, organizationID, projectID string) (Project, error) {
	project, err := scanProject(r.pool.QueryRow(ctx, `
		SELECT
			id,
			organization_id,
			name,
			slug,
			canonical_revision,
			version,
			created_at,
			updated_at
		FROM identity.projects
		WHERE organization_id = $1
		  AND id = $2
	`, organizationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	return project, nil
}

func insertProject(ctx context.Context, tx pgx.Tx, organizationID string, input CreateInput) (Project, error) {
	return scanProject(tx.QueryRow(ctx, `
		INSERT INTO identity.projects (
			organization_id,
			name,
			slug,
			canonical_revision
		)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id,
			organization_id,
			name,
			slug,
			canonical_revision,
			version,
			created_at,
			updated_at
	`, organizationID, input.Name, input.Slug, input.CanonicalRevision))
}

func nextProjectEventSequence(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	projectID string,
) (int64, error) {
	var sequence int64
	err := tx.QueryRow(ctx, `
		WITH next_sequence AS (
			UPDATE platform.project_event_counters
			SET last_sequence = last_sequence + 1,
			    updated_at = transaction_timestamp()
			WHERE organization_id = $1
			  AND project_id = $2
			RETURNING last_sequence
		)
		UPDATE identity.projects AS project
		SET event_sequence = next_sequence.last_sequence
		FROM next_sequence
		WHERE project.organization_id = $1
		  AND project.id = $2
		RETURNING next_sequence.last_sequence
	`, organizationID, projectID).Scan(&sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("allocate project event sequence: project counter does not exist")
	}
	if err != nil {
		return 0, fmt.Errorf("allocate project event sequence: %w", err)
	}
	return sequence, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanProject(row rowScanner) (Project, error) {
	var project Project
	err := row.Scan(
		&project.ID,
		&project.OrganizationID,
		&project.Name,
		&project.Slug,
		&project.CanonicalRevision,
		&project.Version,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	return project, err
}

func loadStoredCreateResult(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	idempotencyKey string,
	requestHash [sha256.Size]byte,
) (CreateResult, error) {
	var (
		storedHash   []byte
		outcome      *string
		responseBody []byte
	)
	err := tx.QueryRow(ctx, `
		SELECT request_hash, outcome, response_body
		FROM platform.idempotency_records
		WHERE organization_id = $1
		  AND command_type = 'project.create'
		  AND idempotency_key = $2
		  AND project_id IS NULL
	`, organizationID, idempotencyKey).Scan(&storedHash, &outcome, &responseBody)
	if err != nil {
		return CreateResult{}, fmt.Errorf("load project.create result: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return CreateResult{}, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(responseBody) == 0 {
		return CreateResult{}, ErrCommandIncomplete
	}

	var project Project
	if err := json.Unmarshal(responseBody, &project); err != nil {
		return CreateResult{}, fmt.Errorf("decode stored project.create result: %w", err)
	}
	project.OrganizationID = organizationID
	return CreateResult{Project: project, Replayed: true}, nil
}

func mapProjectWriteError(err error) error {
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) &&
		postgresErr.Code == "23505" &&
		postgresErr.ConstraintName == "projects_tenant_slug_uq" {
		return ErrSlugTaken
	}
	return fmt.Errorf("create project: %w", err)
}

func rollbackTransaction(tx pgx.Tx) {
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackCtx)
}
