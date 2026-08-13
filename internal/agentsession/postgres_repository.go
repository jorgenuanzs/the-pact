package agentsession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	bootstrapPrincipalID = "00000000-0000-4000-8000-000000000002"
	sessionTTL           = 45 * time.Second
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Start(
	ctx context.Context,
	organizationID string,
	projectID string,
	input StartInput,
) (Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("begin agent session: %w", err)
	}
	defer rollback(tx)

	var projectExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM identity.projects
			WHERE organization_id = $1 AND id = $2 AND status <> 'archived'
		)
	`, organizationID, projectID).Scan(&projectExists); err != nil {
		return Session{}, fmt.Errorf("verify session project: %w", err)
	}
	if !projectExists {
		return Session{}, ErrNotFound
	}

	capabilities, err := json.Marshal(map[string]bool{
		"workspace.diff.observe.v1": input.ObserveGit,
	})
	if err != nil {
		return Session{}, fmt.Errorf("encode session capabilities: %w", err)
	}
	nodeID, err := upsertNode(ctx, tx, organizationID, input, capabilities)
	if err != nil {
		return Session{}, err
	}
	agentID, err := upsertAgent(ctx, tx, organizationID, input)
	if err != nil {
		return Session{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE identity.sessions
		SET status = 'expired',
		    ended_at = transaction_timestamp(),
		    version = version + 1
		WHERE organization_id = $1
		  AND project_id = $2
		  AND actor_id = $3
		  AND node_id = $4
		  AND status IN ('starting', 'active', 'stale')
	`, organizationID, projectID, agentID, nodeID); err != nil {
		return Session{}, fmt.Errorf("expire previous agent sessions: %w", err)
	}

	var session Session
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.sessions (
			organization_id,
			project_id,
			actor_id,
			node_id,
			status,
			client_type,
			protocol_version,
			announced_capabilities,
			expires_at
		)
		VALUES ($1, $2, $3, $4, 'active', $5, 'pact-cli/v1', $6, transaction_timestamp() + ($7 * interval '1 second'))
		RETURNING id, project_id, actor_id, node_id, client_type, status, started_at, last_seen_at, expires_at
	`, organizationID, projectID, agentID, nodeID, input.ClientType, capabilities, int64(sessionTTL/time.Second)).Scan(
		&session.ID,
		&session.ProjectID,
		&session.ActorID,
		&session.NodeID,
		&session.ClientType,
		&session.Status,
		&session.StartedAt,
		&session.LastSeenAt,
		&session.ExpiresAt,
	)
	if err != nil {
		return Session{}, fmt.Errorf("create agent session: %w", err)
	}
	session.ActorName = input.AgentName
	session.NodeName = input.NodeName
	if err := tx.Commit(ctx); err != nil {
		return Session{}, fmt.Errorf("commit agent session: %w", err)
	}
	return session, nil
}

func (r *PostgresRepository) Heartbeat(
	ctx context.Context,
	organizationID string,
	sessionID string,
) (Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("begin agent heartbeat: %w", err)
	}
	defer rollback(tx)

	var session Session
	err = tx.QueryRow(ctx, `
		UPDATE identity.sessions AS session
		SET last_seen_at = transaction_timestamp(),
		    expires_at = transaction_timestamp() + ($3 * interval '1 second'),
		    version = session.version + 1
		FROM identity.actors AS actor,
		     identity.nodes AS node
		WHERE session.organization_id = $1
		  AND session.id = $2
		  AND session.status = 'active'
		  AND actor.organization_id = session.organization_id
		  AND actor.id = session.actor_id
		  AND node.organization_id = session.organization_id
		  AND node.id = session.node_id
		RETURNING session.id, session.project_id, session.actor_id, actor.display_name,
		          session.node_id, node.name, session.client_type, session.status,
		          session.started_at, session.last_seen_at, session.expires_at
	`, organizationID, sessionID, int64(sessionTTL/time.Second)).Scan(
		&session.ID,
		&session.ProjectID,
		&session.ActorID,
		&session.ActorName,
		&session.NodeID,
		&session.NodeName,
		&session.ClientType,
		&session.Status,
		&session.StartedAt,
		&session.LastSeenAt,
		&session.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("heartbeat agent session: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.nodes
		SET lifecycle_status = 'active',
		    last_seen_at = transaction_timestamp(),
		    updated_at = transaction_timestamp(),
		    version = version + 1
		WHERE organization_id = $1 AND id = $2
	`, organizationID, session.NodeID); err != nil {
		return Session{}, fmt.Errorf("heartbeat Pact node: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, fmt.Errorf("commit agent heartbeat: %w", err)
	}
	return session, nil
}

func (r *PostgresRepository) Close(ctx context.Context, organizationID, sessionID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin agent session close: %w", err)
	}
	defer rollback(tx)
	var nodeID string
	err = tx.QueryRow(ctx, `
		UPDATE identity.sessions
		SET status = 'closed',
		    ended_at = transaction_timestamp(),
		    last_seen_at = transaction_timestamp(),
		    version = version + 1
		WHERE organization_id = $1
		  AND id = $2
		  AND status IN ('starting', 'active', 'stale')
		RETURNING node_id
	`, organizationID, sessionID).Scan(&nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("close agent session: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.nodes AS node
		SET lifecycle_status = 'offline',
		    updated_at = transaction_timestamp(),
		    version = version + 1
		WHERE node.organization_id = $1
		  AND node.id = $2
		  AND NOT EXISTS (
		      SELECT 1 FROM identity.sessions AS session
		      WHERE session.organization_id = node.organization_id
		        AND session.node_id = node.id
		        AND session.status = 'active'
		        AND session.expires_at > transaction_timestamp()
		  )
	`, organizationID, nodeID); err != nil {
		return fmt.Errorf("mark Pact node offline: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit agent session close: %w", err)
	}
	return nil
}

func upsertNode(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	input StartInput,
	capabilities []byte,
) (string, error) {
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
	`, "pact-node:"+organizationID+":"+input.NodeKey); err != nil {
		return "", fmt.Errorf("lock Pact node identity: %w", err)
	}
	var nodeID string
	err := tx.QueryRow(ctx, `
		SELECT id FROM identity.nodes
		WHERE organization_id = $1 AND node_key = $2
		FOR UPDATE
	`, organizationID, input.NodeKey).Scan(&nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO identity.actors (organization_id, kind, display_name)
			VALUES ($1, 'node', $2)
			RETURNING id
		`, organizationID, input.NodeName).Scan(&nodeID)
		if err != nil {
			return "", fmt.Errorf("create Pact node actor: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO identity.nodes (
				id, organization_id, node_key, name, lifecycle_status,
				capabilities, last_seen_at
			)
			VALUES ($1, $2, $3, $4, 'active', $5, transaction_timestamp())
		`, nodeID, organizationID, input.NodeKey, input.NodeName, capabilities)
		if err != nil {
			return "", fmt.Errorf("create Pact node: %w", err)
		}
		return nodeID, nil
	}
	if err != nil {
		return "", fmt.Errorf("find Pact node: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.nodes
		SET name = $3,
		    lifecycle_status = 'active',
		    capabilities = $4,
		    last_seen_at = transaction_timestamp(),
		    updated_at = transaction_timestamp(),
		    version = version + 1
		WHERE organization_id = $1 AND id = $2
	`, organizationID, nodeID, input.NodeName, capabilities); err != nil {
		return "", fmt.Errorf("activate Pact node: %w", err)
	}
	return nodeID, nil
}

func upsertAgent(ctx context.Context, tx pgx.Tx, organizationID string, input StartInput) (string, error) {
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
	`, "pact-agent:"+organizationID+":"+input.AgentType+":"+input.AgentName); err != nil {
		return "", fmt.Errorf("lock agent identity: %w", err)
	}
	var agentID string
	err := tx.QueryRow(ctx, `
		SELECT id FROM identity.agents
		WHERE organization_id = $1
		  AND sponsor_principal_id = $2
		  AND agent_type = $3
		  AND lower(name) = lower($4)
	`, organizationID, bootstrapPrincipalID, input.AgentType, input.AgentName).Scan(&agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO identity.actors (organization_id, kind, display_name)
			VALUES ($1, 'agent', $2)
			RETURNING id
		`, organizationID, input.AgentName).Scan(&agentID)
		if err != nil {
			return "", fmt.Errorf("create agent actor: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO identity.agents (
				id, organization_id, sponsor_principal_id, name, agent_type,
				declared_capabilities
			)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, agentID, organizationID, bootstrapPrincipalID, input.AgentName, input.AgentType, []byte(`{"pact.cli.session.v1":true}`))
		if err != nil {
			return "", fmt.Errorf("create agent identity: %w", err)
		}
		return agentID, nil
	}
	if err != nil {
		return "", fmt.Errorf("find agent identity: %w", err)
	}
	return agentID, nil
}

func rollback(tx pgx.Tx) {
	rollbackContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackContext)
}
