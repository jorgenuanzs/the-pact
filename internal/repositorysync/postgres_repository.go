package repositorysync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	syncCommandType = "repository.sync"
	syncedEventType = "pact.repository.canonical_synced.v1"
	failedEventType = "pact.repository.sync_failed.v1"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Get(
	ctx context.Context, organizationID, projectID, repositoryID string,
) (State, bool, error) {
	state, err := scanState(r.pool.QueryRow(ctx, stateSelectSQL+`
		WHERE organization_id = $1 AND project_id = $2 AND repository_id = $3
	`, organizationID, projectID, repositoryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("get repository provider state: %w", err)
	}
	return state, true, nil
}

func (r *PostgresRepository) Replay(
	ctx context.Context, organizationID, projectID, key string, requestHash [sha256.Size]byte,
) (Result, bool, error) {
	var storedHash []byte
	var outcome *string
	var body json.RawMessage
	err := r.pool.QueryRow(ctx, `
		SELECT request_hash, outcome, response_body
		FROM platform.idempotency_records
		WHERE organization_id = $1 AND project_id = $2
		  AND command_type = $3 AND idempotency_key = $4
	`, organizationID, projectID, syncCommandType, key).Scan(&storedHash, &outcome, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, fmt.Errorf("lookup stored repository sync result: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return Result{}, false, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(body) == 0 {
		return Result{}, false, ErrCommandIncomplete
	}
	var result Result
	if err := json.Unmarshal(body, &result); err != nil {
		return Result{}, false, fmt.Errorf("decode stored repository sync result: %w", err)
	}
	return result, true, nil
}

func (r *PostgresRepository) Apply(
	ctx context.Context,
	organizationID, principalID, projectID, repositoryID, idempotencyKey string,
	requestHash [sha256.Size]byte,
	snapshot Snapshot,
) (Result, error) {
	return r.apply(ctx, organizationID, principalID, projectID, repositoryID, idempotencyKey, &requestHash, snapshot)
}

func (r *PostgresRepository) ApplyScheduled(
	ctx context.Context, organizationID, projectID, repositoryID string, snapshot Snapshot,
) (Result, error) {
	return r.apply(ctx, organizationID, "", projectID, repositoryID, "", nil, snapshot)
}

func (r *PostgresRepository) apply(
	ctx context.Context,
	organizationID, principalID, projectID, repositoryID, idempotencyKey string,
	requestHash *[sha256.Size]byte,
	snapshot Snapshot,
) (Result, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("begin repository sync: %w", err)
	}
	defer rollback(tx)
	if err := lockProject(ctx, tx, organizationID, projectID); err != nil {
		return Result{}, err
	}

	var commandID string
	if requestHash != nil {
		reservedCommandID, stored, replayed, err := reserveSyncCommand(
			ctx, tx, organizationID, projectID, idempotencyKey, *requestHash,
		)
		if err != nil {
			return Result{}, err
		}
		if replayed {
			stored.Replayed = true
			return stored, nil
		}
		commandID = reservedCommandID
	} else if err := tx.QueryRow(ctx, `SELECT uuidv7()`).Scan(&commandID); err != nil {
		return Result{}, fmt.Errorf("allocate scheduled repository sync command: %w", err)
	}

	var projectRevision *string
	var repositoryBranch string
	var primary bool
	err = tx.QueryRow(ctx, `
		SELECT project.canonical_revision, repository.default_branch,
		       repository.id = project.root_repository_id
		FROM identity.projects AS project
		JOIN coordination.repositories AS repository
		  ON repository.organization_id = project.organization_id
		 AND repository.project_id = project.id
		WHERE project.organization_id = $1 AND project.id = $2
		  AND project.status <> 'archived' AND repository.id = $3
		  AND repository.status = 'active'
		FOR UPDATE OF project, repository
	`, organizationID, projectID, repositoryID).Scan(&projectRevision, &repositoryBranch, &primary)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrRepositoryUnavailable
	}
	if err != nil {
		return Result{}, fmt.Errorf("lock project repository for sync: %w", err)
	}

	previous, found, err := loadState(ctx, tx, organizationID, projectID, repositoryID, true)
	if err != nil {
		return Result{}, err
	}
	changed := !found || previous.Status != StatusSynced ||
		previous.Provider != snapshot.Provider ||
		previous.RepositoryFullName != snapshot.RepositoryFullName ||
		previous.DefaultBranch != snapshot.DefaultBranch ||
		stringValue(previous.CanonicalRevision) != snapshot.CanonicalRevision ||
		previous.Visibility != snapshot.Visibility ||
		(primary && stringValue(projectRevision) != snapshot.CanonicalRevision) ||
		repositoryBranch != snapshot.DefaultBranch

	var state State
	if found {
		state, err = scanState(tx.QueryRow(ctx, `
			UPDATE coordination.repository_provider_states
			SET provider = $4, repository_full_name = $5, status = 'synced',
			    default_branch = $6, canonical_revision = $7, visibility = $8,
			    provider_updated_at = $9, last_attempt_at = transaction_timestamp(),
			    last_success_at = transaction_timestamp(), last_error_code = NULL,
			    version = version + CASE WHEN $10 THEN 1 ELSE 0 END,
			    updated_at = CASE WHEN $10 THEN transaction_timestamp() ELSE updated_at END
			WHERE organization_id = $1 AND project_id = $2 AND repository_id = $3
			RETURNING repository_id, project_id, provider, repository_full_name, status,
			          default_branch, canonical_revision, visibility, provider_updated_at,
			          last_attempt_at, last_success_at, last_error_code, version
		`, organizationID, projectID, repositoryID, snapshot.Provider,
			snapshot.RepositoryFullName, snapshot.DefaultBranch, snapshot.CanonicalRevision,
			snapshot.Visibility, snapshot.ProviderUpdatedAt, changed))
	} else {
		state, err = scanState(tx.QueryRow(ctx, `
			INSERT INTO coordination.repository_provider_states (
				repository_id, organization_id, project_id, provider,
				repository_full_name, status, default_branch, canonical_revision,
				visibility, provider_updated_at, last_attempt_at, last_success_at
			)
			VALUES ($1, $2, $3, $4, $5, 'synced', $6, $7, $8, $9,
			        transaction_timestamp(), transaction_timestamp())
			RETURNING repository_id, project_id, provider, repository_full_name, status,
			          default_branch, canonical_revision, visibility, provider_updated_at,
			          last_attempt_at, last_success_at, last_error_code, version
		`, repositoryID, organizationID, projectID, snapshot.Provider,
			snapshot.RepositoryFullName, snapshot.DefaultBranch, snapshot.CanonicalRevision,
			snapshot.Visibility, snapshot.ProviderUpdatedAt))
	}
	if err != nil {
		return Result{}, fmt.Errorf("store repository provider state: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE coordination.repositories
		SET default_branch = $4,
		    version = version + CASE WHEN default_branch IS DISTINCT FROM $4 THEN 1 ELSE 0 END,
		    updated_at = CASE WHEN default_branch IS DISTINCT FROM $4 THEN transaction_timestamp() ELSE updated_at END
		WHERE organization_id = $1 AND project_id = $2 AND id = $3
	`, organizationID, projectID, repositoryID, snapshot.DefaultBranch); err != nil {
		return Result{}, fmt.Errorf("update root repository branch: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.projects
		SET canonical_revision = $3,
		    version = version + CASE WHEN canonical_revision IS DISTINCT FROM $3 THEN 1 ELSE 0 END,
		    updated_at = CASE WHEN canonical_revision IS DISTINCT FROM $3 THEN transaction_timestamp() ELSE updated_at END
		WHERE organization_id = $1 AND id = $2
		  AND root_repository_id = $4
	`, organizationID, projectID, snapshot.CanonicalRevision, repositoryID); err != nil {
		return Result{}, fmt.Errorf("update project canonical revision: %w", err)
	}

	result := Result{State: state, Changed: changed}
	if changed {
		eventID, err := appendSyncEvent(
			ctx, tx, organizationID, projectID, commandID, principalID,
			syncedEventType, repositoryID, state.Version,
			snapshot.CanonicalRevision, map[string]any{"repository_sync": state},
		)
		if err != nil {
			return Result{}, err
		}
		result.EventID = &eventID
	}
	if requestHash != nil {
		if err := completeSyncCommand(
			ctx, tx, organizationID, projectID, idempotencyKey, commandID, result,
		); err != nil {
			return Result{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit repository sync: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) RecordFailure(
	ctx context.Context,
	organizationID, projectID, repositoryID, repositoryFullName, errorCode string,
) (State, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return State{}, fmt.Errorf("begin repository sync failure: %w", err)
	}
	defer rollback(tx)
	if err := lockProject(ctx, tx, organizationID, projectID); err != nil {
		return State{}, err
	}

	var defaultBranch string
	var repositoryRevision *string
	err = tx.QueryRow(ctx, `
		SELECT repository.default_branch,
		       CASE WHEN repository.id = project.root_repository_id
		            THEN project.canonical_revision ELSE state.canonical_revision END
		FROM identity.projects AS project
		JOIN coordination.repositories AS repository
		  ON repository.organization_id = project.organization_id
		 AND repository.project_id = project.id
		LEFT JOIN coordination.repository_provider_states AS state
		  ON state.organization_id = repository.organization_id
		 AND state.project_id = repository.project_id
		 AND state.repository_id = repository.id
		WHERE project.organization_id = $1 AND project.id = $2
		  AND repository.id = $3 AND repository.status = 'active'
		FOR UPDATE OF project, repository
	`, organizationID, projectID, repositoryID).Scan(&defaultBranch, &repositoryRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return State{}, ErrRepositoryUnavailable
	}
	if err != nil {
		return State{}, fmt.Errorf("lock project repository for failed sync: %w", err)
	}

	previous, found, err := loadState(ctx, tx, organizationID, projectID, repositoryID, true)
	if err != nil {
		return State{}, err
	}
	changed := !found || previous.Status != StatusFailed ||
		previous.Provider != "github" || previous.RepositoryFullName != repositoryFullName ||
		stringValue(previous.LastErrorCode) != errorCode
	var state State
	if found {
		state, err = scanState(tx.QueryRow(ctx, `
			UPDATE coordination.repository_provider_states
			SET provider = 'github', repository_full_name = $4, status = 'failed',
			    last_attempt_at = transaction_timestamp(), last_error_code = $5,
			    version = version + CASE WHEN $6 THEN 1 ELSE 0 END,
			    updated_at = CASE WHEN $6 THEN transaction_timestamp() ELSE updated_at END
			WHERE organization_id = $1 AND project_id = $2 AND repository_id = $3
			RETURNING repository_id, project_id, provider, repository_full_name, status,
			          default_branch, canonical_revision, visibility, provider_updated_at,
			          last_attempt_at, last_success_at, last_error_code, version
		`, organizationID, projectID, repositoryID, repositoryFullName, errorCode, changed))
	} else {
		state, err = scanState(tx.QueryRow(ctx, `
			INSERT INTO coordination.repository_provider_states (
				repository_id, organization_id, project_id, provider,
				repository_full_name, status, default_branch, canonical_revision,
				visibility, last_attempt_at, last_error_code
			)
			VALUES ($1, $2, $3, 'github', $4, 'failed', $5, $6,
			        'unknown', transaction_timestamp(), $7)
			RETURNING repository_id, project_id, provider, repository_full_name, status,
			          default_branch, canonical_revision, visibility, provider_updated_at,
			          last_attempt_at, last_success_at, last_error_code, version
		`, repositoryID, organizationID, projectID, repositoryFullName,
			defaultBranch, repositoryRevision, errorCode))
	}
	if err != nil {
		return State{}, fmt.Errorf("store repository sync failure: %w", err)
	}
	if changed {
		var commandID string
		if err := tx.QueryRow(ctx, `SELECT uuidv7()`).Scan(&commandID); err != nil {
			return State{}, fmt.Errorf("allocate failed repository sync command: %w", err)
		}
		if _, err := appendSyncEvent(
			ctx, tx, organizationID, projectID, commandID, "", failedEventType,
			repositoryID, state.Version, stringValue(state.CanonicalRevision),
			map[string]any{
				"repository_id": repositoryID, "provider": "github",
				"repository_full_name": repositoryFullName, "error_code": errorCode,
			},
		); err != nil {
			return State{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return State{}, fmt.Errorf("commit repository sync failure: %w", err)
	}
	return state, nil
}

func reserveSyncCommand(
	ctx context.Context, tx pgx.Tx, organizationID, projectID, key string,
	requestHash [sha256.Size]byte,
) (string, Result, bool, error) {
	var commandID string
	err := tx.QueryRow(ctx, `
		INSERT INTO platform.idempotency_records (
			organization_id, project_id, command_type, idempotency_key, request_hash
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT ON CONSTRAINT idempotency_records_scope_key_uq DO NOTHING
		RETURNING command_id
	`, organizationID, projectID, syncCommandType, key, requestHash[:]).Scan(&commandID)
	if err == nil {
		return commandID, Result{}, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", Result{}, false, fmt.Errorf("reserve repository sync idempotency key: %w", err)
	}
	var storedHash []byte
	var outcome *string
	var body json.RawMessage
	err = tx.QueryRow(ctx, `
		SELECT request_hash, outcome, response_body
		FROM platform.idempotency_records
		WHERE organization_id = $1 AND project_id = $2
		  AND command_type = $3 AND idempotency_key = $4
	`, organizationID, projectID, syncCommandType, key).Scan(&storedHash, &outcome, &body)
	if err != nil {
		return "", Result{}, false, fmt.Errorf("load stored repository sync result: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return "", Result{}, false, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(body) == 0 {
		return "", Result{}, false, ErrCommandIncomplete
	}
	var result Result
	if err := json.Unmarshal(body, &result); err != nil {
		return "", Result{}, false, fmt.Errorf("decode stored repository sync result: %w", err)
	}
	return "", result, true, nil
}

func completeSyncCommand(
	ctx context.Context, tx pgx.Tx,
	organizationID, projectID, key, commandID string, result Result,
) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode repository sync result: %w", err)
	}
	eventID := ""
	if result.EventID != nil {
		eventID = *result.EventID
	}
	tag, err := tx.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET outcome = 'succeeded', response_status = 200, response_body = $6,
		    event_id = NULLIF($7, '')::uuid, aggregate_id = $8,
		    completed_at = transaction_timestamp()
		WHERE organization_id = $1 AND project_id = $2 AND command_type = $3
		  AND idempotency_key = $4 AND command_id = $5
	`, organizationID, projectID, syncCommandType, key, commandID,
		body, eventID, result.State.RepositoryID)
	if err != nil {
		return fmt.Errorf("store repository sync result: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("store repository sync result: idempotency reservation disappeared")
	}
	return nil
}

func appendSyncEvent(
	ctx context.Context, tx pgx.Tx,
	organizationID, projectID, commandID, actorID, eventType, repositoryID string,
	aggregateVersion int64, gitRevision string, payload any,
) (string, error) {
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
	`, organizationID, projectID).Scan(&sequence)
	if err != nil {
		return "", fmt.Errorf("allocate repository sync event sequence: %w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode repository sync event: %w", err)
	}
	var eventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.events (
			organization_id, project_id, project_sequence, event_type, event_version,
			aggregate_type, aggregate_id, aggregate_version, actor_id,
			command_id, correlation_id, git_revision, payload, payload_hash
		)
		VALUES (
			$1, $2, $3, $4, 1, 'repository_provider_state', $5, $6,
			NULLIF($7, '')::uuid, $8, $8, NULLIF($9, ''), $10,
			sha256(convert_to(($10::jsonb)::text, 'UTF8'))
		)
		RETURNING id
	`, organizationID, projectID, sequence, eventType, repositoryID,
		aggregateVersion, actorID, commandID, gitRevision, body).Scan(&eventID)
	if err != nil {
		return "", fmt.Errorf("append %s event: %w", eventType, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform.outbox (organization_id, project_id, event_id, channel)
		VALUES ($1, $2, $3, 'project-events')
	`, organizationID, projectID, eventID); err != nil {
		return "", fmt.Errorf("enqueue %s event: %w", eventType, err)
	}
	return eventID, nil
}

func lockProject(ctx context.Context, tx pgx.Tx, organizationID, projectID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, organizationID+":"+projectID+":repository-sync"); err != nil {
		return fmt.Errorf("lock project repository sync: %w", err)
	}
	return nil
}

const stateSelectSQL = `
	SELECT repository_id, project_id, provider, repository_full_name, status,
	       default_branch, canonical_revision, visibility, provider_updated_at,
	       last_attempt_at, last_success_at, last_error_code, version
	FROM coordination.repository_provider_states
`

func loadState(
	ctx context.Context, tx pgx.Tx, organizationID, projectID, repositoryID string, lock bool,
) (State, bool, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	state, err := scanState(tx.QueryRow(ctx, stateSelectSQL+`
		WHERE organization_id = $1 AND project_id = $2 AND repository_id = $3
	`+suffix, organizationID, projectID, repositoryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("load repository provider state: %w", err)
	}
	return state, true, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanState(row rowScanner) (State, error) {
	var state State
	err := row.Scan(
		&state.RepositoryID, &state.ProjectID, &state.Provider,
		&state.RepositoryFullName, &state.Status, &state.DefaultBranch,
		&state.CanonicalRevision, &state.Visibility, &state.ProviderUpdatedAt,
		&state.LastAttemptAt, &state.LastSuccessAt, &state.LastErrorCode, &state.Version,
	)
	return state, err
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
