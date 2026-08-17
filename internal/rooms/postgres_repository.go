package rooms

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

func (r *PostgresRepository) CreateRoom(
	ctx context.Context,
	organizationID, actorID, workspaceID, key string,
	requestHash [sha256.Size]byte,
	input CreateRoomInput,
) (CreateRoomResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CreateRoomResult{}, fmt.Errorf("begin room.create: %w", err)
	}
	defer rollback(tx)
	_, replay, err := reserveCommand(ctx, tx, organizationID, "room.create", key, requestHash)
	if err != nil {
		return CreateRoomResult{}, err
	}
	if replay {
		var stored Room
		if err := loadStoredResponse(ctx, tx, organizationID, "room.create", key, requestHash, &stored); err != nil {
			return CreateRoomResult{}, err
		}
		return CreateRoomResult{Room: stored, Replayed: true}, nil
	}

	room, err := scanRoom(tx.QueryRow(ctx, `
		INSERT INTO collaboration.rooms (
			organization_id, workspace_id, slug, name, description,
			created_by_actor_id
		)
		SELECT $1, workspace.id, $3, $4, $5, $6
		FROM identity.workspaces AS workspace
		WHERE workspace.organization_id = $1
		  AND workspace.id = $2
		  AND workspace.status = 'active'
		RETURNING id, workspace_id, slug, name, description, status,
		          managed_default, created_by_actor_id, version, created_at,
		          updated_at, last_message_at
	`, organizationID, workspaceID, input.Slug, input.Name, input.Description, actorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CreateRoomResult{}, ErrWorkspaceNotFound
	}
	if err != nil {
		return CreateRoomResult{}, mapRoomWriteError(err)
	}
	if err := completeCommand(ctx, tx, organizationID, "room.create", key, room.ID, room, 201); err != nil {
		return CreateRoomResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateRoomResult{}, fmt.Errorf("commit room.create: %w", err)
	}
	return CreateRoomResult{Room: room}, nil
}

func (r *PostgresRepository) ListRooms(ctx context.Context, organizationID, workspaceID string) ([]Room, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, slug, name, description, status,
		       managed_default, created_by_actor_id, version, created_at,
		       updated_at, last_message_at
		FROM collaboration.rooms
		WHERE organization_id = $1 AND workspace_id = $2
		ORDER BY status, managed_default DESC,
		         last_message_at DESC NULLS LAST, lower(name), id
	`, organizationID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace rooms: %w", err)
	}
	defer rows.Close()
	result := make([]Room, 0)
	for rows.Next() {
		room, scanErr := scanRoom(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workspace rooms: %w", scanErr)
		}
		result = append(result, room)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workspace rooms: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListParticipants(ctx context.Context, organizationID, workspaceID string) ([]Participant, error) {
	rows, err := r.pool.Query(ctx, `
		WITH eligible_principals AS (
			SELECT membership.principal_id
			FROM identity.organization_memberships AS membership
			WHERE membership.organization_id = $1
			UNION
			SELECT member.principal_id
			FROM identity.workspace_members AS member
			WHERE member.organization_id = $1 AND member.workspace_id = $2
			UNION
			SELECT membership.principal_id
			FROM identity.project_memberships AS membership
			JOIN identity.workspace_projects AS relation
			  ON relation.organization_id = membership.organization_id
			 AND relation.project_id = membership.project_id
			WHERE relation.organization_id = $1 AND relation.workspace_id = $2
		), eligible_actors AS (
			SELECT principal_id AS actor_id FROM eligible_principals
			UNION
			SELECT agent.id
			FROM identity.agents AS agent
			WHERE agent.organization_id = $1
			  AND (
			      agent.sponsor_principal_id IN (SELECT principal_id FROM eligible_principals)
			      OR EXISTS (
			          SELECT 1
			          FROM identity.sessions AS session
			          JOIN identity.workspace_projects AS relation
			            ON relation.organization_id = session.organization_id
			           AND relation.project_id = session.project_id
			          WHERE session.organization_id = agent.organization_id
			            AND session.actor_id = agent.id
			            AND relation.workspace_id = $2
			      )
			  )
		)
		SELECT actor.id, actor.display_name, actor.kind,
		       COALESCE(agent.agent_type, ''),
		       EXISTS (
		           SELECT 1 FROM identity.sessions AS session
		           WHERE session.organization_id = actor.organization_id
		             AND session.actor_id = actor.id
		             AND session.status = 'active'
		             AND session.expires_at > transaction_timestamp()
		       )
		FROM identity.actors AS actor
		LEFT JOIN identity.agents AS agent
		  ON agent.organization_id = actor.organization_id
		 AND agent.id = actor.id
		WHERE actor.organization_id = $1
		  AND actor.id IN (SELECT actor_id FROM eligible_actors)
		  AND actor.kind IN ('principal', 'agent')
		  AND actor.status = 'active'
		ORDER BY CASE actor.kind WHEN 'principal' THEN 0 ELSE 1 END,
		         lower(actor.display_name), actor.id
	`, organizationID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list room participants: %w", err)
	}
	defer rows.Close()
	result := make([]Participant, 0)
	for rows.Next() {
		var participant Participant
		if err := rows.Scan(&participant.ActorID, &participant.DisplayName, &participant.Kind, &participant.AgentType, &participant.Online); err != nil {
			return nil, fmt.Errorf("list room participants: %w", err)
		}
		result = append(result, participant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list room participants: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) CreateMessage(
	ctx context.Context,
	organizationID, principalID string,
	allowAll bool,
	workspaceID, roomID, key string,
	requestHash [sha256.Size]byte,
	input CreateMessageInput,
) (CreateMessageResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CreateMessageResult{}, fmt.Errorf("begin room.message.create: %w", err)
	}
	defer rollback(tx)
	_, replay, err := reserveCommand(ctx, tx, organizationID, "room.message.create", key, requestHash)
	if err != nil {
		return CreateMessageResult{}, err
	}
	if replay {
		var stored Message
		if err := loadStoredResponse(ctx, tx, organizationID, "room.message.create", key, requestHash, &stored); err != nil {
			return CreateMessageResult{}, err
		}
		return CreateMessageResult{Message: stored, Replayed: true}, nil
	}

	authorID, sessionID, err := resolveAuthor(ctx, tx, organizationID, principalID, allowAll, input.AuthorSessionID)
	if err != nil {
		return CreateMessageResult{}, err
	}
	var replyID, rootID *string
	if input.ReplyToMessageID != "" {
		var root string
		err = tx.QueryRow(ctx, `
			SELECT COALESCE(thread_root_message_id, id)
			FROM collaboration.messages
			WHERE organization_id = $1 AND workspace_id = $2
			  AND room_id = $3 AND id = $4
		`, organizationID, workspaceID, roomID, input.ReplyToMessageID).Scan(&root)
		if errors.Is(err, pgx.ErrNoRows) {
			return CreateMessageResult{}, ErrMessageNotFound
		}
		if err != nil {
			return CreateMessageResult{}, fmt.Errorf("resolve room message reply: %w", err)
		}
		reply := input.ReplyToMessageID
		replyID, rootID = &reply, &root
	}

	message, err := scanMessage(tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO collaboration.messages (
				organization_id, workspace_id, room_id, author_actor_id,
				author_session_id, reply_to_message_id, thread_root_message_id, body
			)
			SELECT $1, room.workspace_id, room.id, $4, $5, $6, $7, $8
			FROM collaboration.rooms AS room
			WHERE room.organization_id = $1
			  AND room.workspace_id = $2
			  AND room.id = $3
			  AND room.status = 'active'
			RETURNING *
		)
		SELECT message.id, message.workspace_id, message.room_id,
		       message.author_actor_id, actor.display_name, actor.kind,
		       message.author_session_id, message.reply_to_message_id,
		       message.thread_root_message_id, message.body, message.created_at
		FROM inserted AS message
		JOIN identity.actors AS actor
		  ON actor.organization_id = message.organization_id
		 AND actor.id = message.author_actor_id
	`, organizationID, workspaceID, roomID, authorID, sessionID, replyID, rootID, input.Body))
	if errors.Is(err, pgx.ErrNoRows) {
		return CreateMessageResult{}, ErrRoomNotFound
	}
	if err != nil {
		return CreateMessageResult{}, fmt.Errorf("create room message: %w", err)
	}
	for _, mentionedActorID := range input.MentionActorIDs {
		if mentionedActorID == authorID {
			continue
		}
		var mentionID string
		err = tx.QueryRow(ctx, `
			INSERT INTO collaboration.mentions (
				organization_id, workspace_id, room_id, message_id, mentioned_actor_id
			)
			SELECT $1, $2, $3, $4, actor.id
			FROM identity.actors AS actor
			WHERE actor.organization_id = $1
			  AND actor.id = $5
			  AND actor.kind IN ('principal', 'agent')
			  AND actor.status = 'active'
			RETURNING id
		`, organizationID, workspaceID, roomID, message.ID, mentionedActorID).Scan(&mentionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return CreateMessageResult{}, ErrParticipantNotFound
		}
		if err != nil {
			return CreateMessageResult{}, fmt.Errorf("create room mention: %w", err)
		}
	}
	message.Mentions, err = loadMessageMentions(ctx, tx, organizationID, message.ID)
	if err != nil {
		return CreateMessageResult{}, err
	}
	if err := completeCommand(ctx, tx, organizationID, "room.message.create", key, message.ID, message, 201); err != nil {
		return CreateMessageResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateMessageResult{}, fmt.Errorf("commit room.message.create: %w", err)
	}
	return CreateMessageResult{Message: message}, nil
}

func (r *PostgresRepository) ListMessages(
	ctx context.Context,
	organizationID, workspaceID, roomID string,
	options MessageListOptions,
) ([]Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT message.id, message.workspace_id, message.room_id,
		       message.author_actor_id, actor.display_name, actor.kind,
		       message.author_session_id, message.reply_to_message_id,
		       message.thread_root_message_id, message.body, message.created_at
		FROM collaboration.messages AS message
		JOIN identity.actors AS actor
		  ON actor.organization_id = message.organization_id
		 AND actor.id = message.author_actor_id
		WHERE message.organization_id = $1
		  AND message.workspace_id = $2
		  AND message.room_id = $3
		  AND (
		      $4 = '' OR message.ingest_sequence < COALESCE((
		          SELECT anchor.ingest_sequence
		          FROM collaboration.messages AS anchor
		          WHERE anchor.organization_id = $1
		            AND anchor.workspace_id = $2
		            AND anchor.room_id = $3
		            AND anchor.id::text = $4
		      ), 0)
		  )
		  AND ($5 = '' OR message.id::text = $5 OR message.thread_root_message_id::text = $5)
		  AND ($6 = '' OR message.search_document @@ websearch_to_tsquery('simple', $6))
		ORDER BY message.ingest_sequence DESC
		LIMIT $7
	`, organizationID, workspaceID, roomID, options.BeforeMessageID,
		options.ThreadRootMessageID, options.Query, options.Limit)
	if err != nil {
		return nil, fmt.Errorf("list room messages: %w", err)
	}
	reversed := make([]Message, 0)
	for rows.Next() {
		message, scanErr := scanMessage(rows)
		if scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("list room messages: %w", scanErr)
		}
		reversed = append(reversed, message)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("list room messages: %w", err)
	}
	rows.Close()
	for index := range reversed {
		reversed[index].Mentions, err = loadMessageMentions(ctx, r.pool, organizationID, reversed[index].ID)
		if err != nil {
			return nil, err
		}
	}
	result := make([]Message, len(reversed))
	for index := range reversed {
		result[len(reversed)-1-index] = reversed[index]
	}
	return result, nil
}

func (r *PostgresRepository) ListInbox(
	ctx context.Context,
	organizationID, principalID string,
	allowAll bool,
	sessionID string,
	options InboxOptions,
) ([]Mention, error) {
	targetActorID, err := resolveInboxActor(ctx, r.pool, organizationID, principalID, allowAll, sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT mention.id, mention.workspace_id, mention.room_id, room.name,
		       mention.status, mention.created_at, mention.read_at,
		       mention.responded_at, mention.dismissed_at,
		       message.id, message.workspace_id, message.room_id,
		       message.author_actor_id, actor.display_name, actor.kind,
		       message.author_session_id, message.reply_to_message_id,
		       message.thread_root_message_id, message.body, message.created_at
		FROM collaboration.mentions AS mention
		JOIN collaboration.rooms AS room
		  ON room.organization_id = mention.organization_id
		 AND room.workspace_id = mention.workspace_id
		 AND room.id = mention.room_id
		JOIN collaboration.messages AS message
		  ON message.organization_id = mention.organization_id
		 AND message.workspace_id = mention.workspace_id
		 AND message.room_id = mention.room_id
		 AND message.id = mention.message_id
		JOIN identity.actors AS actor
		  ON actor.organization_id = message.organization_id
		 AND actor.id = message.author_actor_id
		WHERE mention.organization_id = $1
		  AND mention.mentioned_actor_id = $2
		  AND ($3 = 'all' OR mention.status = $3)
		  AND ($5 = '' OR mention.workspace_id::text = $5)
		ORDER BY mention.created_at DESC, mention.id
		LIMIT $4
	`, organizationID, targetActorID, options.Status, options.Limit, options.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("list room mention inbox: %w", err)
	}
	result := make([]Mention, 0)
	for rows.Next() {
		mention, scanErr := scanMention(rows)
		if scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("list room mention inbox: %w", scanErr)
		}
		result = append(result, mention)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("list room mention inbox: %w", err)
	}
	rows.Close()
	for index := range result {
		result[index].Message.Mentions, err = loadMessageMentions(ctx, r.pool, organizationID, result[index].Message.ID)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *PostgresRepository) UpdateMention(
	ctx context.Context,
	organizationID, principalID string,
	allowAll bool,
	sessionID, mentionID, status string,
) (Mention, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Mention{}, fmt.Errorf("begin room mention update: %w", err)
	}
	defer rollback(tx)
	targetActorID, err := resolveInboxActor(ctx, tx, organizationID, principalID, allowAll, sessionID)
	if err != nil {
		return Mention{}, err
	}
	row := tx.QueryRow(ctx, `
		WITH updated AS (
			UPDATE collaboration.mentions
			SET status = $4,
			    read_at = CASE
			        WHEN $4 IN ('read', 'responded') THEN COALESCE(read_at, transaction_timestamp())
			        ELSE read_at
			    END,
			    responded_at = CASE WHEN $4 = 'responded' THEN transaction_timestamp() ELSE NULL END,
			    dismissed_at = CASE WHEN $4 = 'dismissed' THEN transaction_timestamp() ELSE NULL END
			WHERE organization_id = $1
			  AND id = $2
			  AND mentioned_actor_id = $3
			RETURNING *
		)
		SELECT mention.id, mention.workspace_id, mention.room_id, room.name,
		       mention.status, mention.created_at, mention.read_at,
		       mention.responded_at, mention.dismissed_at,
		       message.id, message.workspace_id, message.room_id,
		       message.author_actor_id, actor.display_name, actor.kind,
		       message.author_session_id, message.reply_to_message_id,
		       message.thread_root_message_id, message.body, message.created_at
		FROM updated AS mention
		JOIN collaboration.rooms AS room
		  ON room.organization_id = mention.organization_id
		 AND room.workspace_id = mention.workspace_id
		 AND room.id = mention.room_id
		JOIN collaboration.messages AS message
		  ON message.organization_id = mention.organization_id
		 AND message.id = mention.message_id
		JOIN identity.actors AS actor
		  ON actor.organization_id = message.organization_id
		 AND actor.id = message.author_actor_id
	`, organizationID, mentionID, targetActorID, status)
	mention, err := scanMention(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mention{}, ErrMentionNotFound
	}
	if err != nil {
		return Mention{}, fmt.Errorf("update room mention: %w", err)
	}
	mention.Message.Mentions, err = loadMessageMentions(ctx, tx, organizationID, mention.Message.ID)
	if err != nil {
		return Mention{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Mention{}, fmt.Errorf("commit room mention update: %w", err)
	}
	return mention, nil
}

func resolveAuthor(ctx context.Context, db rowQueryer, organizationID, principalID string, allowAll bool, requestedSessionID string) (string, *string, error) {
	if requestedSessionID == "" {
		var actorID string
		err := db.QueryRow(ctx, `
			SELECT actor.id
			FROM identity.actors AS actor
			WHERE actor.organization_id = $1
			  AND actor.id = $2
			  AND actor.kind = 'principal'
			  AND actor.status = 'active'
		`, organizationID, principalID).Scan(&actorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, ErrForbidden
		}
		if err != nil {
			return "", nil, fmt.Errorf("resolve room principal author: %w", err)
		}
		return actorID, nil, nil
	}
	var actorID string
	err := db.QueryRow(ctx, `
		SELECT session.actor_id
		FROM identity.sessions AS session
		JOIN identity.agents AS agent
		  ON agent.organization_id = session.organization_id
		 AND agent.id = session.actor_id
		WHERE session.organization_id = $1
		  AND session.id = $2
		  AND session.status = 'active'
		  AND session.expires_at > transaction_timestamp()
		  AND ($4 OR agent.sponsor_principal_id = $3)
	`, organizationID, requestedSessionID, principalID, allowAll).Scan(&actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrForbidden
	}
	if err != nil {
		return "", nil, fmt.Errorf("resolve room agent author: %w", err)
	}
	return actorID, &requestedSessionID, nil
}

func resolveInboxActor(ctx context.Context, db rowQueryer, organizationID, principalID string, allowAll bool, sessionID string) (string, error) {
	if sessionID == "" {
		return principalID, nil
	}
	actorID, _, err := resolveAuthor(ctx, db, organizationID, principalID, allowAll, sessionID)
	return actorID, err
}

func loadMessageMentions(ctx context.Context, db queryer, organizationID, messageID string) ([]MessageMention, error) {
	rows, err := db.Query(ctx, `
		SELECT mention.id, actor.id, actor.display_name, actor.kind, mention.status
		FROM collaboration.mentions AS mention
		JOIN identity.actors AS actor
		  ON actor.organization_id = mention.organization_id
		 AND actor.id = mention.mentioned_actor_id
		WHERE mention.organization_id = $1 AND mention.message_id = $2
		ORDER BY lower(actor.display_name), actor.id
	`, organizationID, messageID)
	if err != nil {
		return nil, fmt.Errorf("list message mentions: %w", err)
	}
	defer rows.Close()
	result := make([]MessageMention, 0)
	for rows.Next() {
		var mention MessageMention
		if err := rows.Scan(&mention.MentionID, &mention.ActorID, &mention.DisplayName, &mention.Kind, &mention.Status); err != nil {
			return nil, fmt.Errorf("list message mentions: %w", err)
		}
		result = append(result, mention)
	}
	return result, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

type rowQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func scanRoom(row rowScanner) (Room, error) {
	var room Room
	err := row.Scan(
		&room.ID, &room.WorkspaceID, &room.Slug, &room.Name,
		&room.Description, &room.Status, &room.ManagedDefault,
		&room.CreatedByActorID, &room.Version, &room.CreatedAt,
		&room.UpdatedAt, &room.LastMessageAt,
	)
	return room, err
}

func scanMessage(row rowScanner) (Message, error) {
	var message Message
	err := row.Scan(
		&message.ID, &message.WorkspaceID, &message.RoomID,
		&message.AuthorActorID, &message.AuthorDisplayName, &message.AuthorKind,
		&message.AuthorSessionID, &message.ReplyToMessageID,
		&message.ThreadRootMessageID, &message.Body, &message.CreatedAt,
	)
	message.Mentions = make([]MessageMention, 0)
	return message, err
}

func scanMention(row rowScanner) (Mention, error) {
	var mention Mention
	err := row.Scan(
		&mention.ID, &mention.WorkspaceID, &mention.RoomID, &mention.RoomName,
		&mention.Status, &mention.CreatedAt, &mention.ReadAt,
		&mention.RespondedAt, &mention.DismissedAt,
		&mention.Message.ID, &mention.Message.WorkspaceID, &mention.Message.RoomID,
		&mention.Message.AuthorActorID, &mention.Message.AuthorDisplayName,
		&mention.Message.AuthorKind, &mention.Message.AuthorSessionID,
		&mention.Message.ReplyToMessageID, &mention.Message.ThreadRootMessageID,
		&mention.Message.Body, &mention.Message.CreatedAt,
	)
	mention.Message.Mentions = make([]MessageMention, 0)
	return mention, err
}

func reserveCommand(ctx context.Context, tx pgx.Tx, organizationID, commandType, key string, requestHash [sha256.Size]byte) (string, bool, error) {
	var commandID string
	err := tx.QueryRow(ctx, `
		INSERT INTO platform.idempotency_records (
			organization_id, project_id, command_type, idempotency_key, request_hash
		)
		VALUES ($1, NULL, $2, $3, $4)
		ON CONFLICT ON CONSTRAINT idempotency_records_scope_key_uq DO NOTHING
		RETURNING command_id
	`, organizationID, commandType, key, requestHash[:]).Scan(&commandID)
	if err == nil {
		return commandID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("reserve %s idempotency key: %w", commandType, err)
	}
	var storedHash []byte
	var outcome *string
	err = tx.QueryRow(ctx, `
		SELECT command_id, request_hash, outcome
		FROM platform.idempotency_records
		WHERE organization_id = $1 AND project_id IS NULL
		  AND command_type = $2 AND idempotency_key = $3
	`, organizationID, commandType, key).Scan(&commandID, &storedHash, &outcome)
	if err != nil {
		return "", false, fmt.Errorf("load %s idempotency key: %w", commandType, err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return "", false, ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" {
		return "", false, ErrCommandIncomplete
	}
	return commandID, true, nil
}

func completeCommand(ctx context.Context, tx pgx.Tx, organizationID, commandType, key, aggregateID string, value any, responseStatus int) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s response: %w", commandType, err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE platform.idempotency_records
		SET outcome = 'succeeded', response_status = $4, response_body = $5,
		    aggregate_id = $6, completed_at = transaction_timestamp()
		WHERE organization_id = $1 AND project_id IS NULL
		  AND command_type = $2 AND idempotency_key = $3
	`, organizationID, commandType, key, responseStatus, body, aggregateID)
	if err != nil {
		return fmt.Errorf("complete %s command: %w", commandType, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("complete %s command: idempotency reservation disappeared", commandType)
	}
	return nil
}

func loadStoredResponse(ctx context.Context, tx pgx.Tx, organizationID, commandType, key string, requestHash [sha256.Size]byte, target any) error {
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
		return fmt.Errorf("load %s response: %w", commandType, err)
	}
	if !bytes.Equal(storedHash, requestHash[:]) {
		return ErrIdempotencyConflict
	}
	if outcome == nil || *outcome != "succeeded" || len(body) == 0 {
		return ErrCommandIncomplete
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode %s response: %w", commandType, err)
	}
	return nil
}

func mapRoomWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "rooms_workspace_slug_uq" {
		return ErrSlugTaken
	}
	return fmt.Errorf("create room: %w", err)
}

func rollback(tx pgx.Tx) {
	_ = tx.Rollback(context.Background())
}
