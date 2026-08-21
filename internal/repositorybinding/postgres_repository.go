package repositorybinding

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListCandidates(ctx context.Context, organizationID string) ([]Candidate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT workspace.id, workspace.name, workspace.slug,
		       project.id, project.name,
		       repository.id, repository.name, repository.slug,
		       repository.id = project.root_repository_id,
		       repository.remote_url
		FROM coordination.repositories AS repository
		JOIN identity.projects AS project
		  ON project.organization_id = repository.organization_id
		 AND project.id = repository.project_id
		JOIN identity.workspace_projects AS relation
		  ON relation.organization_id = project.organization_id
		 AND relation.project_id = project.id
		JOIN identity.workspaces AS workspace
		  ON workspace.organization_id = relation.organization_id
		 AND workspace.id = relation.workspace_id
		WHERE repository.organization_id = $1
		  AND repository.status = 'active'
		  AND project.status <> 'archived'
		  AND workspace.status <> 'archived'
		  AND repository.remote_url IS NOT NULL
		ORDER BY lower(workspace.name), lower(project.name), lower(repository.name), repository.id
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list repository binding candidates: %w", err)
	}
	defer rows.Close()
	result := make([]Candidate, 0)
	for rows.Next() {
		var candidate Candidate
		if err := rows.Scan(
			&candidate.WorkspaceID, &candidate.WorkspaceName, &candidate.WorkspaceSlug,
			&candidate.ProjectID, &candidate.ProjectName,
			&candidate.RepositoryID, &candidate.RepositoryName, &candidate.RepositorySlug,
			&candidate.Primary, &candidate.RemoteURL,
		); err != nil {
			return nil, fmt.Errorf("scan repository binding candidate: %w", err)
		}
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list repository binding candidates: %w", err)
	}
	return result, nil
}
