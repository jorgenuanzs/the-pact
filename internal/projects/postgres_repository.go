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
	if err := insertDefaultWorkspace(ctx, tx, project); err != nil {
		return CreateResult{}, err
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
		Name              string            `json:"name"`
		Slug              string            `json:"slug"`
		CanonicalRevision *string           `json:"canonical_revision"`
		RootRepository    *SourceRepository `json:"root_repository"`
	}{
		Name:              project.Name,
		Slug:              project.Slug,
		CanonicalRevision: project.CanonicalRevision,
		RootRepository:    project.RootRepository,
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

func insertDefaultWorkspace(ctx context.Context, tx pgx.Tx, project Project) error {
	var workspaceID string
	err := tx.QueryRow(ctx, `
		INSERT INTO identity.workspaces (
			organization_id, name, slug, description, status, settings
		)
		VALUES ($1, $2, $3, '', $4, '{"managed_default": true}'::jsonb)
		RETURNING id
	`, project.OrganizationID, project.Name, project.Slug,
		mapProjectStatusToWorkspace(project.Status)).Scan(&workspaceID)
	if err != nil {
		return fmt.Errorf("create default project workspace: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.workspace_projects (organization_id, workspace_id, project_id)
		VALUES ($1, $2, $3)
	`, project.OrganizationID, workspaceID, project.ID); err != nil {
		return fmt.Errorf("attach project to default workspace: %w", err)
	}
	return nil
}

func mapProjectStatusToWorkspace(status string) string {
	if status == "archived" {
		return "archived"
	}
	return "active"
}

func (r *PostgresRepository) Get(ctx context.Context, organizationID, projectID string) (Project, error) {
	project, err := scanProject(r.pool.QueryRow(ctx, `
		SELECT
			project.id,
			project.organization_id,
			project.name,
			project.slug,
			project.status,
			project.canonical_revision,
			project.version,
			project.created_at,
			project.updated_at,
			repository.id,
			repository.slug,
			repository.name,
			repository.vcs_type,
			repository.status,
			repository.remote_url,
			repository.default_branch,
			repository.object_format,
			repository.version
		FROM identity.projects AS project
		LEFT JOIN coordination.repositories AS repository
		  ON repository.organization_id = project.organization_id
		 AND repository.project_id = project.id
		 AND repository.id = project.root_repository_id
		WHERE project.organization_id = $1
		  AND project.id = $2
	`, organizationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	return project, nil
}

func (r *PostgresRepository) List(ctx context.Context, organizationID string) ([]Project, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			project.id,
			project.organization_id,
			project.name,
			project.slug,
			project.status,
			project.canonical_revision,
			project.version,
			project.created_at,
			project.updated_at,
			repository.id,
			repository.slug,
			repository.name,
			repository.vcs_type,
			repository.status,
			repository.remote_url,
			repository.default_branch,
			repository.object_format,
			repository.version
		FROM identity.projects AS project
		LEFT JOIN coordination.repositories AS repository
		  ON repository.organization_id = project.organization_id
		 AND repository.project_id = project.id
		 AND repository.id = project.root_repository_id
		WHERE project.organization_id = $1
		ORDER BY project.updated_at DESC, project.id DESC
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projectList := make([]Project, 0)
	for rows.Next() {
		project, scanErr := scanProject(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list projects: %w", scanErr)
		}
		projectList = append(projectList, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projectList, nil
}

func insertProject(ctx context.Context, tx pgx.Tx, organizationID string, input CreateInput) (Project, error) {
	project, err := scanBaseProject(tx.QueryRow(ctx, `
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
			status,
			canonical_revision,
			version,
			created_at,
			updated_at
	`, organizationID, input.Name, input.Slug, input.CanonicalRevision))
	if err != nil {
		return Project{}, err
	}
	if input.RootRepository == nil {
		return project, nil
	}

	repositoryInput := input.RootRepository
	var repository SourceRepository
	err = tx.QueryRow(ctx, `
		INSERT INTO coordination.repositories (
			organization_id,
			project_id,
			slug,
			name,
			remote_url,
			default_branch,
			object_format
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING
			id,
			slug,
			name,
			vcs_type,
			status,
			remote_url,
			default_branch,
			object_format,
			version
	`,
		organizationID,
		project.ID,
		repositoryInput.Slug,
		repositoryInput.Name,
		repositoryInput.RemoteURL,
		repositoryInput.DefaultBranch,
		repositoryInput.ObjectFormat,
	).Scan(
		&repository.ID,
		&repository.Slug,
		&repository.Name,
		&repository.VCSType,
		&repository.Status,
		&repository.RemoteURL,
		&repository.DefaultBranch,
		&repository.ObjectFormat,
		&repository.Version,
	)
	if err != nil {
		return Project{}, err
	}

	err = tx.QueryRow(ctx, `
		UPDATE identity.projects
		SET root_repository_id = $3,
		    status = 'active',
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1
		  AND id = $2
		RETURNING status, updated_at
	`, organizationID, project.ID, repository.ID).Scan(&project.Status, &project.UpdatedAt)
	if err != nil {
		return Project{}, err
	}
	project.RootRepository = &repository
	return project, nil
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
	var repository SourceRepository
	var (
		repositoryID            *string
		repositorySlug          *string
		repositoryName          *string
		repositoryVCSType       *string
		repositoryStatus        *string
		repositoryRemoteURL     *string
		repositoryDefaultBranch *string
		repositoryObjectFormat  *string
		repositoryVersion       *int64
	)
	err := row.Scan(
		&project.ID,
		&project.OrganizationID,
		&project.Name,
		&project.Slug,
		&project.Status,
		&project.CanonicalRevision,
		&project.Version,
		&project.CreatedAt,
		&project.UpdatedAt,
		&repositoryID,
		&repositorySlug,
		&repositoryName,
		&repositoryVCSType,
		&repositoryStatus,
		&repositoryRemoteURL,
		&repositoryDefaultBranch,
		&repositoryObjectFormat,
		&repositoryVersion,
	)
	if err == nil && repositoryID != nil {
		repository.ID = *repositoryID
		repository.Slug = *repositorySlug
		repository.Name = *repositoryName
		repository.VCSType = *repositoryVCSType
		repository.Status = *repositoryStatus
		repository.RemoteURL = repositoryRemoteURL
		repository.DefaultBranch = *repositoryDefaultBranch
		repository.ObjectFormat = *repositoryObjectFormat
		repository.Version = *repositoryVersion
		project.RootRepository = &repository
	}
	return project, err
}

func scanBaseProject(row rowScanner) (Project, error) {
	var project Project
	err := row.Scan(
		&project.ID,
		&project.OrganizationID,
		&project.Name,
		&project.Slug,
		&project.Status,
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
	if project.Status == "" {
		// Project creation always starts in initializing. Older stored responses
		// predate the status field, so reconstruct the original response rather
		// than leaking a later project transition into an idempotent replay.
		project.Status = "initializing"
	}
	return CreateResult{Project: project, Replayed: true}, nil
}

func mapProjectWriteError(err error) error {
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) &&
		postgresErr.Code == "23505" &&
		postgresErr.ConstraintName == "projects_tenant_slug_uq" {
		return ErrSlugTaken
	}
	if errors.As(err, &postgresErr) &&
		postgresErr.Code == "23505" &&
		postgresErr.ConstraintName == "repositories_tenant_remote_active_uq" {
		return ErrRepositoryTaken
	}
	return fmt.Errorf("create project: %w", err)
}

func rollbackTransaction(tx pgx.Tx) {
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackCtx)
}
