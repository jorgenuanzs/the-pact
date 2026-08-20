package workspaces

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
		return CreateResult{}, fmt.Errorf("begin workspace.create: %w", err)
	}
	defer rollback(tx)

	var commandID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.idempotency_records (
			organization_id, project_id, command_type, idempotency_key, request_hash
		)
		VALUES ($1, NULL, 'workspace.create', $2, $3)
		ON CONFLICT ON CONSTRAINT idempotency_records_scope_key_uq DO NOTHING
		RETURNING command_id
	`, organizationID, idempotencyKey, requestHash[:]).Scan(&commandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return loadStoredCreate(ctx, tx, organizationID, idempotencyKey, requestHash)
	}
	if err != nil {
		return CreateResult{}, fmt.Errorf("reserve workspace.create idempotency key: %w", err)
	}

	var workspaceID string
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.workspaces (organization_id, name, slug, description, settings)
		VALUES ($1, $2, $3, $4, jsonb_build_object('color', $5::text))
		RETURNING id
	`, organizationID, input.Name, input.Slug, input.Description, input.Color).Scan(&workspaceID)
	if err != nil {
		return CreateResult{}, mapWriteError(err)
	}
	for _, projectID := range input.ProjectIDs {
		if err := attachProject(ctx, tx, organizationID, workspaceID, projectID); err != nil {
			return CreateResult{}, err
		}
	}
	workspace, err := loadWorkspace(ctx, tx, organizationID, workspaceID)
	if err != nil {
		return CreateResult{}, err
	}
	responseBody, err := json.Marshal(workspace)
	if err != nil {
		return CreateResult{}, fmt.Errorf("encode workspace.create response: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET outcome = 'succeeded', response_status = 201, response_body = $3,
		    aggregate_id = $4, completed_at = transaction_timestamp()
		WHERE organization_id = $1 AND project_id IS NULL
		  AND command_type = 'workspace.create' AND idempotency_key = $2
	`, organizationID, idempotencyKey, responseBody, workspace.ID)
	if err != nil {
		return CreateResult{}, fmt.Errorf("store workspace.create result: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return CreateResult{}, errors.New("store workspace.create result: idempotency reservation disappeared")
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, fmt.Errorf("commit workspace.create: %w", err)
	}
	return CreateResult{Workspace: workspace}, nil
}

func (r *PostgresRepository) Get(ctx context.Context, organizationID, reference string) (Workspace, error) {
	return loadWorkspace(ctx, r.pool, organizationID, reference)
}

func (r *PostgresRepository) List(ctx context.Context, organizationID string) ([]Workspace, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, slug, description,
		       COALESCE(settings->>'color', $2), status, version,
		       created_at, updated_at, archived_at
		FROM identity.workspaces
		WHERE organization_id = $1
		ORDER BY status, lower(name), id
	`, organizationID, DefaultColor)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()
	result := make([]Workspace, 0)
	for rows.Next() {
		workspace, scanErr := scanWorkspace(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workspaces: %w", scanErr)
		}
		projects, projectErr := loadProjects(ctx, r.pool, organizationID, workspace.ID)
		if projectErr != nil {
			return nil, projectErr
		}
		workspace.Projects = projects
		result = append(result, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) Update(ctx context.Context, organizationID, reference string, input UpdateInput) (Workspace, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE identity.workspaces
		SET name = $3,
		    description = $4,
		    settings = jsonb_set(settings, '{color}', to_jsonb($5::text), true),
		    version = version + 1,
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND (id::text = $2 OR slug = $2)
	`, organizationID, reference, input.Name, input.Description, input.Color)
	if err != nil {
		return Workspace{}, fmt.Errorf("update workspace: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return Workspace{}, ErrNotFound
	}
	return loadWorkspace(ctx, r.pool, organizationID, reference)
}

func (r *PostgresRepository) AttachProject(ctx context.Context, organizationID, workspaceID, projectID string) (Workspace, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Workspace{}, fmt.Errorf("begin workspace.attach_project: %w", err)
	}
	defer rollback(tx)
	if _, err := loadWorkspace(ctx, tx, organizationID, workspaceID); err != nil {
		return Workspace{}, err
	}
	if err := attachProject(ctx, tx, organizationID, workspaceID, projectID); err != nil {
		return Workspace{}, err
	}
	workspace, err := loadWorkspace(ctx, tx, organizationID, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Workspace{}, fmt.Errorf("commit workspace.attach_project: %w", err)
	}
	return workspace, nil
}

type database interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadWorkspace(ctx context.Context, db database, organizationID, reference string) (Workspace, error) {
	workspace, err := scanWorkspace(db.QueryRow(ctx, `
		SELECT id, organization_id, name, slug, description,
		       COALESCE(settings->>'color', $3), status, version,
		       created_at, updated_at, archived_at
		FROM identity.workspaces
		WHERE organization_id = $1 AND (id::text = $2 OR slug = $2)
	`, organizationID, reference, DefaultColor))
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	workspace.Projects, err = loadProjects(ctx, db, organizationID, workspace.ID)
	if err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}

func loadProjects(ctx context.Context, db database, organizationID, workspaceID string) ([]Project, error) {
	rows, err := db.Query(ctx, `
		SELECT project.id, project.name, project.slug, project.status, repository.remote_url
		FROM identity.workspace_projects AS relation
		JOIN identity.projects AS project
		  ON project.organization_id = relation.organization_id
		 AND project.id = relation.project_id
		LEFT JOIN coordination.repositories AS repository
		  ON repository.organization_id = project.organization_id
		 AND repository.project_id = project.id
		 AND repository.id = project.root_repository_id
		WHERE relation.organization_id = $1 AND relation.workspace_id = $2
		ORDER BY lower(project.name), project.id
	`, organizationID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace projects: %w", err)
	}
	defer rows.Close()
	projects := make([]Project, 0)
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.Name, &project.Slug, &project.Status, &project.RootRepositoryRemoteURL); err != nil {
			return nil, fmt.Errorf("list workspace projects: %w", err)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workspace projects: %w", err)
	}
	return projects, nil
}

func attachProject(ctx context.Context, tx pgx.Tx, organizationID, workspaceID, projectID string) error {
	var previousWorkspaceID string
	err := tx.QueryRow(ctx, `
		SELECT workspace_id
		FROM identity.workspace_projects
		WHERE organization_id = $1 AND project_id = $2
	`, organizationID, projectID).Scan(&previousWorkspaceID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load current project workspace: %w", err)
	}
	if previousWorkspaceID == workspaceID {
		return nil
	}

	var attachedID string
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.workspace_projects (organization_id, workspace_id, project_id)
		SELECT $1, $2, project.id
		FROM identity.projects AS project
		WHERE project.organization_id = $1 AND project.id = $3
		ON CONFLICT ON CONSTRAINT workspace_projects_one_workspace_uq
		DO UPDATE SET workspace_id = EXCLUDED.workspace_id,
		              added_at = transaction_timestamp()
		RETURNING project_id
	`, organizationID, workspaceID, projectID).Scan(&attachedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProjectNotFound
	}
	if err != nil {
		return fmt.Errorf("attach project to workspace: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE identity.workspaces
		SET version = version + 1,
		    status = 'active',
		    archived_at = NULL,
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, workspaceID)
	if err != nil {
		return fmt.Errorf("touch workspace after project attachment: %w", err)
	}
	if previousWorkspaceID != "" {
		_, err = tx.Exec(ctx, `
			UPDATE identity.workspaces AS workspace
			SET version = version + 1,
			    status = CASE
			        WHEN workspace.settings @> '{"managed_default": true}'::jsonb
			         AND NOT EXISTS (
			             SELECT 1 FROM identity.workspace_projects AS relation
			             WHERE relation.organization_id = workspace.organization_id
			               AND relation.workspace_id = workspace.id
			         ) THEN 'archived'
			        ELSE workspace.status
			    END,
			    archived_at = CASE
			        WHEN workspace.settings @> '{"managed_default": true}'::jsonb
			         AND NOT EXISTS (
			             SELECT 1 FROM identity.workspace_projects AS relation
			             WHERE relation.organization_id = workspace.organization_id
			               AND relation.workspace_id = workspace.id
			         ) THEN transaction_timestamp()
			        ELSE workspace.archived_at
			    END,
			    updated_at = transaction_timestamp()
			WHERE workspace.organization_id = $1 AND workspace.id = $2
		`, organizationID, previousWorkspaceID)
		if err != nil {
			return fmt.Errorf("touch previous workspace after project attachment: %w", err)
		}
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanWorkspace(row rowScanner) (Workspace, error) {
	var workspace Workspace
	err := row.Scan(
		&workspace.ID, &workspace.OrganizationID, &workspace.Name, &workspace.Slug,
		&workspace.Description, &workspace.Color, &workspace.Status, &workspace.Version,
		&workspace.CreatedAt, &workspace.UpdatedAt, &workspace.ArchivedAt,
	)
	if workspace.Projects == nil {
		workspace.Projects = make([]Project, 0)
	}
	return workspace, err
}

func loadStoredCreate(ctx context.Context, tx pgx.Tx, organizationID, key string, requestHash [sha256.Size]byte) (CreateResult, error) {
	var storedHash []byte
	var outcome *string
	var responseBody []byte
	err := tx.QueryRow(ctx, `
		SELECT request_hash, outcome, response_body
		FROM platform.idempotency_records
		WHERE organization_id = $1 AND project_id IS NULL
		  AND command_type = 'workspace.create' AND idempotency_key = $2
	`, organizationID, key).Scan(&storedHash, &outcome, &responseBody)
	if err != nil {
		return CreateResult{}, fmt.Errorf("load workspace.create result: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return CreateResult{}, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(responseBody) == 0 {
		return CreateResult{}, ErrCommandIncomplete
	}
	var workspace Workspace
	if err := json.Unmarshal(responseBody, &workspace); err != nil {
		return CreateResult{}, fmt.Errorf("decode workspace.create result: %w", err)
	}
	return CreateResult{Workspace: workspace, Replayed: true}, nil
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "workspaces_tenant_slug_uq" {
		return ErrSlugTaken
	}
	return fmt.Errorf("create workspace: %w", err)
}

func rollback(tx pgx.Tx) {
	_ = tx.Rollback(context.Background())
}
