package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (r *PostgresRepository) OfferHandoff(
	ctx context.Context,
	organizationID, principalID string,
	allowAll bool,
	projectID, intentID, key string,
	requestHash [sha256.Size]byte,
	input OfferHandoffInput,
) (HandoffResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("begin handoff.offer: %w", err)
	}
	defer rollback(tx)
	if err := lockProjectCoordination(ctx, tx, organizationID, projectID); err != nil {
		return HandoffResult{}, err
	}
	var responsibleActorID, workspaceID, baseRevision string
	err = tx.QueryRow(ctx, `
		SELECT intent.responsible_agent_id, relation.workspace_id, intent.base_revision
		FROM coordination.intents AS intent
		JOIN identity.workspace_projects AS relation
		  ON relation.organization_id = intent.organization_id
		 AND relation.project_id = intent.project_id
		WHERE intent.organization_id = $1 AND intent.project_id = $2 AND intent.id = $3
		  AND intent.status NOT IN ('cancelled', 'abandoned')
		FOR UPDATE OF intent
	`, organizationID, projectID, intentID).Scan(&responsibleActorID, &workspaceID, &baseRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoffResult{}, ErrNotFound
	}
	if err != nil {
		return HandoffResult{}, fmt.Errorf("load intent for handoff: %w", err)
	}
	actorID, err := authorizeSession(ctx, tx, organizationID, principalID, allowAll, projectID, input.SessionID)
	if err != nil {
		return HandoffResult{}, err
	}
	if actorID != responsibleActorID && !allowAll {
		return HandoffResult{}, ErrForbidden
	}
	commandID, stored, replayed, err := reserveCommand(
		ctx, tx, organizationID, projectID, "handoff.offer", key, requestHash,
	)
	if err != nil {
		return HandoffResult{}, err
	}
	if replayed {
		var result HandoffResult
		if err := json.Unmarshal(stored, &result); err != nil {
			return HandoffResult{}, fmt.Errorf("decode stored handoff.offer result: %w", err)
		}
		result.Replayed = true
		return result, nil
	}
	var expiredHandoffID string
	var expiredHandoffVersion int64
	err = tx.QueryRow(ctx, `
		UPDATE coordination.handoffs
		SET status = 'expired', responded_at = transaction_timestamp(),
		    updated_at = transaction_timestamp(), version = version + 1
		WHERE organization_id = $1 AND project_id = $2 AND intent_id = $3
		  AND status = 'offered' AND expires_at <= transaction_timestamp()
		RETURNING id, version
	`, organizationID, projectID, intentID).Scan(&expiredHandoffID, &expiredHandoffVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return HandoffResult{}, fmt.Errorf("expire previous handoff: %w", err)
	}
	if expiredHandoffID != "" {
		expired, loadErr := loadHandoff(ctx, tx, organizationID, projectID, intentID, expiredHandoffID, false)
		if loadErr != nil {
			return HandoffResult{}, loadErr
		}
		if _, appendErr := appendEvent(
			ctx, tx, organizationID, projectID, commandID, actorID, input.SessionID,
			intentID, "pact.handoff.expired.v1", "handoff", expiredHandoffID,
			expiredHandoffVersion, baseRevision, map[string]any{"handoff": expired},
		); appendErr != nil {
			return HandoffResult{}, appendErr
		}
	}
	completed, err := json.Marshal(input.Completed)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("encode handoff completed work: %w", err)
	}
	remaining, err := json.Marshal(input.RemainingWork)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("encode handoff remaining work: %w", err)
	}
	blockers, err := json.Marshal(input.Blockers)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("encode handoff blockers: %w", err)
	}
	nextSteps, err := json.Marshal(input.NextSteps)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("encode handoff next steps: %w", err)
	}
	validations, err := json.Marshal(input.Validations)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("encode handoff validations: %w", err)
	}
	var handoffID string
	err = tx.QueryRow(ctx, `
		INSERT INTO coordination.handoffs (
			organization_id, workspace_id, project_id, intent_id,
			from_session_id, from_actor_id, summary, completed,
			remaining_work, blockers, next_steps, validations, expires_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			transaction_timestamp() + ($13 * interval '1 hour')
		)
		RETURNING id
	`, organizationID, workspaceID, projectID, intentID, input.SessionID, actorID,
		input.Summary, completed, remaining, blockers, nextSteps, validations,
		input.ExpiresInHours).Scan(&handoffID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "handoffs_one_offered_intent_idx" {
			return HandoffResult{}, ErrHandoffExists
		}
		return HandoffResult{}, fmt.Errorf("create handoff: %w", err)
	}
	for _, recordID := range input.LinkedRecordIDs {
		tag, insertErr := tx.Exec(ctx, `
			INSERT INTO coordination.handoff_records (
				organization_id, workspace_id, project_id, handoff_id, record_id
			)
			SELECT $1, $2, $3, $4, record.id
			FROM knowledge.records AS record
			WHERE record.organization_id = $1 AND record.workspace_id = $2 AND record.id = $5
		`, organizationID, workspaceID, projectID, handoffID, recordID)
		if insertErr != nil {
			return HandoffResult{}, fmt.Errorf("link handoff knowledge record: %w", insertErr)
		}
		if tag.RowsAffected() != 1 {
			return HandoffResult{}, ErrKnowledgeRecordNotFound
		}
	}
	handoff, err := loadHandoff(ctx, tx, organizationID, projectID, intentID, handoffID, false)
	if err != nil {
		return HandoffResult{}, err
	}
	eventID, err := appendEvent(
		ctx, tx, organizationID, projectID, commandID, actorID, input.SessionID,
		intentID, "pact.handoff.offered.v1", "handoff", handoff.ID,
		handoff.Version, baseRevision, map[string]any{"handoff": handoff},
	)
	if err != nil {
		return HandoffResult{}, err
	}
	result := HandoffResult{Handoff: handoff, EventID: eventID}
	if err := completeCommand(ctx, tx, organizationID, projectID, "handoff.offer", key,
		commandID, 201, eventID, handoff.ID, result); err != nil {
		return HandoffResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return HandoffResult{}, fmt.Errorf("commit handoff.offer: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListHandoffs(
	ctx context.Context, organizationID, projectID, intentID string,
) ([]Handoff, error) {
	rows, err := r.pool.Query(ctx, handoffSelectSQL+`
		WHERE handoff.organization_id = $1 AND handoff.project_id = $2
		  AND ($3 = '' OR handoff.intent_id::text = $3)
		ORDER BY CASE handoff.status WHEN 'offered' THEN 0 ELSE 1 END,
		         handoff.updated_at DESC, handoff.id
	`, organizationID, projectID, intentID)
	if err != nil {
		return nil, fmt.Errorf("list handoffs: %w", err)
	}
	defer rows.Close()
	result := make([]Handoff, 0)
	for rows.Next() {
		handoff, scanErr := scanHandoff(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list handoffs: %w", scanErr)
		}
		handoff.LinkedRecordIDs, scanErr = loadHandoffRecordIDs(ctx, r.pool, organizationID, projectID, handoff.ID)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, handoff)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list handoffs: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) UpdateHandoffStatus(
	ctx context.Context,
	organizationID, principalID string,
	allowAll bool,
	projectID, intentID, handoffID, key string,
	requestHash [sha256.Size]byte,
	input HandoffStatusInput,
) (HandoffResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("begin handoff.status: %w", err)
	}
	defer rollback(tx)
	if err := lockProjectCoordination(ctx, tx, organizationID, projectID); err != nil {
		return HandoffResult{}, err
	}
	var currentStatus, fromActorID, baseRevision string
	var currentVersion int64
	var unexpired bool
	err = tx.QueryRow(ctx, `
		SELECT handoff.status, handoff.from_actor_id, handoff.version,
		       handoff.expires_at > transaction_timestamp(), intent.base_revision
		FROM coordination.handoffs AS handoff
		JOIN coordination.intents AS intent
		  ON intent.organization_id = handoff.organization_id
		 AND intent.project_id = handoff.project_id
		 AND intent.id = handoff.intent_id
		WHERE handoff.organization_id = $1 AND handoff.project_id = $2
		  AND handoff.intent_id = $3 AND handoff.id = $4
		FOR UPDATE OF handoff
	`, organizationID, projectID, intentID, handoffID).Scan(
		&currentStatus, &fromActorID, &currentVersion, &unexpired, &baseRevision,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoffResult{}, ErrNotFound
	}
	if err != nil {
		return HandoffResult{}, fmt.Errorf("load handoff for status update: %w", err)
	}
	actorID, err := authorizeSession(ctx, tx, organizationID, principalID, allowAll, projectID, input.SessionID)
	if err != nil {
		return HandoffResult{}, err
	}
	commandID, stored, replayed, err := reserveCommand(
		ctx, tx, organizationID, projectID, "handoff.status", key, requestHash,
	)
	if err != nil {
		return HandoffResult{}, err
	}
	if replayed {
		var result HandoffResult
		if err := json.Unmarshal(stored, &result); err != nil {
			return HandoffResult{}, fmt.Errorf("decode stored handoff.status result: %w", err)
		}
		result.Replayed = true
		return result, nil
	}
	if currentVersion != input.ExpectedVersion {
		return HandoffResult{}, ErrVersionConflict
	}
	if currentStatus != "offered" || !unexpired {
		return HandoffResult{}, ErrInvalidHandoffStatus
	}
	if input.Status == "withdrawn" && actorID != fromActorID && !allowAll {
		return HandoffResult{}, ErrForbidden
	}
	if input.Status == "accepted" && actorID == fromActorID {
		return HandoffResult{}, &ValidationError{Field: "session_id", Message: "the offering actor cannot accept its own handoff"}
	}
	var toSession, toActor any
	if input.Status == "accepted" {
		toSession, toActor = input.SessionID, actorID
	}
	tag, err := tx.Exec(ctx, `
		UPDATE coordination.handoffs
		SET status = $5, to_session_id = $6, to_actor_id = $7,
		    responded_at = transaction_timestamp(), updated_at = transaction_timestamp(),
		    version = version + 1
		WHERE organization_id = $1 AND project_id = $2 AND intent_id = $3
		  AND id = $4 AND version = $8 AND status = 'offered'
	`, organizationID, projectID, intentID, handoffID, input.Status,
		toSession, toActor, input.ExpectedVersion)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("update handoff status: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return HandoffResult{}, ErrVersionConflict
	}
	handoff, err := loadHandoff(ctx, tx, organizationID, projectID, intentID, handoffID, false)
	if err != nil {
		return HandoffResult{}, err
	}
	eventID, err := appendEvent(
		ctx, tx, organizationID, projectID, commandID, actorID, input.SessionID,
		intentID, "pact.handoff."+input.Status+".v1", "handoff", handoff.ID,
		handoff.Version, baseRevision, map[string]any{"handoff": handoff},
	)
	if err != nil {
		return HandoffResult{}, err
	}
	result := HandoffResult{Handoff: handoff, EventID: eventID}
	if err := completeCommand(ctx, tx, organizationID, projectID, "handoff.status", key,
		commandID, 200, eventID, handoff.ID, result); err != nil {
		return HandoffResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return HandoffResult{}, fmt.Errorf("commit handoff.status: %w", err)
	}
	return result, nil
}

const handoffSelectSQL = `
	SELECT handoff.id, handoff.workspace_id, handoff.project_id, handoff.intent_id,
	       handoff.from_session_id, handoff.from_actor_id, source.display_name,
	       handoff.to_session_id, handoff.to_actor_id, recipient.display_name,
	       CASE
	         WHEN handoff.status = 'offered' AND handoff.expires_at <= transaction_timestamp()
	           THEN 'expired'
	         ELSE handoff.status
	       END,
	       handoff.summary, handoff.completed, handoff.remaining_work,
	       handoff.blockers, handoff.next_steps, handoff.validations, handoff.version,
	       handoff.offered_at,
	       CASE
	         WHEN handoff.status = 'offered' AND handoff.expires_at <= transaction_timestamp()
	           THEN handoff.expires_at
	         ELSE handoff.responded_at
	       END,
	       handoff.expires_at, handoff.created_at, handoff.updated_at
	FROM coordination.handoffs AS handoff
	JOIN identity.actors AS source
	  ON source.organization_id = handoff.organization_id AND source.id = handoff.from_actor_id
	LEFT JOIN identity.actors AS recipient
	  ON recipient.organization_id = handoff.organization_id AND recipient.id = handoff.to_actor_id
`

func loadHandoff(
	ctx context.Context, db handoffDB,
	organizationID, projectID, intentID, handoffID string,
	forUpdate bool,
) (Handoff, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE OF handoff"
	}
	handoff, err := scanHandoff(db.QueryRow(ctx, handoffSelectSQL+`
		WHERE handoff.organization_id = $1 AND handoff.project_id = $2
		  AND handoff.intent_id = $3 AND handoff.id = $4
	`+lock, organizationID, projectID, intentID, handoffID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Handoff{}, ErrNotFound
	}
	if err != nil {
		return Handoff{}, fmt.Errorf("get handoff: %w", err)
	}
	handoff.LinkedRecordIDs, err = loadHandoffRecordIDs(ctx, db, organizationID, projectID, handoff.ID)
	return handoff, err
}

type handoffDB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func scanHandoff(row rowScanner) (Handoff, error) {
	var handoff Handoff
	var completed, remaining, blockers, nextSteps, validations []byte
	err := row.Scan(
		&handoff.ID, &handoff.WorkspaceID, &handoff.ProjectID, &handoff.IntentID,
		&handoff.FromSessionID, &handoff.FromActorID, &handoff.FromActorName,
		&handoff.ToSessionID, &handoff.ToActorID, &handoff.ToActorName,
		&handoff.Status, &handoff.Summary, &completed, &remaining, &blockers,
		&nextSteps, &validations, &handoff.Version, &handoff.OfferedAt,
		&handoff.RespondedAt, &handoff.ExpiresAt, &handoff.CreatedAt, &handoff.UpdatedAt,
	)
	if err != nil {
		return Handoff{}, err
	}
	if err := json.Unmarshal(completed, &handoff.Completed); err != nil {
		return Handoff{}, fmt.Errorf("decode handoff completed work: %w", err)
	}
	if err := json.Unmarshal(remaining, &handoff.RemainingWork); err != nil {
		return Handoff{}, fmt.Errorf("decode handoff remaining work: %w", err)
	}
	if err := json.Unmarshal(blockers, &handoff.Blockers); err != nil {
		return Handoff{}, fmt.Errorf("decode handoff blockers: %w", err)
	}
	if err := json.Unmarshal(nextSteps, &handoff.NextSteps); err != nil {
		return Handoff{}, fmt.Errorf("decode handoff next steps: %w", err)
	}
	if err := json.Unmarshal(validations, &handoff.Validations); err != nil {
		return Handoff{}, fmt.Errorf("decode handoff validations: %w", err)
	}
	return handoff, nil
}

func loadHandoffRecordIDs(
	ctx context.Context, db handoffDB, organizationID, projectID, handoffID string,
) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT record_id
		FROM coordination.handoff_records
		WHERE organization_id = $1 AND project_id = $2 AND handoff_id = $3
		ORDER BY created_at, record_id
	`, organizationID, projectID, handoffID)
	if err != nil {
		return nil, fmt.Errorf("list handoff records: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var recordID string
		if err := rows.Scan(&recordID); err != nil {
			return nil, fmt.Errorf("list handoff records: %w", err)
		}
		result = append(result, recordID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list handoff records: %w", err)
	}
	return result, nil
}
