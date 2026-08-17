package access

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

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

func (r *PostgresRepository) Authenticate(
	ctx context.Context,
	organizationID string,
	digest [sha256.Size]byte,
) (Principal, error) {
	var principal Principal
	err := r.pool.QueryRow(ctx, `
		UPDATE identity.access_tokens AS token
		SET last_used_at = transaction_timestamp()
		FROM identity.principals AS principal,
		     identity.actors AS actor,
		     identity.organization_memberships AS membership
		WHERE token.organization_id = $1
		  AND token.token_digest = $2
		  AND token.status = 'active'
		  AND token.expires_at > transaction_timestamp()
		  AND principal.organization_id = token.organization_id
		  AND principal.id = token.principal_id
		  AND actor.organization_id = principal.organization_id
		  AND actor.id = principal.id
		  AND actor.status = 'active'
		  AND membership.organization_id = principal.organization_id
		  AND membership.principal_id = principal.id
		RETURNING principal.id, principal.organization_id, actor.display_name,
		          principal.principal_type, membership.role, token.id
	`, organizationID, digest[:]).Scan(
		&principal.ID,
		&principal.OrganizationID,
		&principal.DisplayName,
		&principal.PrincipalType,
		&principal.OrganizationRole,
		&principal.TokenID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthorized
	}
	if err != nil {
		return Principal{}, fmt.Errorf("authenticate personal access token: %w", err)
	}
	return principal, nil
}

func (r *PostgresRepository) ProjectRole(
	ctx context.Context,
	organizationID string,
	principalID string,
	projectID string,
) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `
		SELECT membership.role
		FROM identity.project_memberships AS membership
		JOIN identity.projects AS project
		  ON project.organization_id = membership.organization_id
		 AND project.id = membership.project_id
		WHERE membership.organization_id = $1
		  AND membership.principal_id = $2
		  AND membership.project_id = $3
		  AND project.status <> 'archived'
	`, organizationID, principalID, projectID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("read project membership: %w", err)
	}
	return role, nil
}

func (r *PostgresRepository) VisibleProjectIDs(
	ctx context.Context,
	organizationID string,
	principalID string,
) (map[string]struct{}, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT project_id
		FROM identity.project_memberships
		WHERE organization_id = $1 AND principal_id = $2
	`, organizationID, principalID)
	if err != nil {
		return nil, fmt.Errorf("list visible project memberships: %w", err)
	}
	defer rows.Close()
	visible := make(map[string]struct{})
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return nil, fmt.Errorf("read visible project membership: %w", err)
		}
		visible[projectID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate visible project memberships: %w", err)
	}
	return visible, nil
}

func (r *PostgresRepository) GetProjectAccess(
	ctx context.Context,
	organizationID string,
	projectID string,
	now time.Time,
	freshAfter time.Time,
) (ProjectAccess, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ProjectAccess{}, fmt.Errorf("begin project access snapshot: %w", err)
	}
	defer rollback(tx)

	var projectExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM identity.projects
			WHERE organization_id = $1 AND id = $2 AND status <> 'archived'
		)
	`, organizationID, projectID).Scan(&projectExists); err != nil {
		return ProjectAccess{}, fmt.Errorf("verify access project: %w", err)
	}
	if !projectExists {
		return ProjectAccess{}, ErrNotFound
	}

	result := ProjectAccess{
		ProjectID:   projectID,
		Members:     make([]ProjectMember, 0),
		Agents:      make([]ProjectAgent, 0),
		GeneratedAt: now,
	}

	members, err := tx.Query(ctx, `
		SELECT principal.id,
		       actor.display_name,
		       principal.principal_type,
		       actor.status,
		       organization_membership.role,
		       COALESCE(project_membership.role, ''),
		       CASE
		           WHEN organization_membership.role IN ('owner', 'admin') THEN 'owner'
		           ELSE project_membership.role
		       END AS effective_role,
		       CASE
		           WHEN organization_membership.role IN ('owner', 'admin') THEN 'organization'
		           ELSE 'project'
		       END AS access_source,
		       COALESCE((actor.attributes->>'bootstrap')::boolean, false)
		FROM identity.organization_memberships AS organization_membership
		JOIN identity.principals AS principal
		  ON principal.organization_id = organization_membership.organization_id
		 AND principal.id = organization_membership.principal_id
		JOIN identity.actors AS actor
		  ON actor.organization_id = principal.organization_id
		 AND actor.id = principal.id
		LEFT JOIN identity.project_memberships AS project_membership
		  ON project_membership.organization_id = principal.organization_id
		 AND project_membership.principal_id = principal.id
		 AND project_membership.project_id = $2
		WHERE organization_membership.organization_id = $1
		  AND actor.status <> 'retired'
		  AND (
		      organization_membership.role IN ('owner', 'admin')
		      OR project_membership.project_id IS NOT NULL
		  )
		ORDER BY
		  CASE
		      WHEN organization_membership.role = 'owner' THEN 1
		      WHEN organization_membership.role = 'admin' THEN 2
		      WHEN project_membership.role = 'owner' THEN 3
		      WHEN project_membership.role = 'maintainer' THEN 4
		      WHEN project_membership.role = 'contributor' THEN 5
		      ELSE 6
		  END,
		  lower(actor.display_name),
		  principal.id
	`, organizationID, projectID)
	if err != nil {
		return ProjectAccess{}, fmt.Errorf("list project members: %w", err)
	}
	for members.Next() {
		var member ProjectMember
		if err := members.Scan(
			&member.PrincipalID,
			&member.DisplayName,
			&member.PrincipalType,
			&member.Status,
			&member.OrganizationRole,
			&member.ProjectRole,
			&member.EffectiveRole,
			&member.AccessSource,
			&member.Bootstrap,
		); err != nil {
			members.Close()
			return ProjectAccess{}, fmt.Errorf("read project member: %w", err)
		}
		result.Members = append(result.Members, member)
	}
	if err := members.Err(); err != nil {
		members.Close()
		return ProjectAccess{}, fmt.Errorf("iterate project members: %w", err)
	}
	members.Close()

	agents, err := tx.Query(ctx, `
		SELECT agent.id,
		       agent_actor.display_name,
		       agent.agent_type,
		       agent_actor.status,
		       agent.sponsor_principal_id,
		       sponsor_actor.display_name,
		       COALESCE(
		           CASE
		               WHEN sponsor_organization.role IN ('owner', 'admin') THEN 'owner'
		               ELSE sponsor_project.role
		           END,
		           ''
		       ) AS sponsor_effective_role,
		       (
		           agent_actor.status = 'active'
		           AND sponsor_actor.status = 'active'
		           AND (
		               sponsor_organization.role IN ('owner', 'admin')
		               OR sponsor_project.project_id IS NOT NULL
		           )
		       ) AS access_active,
		       session_stats.active_sessions,
		       session_stats.session_count,
		       latest_session.client_type,
		       COALESCE(latest_session.node_name, ''),
		       latest_session.last_seen_at
		FROM identity.agents AS agent
		JOIN identity.actors AS agent_actor
		  ON agent_actor.organization_id = agent.organization_id
		 AND agent_actor.id = agent.id
		JOIN identity.principals AS sponsor
		  ON sponsor.organization_id = agent.organization_id
		 AND sponsor.id = agent.sponsor_principal_id
		JOIN identity.actors AS sponsor_actor
		  ON sponsor_actor.organization_id = sponsor.organization_id
		 AND sponsor_actor.id = sponsor.id
		LEFT JOIN identity.organization_memberships AS sponsor_organization
		  ON sponsor_organization.organization_id = sponsor.organization_id
		 AND sponsor_organization.principal_id = sponsor.id
		LEFT JOIN identity.project_memberships AS sponsor_project
		  ON sponsor_project.organization_id = sponsor.organization_id
		 AND sponsor_project.principal_id = sponsor.id
		 AND sponsor_project.project_id = $2
		JOIN LATERAL (
			SELECT count(*) AS session_count,
			       count(*) FILTER (
			           WHERE session.status = 'active'
			             AND session.expires_at > $3
			             AND session.last_seen_at >= $4
			       ) AS active_sessions
			FROM identity.sessions AS session
			WHERE session.organization_id = agent.organization_id
			  AND session.project_id = $2
			  AND session.actor_id = agent.id
		) AS session_stats ON session_stats.session_count > 0
		JOIN LATERAL (
			SELECT session.client_type,
			       session.last_seen_at,
			       node.name AS node_name
			FROM identity.sessions AS session
			LEFT JOIN identity.nodes AS node
			  ON node.organization_id = session.organization_id
			 AND node.id = session.node_id
			WHERE session.organization_id = agent.organization_id
			  AND session.project_id = $2
			  AND session.actor_id = agent.id
			ORDER BY session.last_seen_at DESC, session.started_at DESC
			LIMIT 1
		) AS latest_session ON true
		WHERE agent.organization_id = $1
		ORDER BY (session_stats.active_sessions > 0) DESC,
		         latest_session.last_seen_at DESC,
		         lower(agent_actor.display_name),
		         agent.id
	`, organizationID, projectID, now, freshAfter)
	if err != nil {
		return ProjectAccess{}, fmt.Errorf("list project agents: %w", err)
	}
	for agents.Next() {
		var agent ProjectAgent
		if err := agents.Scan(
			&agent.AgentID,
			&agent.DisplayName,
			&agent.AgentType,
			&agent.Status,
			&agent.SponsorPrincipalID,
			&agent.SponsorDisplayName,
			&agent.SponsorEffectiveRole,
			&agent.AccessActive,
			&agent.ActiveSessions,
			&agent.SessionCount,
			&agent.LastClientType,
			&agent.LastNodeName,
			&agent.LastSeenAt,
		); err != nil {
			agents.Close()
			return ProjectAccess{}, fmt.Errorf("read project agent: %w", err)
		}
		agent.Connected = agent.ActiveSessions > 0
		result.Agents = append(result.Agents, agent)
	}
	if err := agents.Err(); err != nil {
		agents.Close()
		return ProjectAccess{}, fmt.Errorf("iterate project agents: %w", err)
	}
	agents.Close()

	if err := tx.Commit(ctx); err != nil {
		return ProjectAccess{}, fmt.Errorf("commit project access snapshot: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) CreateInvitation(
	ctx context.Context,
	organizationID string,
	principal Principal,
	projectID string,
	input CreateInvitationInput,
	digest [sha256.Size]byte,
) (Invitation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, fmt.Errorf("begin invitation create: %w", err)
	}
	defer rollback(tx)
	if _, err := tx.Exec(ctx, `
		UPDATE identity.invitations
		SET status = 'expired'
		WHERE organization_id = $1
		  AND project_id = $2
		  AND lower(email) = lower($3)
		  AND status = 'pending'
		  AND expires_at <= transaction_timestamp()
	`, organizationID, projectID, input.Email); err != nil {
		return Invitation{}, fmt.Errorf("expire previous invitation: %w", err)
	}
	var invitation Invitation
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.invitations (
			organization_id, project_id, email, role, token_digest,
			expires_at, created_by_principal_id
		)
		VALUES ($1, $2, $3, $4, $5, transaction_timestamp() + ($6 * interval '1 second'), $7)
		RETURNING id, project_id, email, role, status, expires_at
	`, organizationID, projectID, input.Email, input.Role, digest[:],
		int64(input.ExpiresAfter/time.Second), principal.ID).Scan(
		&invitation.ID,
		&invitation.ProjectID,
		&invitation.Email,
		&invitation.Role,
		&invitation.Status,
		&invitation.ExpiresAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "invitations_pending_email_uq" {
			return Invitation{}, ErrInvitationExists
		}
		return Invitation{}, fmt.Errorf("create invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("commit invitation create: %w", err)
	}
	return invitation, nil
}

func (r *PostgresRepository) AcceptInvitation(
	ctx context.Context,
	organizationID string,
	input AcceptInvitationInput,
	inviteDigest [sha256.Size]byte,
	accessDigest [sha256.Size]byte,
	accessExpiresAt time.Time,
) (AcceptedInvitation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AcceptedInvitation{}, fmt.Errorf("begin invitation accept: %w", err)
	}
	defer rollback(tx)
	var invitation Invitation
	err = tx.QueryRow(ctx, `
		SELECT id, project_id, email, role, status, expires_at
		FROM identity.invitations
		WHERE organization_id = $1
		  AND token_digest = $2
		  AND status = 'pending'
		  AND expires_at > transaction_timestamp()
		FOR UPDATE
	`, organizationID, inviteDigest[:]).Scan(
		&invitation.ID,
		&invitation.ProjectID,
		&invitation.Email,
		&invitation.Role,
		&invitation.Status,
		&invitation.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AcceptedInvitation{}, ErrInvitationInvalid
	}
	if err != nil {
		return AcceptedInvitation{}, fmt.Errorf("load invitation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
	`, "pact-principal:"+organizationID+":"+invitation.Email); err != nil {
		return AcceptedInvitation{}, fmt.Errorf("lock invited principal: %w", err)
	}
	principal, err := findOrCreatePrincipal(ctx, tx, organizationID, invitation.Email, input.DisplayName)
	if err != nil {
		return AcceptedInvitation{}, err
	}
	organizationRole := "member"
	if invitation.Role == "owner" {
		organizationRole = "owner"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.organization_memberships (organization_id, principal_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, principal_id) DO UPDATE
		SET role = CASE
			WHEN identity.organization_memberships.role = 'owner' THEN 'owner'
			WHEN EXCLUDED.role = 'owner' THEN 'owner'
			WHEN identity.organization_memberships.role = 'admin' THEN 'admin'
			ELSE 'member'
		END,
		updated_at = transaction_timestamp()
	`, organizationID, principal.ID, organizationRole); err != nil {
		return AcceptedInvitation{}, fmt.Errorf("create organization membership: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.project_memberships (organization_id, project_id, principal_id, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id, project_id, principal_id) DO UPDATE
		SET role = CASE
			WHEN identity.project_memberships.role = 'owner' THEN 'owner'
			WHEN EXCLUDED.role = 'owner' THEN 'owner'
			WHEN identity.project_memberships.role = 'maintainer' THEN 'maintainer'
			WHEN EXCLUDED.role = 'maintainer' THEN 'maintainer'
			WHEN identity.project_memberships.role = 'contributor' THEN 'contributor'
			WHEN EXCLUDED.role = 'contributor' THEN 'contributor'
			ELSE 'viewer'
		END,
		updated_at = transaction_timestamp()
	`, organizationID, invitation.ProjectID, principal.ID, invitation.Role); err != nil {
		return AcceptedInvitation{}, fmt.Errorf("create project membership: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.invitations
		SET status = 'accepted',
		    accepted_by_principal_id = $3,
		    accepted_at = transaction_timestamp()
		WHERE organization_id = $1 AND id = $2 AND status = 'pending'
	`, organizationID, invitation.ID, principal.ID); err != nil {
		return AcceptedInvitation{}, fmt.Errorf("mark invitation accepted: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO identity.access_tokens (
			organization_id, principal_id, name, token_digest, expires_at
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, organizationID, principal.ID, input.TokenName, accessDigest[:], accessExpiresAt).Scan(&principal.TokenID); err != nil {
		return AcceptedInvitation{}, fmt.Errorf("create personal access token: %w", err)
	}
	principal.OrganizationRole = organizationRole
	if err := tx.Commit(ctx); err != nil {
		return AcceptedInvitation{}, fmt.Errorf("commit invitation accept: %w", err)
	}
	return AcceptedInvitation{
		Principal: principal, ProjectID: invitation.ProjectID,
		ProjectRole: invitation.Role, ExpiresAt: accessExpiresAt,
	}, nil
}

func (r *PostgresRepository) RevokeInvitation(
	ctx context.Context,
	organizationID string,
	principal Principal,
	invitationID string,
) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE identity.invitations AS invitation
		SET status = 'revoked', revoked_at = transaction_timestamp()
		WHERE invitation.organization_id = $1
		  AND invitation.id = $2
		  AND invitation.status = 'pending'
		  AND (
			$4 IN ('owner', 'admin')
			OR EXISTS (
				SELECT 1 FROM identity.project_memberships AS membership
				WHERE membership.organization_id = invitation.organization_id
				  AND membership.project_id = invitation.project_id
				  AND membership.principal_id = $3
				  AND membership.role IN ('owner', 'maintainer')
			)
		  )
	`, organizationID, invitationID, principal.ID, principal.OrganizationRole)
	if err != nil {
		return fmt.Errorf("revoke invitation: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) RevokeToken(
	ctx context.Context,
	organizationID string,
	principal Principal,
) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE identity.access_tokens
		SET status = 'revoked', revoked_at = transaction_timestamp()
		WHERE organization_id = $1
		  AND id = $2
		  AND principal_id = $3
		  AND status = 'active'
	`, organizationID, principal.TokenID, principal.ID)
	if err != nil {
		return fmt.Errorf("revoke personal access token: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) GrantProjectOwner(
	ctx context.Context,
	organizationID string,
	projectID string,
	principalID string,
) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO identity.project_memberships (organization_id, project_id, principal_id, role)
		VALUES ($1, $2, $3, 'owner')
		ON CONFLICT (organization_id, project_id, principal_id) DO UPDATE
		SET role = 'owner', updated_at = transaction_timestamp()
	`, organizationID, projectID, principalID)
	if err != nil {
		return fmt.Errorf("grant project ownership: %w", err)
	}
	return nil
}

func findOrCreatePrincipal(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	email string,
	displayName string,
) (Principal, error) {
	principal := Principal{OrganizationID: organizationID, DisplayName: displayName, PrincipalType: "human"}
	err := tx.QueryRow(ctx, `
		SELECT principal.id, actor.display_name, principal.principal_type
		FROM identity.principals AS principal
		JOIN identity.actors AS actor
		  ON actor.organization_id = principal.organization_id
		 AND actor.id = principal.id
		WHERE principal.organization_id = $1
		  AND principal.external_issuer = 'pact.invitation'
		  AND principal.external_subject = $2
	`, organizationID, email).Scan(&principal.ID, &principal.DisplayName, &principal.PrincipalType)
	if err == nil {
		return principal, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, fmt.Errorf("find invited principal: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO identity.actors (organization_id, kind, display_name, attributes)
		VALUES ($1, 'principal', $2, jsonb_build_object('email', $3::text))
		RETURNING id
	`, organizationID, displayName, email).Scan(&principal.ID); err != nil {
		return Principal{}, fmt.Errorf("create invited actor: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.principals (
			id, organization_id, principal_type, external_issuer, external_subject
		)
		VALUES ($1, $2, 'human', 'pact.invitation', $3)
	`, principal.ID, organizationID, email); err != nil {
		return Principal{}, fmt.Errorf("create invited principal: %w", err)
	}
	return principal, nil
}

func rollback(tx pgx.Tx) {
	rollbackContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackContext)
}
