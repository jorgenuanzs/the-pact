package agentsession

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	workspaceDiffEventType = "pact.workspace.diff_updated.v1"
	externalGitEventType   = "pact.git.external_change_detected.v1"
)

func (r *PostgresRepository) Observe(
	ctx context.Context,
	organizationID string,
	sponsorPrincipalID string,
	sessionID string,
	idempotencyKey string,
	requestHash [sha256.Size]byte,
	input ObservationInput,
) (ObservationResult, error) {
	fingerprint, err := hex.DecodeString(input.DiffFingerprint)
	if err != nil {
		return ObservationResult{}, fmt.Errorf("decode observation fingerprint: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ObservationResult{}, fmt.Errorf("begin repository observation: %w", err)
	}
	defer rollback(tx)

	var (
		projectID string
		actorID   string
		nodeID    string
	)
	err = tx.QueryRow(ctx, `
		SELECT session.project_id, session.actor_id, session.node_id
		FROM identity.sessions AS session
		JOIN identity.agents AS agent
		  ON agent.organization_id = session.organization_id
		 AND agent.id = session.actor_id
		WHERE session.organization_id = $1
		  AND session.id = $2
		  AND agent.sponsor_principal_id = $3
		  AND session.status = 'active'
		  AND session.expires_at > transaction_timestamp()
		  AND session.announced_capabilities
		      @> '{"workspace.diff.observe.v1": true}'::jsonb
		FOR UPDATE OF session
	`, organizationID, sessionID, sponsorPrincipalID).Scan(&projectID, &actorID, &nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ObservationResult{}, ErrNotFound
	}
	if err != nil {
		return ObservationResult{}, fmt.Errorf("authorize repository observation: %w", err)
	}

	var commandID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.idempotency_records (
			organization_id, project_id, command_type, idempotency_key, request_hash
		)
		VALUES ($1, $2, 'repository.observe', $3, $4)
		ON CONFLICT ON CONSTRAINT idempotency_records_scope_key_uq DO NOTHING
		RETURNING command_id
	`, organizationID, projectID, idempotencyKey, requestHash[:]).Scan(&commandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return loadStoredObservation(ctx, tx, organizationID, projectID, idempotencyKey, requestHash)
	}
	if err != nil {
		return ObservationResult{}, fmt.Errorf("reserve repository observation idempotency key: %w", err)
	}

	var (
		observation         RepositoryObservation
		previousDirty       bool
		previousFingerprint []byte
		previousRevision    *string
		existed             bool
	)
	err = tx.QueryRow(ctx, `
		SELECT id, worktree_dirty, diff_fingerprint, git_revision
		FROM coordination.repository_observations
		WHERE organization_id = $1
		  AND project_id = $2
		  AND session_id = $3
		FOR UPDATE
	`, organizationID, projectID, sessionID).Scan(
		&observation.ID, &previousDirty, &previousFingerprint, &previousRevision,
	)
	if err == nil {
		existed = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ObservationResult{}, fmt.Errorf("load previous repository observation: %w", err)
	}

	if existed {
		err = tx.QueryRow(ctx, `
			UPDATE coordination.repository_observations
			SET worktree_dirty = $4,
			    diff_fingerprint = $5,
			    changed_paths = $6,
			    git_revision = NULLIF($7, ''),
			    git_branch = NULLIF($8, ''),
			    version = version + 1,
			    observed_at = transaction_timestamp()
			WHERE organization_id = $1
			  AND project_id = $2
			  AND session_id = $3
			RETURNING id, project_id, session_id, actor_id, node_id,
			          worktree_dirty, encode(diff_fingerprint, 'hex'), changed_paths,
			          COALESCE(git_revision, ''), COALESCE(git_branch, ''), version, observed_at
		`, organizationID, projectID, sessionID, input.Dirty, fingerprint, input.ChangedPaths,
			input.HeadRevision, input.Branch).Scan(
			&observation.ID, &observation.ProjectID, &observation.SessionID,
			&observation.ActorID, &observation.NodeID, &observation.Dirty,
			&observation.DiffFingerprint, &observation.ChangedPaths,
			&observation.HeadRevision, &observation.Branch,
			&observation.Version, &observation.ObservedAt,
		)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO coordination.repository_observations (
				organization_id, project_id, session_id, actor_id, node_id,
				worktree_dirty, diff_fingerprint, changed_paths, git_revision, git_branch
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''))
			RETURNING id, project_id, session_id, actor_id, node_id,
			          worktree_dirty, encode(diff_fingerprint, 'hex'), changed_paths,
			          COALESCE(git_revision, ''), COALESCE(git_branch, ''), version, observed_at
		`, organizationID, projectID, sessionID, actorID, nodeID, input.Dirty, fingerprint,
			input.ChangedPaths, input.HeadRevision, input.Branch).Scan(
			&observation.ID, &observation.ProjectID, &observation.SessionID,
			&observation.ActorID, &observation.NodeID, &observation.Dirty,
			&observation.DiffFingerprint, &observation.ChangedPaths,
			&observation.HeadRevision, &observation.Branch,
			&observation.Version, &observation.ObservedAt,
		)
	}
	if err != nil {
		return ObservationResult{}, fmt.Errorf("store repository observation: %w", err)
	}

	result := ObservationResult{Observation: observation}
	diffChanged := input.Dirty && (!existed || !previousDirty || !bytes.Equal(previousFingerprint, fingerprint))
	previousHead := ""
	if previousRevision != nil {
		previousHead = *previousRevision
	}
	headChanged := existed && input.HeadRevision != "" && previousHead != input.HeadRevision
	var eventType string
	if diffChanged {
		eventType = workspaceDiffEventType
	} else if headChanged {
		eventType = externalGitEventType
	}
	if eventType != "" {
		eventID, appendErr := appendObservationEvent(
			ctx, tx, organizationID, commandID, eventType, observation,
		)
		if appendErr != nil {
			return ObservationResult{}, appendErr
		}
		result.EventID = &eventID
		result.EventType = &eventType
	}

	responseBody, err := json.Marshal(result)
	if err != nil {
		return ObservationResult{}, fmt.Errorf("encode repository observation response: %w", err)
	}
	var eventID any
	if result.EventID != nil {
		eventID = *result.EventID
	}
	tag, err := tx.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET outcome = 'succeeded',
		    response_status = 200,
		    response_body = $5,
		    event_id = $6,
		    aggregate_id = $7,
		    completed_at = transaction_timestamp()
		WHERE organization_id = $1
		  AND project_id = $2
		  AND command_type = 'repository.observe'
		  AND idempotency_key = $3
		  AND command_id = $4
	`, organizationID, projectID, idempotencyKey, commandID, responseBody, eventID, observation.ID)
	if err != nil {
		return ObservationResult{}, fmt.Errorf("store repository observation result: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ObservationResult{}, errors.New("store repository observation result: idempotency reservation disappeared")
	}
	if err := tx.Commit(ctx); err != nil {
		return ObservationResult{}, fmt.Errorf("commit repository observation: %w", err)
	}
	return result, nil
}

func appendObservationEvent(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	commandID string,
	eventType string,
	observation RepositoryObservation,
) (string, error) {
	var projectSequence int64
	err := tx.QueryRow(ctx, `
		UPDATE platform.project_event_counters
		SET last_sequence = last_sequence + 1,
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND project_id = $2
		RETURNING last_sequence
	`, organizationID, observation.ProjectID).Scan(&projectSequence)
	if err != nil {
		return "", fmt.Errorf("allocate repository observation event sequence: %w", err)
	}
	payload, err := json.Marshal(struct {
		ObservationID   string `json:"observation_id"`
		NodeID          string `json:"node_id"`
		Dirty           bool   `json:"dirty"`
		ChangedPaths    int    `json:"changed_paths"`
		DiffFingerprint string `json:"diff_fingerprint"`
		HeadRevision    string `json:"head_revision,omitempty"`
		Branch          string `json:"branch,omitempty"`
	}{
		ObservationID: observation.ID, NodeID: observation.NodeID,
		Dirty: observation.Dirty, ChangedPaths: observation.ChangedPaths,
		DiffFingerprint: observation.DiffFingerprint,
		HeadRevision:    observation.HeadRevision, Branch: observation.Branch,
	})
	if err != nil {
		return "", fmt.Errorf("encode repository observation event: %w", err)
	}
	var eventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.events (
			organization_id, project_id, project_sequence, event_type,
			event_version, aggregate_type, aggregate_id, aggregate_version,
			actor_id, session_id, command_id, correlation_id, git_revision,
			payload, payload_hash
		)
		VALUES (
			$1, $2, $3, $4, 1, 'repository_observation', $5, $6,
			$7, $8, $9, $9, NULLIF($10, ''), $11,
			sha256(convert_to(($11::jsonb)::text, 'UTF8'))
		)
		RETURNING id
	`, organizationID, observation.ProjectID, projectSequence, eventType,
		observation.ID, observation.Version, observation.ActorID, observation.SessionID,
		commandID, observation.HeadRevision, payload).Scan(&eventID)
	if err != nil {
		return "", fmt.Errorf("append repository observation event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform.outbox (organization_id, project_id, event_id, channel)
		VALUES ($1, $2, $3, 'project-events')
	`, organizationID, observation.ProjectID, eventID); err != nil {
		return "", fmt.Errorf("enqueue repository observation event: %w", err)
	}
	return eventID, nil
}

func loadStoredObservation(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	projectID string,
	idempotencyKey string,
	requestHash [sha256.Size]byte,
) (ObservationResult, error) {
	var (
		storedHash   []byte
		outcome      *string
		responseBody []byte
	)
	err := tx.QueryRow(ctx, `
		SELECT request_hash, outcome, response_body
		FROM platform.idempotency_records
		WHERE organization_id = $1
		  AND project_id = $2
		  AND command_type = 'repository.observe'
		  AND idempotency_key = $3
	`, organizationID, projectID, idempotencyKey).Scan(&storedHash, &outcome, &responseBody)
	if err != nil {
		return ObservationResult{}, fmt.Errorf("load repository observation result: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return ObservationResult{}, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(responseBody) == 0 {
		return ObservationResult{}, ErrCommandIncomplete
	}
	var result ObservationResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return ObservationResult{}, fmt.Errorf("decode stored repository observation result: %w", err)
	}
	result.Replayed = true
	return result, nil
}
