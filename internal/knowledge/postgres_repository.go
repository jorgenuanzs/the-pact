package knowledge

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

func (r *PostgresRepository) CreateResource(
	ctx context.Context,
	organizationID, actorID, workspaceID, key string,
	requestHash [sha256.Size]byte,
	input CreateResourceInput,
) (CreateResourceResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CreateResourceResult{}, fmt.Errorf("begin knowledge.resource.create: %w", err)
	}
	defer rollback(tx)
	commandID, replay, err := reserveCommand(ctx, tx, organizationID, "knowledge.resource.create", key, requestHash)
	if err != nil {
		return CreateResourceResult{}, err
	}
	if replay {
		return loadStoredResource(ctx, tx, organizationID, "knowledge.resource.create", key, requestHash)
	}

	metadata, err := json.Marshal(input.Metadata)
	if err != nil {
		return CreateResourceResult{}, fmt.Errorf("encode resource metadata: %w", err)
	}
	resource, err := scanResource(tx.QueryRow(ctx, `
		INSERT INTO knowledge.resources (
			organization_id, workspace_id, kind, title, locator, description,
			classification, metadata, created_by_actor_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, workspace_id, kind, title, locator, description,
		          classification, status, metadata, created_by_actor_id,
		          version, created_at, updated_at, archived_at
	`, organizationID, workspaceID, input.Kind, input.Title, input.Locator,
		input.Description, input.Classification, metadata, actorID))
	if err != nil {
		return CreateResourceResult{}, mapResourceWriteError(err)
	}
	if err := appendKnowledgeEvent(ctx, tx, organizationID, workspaceID, actorID,
		"pact.knowledge.resource.added.v1", "resource", resource.ID, resource.Version,
		map[string]any{"resource": resource, "command_id": commandID}); err != nil {
		return CreateResourceResult{}, err
	}
	if err := completeCommand(ctx, tx, organizationID, "knowledge.resource.create", key, resource.ID, resource); err != nil {
		return CreateResourceResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateResourceResult{}, fmt.Errorf("commit knowledge.resource.create: %w", err)
	}
	return CreateResourceResult{Resource: resource}, nil
}

func (r *PostgresRepository) ListResources(
	ctx context.Context, organizationID, workspaceID string, options ListOptions,
) ([]Resource, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, kind, title, locator, description,
		       classification, status, metadata, created_by_actor_id,
		       version, created_at, updated_at, archived_at
		FROM knowledge.resources
		WHERE organization_id = $1 AND workspace_id = $2
		  AND ($3 = '' OR kind = $3)
		  AND ($4 = '' OR status = $4)
		  AND ($5 = '' OR search_document @@ websearch_to_tsquery('simple', $5))
		ORDER BY status, updated_at DESC, id
		LIMIT $6
	`, organizationID, workspaceID, options.Kind, options.Status, options.Query, options.Limit)
	if err != nil {
		return nil, fmt.Errorf("list knowledge resources: %w", err)
	}
	defer rows.Close()
	resources := make([]Resource, 0)
	for rows.Next() {
		resource, scanErr := scanResource(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list knowledge resources: %w", scanErr)
		}
		resources = append(resources, resource)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list knowledge resources: %w", err)
	}
	return resources, nil
}

func (r *PostgresRepository) CreateRecord(
	ctx context.Context,
	organizationID, actorID, workspaceID, key string,
	requestHash [sha256.Size]byte,
	input CreateRecordInput,
) (CreateRecordResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CreateRecordResult{}, fmt.Errorf("begin knowledge.record.create: %w", err)
	}
	defer rollback(tx)
	commandID, replay, err := reserveCommand(ctx, tx, organizationID, "knowledge.record.create", key, requestHash)
	if err != nil {
		return CreateRecordResult{}, err
	}
	if replay {
		return loadStoredRecord(ctx, tx, organizationID, "knowledge.record.create", key, requestHash)
	}
	metadata, err := json.Marshal(input.Metadata)
	if err != nil {
		return CreateRecordResult{}, fmt.Errorf("encode record metadata: %w", err)
	}
	record, err := scanRecord(tx.QueryRow(ctx, `
		INSERT INTO knowledge.records (
			organization_id, workspace_id, record_type, title, body, authority,
			valid_from, valid_to, metadata, created_by_actor_id, last_changed_by_actor_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, transaction_timestamp()), $8, $9, $10, $10)
		RETURNING id, workspace_id, record_type, title, body, status, authority,
		          valid_from, valid_to, superseded_by_record_id, metadata,
		          created_by_actor_id, last_changed_by_actor_id, version,
		          created_at, updated_at
	`, organizationID, workspaceID, input.Type, input.Title, input.Body, input.Authority,
		input.ValidFrom, input.ValidTo, metadata, actorID))
	if err != nil {
		return CreateRecordResult{}, mapRecordWriteError(err)
	}
	for _, evidence := range input.Evidence {
		tag, insertErr := tx.Exec(ctx, `
			INSERT INTO knowledge.record_evidence (
				organization_id, workspace_id, record_id, resource_id,
				relation, note, created_by_actor_id
			)
			SELECT $1, $2, $3, resource.id, $5, $6, $7
			FROM knowledge.resources AS resource
			WHERE resource.organization_id = $1
			  AND resource.workspace_id = $2
			  AND resource.id = $4
		`, organizationID, workspaceID, record.ID, evidence.ResourceID,
			evidence.Relation, evidence.Note, actorID)
		if insertErr != nil {
			return CreateRecordResult{}, fmt.Errorf("attach record evidence: %w", insertErr)
		}
		if tag.RowsAffected() != 1 {
			return CreateRecordResult{}, ErrResourceNotFound
		}
	}
	record.Evidence, err = loadEvidence(ctx, tx, organizationID, workspaceID, record.ID)
	if err != nil {
		return CreateRecordResult{}, err
	}
	if err := appendKnowledgeEvent(ctx, tx, organizationID, workspaceID, actorID,
		"pact.knowledge.record.proposed.v1", "record", record.ID, record.Version,
		map[string]any{"record": record, "command_id": commandID}); err != nil {
		return CreateRecordResult{}, err
	}
	if err := completeCommand(ctx, tx, organizationID, "knowledge.record.create", key, record.ID, record); err != nil {
		return CreateRecordResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateRecordResult{}, fmt.Errorf("commit knowledge.record.create: %w", err)
	}
	return CreateRecordResult{Record: record}, nil
}

func (r *PostgresRepository) GetRecord(
	ctx context.Context, organizationID, workspaceID, recordID string,
) (Record, error) {
	record, err := scanRecord(r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, record_type, title, body, status, authority,
		       valid_from, valid_to, superseded_by_record_id, metadata,
		       created_by_actor_id, last_changed_by_actor_id, version,
		       created_at, updated_at
		FROM knowledge.records
		WHERE organization_id = $1 AND workspace_id = $2 AND id = $3
	`, organizationID, workspaceID, recordID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("get knowledge record: %w", err)
	}
	record.Evidence, err = loadEvidence(ctx, r.pool, organizationID, workspaceID, record.ID)
	return record, err
}

func (r *PostgresRepository) ListRecords(
	ctx context.Context, organizationID, workspaceID string, options ListOptions,
) ([]Record, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, record_type, title, body, status, authority,
		       valid_from, valid_to, superseded_by_record_id, metadata,
		       created_by_actor_id, last_changed_by_actor_id, version,
		       created_at, updated_at
		FROM knowledge.records
		WHERE organization_id = $1 AND workspace_id = $2
		  AND ($3 = '' OR record_type = $3)
		  AND ($4 = '' OR status = $4)
		  AND ($5 = '' OR search_document @@ websearch_to_tsquery('simple', $5))
		ORDER BY CASE status WHEN 'accepted' THEN 0 WHEN 'disputed' THEN 1 WHEN 'proposed' THEN 2 ELSE 3 END,
		         updated_at DESC, id
		LIMIT $6
	`, organizationID, workspaceID, options.Kind, options.Status, options.Query, options.Limit)
	if err != nil {
		return nil, fmt.Errorf("list knowledge records: %w", err)
	}
	defer rows.Close()
	records := make([]Record, 0)
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list knowledge records: %w", scanErr)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list knowledge records: %w", err)
	}
	for index := range records {
		records[index].Evidence, err = loadEvidence(ctx, r.pool, organizationID, workspaceID, records[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (r *PostgresRepository) UpdateRecordStatus(
	ctx context.Context,
	organizationID, actorID, workspaceID, recordID, key string,
	requestHash [sha256.Size]byte,
	input RecordStatusInput,
) (RecordStatusResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RecordStatusResult{}, fmt.Errorf("begin knowledge.record.status: %w", err)
	}
	defer rollback(tx)
	commandID, replay, err := reserveCommand(ctx, tx, organizationID, "knowledge.record.status", key, requestHash)
	if err != nil {
		return RecordStatusResult{}, err
	}
	if replay {
		return loadStoredStatus(ctx, tx, organizationID, key, requestHash)
	}
	var currentStatus string
	var currentVersion int64
	err = tx.QueryRow(ctx, `
		SELECT status, version
		FROM knowledge.records
		WHERE organization_id = $1 AND workspace_id = $2 AND id = $3
		FOR UPDATE
	`, organizationID, workspaceID, recordID).Scan(&currentStatus, &currentVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecordStatusResult{}, ErrNotFound
	}
	if err != nil {
		return RecordStatusResult{}, fmt.Errorf("lock knowledge record: %w", err)
	}
	if currentVersion != input.ExpectedVersion {
		return RecordStatusResult{}, ErrVersionConflict
	}
	if !transitions[currentStatus][input.Status] {
		return RecordStatusResult{}, ErrInvalidTransition
	}
	var superseding any
	if input.SupersedingRecordID != "" {
		var targetStatus string
		err = tx.QueryRow(ctx, `
			SELECT status
			FROM knowledge.records
			WHERE organization_id = $1 AND workspace_id = $2 AND id = $3
		`, organizationID, workspaceID, input.SupersedingRecordID).Scan(&targetStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return RecordStatusResult{}, ErrNotFound
		}
		if err != nil {
			return RecordStatusResult{}, fmt.Errorf("get superseding knowledge record: %w", err)
		}
		if targetStatus != RecordStatusAccepted {
			return RecordStatusResult{}, &ValidationError{Field: "superseding_record_id", Message: "must identify an accepted record"}
		}
		superseding = input.SupersedingRecordID
	}
	record, err := scanRecord(tx.QueryRow(ctx, `
		UPDATE knowledge.records
		SET status = $4,
		    superseded_by_record_id = $5,
		    valid_to = CASE WHEN $4 IN ('superseded', 'revoked', 'expired') THEN transaction_timestamp() ELSE valid_to END,
		    last_changed_by_actor_id = $6,
		    version = version + 1,
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND workspace_id = $2 AND id = $3 AND version = $7
		RETURNING id, workspace_id, record_type, title, body, status, authority,
		          valid_from, valid_to, superseded_by_record_id, metadata,
		          created_by_actor_id, last_changed_by_actor_id, version,
		          created_at, updated_at
	`, organizationID, workspaceID, recordID, input.Status, superseding, actorID, input.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return RecordStatusResult{}, ErrVersionConflict
	}
	if err != nil {
		return RecordStatusResult{}, fmt.Errorf("update knowledge record status: %w", err)
	}
	record.Evidence, err = loadEvidence(ctx, tx, organizationID, workspaceID, record.ID)
	if err != nil {
		return RecordStatusResult{}, err
	}
	if err := appendKnowledgeEvent(ctx, tx, organizationID, workspaceID, actorID,
		"pact.knowledge.record."+input.Status+".v1", "record", record.ID, record.Version,
		map[string]any{"record": record, "previous_status": currentStatus, "reason": input.Reason, "command_id": commandID}); err != nil {
		return RecordStatusResult{}, err
	}
	if err := completeCommand(ctx, tx, organizationID, "knowledge.record.status", key, record.ID, record); err != nil {
		return RecordStatusResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecordStatusResult{}, fmt.Errorf("commit knowledge.record.status: %w", err)
	}
	return RecordStatusResult{Record: record}, nil
}

type rowScanner interface {
	Scan(...any) error
}

type database interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func scanResource(row rowScanner) (Resource, error) {
	var resource Resource
	var metadata []byte
	err := row.Scan(
		&resource.ID, &resource.WorkspaceID, &resource.Kind, &resource.Title,
		&resource.Locator, &resource.Description, &resource.Classification,
		&resource.Status, &metadata, &resource.CreatedByActorID, &resource.Version,
		&resource.CreatedAt, &resource.UpdatedAt, &resource.ArchivedAt,
	)
	if err != nil {
		return Resource{}, err
	}
	if err := json.Unmarshal(metadata, &resource.Metadata); err != nil {
		return Resource{}, fmt.Errorf("decode resource metadata: %w", err)
	}
	return resource, nil
}

func scanRecord(row rowScanner) (Record, error) {
	var record Record
	var metadata []byte
	err := row.Scan(
		&record.ID, &record.WorkspaceID, &record.Type, &record.Title, &record.Body,
		&record.Status, &record.Authority, &record.ValidFrom, &record.ValidTo,
		&record.SupersededByRecordID, &metadata, &record.CreatedByActorID,
		&record.LastChangedByActorID, &record.Version, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return Record{}, err
	}
	if err := json.Unmarshal(metadata, &record.Metadata); err != nil {
		return Record{}, fmt.Errorf("decode record metadata: %w", err)
	}
	record.Evidence = make([]Evidence, 0)
	return record, nil
}

func loadEvidence(
	ctx context.Context, db database, organizationID, workspaceID, recordID string,
) ([]Evidence, error) {
	rows, err := db.Query(ctx, `
		SELECT evidence.relation, evidence.note, evidence.created_at,
		       resource.id, resource.workspace_id, resource.kind, resource.title,
		       resource.locator, resource.description, resource.classification,
		       resource.status, resource.metadata, resource.created_by_actor_id,
		       resource.version, resource.created_at, resource.updated_at, resource.archived_at
		FROM knowledge.record_evidence AS evidence
		JOIN knowledge.resources AS resource
		  ON resource.organization_id = evidence.organization_id
		 AND resource.workspace_id = evidence.workspace_id
		 AND resource.id = evidence.resource_id
		WHERE evidence.organization_id = $1
		  AND evidence.workspace_id = $2
		  AND evidence.record_id = $3
		ORDER BY evidence.created_at, resource.id, evidence.relation
	`, organizationID, workspaceID, recordID)
	if err != nil {
		return nil, fmt.Errorf("list record evidence: %w", err)
	}
	defer rows.Close()
	result := make([]Evidence, 0)
	for rows.Next() {
		var evidence Evidence
		var metadata []byte
		if err := rows.Scan(
			&evidence.Relation, &evidence.Note, &evidence.CreatedAt,
			&evidence.Resource.ID, &evidence.Resource.WorkspaceID, &evidence.Resource.Kind,
			&evidence.Resource.Title, &evidence.Resource.Locator, &evidence.Resource.Description,
			&evidence.Resource.Classification, &evidence.Resource.Status, &metadata,
			&evidence.Resource.CreatedByActorID, &evidence.Resource.Version,
			&evidence.Resource.CreatedAt, &evidence.Resource.UpdatedAt, &evidence.Resource.ArchivedAt,
		); err != nil {
			return nil, fmt.Errorf("list record evidence: %w", err)
		}
		if err := json.Unmarshal(metadata, &evidence.Resource.Metadata); err != nil {
			return nil, fmt.Errorf("decode evidence resource metadata: %w", err)
		}
		result = append(result, evidence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list record evidence: %w", err)
	}
	return result, nil
}

func reserveCommand(
	ctx context.Context, tx pgx.Tx, organizationID, commandType, key string,
	requestHash [sha256.Size]byte,
) (string, bool, error) {
	var commandID string
	err := tx.QueryRow(ctx, `
		INSERT INTO platform.idempotency_records (
			organization_id, project_id, command_type, idempotency_key, request_hash
		)
		VALUES ($1, NULL, $2, $3, $4)
		ON CONFLICT ON CONSTRAINT idempotency_records_scope_key_uq DO NOTHING
		RETURNING command_id
	`, organizationID, commandType, key, requestHash[:]).Scan(&commandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reserve %s idempotency key: %w", commandType, err)
	}
	return commandID, false, nil
}

func completeCommand(
	ctx context.Context, tx pgx.Tx, organizationID, commandType, key, aggregateID string, response any,
) error {
	body, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode %s response: %w", commandType, err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET outcome = 'succeeded',
		    response_status = CASE WHEN $2 = 'knowledge.record.status' THEN 200 ELSE 201 END,
		    response_body = $4,
		    aggregate_id = $5, completed_at = transaction_timestamp()
		WHERE organization_id = $1 AND project_id IS NULL
		  AND command_type = $2 AND idempotency_key = $3
	`, organizationID, commandType, key, body, aggregateID)
	if err != nil {
		return fmt.Errorf("store %s result: %w", commandType, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("store %s result: idempotency reservation disappeared", commandType)
	}
	return nil
}

func loadStoredResponse(
	ctx context.Context, tx pgx.Tx, organizationID, commandType, key string,
	requestHash [sha256.Size]byte,
) ([]byte, error) {
	var storedHash []byte
	var outcome *string
	var body []byte
	err := tx.QueryRow(ctx, `
		SELECT request_hash, outcome, response_body
		FROM platform.idempotency_records
		WHERE organization_id = $1 AND project_id IS NULL
		  AND command_type = $2 AND idempotency_key = $3
	`, organizationID, commandType, key).Scan(&storedHash, &outcome, &body)
	if err != nil {
		return nil, fmt.Errorf("load %s result: %w", commandType, err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return nil, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(body) == 0 {
		return nil, ErrCommandIncomplete
	}
	return body, nil
}

func loadStoredResource(
	ctx context.Context, tx pgx.Tx, organizationID, commandType, key string,
	requestHash [sha256.Size]byte,
) (CreateResourceResult, error) {
	body, err := loadStoredResponse(ctx, tx, organizationID, commandType, key, requestHash)
	if err != nil {
		return CreateResourceResult{}, err
	}
	var resource Resource
	if err := json.Unmarshal(body, &resource); err != nil {
		return CreateResourceResult{}, fmt.Errorf("decode %s result: %w", commandType, err)
	}
	return CreateResourceResult{Resource: resource, Replayed: true}, nil
}

func loadStoredRecord(
	ctx context.Context, tx pgx.Tx, organizationID, commandType, key string,
	requestHash [sha256.Size]byte,
) (CreateRecordResult, error) {
	body, err := loadStoredResponse(ctx, tx, organizationID, commandType, key, requestHash)
	if err != nil {
		return CreateRecordResult{}, err
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return CreateRecordResult{}, fmt.Errorf("decode %s result: %w", commandType, err)
	}
	return CreateRecordResult{Record: record, Replayed: true}, nil
}

func loadStoredStatus(
	ctx context.Context, tx pgx.Tx, organizationID, key string,
	requestHash [sha256.Size]byte,
) (RecordStatusResult, error) {
	body, err := loadStoredResponse(ctx, tx, organizationID, "knowledge.record.status", key, requestHash)
	if err != nil {
		return RecordStatusResult{}, err
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return RecordStatusResult{}, fmt.Errorf("decode knowledge.record.status result: %w", err)
	}
	return RecordStatusResult{Record: record, Replayed: true}, nil
}

func appendKnowledgeEvent(
	ctx context.Context, tx pgx.Tx,
	organizationID, workspaceID, actorID, eventType, aggregateType, aggregateID string,
	aggregateVersion int64, payload any,
) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s event: %w", eventType, err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO knowledge.events (
			organization_id, workspace_id, event_type, aggregate_type,
			aggregate_id, aggregate_version, actor_id, payload, payload_hash
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8,
		        sha256(convert_to(($8::jsonb)::text, 'UTF8')))
	`, organizationID, workspaceID, eventType, aggregateType, aggregateID,
		aggregateVersion, actorID, encoded)
	if err != nil {
		return fmt.Errorf("append %s event: %w", eventType, err)
	}
	return nil
}

func mapResourceWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "resources_workspace_locator_uq" {
		return ErrResourceExists
	}
	return fmt.Errorf("create knowledge resource: %w", err)
}

func mapRecordWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.ConstraintName == "records_validity_ck" || pgErr.ConstraintName == "records_terminal_validity_ck") {
		return &ValidationError{Field: "valid_to", Message: "must not be before valid_from"}
	}
	return fmt.Errorf("create knowledge record: %w", err)
}

func rollback(tx pgx.Tx) {
	_ = tx.Rollback(context.Background())
}
