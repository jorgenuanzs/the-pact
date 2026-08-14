package contextpack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

func (r *PostgresRepository) Create(
	ctx context.Context,
	organizationID, principalID string,
	allowAll bool,
	key string,
	requestHash [sha256.Size]byte,
	input CompileInput,
	draft Draft,
) (CompileResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CompileResult{}, fmt.Errorf("begin context.compile: %w", err)
	}
	defer rollback(tx)
	actorID, err := contextPackActor(ctx, tx, organizationID, principalID, allowAll, draft.ProjectID, input.SessionID)
	if err != nil {
		return CompileResult{}, err
	}
	var relationshipExists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM coordination.intents AS intent
			JOIN identity.workspace_projects AS relation
			  ON relation.organization_id = intent.organization_id
			 AND relation.project_id = intent.project_id
			WHERE intent.organization_id = $1 AND intent.project_id = $2
			  AND intent.id = $3 AND relation.workspace_id = $4
		)
	`, organizationID, draft.ProjectID, draft.IntentID, draft.WorkspaceID).Scan(&relationshipExists)
	if err != nil {
		return CompileResult{}, fmt.Errorf("verify context pack scope: %w", err)
	}
	if !relationshipExists {
		return CompileResult{}, ErrNotFound
	}
	commandID, stored, replayed, err := reserveContextCommand(
		ctx, tx, organizationID, draft.ProjectID, key, requestHash,
	)
	if err != nil {
		return CompileResult{}, err
	}
	if replayed {
		var result CompileResult
		if err := json.Unmarshal(stored, &result); err != nil {
			return CompileResult{}, fmt.Errorf("decode stored context.compile result: %w", err)
		}
		result.Replayed = true
		return result, nil
	}
	fingerprintDraft := draft
	// GeneratedAt describes this retrieval, not a durable source. Excluding it
	// keeps the fingerprint stable when the underlying context did not change.
	fingerprintDraft.Knowledge.GeneratedAt = time.Time{}
	draftBody, err := json.Marshal(fingerprintDraft)
	if err != nil {
		return CompileResult{}, fmt.Errorf("encode context pack sources: %w", err)
	}
	fingerprint := sha256.Sum256(draftBody)
	var packID string
	var eventCursor int64
	var generatedAt, expiresAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT uuidv7(), project.event_sequence, transaction_timestamp(),
		       transaction_timestamp() + ($3 * interval '1 minute')
		FROM identity.projects AS project
		WHERE project.organization_id = $1 AND project.id = $2
		FOR SHARE
	`, organizationID, draft.ProjectID, input.TTLMinutes).Scan(
		&packID, &eventCursor, &generatedAt, &expiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CompileResult{}, ErrNotFound
	}
	if err != nil {
		return CompileResult{}, fmt.Errorf("allocate context pack: %w", err)
	}
	var requestingSessionID *string
	if input.SessionID != "" {
		value := input.SessionID
		requestingSessionID = &value
	}
	gitRevision := draft.Intent.BaseRevision
	pack := ContextPack{
		ID: packID, Type: draft.Type, WorkspaceID: draft.WorkspaceID,
		ProjectID: draft.ProjectID, IntentID: draft.IntentID,
		RequestingSessionID: requestingSessionID, RequestedByActorID: actorID,
		Project: draft.Project, Workspace: draft.Workspace, Intent: draft.Intent,
		ActiveWork: draft.ActiveWork, Knowledge: draft.Knowledge,
		Handoffs: draft.Handoffs, Warnings: draft.Warnings,
		Snapshot: Snapshot{
			EventCursor: strconv.FormatInt(eventCursor, 10), GitRevision: &gitRevision,
			Consistency: "eventual", SourceFingerprint: hex.EncodeToString(fingerprint[:]),
			GeneratedAt: generatedAt, ExpiresAt: expiresAt,
		},
	}
	payload, err := json.Marshal(pack)
	if err != nil {
		return CompileResult{}, fmt.Errorf("encode context pack: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO knowledge.context_packs (
			id, organization_id, workspace_id, project_id, intent_id,
			requesting_session_id, requested_by_actor_id, pack_type, consistency,
			event_cursor, git_revision, payload, payload_hash, source_fingerprint,
			created_at, expires_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, 'eventual', $9, $10, $11,
			sha256(convert_to(($11::jsonb)::text, 'UTF8')), $12, $13, $14
		)
	`, pack.ID, organizationID, pack.WorkspaceID, pack.ProjectID, pack.IntentID,
		pack.RequestingSessionID, actorID, pack.Type, eventCursor, pack.Snapshot.GitRevision,
		payload, fingerprint[:], pack.Snapshot.GeneratedAt, pack.Snapshot.ExpiresAt)
	if err != nil {
		return CompileResult{}, fmt.Errorf("store context pack: %w", err)
	}
	eventID, err := appendContextEvent(
		ctx, tx, organizationID, pack.ProjectID, commandID, actorID,
		requestingSessionID, pack.IntentID, pack.ID, pack.Snapshot.GitRevision,
		map[string]any{
			"context_pack_id": pack.ID, "type": pack.Type,
			"snapshot_event_cursor": pack.Snapshot.EventCursor,
			"source_fingerprint":    pack.Snapshot.SourceFingerprint,
			"expires_at":            pack.Snapshot.ExpiresAt,
		},
	)
	if err != nil {
		return CompileResult{}, err
	}
	result := CompileResult{Pack: pack, EventID: eventID}
	if err := completeContextCommand(ctx, tx, organizationID, pack.ProjectID, key,
		commandID, eventID, pack.ID, result); err != nil {
		return CompileResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CompileResult{}, fmt.Errorf("commit context.compile: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) Get(
	ctx context.Context, organizationID, projectID, packID string,
) (ContextPack, error) {
	var payload []byte
	var valid bool
	err := r.pool.QueryRow(ctx, `
		SELECT payload,
		       payload_hash = sha256(convert_to(payload::text, 'UTF8'))
		FROM knowledge.context_packs
		WHERE organization_id = $1 AND project_id = $2 AND id = $3
	`, organizationID, projectID, packID).Scan(&payload, &valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContextPack{}, ErrNotFound
	}
	if err != nil {
		return ContextPack{}, fmt.Errorf("get context pack: %w", err)
	}
	if !valid {
		return ContextPack{}, ErrIntegrity
	}
	var pack ContextPack
	if err := json.Unmarshal(payload, &pack); err != nil {
		return ContextPack{}, fmt.Errorf("decode context pack: %w", err)
	}
	return pack, nil
}

func contextPackActor(
	ctx context.Context, tx pgx.Tx,
	organizationID, principalID string,
	allowAll bool,
	projectID, sessionID string,
) (string, error) {
	if sessionID == "" {
		var actorID string
		err := tx.QueryRow(ctx, `
			SELECT id
			FROM identity.actors
			WHERE organization_id = $1 AND id = $2 AND status = 'active'
		`, organizationID, principalID).Scan(&actorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrForbidden
		}
		if err != nil {
			return "", fmt.Errorf("authorize context pack actor: %w", err)
		}
		return actorID, nil
	}
	var actorID string
	err := tx.QueryRow(ctx, `
		SELECT session.actor_id
		FROM identity.sessions AS session
		JOIN identity.agents AS agent
		  ON agent.organization_id = session.organization_id AND agent.id = session.actor_id
		WHERE session.organization_id = $1 AND session.project_id = $2 AND session.id = $3
		  AND session.status = 'active' AND session.expires_at > transaction_timestamp()
		  AND ($5 OR agent.sponsor_principal_id = $4)
		FOR UPDATE OF session
	`, organizationID, projectID, sessionID, principalID, allowAll).Scan(&actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("authorize context pack session: %w", err)
	}
	return actorID, nil
}

func reserveContextCommand(
	ctx context.Context, tx pgx.Tx,
	organizationID, projectID, key string,
	requestHash [sha256.Size]byte,
) (string, json.RawMessage, bool, error) {
	var commandID string
	err := tx.QueryRow(ctx, `
		INSERT INTO platform.idempotency_records (
			organization_id, project_id, command_type, idempotency_key, request_hash
		)
		VALUES ($1, $2, 'context.compile', $3, $4)
		ON CONFLICT ON CONSTRAINT idempotency_records_scope_key_uq DO NOTHING
		RETURNING command_id
	`, organizationID, projectID, key, requestHash[:]).Scan(&commandID)
	if err == nil {
		return commandID, nil, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, false, fmt.Errorf("reserve context.compile idempotency key: %w", err)
	}
	var storedHash []byte
	var outcome *string
	var body json.RawMessage
	err = tx.QueryRow(ctx, `
		SELECT request_hash, outcome, response_body
		FROM platform.idempotency_records
		WHERE organization_id = $1 AND project_id = $2
		  AND command_type = 'context.compile' AND idempotency_key = $3
	`, organizationID, projectID, key).Scan(&storedHash, &outcome, &body)
	if err != nil {
		return "", nil, false, fmt.Errorf("load stored context.compile result: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return "", nil, false, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(body) == 0 {
		return "", nil, false, ErrCommandIncomplete
	}
	return "", body, true, nil
}

func appendContextEvent(
	ctx context.Context, tx pgx.Tx,
	organizationID, projectID, commandID, actorID string,
	sessionID *string,
	intentID, packID string,
	gitRevision *string,
	payload any,
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
		return "", fmt.Errorf("allocate context event sequence: %w", err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode context event: %w", err)
	}
	var eventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO platform.events (
			organization_id, project_id, project_sequence, event_type, event_version,
			aggregate_type, aggregate_id, aggregate_version, actor_id, session_id,
			intent_id, command_id, correlation_id, git_revision, payload, payload_hash
		)
		VALUES (
			$1, $2, $3, 'pact.context.compiled.v1', 1, 'context_pack', $4, 1,
			$5, $6, $7, $8, $8, $9, $10,
			sha256(convert_to(($10::jsonb)::text, 'UTF8'))
		)
		RETURNING id
	`, organizationID, projectID, sequence, packID, actorID, sessionID,
		intentID, commandID, gitRevision, body).Scan(&eventID)
	if err != nil {
		return "", fmt.Errorf("append context.compiled event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform.outbox (organization_id, project_id, event_id, channel)
		VALUES ($1, $2, $3, 'project-events')
	`, organizationID, projectID, eventID); err != nil {
		return "", fmt.Errorf("enqueue context.compiled event: %w", err)
	}
	return eventID, nil
}

func completeContextCommand(
	ctx context.Context, tx pgx.Tx,
	organizationID, projectID, key, commandID, eventID, packID string,
	result CompileResult,
) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode context.compile response: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET outcome = 'succeeded', response_status = 201, response_body = $5,
		    event_id = $6, aggregate_id = $7, completed_at = transaction_timestamp()
		WHERE organization_id = $1 AND project_id = $2
		  AND command_type = 'context.compile' AND idempotency_key = $3 AND command_id = $4
	`, organizationID, projectID, key, commandID, body, eventID, packID)
	if err != nil {
		return fmt.Errorf("store context.compile result: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("store context.compile result: idempotency reservation disappeared")
	}
	return nil
}

func rollback(tx pgx.Tx) {
	_ = tx.Rollback(context.Background())
}
