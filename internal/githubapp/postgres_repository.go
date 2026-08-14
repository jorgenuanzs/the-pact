package githubapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateAttempt(
	ctx context.Context, organizationID, principalID string, digest [sha256.Size]byte, expiresAt time.Time,
) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO integrations.github_connection_attempts (
			organization_id, principal_id, state_digest, expires_at
		) VALUES ($1, $2, $3, $4)
	`, organizationID, principalID, digest[:], expiresAt)
	if err != nil {
		return fmt.Errorf("create GitHub connection attempt: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ConsumeAttempt(
	ctx context.Context, digest [sha256.Size]byte,
) (string, string, int64, error) {
	var organizationID, principalID string
	var installationID int64
	err := r.pool.QueryRow(ctx, `
		UPDATE integrations.github_connection_attempts
		SET status = 'completed', consumed_at = transaction_timestamp()
		WHERE state_digest = $1 AND status = 'pending'
		  AND expires_at > transaction_timestamp() AND installation_id IS NOT NULL
		RETURNING organization_id, principal_id, installation_id
	`, digest[:]).Scan(&organizationID, &principalID, &installationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", 0, ErrInvalidState
	}
	if err != nil {
		return "", "", 0, fmt.Errorf("consume GitHub connection attempt: %w", err)
	}
	return organizationID, principalID, installationID, nil
}

func (r *PostgresRepository) SetAttemptInstallation(
	ctx context.Context, digest [sha256.Size]byte, installationID int64,
) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE integrations.github_connection_attempts
		SET installation_id = $2
		WHERE state_digest = $1 AND status = 'pending'
		  AND expires_at > transaction_timestamp()
		  AND (installation_id IS NULL OR installation_id = $2)
	`, digest[:], installationID)
	if err != nil {
		return fmt.Errorf("record GitHub installation for connection: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidState
	}
	return nil
}

func (r *PostgresRepository) UpsertInstallation(
	ctx context.Context, organizationID string, installation ProviderInstallation, repositories []ProviderRepository,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin GitHub installation update: %w", err)
	}
	defer rollback(tx)
	permissions, err := json.Marshal(installation.Permissions)
	if err != nil {
		return fmt.Errorf("encode GitHub App permissions: %w", err)
	}
	status := "active"
	if installation.SuspendedAt != nil {
		status = "suspended"
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO integrations.github_installations (
			organization_id, installation_id, account_id, account_login, account_type,
			repository_selection, permissions, status, suspended_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (organization_id, installation_id) DO UPDATE
		SET account_id = EXCLUDED.account_id,
		    account_login = EXCLUDED.account_login,
		    account_type = EXCLUDED.account_type,
		    repository_selection = EXCLUDED.repository_selection,
		    permissions = EXCLUDED.permissions,
		    status = EXCLUDED.status,
		    suspended_at = EXCLUDED.suspended_at,
		    deleted_at = NULL,
		    updated_at = transaction_timestamp(),
		    version = integrations.github_installations.version + 1
	`, organizationID, installation.ID, installation.AccountID, installation.AccountLogin,
		installation.AccountType, installation.RepositorySelection, permissions, status, installation.SuspendedAt)
	if err != nil {
		return fmt.Errorf("upsert GitHub installation: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE integrations.github_repositories
		SET status = 'removed', updated_at = transaction_timestamp(), version = version + 1
		WHERE organization_id = $1 AND installation_id = $2 AND status <> 'removed'
	`, organizationID, installation.ID); err != nil {
		return fmt.Errorf("mark previous GitHub repositories removed: %w", err)
	}
	for _, repository := range repositories {
		_, err := tx.Exec(ctx, `
			INSERT INTO integrations.github_repositories (
				organization_id, github_repository_id, installation_id, owner_login,
				name, full_name, private, visibility, default_branch, html_url,
				clone_url, status, provider_updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active', $12)
			ON CONFLICT (organization_id, github_repository_id) DO UPDATE
			SET installation_id = EXCLUDED.installation_id,
			    owner_login = EXCLUDED.owner_login,
			    name = EXCLUDED.name,
			    full_name = EXCLUDED.full_name,
			    private = EXCLUDED.private,
			    visibility = EXCLUDED.visibility,
			    default_branch = EXCLUDED.default_branch,
			    html_url = EXCLUDED.html_url,
			    clone_url = EXCLUDED.clone_url,
			    status = 'active',
			    provider_updated_at = EXCLUDED.provider_updated_at,
			    updated_at = transaction_timestamp(),
			    version = integrations.github_repositories.version + 1
		`, organizationID, repository.ID, installation.ID, repository.OwnerLogin,
			repository.Name, repository.FullName, repository.Private, repository.Visibility,
			repository.DefaultBranch, repository.HTMLURL, repository.CloneURL,
			repository.ProviderUpdatedAt)
		if err != nil {
			return fmt.Errorf("upsert GitHub repository %s: %w", repository.FullName, err)
		}
	}
	if status != "active" {
		if _, err := tx.Exec(ctx, `
			UPDATE integrations.github_repositories
			SET status = 'unavailable', updated_at = transaction_timestamp(), version = version + 1
			WHERE organization_id = $1 AND installation_id = $2 AND status = 'active'
		`, organizationID, installation.ID); err != nil {
			return fmt.Errorf("suspend GitHub repositories: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit GitHub installation update: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpdateInstallationStatus(
	ctx context.Context, organizationID string, installationID int64, status string, at time.Time,
) error {
	var suspendedAt, deletedAt *time.Time
	if status == "suspended" {
		suspendedAt = &at
	}
	if status == "deleted" {
		deletedAt = &at
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin GitHub installation status update: %w", err)
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, `
		UPDATE integrations.github_installations
		SET status = $3, suspended_at = $4, deleted_at = $5,
		    updated_at = transaction_timestamp(), version = version + 1
		WHERE organization_id = $1 AND installation_id = $2
	`, organizationID, installationID, status, suspendedAt, deletedAt)
	if err != nil {
		return fmt.Errorf("update GitHub installation status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	repositoryStatus := "unavailable"
	if status == "deleted" {
		repositoryStatus = "removed"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE integrations.github_repositories
		SET status = $3, updated_at = transaction_timestamp(), version = version + 1
		WHERE organization_id = $1 AND installation_id = $2 AND status <> $3
	`, organizationID, installationID, repositoryStatus); err != nil {
		return fmt.Errorf("update GitHub repository availability: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit GitHub installation status update: %w", err)
	}
	return nil
}

func (r *PostgresRepository) Status(ctx context.Context, organizationID string, configured bool) (Status, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT installation_id, account_id, account_login, account_type,
		       repository_selection, permissions, status, installed_at,
		       suspended_at, updated_at, version
		FROM integrations.github_installations
		WHERE organization_id = $1 AND status <> 'deleted'
		ORDER BY account_login, installation_id
	`, organizationID)
	if err != nil {
		return Status{}, fmt.Errorf("list GitHub installations: %w", err)
	}
	defer rows.Close()
	result := Status{Configured: configured, Installations: make([]Installation, 0)}
	for rows.Next() {
		var installation Installation
		var permissions []byte
		if err := rows.Scan(
			&installation.InstallationID, &installation.AccountID, &installation.AccountLogin,
			&installation.AccountType, &installation.RepositorySelection, &permissions,
			&installation.Status, &installation.InstalledAt, &installation.SuspendedAt,
			&installation.UpdatedAt, &installation.Version,
		); err != nil {
			return Status{}, fmt.Errorf("scan GitHub installation: %w", err)
		}
		if err := json.Unmarshal(permissions, &installation.Permissions); err != nil {
			return Status{}, fmt.Errorf("decode GitHub installation permissions: %w", err)
		}
		result.Installations = append(result.Installations, installation)
	}
	if err := rows.Err(); err != nil {
		return Status{}, fmt.Errorf("list GitHub installations: %w", err)
	}
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM integrations.github_repositories
		WHERE organization_id = $1 AND status = 'active'
	`, organizationID).Scan(&result.RepositoryCount); err != nil {
		return Status{}, fmt.Errorf("count authorized GitHub repositories: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) InstallationForRepository(
	ctx context.Context, organizationID, fullName string,
) (int64, int64, error) {
	var installationID, repositoryID int64
	err := r.pool.QueryRow(ctx, `
		SELECT repository.installation_id, repository.github_repository_id
		FROM integrations.github_repositories AS repository
		JOIN integrations.github_installations AS installation
		  ON installation.organization_id = repository.organization_id
		 AND installation.installation_id = repository.installation_id
		WHERE repository.organization_id = $1
		  AND lower(repository.full_name) = lower($2)
		  AND repository.status = 'active'
		  AND installation.status = 'active'
	`, organizationID, fullName).Scan(&installationID, &repositoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, fmt.Errorf("resolve GitHub repository installation: %w", err)
	}
	return installationID, repositoryID, nil
}

func (r *PostgresRepository) BeginDelivery(
	ctx context.Context, organizationID, deliveryID, eventType string, payloadHash [sha256.Size]byte,
) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO integrations.github_webhook_deliveries (
			organization_id, delivery_id, event_type, payload_hash
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id, delivery_id) DO NOTHING
	`, organizationID, deliveryID, eventType, payloadHash[:])
	if err != nil {
		return false, fmt.Errorf("reserve GitHub webhook delivery: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *PostgresRepository) CompleteDelivery(
	ctx context.Context, organizationID, deliveryID, status, errorCode string,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE integrations.github_webhook_deliveries
		SET status = $3, error_code = NULLIF($4, ''), processed_at = transaction_timestamp()
		WHERE organization_id = $1 AND delivery_id = $2
	`, organizationID, deliveryID, status, errorCode)
	if err != nil {
		return fmt.Errorf("complete GitHub webhook delivery: %w", err)
	}
	return nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
