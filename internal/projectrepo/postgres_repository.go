package projectrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) List(
	ctx context.Context, organizationID, projectID string,
) ([]Repository, error) {
	rows, err := r.pool.Query(ctx, repositorySelectSQL+`
		WHERE repository.organization_id = $1 AND repository.project_id = $2
		ORDER BY (repository.id = project.root_repository_id) DESC,
		         repository.status = 'active' DESC, repository.purpose, repository.name
	`, organizationID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project repositories: %w", err)
	}
	defer rows.Close()
	result := make([]Repository, 0)
	for rows.Next() {
		repository, err := scanRepository(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project repository: %w", err)
		}
		result = append(result, repository)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list project repositories: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListAvailable(
	ctx context.Context, organizationID, projectID string,
) ([]AvailableRepository, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT github.github_repository_id, github.installation_id,
		       installation.account_login, github.name, github.full_name,
		       github.private, github.visibility, github.default_branch,
		       github.html_url, github.clone_url, repository.id, github.updated_at
		FROM integrations.github_repositories AS github
		JOIN integrations.github_installations AS installation
		  ON installation.organization_id = github.organization_id
		 AND installation.installation_id = github.installation_id
		LEFT JOIN coordination.repositories AS repository
		  ON repository.organization_id = github.organization_id
		 AND repository.project_id = $2
		 AND repository.github_repository_id = github.github_repository_id
		 AND repository.status <> 'archived'
		WHERE github.organization_id = $1 AND github.status = 'active'
		  AND installation.status = 'active'
		ORDER BY github.full_name
	`, organizationID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list authorized GitHub repositories: %w", err)
	}
	defer rows.Close()
	result := make([]AvailableRepository, 0)
	for rows.Next() {
		var repository AvailableRepository
		if err := rows.Scan(
			&repository.GitHubRepositoryID, &repository.InstallationID,
			&repository.AccountLogin, &repository.Name, &repository.FullName,
			&repository.Private, &repository.Visibility, &repository.DefaultBranch,
			&repository.HTMLURL, &repository.CloneURL, &repository.AttachedRepositoryID,
			&repository.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan authorized GitHub repository: %w", err)
		}
		result = append(result, repository)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list authorized GitHub repositories: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) Attach(
	ctx context.Context, organizationID, principalID, projectID string, input AttachInput,
) (Repository, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Repository{}, fmt.Errorf("begin repository attachment: %w", err)
	}
	defer rollback(tx)
	var rootRepositoryID *string
	if err := tx.QueryRow(ctx, `
		SELECT root_repository_id FROM identity.projects
		WHERE organization_id = $1 AND id = $2 AND status <> 'archived'
		FOR UPDATE
	`, organizationID, projectID).Scan(&rootRepositoryID); errors.Is(err, pgx.ErrNoRows) {
		return Repository{}, ErrNotFound
	} else if err != nil {
		return Repository{}, fmt.Errorf("lock project for repository attachment: %w", err)
	}

	var provider struct {
		id                                               int64
		name, fullName, cloneURL, htmlURL, defaultBranch string
	}
	err = tx.QueryRow(ctx, `
		SELECT github_repository_id, name, full_name, clone_url, html_url, default_branch
		FROM integrations.github_repositories
		WHERE organization_id = $1 AND github_repository_id = $2 AND status = 'active'
	`, organizationID, input.GitHubRepositoryID).Scan(
		&provider.id, &provider.name, &provider.fullName, &provider.cloneURL,
		&provider.htmlURL, &provider.defaultBranch,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Repository{}, ErrProviderNotFound
	}
	if err != nil {
		return Repository{}, fmt.Errorf("load authorized GitHub repository: %w", err)
	}

	required := true
	if input.Required != nil {
		required = *input.Required
	}
	var repositoryID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM coordination.repositories
		WHERE organization_id = $1 AND project_id = $2 AND status <> 'archived'
		  AND (
			github_repository_id = $3
			OR lower(regexp_replace(regexp_replace(coalesce(remote_url, ''),
			   '^(https?://github.com/|git@github.com:|ssh://git@github.com/)', '', 'i'),
			   '\.git/?$', '', 'i')) = lower($4)
		  )
		ORDER BY github_repository_id IS NOT NULL DESC
		LIMIT 1
		FOR UPDATE
	`, organizationID, projectID, provider.id, provider.fullName).Scan(&repositoryID)
	if err == nil {
		_, err = tx.Exec(ctx, `
			UPDATE coordination.repositories
			SET github_repository_id = $4, purpose = $5, is_required = $6,
			    default_branch = $7, status = 'active', archived_at = NULL,
			    updated_at = transaction_timestamp(), version = version + 1
			WHERE organization_id = $1 AND project_id = $2 AND id = $3
		`, organizationID, projectID, repositoryID, provider.id, input.Purpose, required, provider.defaultBranch)
		if err != nil {
			return Repository{}, fmt.Errorf("link existing project repository: %w", err)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Repository{}, fmt.Errorf("find existing project repository: %w", err)
	} else {
		slug := repositorySlug(provider.name)
		baseSlug := slug
		for suffix := 2; ; suffix++ {
			var exists bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM coordination.repositories
				WHERE organization_id = $1 AND project_id = $2 AND slug = $3)
			`, organizationID, projectID, slug).Scan(&exists); err != nil {
				return Repository{}, fmt.Errorf("check repository slug: %w", err)
			}
			if !exists {
				break
			}
			suffixText := fmt.Sprintf("-%d", suffix)
			trimmed := strings.TrimRight(baseSlug[:min(len(baseSlug), 63-len(suffixText))], "-")
			slug = trimmed + suffixText
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO coordination.repositories (
				organization_id, project_id, slug, name, remote_url, default_branch,
				github_repository_id, purpose, is_required
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`, organizationID, projectID, slug, provider.name, provider.cloneURL,
			provider.defaultBranch, provider.id, input.Purpose, required).Scan(&repositoryID)
		if err != nil {
			return Repository{}, fmt.Errorf("attach GitHub repository to project: %w", err)
		}
	}

	primary := input.Primary || rootRepositoryID == nil
	if primary {
		if rootRepositoryID != nil && *rootRepositoryID == repositoryID {
			_, err = tx.Exec(ctx, `
				UPDATE identity.projects
				SET status = 'active', updated_at = transaction_timestamp(), version = version + 1
				WHERE organization_id = $1 AND id = $2
			`, organizationID, projectID)
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE identity.projects
				SET root_repository_id = $3,
				    canonical_revision = (
				        SELECT state.canonical_revision
				        FROM coordination.repository_provider_states AS state
				        WHERE state.organization_id = $1 AND state.project_id = $2
				          AND state.repository_id = $3 AND state.status = 'synced'
				    ),
				    status = 'active', updated_at = transaction_timestamp(), version = version + 1
				WHERE organization_id = $1 AND id = $2
			`, organizationID, projectID, repositoryID)
		}
		if err != nil {
			return Repository{}, fmt.Errorf("set primary project repository: %w", err)
		}
	}
	repository, err := scanRepository(tx.QueryRow(ctx, repositorySelectSQL+`
		WHERE repository.organization_id = $1 AND repository.project_id = $2 AND repository.id = $3
	`, organizationID, projectID, repositoryID))
	if err != nil {
		return Repository{}, fmt.Errorf("load attached project repository: %w", err)
	}
	if err := appendRepositoryAttachedEvent(ctx, tx, organizationID, principalID, repository); err != nil {
		return Repository{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Repository{}, fmt.Errorf("commit repository attachment: %w", err)
	}
	return repository, nil
}

func appendRepositoryAttachedEvent(
	ctx context.Context, tx pgx.Tx, organizationID, principalID string, repository Repository,
) error {
	var sequence int64
	err := tx.QueryRow(ctx, `
		WITH next_sequence AS (
			UPDATE platform.project_event_counters
			SET last_sequence = last_sequence + 1, updated_at = transaction_timestamp()
			WHERE organization_id = $1 AND project_id = $2
			RETURNING last_sequence
		)
		UPDATE identity.projects AS project
		SET event_sequence = next_sequence.last_sequence
		FROM next_sequence
		WHERE project.organization_id = $1 AND project.id = $2
		RETURNING next_sequence.last_sequence
	`, organizationID, repository.ProjectID).Scan(&sequence)
	if err != nil {
		return fmt.Errorf("allocate repository attachment event sequence: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"repository_id": repository.ID, "github_repository_id": repository.GitHubRepositoryID,
		"repository_full_name": repository.GitHubFullName, "purpose": repository.Purpose,
		"required": repository.Required, "primary": repository.Primary,
	})
	if err != nil {
		return fmt.Errorf("encode repository attachment event: %w", err)
	}
	var commandID string
	if err := tx.QueryRow(ctx, `SELECT uuidv7()`).Scan(&commandID); err != nil {
		return fmt.Errorf("allocate repository attachment command: %w", err)
	}
	var eventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.events (
			organization_id, project_id, project_sequence, event_type, event_version,
			aggregate_type, aggregate_id, aggregate_version, actor_id,
			command_id, correlation_id, payload, payload_hash
		) VALUES (
			$1, $2, $3, 'pact.project.repository_attached.v1', 1,
			'repository', $4, $5, $6, $7, $7, $8,
			sha256(convert_to(($8::jsonb)::text, 'UTF8'))
		) RETURNING id
	`, organizationID, repository.ProjectID, sequence, repository.ID,
		repository.Version, principalID, commandID, payload).Scan(&eventID)
	if err != nil {
		return fmt.Errorf("append repository attachment event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform.outbox (organization_id, project_id, event_id, channel)
		VALUES ($1, $2, $3, 'project-events')
	`, organizationID, repository.ProjectID, eventID); err != nil {
		return fmt.Errorf("enqueue repository attachment event: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetSource(
	ctx context.Context, organizationID, projectID, repositoryID string,
) (projects.SourceRepository, error) {
	source, err := scanSource(r.pool.QueryRow(ctx, sourceSelectSQL+`
		WHERE organization_id = $1 AND project_id = $2 AND id = $3 AND status = 'active'
	`, organizationID, projectID, repositoryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return projects.SourceRepository{}, ErrNotFound
	}
	if err != nil {
		return projects.SourceRepository{}, fmt.Errorf("get project repository source: %w", err)
	}
	return source, nil
}

func (r *PostgresRepository) ListSources(
	ctx context.Context, organizationID, projectID string,
) ([]projects.SourceRepository, error) {
	rows, err := r.pool.Query(ctx, sourceSelectSQL+`
		WHERE organization_id = $1 AND project_id = $2 AND status = 'active'
		ORDER BY created_at, id
	`, organizationID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project repository sources: %w", err)
	}
	defer rows.Close()
	result := make([]projects.SourceRepository, 0)
	for rows.Next() {
		source, err := scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project repository source: %w", err)
		}
		result = append(result, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list project repository sources: %w", err)
	}
	return result, nil
}

const repositorySelectSQL = `
	SELECT repository.id, repository.project_id, repository.github_repository_id,
	       repository.slug, repository.name, repository.vcs_type, repository.status,
	       repository.remote_url, repository.default_branch, repository.object_format,
	       repository.purpose, repository.is_required,
	       repository.id = project.root_repository_id,
	       coalesce(github.full_name, ''), coalesce(github.visibility, ''),
	       coalesce(state.status, 'never'), state.canonical_revision,
	       state.last_success_at, repository.version
	FROM coordination.repositories AS repository
	JOIN identity.projects AS project
	  ON project.organization_id = repository.organization_id AND project.id = repository.project_id
	LEFT JOIN integrations.github_repositories AS github
	  ON github.organization_id = repository.organization_id
	 AND github.github_repository_id = repository.github_repository_id
	LEFT JOIN coordination.repository_provider_states AS state
	  ON state.organization_id = repository.organization_id
	 AND state.project_id = repository.project_id
	 AND state.repository_id = repository.id
`

const sourceSelectSQL = `
	SELECT id, slug, name, vcs_type, status, remote_url, default_branch, object_format, version
	FROM coordination.repositories
`

type scanner interface{ Scan(...any) error }

func scanRepository(row scanner) (Repository, error) {
	var repository Repository
	err := row.Scan(
		&repository.ID, &repository.ProjectID, &repository.GitHubRepositoryID,
		&repository.Slug, &repository.Name, &repository.VCSType, &repository.Status,
		&repository.RemoteURL, &repository.DefaultBranch, &repository.ObjectFormat,
		&repository.Purpose, &repository.Required, &repository.Primary,
		&repository.GitHubFullName, &repository.Visibility, &repository.SyncStatus,
		&repository.CanonicalRevision, &repository.LastSuccessAt, &repository.Version,
	)
	return repository, err
}

func scanSource(row scanner) (projects.SourceRepository, error) {
	var source projects.SourceRepository
	err := row.Scan(&source.ID, &source.Slug, &source.Name, &source.VCSType, &source.Status,
		&source.RemoteURL, &source.DefaultBranch, &source.ObjectFormat, &source.Version)
	return source, err
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func repositorySlug(value string) string {
	value = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(value), "-"), "-")
	if value == "" {
		value = "repository"
	}
	if len(value) > 63 {
		value = strings.TrimRight(value[:63], "-")
	}
	return value
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
