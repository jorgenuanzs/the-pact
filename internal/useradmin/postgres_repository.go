package useradmin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jorgenuanzs/the-pact/internal/access"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Directory(
	ctx context.Context,
	organizationID string,
	now time.Time,
) (Directory, error) {
	users, err := r.listUsers(ctx, organizationID, now, "")
	if err != nil {
		return Directory{}, err
	}
	invitations, err := r.listInvitations(ctx, organizationID, now)
	if err != nil {
		return Directory{}, err
	}
	events, err := r.listEvents(ctx, organizationID)
	if err != nil {
		return Directory{}, err
	}
	return Directory{
		Users: users, Invitations: invitations, Events: events, GeneratedAt: now,
	}, nil
}

func (r *PostgresRepository) GetUser(
	ctx context.Context,
	organizationID string,
	principalID string,
	now time.Time,
) (User, error) {
	users, err := r.listUsers(ctx, organizationID, now, principalID)
	if err != nil {
		return User{}, err
	}
	if len(users) != 1 {
		return User{}, ErrNotFound
	}
	return users[0], nil
}

func (r *PostgresRepository) listUsers(
	ctx context.Context,
	organizationID string,
	now time.Time,
	principalID string,
) ([]User, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT principal.id,
		       actor.display_name,
		       account.email,
		       account.username,
		       account.status,
		       membership.role,
		       account.created_at,
		       GREATEST(account.updated_at, actor.updated_at, membership.updated_at),
		       (
		           SELECT count(*)
		           FROM identity.web_sessions AS web_session
		           WHERE web_session.organization_id = account.organization_id
		             AND web_session.principal_id = account.principal_id
		             AND web_session.status = 'active'
		             AND web_session.expires_at > $2
		       ) AS active_sessions,
		       (
		           SELECT count(*)
		           FROM identity.device_credentials AS device
		           WHERE device.organization_id = account.organization_id
		             AND device.principal_id = account.principal_id
		             AND device.status = 'active'
		             AND device.expires_at > $2
		       ) AS active_devices,
		       (
		           SELECT max(web_session.created_at)
		           FROM identity.web_sessions AS web_session
		           WHERE web_session.organization_id = account.organization_id
		             AND web_session.principal_id = account.principal_id
		       ) AS last_login_at
		FROM identity.local_accounts AS account
		JOIN identity.principals AS principal
		  ON principal.organization_id = account.organization_id
		 AND principal.id = account.principal_id
		JOIN identity.actors AS actor
		  ON actor.organization_id = principal.organization_id
		 AND actor.id = principal.id
		JOIN identity.organization_memberships AS membership
		  ON membership.organization_id = principal.organization_id
		 AND membership.principal_id = principal.id
		WHERE account.organization_id = $1
		  AND (NULLIF($3, '') IS NULL OR principal.id = NULLIF($3, '')::uuid)
		ORDER BY
		  CASE membership.role WHEN 'owner' THEN 1 WHEN 'admin' THEN 2 ELSE 3 END,
		  CASE account.status WHEN 'active' THEN 1 ELSE 2 END,
		  lower(actor.display_name), principal.id
	`, organizationID, now, principalID)
	if err != nil {
		return nil, fmt.Errorf("list organization users: %w", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	userIndexByID := make(map[string]int)
	for rows.Next() {
		var user User
		if err := rows.Scan(
			&user.PrincipalID,
			&user.DisplayName,
			&user.Email,
			&user.Username,
			&user.Status,
			&user.OrganizationRole,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.ActiveSessions,
			&user.ActiveDevices,
			&user.LastLoginAt,
		); err != nil {
			return nil, fmt.Errorf("read organization user: %w", err)
		}
		user.ProjectRoles = make([]ProjectPermission, 0)
		users = append(users, user)
		userIndexByID[user.PrincipalID] = len(users) - 1
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organization users: %w", err)
	}

	permissionRows, err := r.pool.Query(ctx, `
		SELECT membership.principal_id,
		       project.id,
		       project.name,
		       project.slug,
		       membership.role
		FROM identity.project_memberships AS membership
		JOIN identity.projects AS project
		  ON project.organization_id = membership.organization_id
		 AND project.id = membership.project_id
		WHERE membership.organization_id = $1
		  AND project.status <> 'archived'
		  AND (NULLIF($2, '') IS NULL OR membership.principal_id = NULLIF($2, '')::uuid)
		ORDER BY lower(project.name), project.id
	`, organizationID, principalID)
	if err != nil {
		return nil, fmt.Errorf("list user project permissions: %w", err)
	}
	defer permissionRows.Close()
	for permissionRows.Next() {
		var targetID string
		var permission ProjectPermission
		if err := permissionRows.Scan(
			&targetID,
			&permission.ProjectID,
			&permission.ProjectName,
			&permission.ProjectSlug,
			&permission.Role,
		); err != nil {
			return nil, fmt.Errorf("read user project permission: %w", err)
		}
		if index, ok := userIndexByID[targetID]; ok {
			users[index].ProjectRoles = append(users[index].ProjectRoles, permission)
		}
	}
	if err := permissionRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user project permissions: %w", err)
	}
	return users, nil
}

func (r *PostgresRepository) listInvitations(
	ctx context.Context,
	organizationID string,
	now time.Time,
) ([]Invitation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT invitation.id,
		       invitation.email,
		       invitation.organization_role,
		       COALESCE(invitation.project_id::text, ''),
		       COALESCE(project.name, ''),
		       COALESCE(invitation.role, ''),
		       invitation.status,
		       invitation.expires_at,
		       invitation.created_at,
		       invitation.created_by_principal_id,
		       creator.display_name
		FROM identity.invitations AS invitation
		JOIN identity.actors AS creator
		  ON creator.organization_id = invitation.organization_id
		 AND creator.id = invitation.created_by_principal_id
		LEFT JOIN identity.projects AS project
		  ON project.organization_id = invitation.organization_id
		 AND project.id = invitation.project_id
		WHERE invitation.organization_id = $1
		  AND invitation.status = 'pending'
		  AND invitation.expires_at > $2
		ORDER BY invitation.created_at DESC, invitation.id DESC
	`, organizationID, now)
	if err != nil {
		return nil, fmt.Errorf("list pending user invitations: %w", err)
	}
	defer rows.Close()
	invitations := make([]Invitation, 0)
	for rows.Next() {
		var invitation Invitation
		if err := rows.Scan(
			&invitation.ID,
			&invitation.Email,
			&invitation.OrganizationRole,
			&invitation.ProjectID,
			&invitation.ProjectName,
			&invitation.ProjectRole,
			&invitation.Status,
			&invitation.ExpiresAt,
			&invitation.CreatedAt,
			&invitation.CreatedByPrincipalID,
			&invitation.CreatedByDisplayName,
		); err != nil {
			return nil, fmt.Errorf("read pending user invitation: %w", err)
		}
		invitations = append(invitations, invitation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending user invitations: %w", err)
	}
	return invitations, nil
}

func (r *PostgresRepository) listEvents(
	ctx context.Context,
	organizationID string,
) ([]AdminEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT event.id,
		       event.action,
		       event.actor_principal_id,
		       actor.display_name,
		       COALESCE(event.target_principal_id::text, ''),
		       COALESCE(target.display_name, ''),
		       event.details,
		       event.created_at
		FROM identity.user_admin_events AS event
		JOIN identity.actors AS actor
		  ON actor.organization_id = event.organization_id
		 AND actor.id = event.actor_principal_id
		LEFT JOIN identity.actors AS target
		  ON target.organization_id = event.organization_id
		 AND target.id = event.target_principal_id
		WHERE event.organization_id = $1
		ORDER BY event.created_at DESC, event.id DESC
		LIMIT 50
	`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list user administration events: %w", err)
	}
	defer rows.Close()
	events := make([]AdminEvent, 0)
	for rows.Next() {
		var event AdminEvent
		var details []byte
		if err := rows.Scan(
			&event.ID,
			&event.Action,
			&event.ActorPrincipalID,
			&event.ActorDisplayName,
			&event.TargetPrincipalID,
			&event.TargetDisplayName,
			&details,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("read user administration event: %w", err)
		}
		event.Details = make(map[string]any)
		if err := json.Unmarshal(details, &event.Details); err != nil {
			return nil, fmt.Errorf("decode user administration event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user administration events: %w", err)
	}
	return events, nil
}

func (r *PostgresRepository) UpdateUser(
	ctx context.Context,
	organizationID string,
	actor access.Principal,
	principalID string,
	input UpdateUserInput,
	now time.Time,
) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin user update: %w", err)
	}
	defer rollback(tx)
	if err := lockUserAdministration(ctx, tx, organizationID); err != nil {
		return User{}, err
	}
	targetRole, targetStatus, err := authorizeTarget(ctx, tx, organizationID, actor.ID, principalID)
	if err != nil {
		return User{}, err
	}
	nextRole := targetRole
	if input.OrganizationRole != nil {
		nextRole = *input.OrganizationRole
	}
	nextStatus := targetStatus
	if input.Status != nil {
		nextStatus = *input.Status
	}
	if actor.ID == principalID && (nextStatus != "active" || nextRole != targetRole) {
		return User{}, ErrSelfManagement
	}
	if targetRole == "owner" && (nextRole != "owner" || nextStatus != "active") {
		var activeOwners int
		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM identity.organization_memberships AS membership
			JOIN identity.local_accounts AS account
			  ON account.organization_id = membership.organization_id
			 AND account.principal_id = membership.principal_id
			JOIN identity.actors AS actor
			  ON actor.organization_id = membership.organization_id
			 AND actor.id = membership.principal_id
			WHERE membership.organization_id = $1
			  AND membership.role = 'owner'
			  AND account.status = 'active'
			  AND actor.status = 'active'
		`, organizationID).Scan(&activeOwners); err != nil {
			return User{}, fmt.Errorf("count active organization owners: %w", err)
		}
		if activeOwners <= 1 {
			return User{}, ErrLastOwner
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.actors
		SET display_name = COALESCE($3, display_name),
		    status = CASE WHEN $4::text IS NULL THEN status ELSE $4 END,
		    version = version + 1,
		    updated_at = $5
		WHERE organization_id = $1 AND id = $2
	`, organizationID, principalID, input.DisplayName, input.Status, now); err != nil {
		return User{}, fmt.Errorf("update user actor: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.local_accounts
		SET email = COALESCE($3, email),
		    username = COALESCE($4, username),
		    status = CASE WHEN $5::text IS NULL THEN status ELSE $5 END,
		    failed_login_attempts = CASE WHEN $5 = 'active' THEN 0 ELSE failed_login_attempts END,
		    locked_until = CASE WHEN $5 = 'active' THEN NULL ELSE locked_until END,
		    updated_at = $6
		WHERE organization_id = $1 AND principal_id = $2
	`, organizationID, principalID, input.Email, input.Username, input.Status, now); err != nil {
		return User{}, accountWriteError("update user account", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.organization_memberships
		SET role = COALESCE($3, role), updated_at = $4
		WHERE organization_id = $1 AND principal_id = $2
	`, organizationID, principalID, input.OrganizationRole, now); err != nil {
		return User{}, fmt.Errorf("update organization role: %w", err)
	}
	if nextStatus == "disabled" && targetStatus != "disabled" {
		if err := revokeUserAccess(ctx, tx, organizationID, principalID, now); err != nil {
			return User{}, err
		}
	}
	details := updateDetails(input)
	action := "user.updated"
	if targetStatus != nextStatus && nextStatus == "disabled" {
		action = "user.disabled"
	} else if targetStatus != nextStatus && nextStatus == "active" {
		action = "user.reactivated"
	}
	if err := insertEvent(ctx, tx, organizationID, actor.ID, principalID, action, details, now); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit user update: %w", err)
	}
	return r.GetUser(ctx, organizationID, principalID, now)
}

func (r *PostgresRepository) SetProjectPermission(
	ctx context.Context,
	organizationID string,
	actor access.Principal,
	principalID string,
	projectID string,
	role string,
	now time.Time,
) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin project permission update: %w", err)
	}
	defer rollback(tx)
	if err := lockUserAdministration(ctx, tx, organizationID); err != nil {
		return User{}, err
	}
	targetRole, targetStatus, err := authorizeTarget(ctx, tx, organizationID, actor.ID, principalID)
	if err != nil {
		return User{}, err
	}
	if targetRole != "member" {
		return User{}, ErrGlobalProjectRole
	}
	if targetStatus != "active" {
		return User{}, ErrInactiveUser
	}
	var projectName string
	if err := tx.QueryRow(ctx, `
		SELECT name FROM identity.projects
		WHERE organization_id = $1 AND id = $2 AND status <> 'archived'
	`, organizationID, projectID).Scan(&projectName); errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	} else if err != nil {
		return User{}, fmt.Errorf("load permission project: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.project_memberships (
			organization_id, project_id, principal_id, role, updated_at
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (organization_id, project_id, principal_id) DO UPDATE
		SET role = EXCLUDED.role, updated_at = EXCLUDED.updated_at
	`, organizationID, projectID, principalID, role, now); err != nil {
		return User{}, fmt.Errorf("set user project permission: %w", err)
	}
	if err := insertEvent(ctx, tx, organizationID, actor.ID, principalID, "project_permission.set", map[string]any{
		"project_id": projectID, "project_name": projectName, "role": role,
	}, now); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit project permission update: %w", err)
	}
	return r.GetUser(ctx, organizationID, principalID, now)
}

func (r *PostgresRepository) RemoveProjectPermission(
	ctx context.Context,
	organizationID string,
	actor access.Principal,
	principalID string,
	projectID string,
	now time.Time,
) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin project permission removal: %w", err)
	}
	defer rollback(tx)
	if err := lockUserAdministration(ctx, tx, organizationID); err != nil {
		return User{}, err
	}
	targetRole, _, err := authorizeTarget(ctx, tx, organizationID, actor.ID, principalID)
	if err != nil {
		return User{}, err
	}
	if targetRole != "member" {
		return User{}, ErrGlobalProjectRole
	}
	command, err := tx.Exec(ctx, `
		DELETE FROM identity.project_memberships
		WHERE organization_id = $1 AND project_id = $2 AND principal_id = $3
	`, organizationID, projectID, principalID)
	if err != nil {
		return User{}, fmt.Errorf("remove user project permission: %w", err)
	}
	if command.RowsAffected() == 0 {
		return User{}, ErrNotFound
	}
	if err := insertEvent(ctx, tx, organizationID, actor.ID, principalID, "project_permission.removed", map[string]any{
		"project_id": projectID,
	}, now); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit project permission removal: %w", err)
	}
	return r.GetUser(ctx, organizationID, principalID, now)
}

func (r *PostgresRepository) RevokeUserSessions(
	ctx context.Context,
	organizationID string,
	actor access.Principal,
	principalID string,
	now time.Time,
) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin user session revocation: %w", err)
	}
	defer rollback(tx)
	if err := lockUserAdministration(ctx, tx, organizationID); err != nil {
		return User{}, err
	}
	if _, _, err := authorizeTarget(ctx, tx, organizationID, actor.ID, principalID); err != nil {
		return User{}, err
	}
	if actor.ID == principalID {
		return User{}, ErrSelfManagement
	}
	if err := revokeUserAccess(ctx, tx, organizationID, principalID, now); err != nil {
		return User{}, err
	}
	if err := insertEvent(ctx, tx, organizationID, actor.ID, principalID, "user.sessions_revoked", map[string]any{}, now); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit user session revocation: %w", err)
	}
	return r.GetUser(ctx, organizationID, principalID, now)
}

func (r *PostgresRepository) CreateInvitation(
	ctx context.Context,
	organizationID string,
	actor access.Principal,
	input CreateInvitationInput,
	digest [sha256.Size]byte,
	now time.Time,
) (Invitation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, fmt.Errorf("begin user invitation: %w", err)
	}
	defer rollback(tx)
	if err := lockUserAdministration(ctx, tx, organizationID); err != nil {
		return Invitation{}, err
	}
	actorRole, err := authorizeAdministrator(ctx, tx, organizationID, actor.ID)
	if err != nil {
		return Invitation{}, err
	}
	if actorRole != "owner" && input.OrganizationRole != "member" {
		return Invitation{}, ErrForbidden
	}
	var accountExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM identity.local_accounts
			WHERE organization_id = $1 AND lower(email) = lower($2)
		)
	`, organizationID, input.Email).Scan(&accountExists); err != nil {
		return Invitation{}, fmt.Errorf("check invitation account: %w", err)
	}
	if accountExists {
		return Invitation{}, ErrAccountExists
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.invitations
		SET status = 'expired'
		WHERE organization_id = $1 AND lower(email) = lower($2)
		  AND status = 'pending' AND expires_at <= $3
	`, organizationID, input.Email, now); err != nil {
		return Invitation{}, fmt.Errorf("expire previous user invitation: %w", err)
	}
	if input.ProjectID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM identity.projects
				WHERE organization_id = $1 AND id = $2 AND status <> 'archived'
			)
		`, organizationID, input.ProjectID).Scan(&exists); err != nil {
			return Invitation{}, fmt.Errorf("check invitation project: %w", err)
		}
		if !exists {
			return Invitation{}, ErrNotFound
		}
	}
	var invitation Invitation
	err = tx.QueryRow(ctx, `
		INSERT INTO identity.invitations (
			organization_id, project_id, email, role, organization_role,
			token_digest, expires_at, created_by_principal_id, created_at
		) VALUES (
			$1, NULLIF($2, '')::uuid, $3, NULLIF($4, ''), $5,
			$6, $7, $8, $9
		)
		RETURNING id, email, organization_role,
		          COALESCE(project_id::text, ''), COALESCE(role, ''),
		          status, expires_at, created_at, created_by_principal_id
	`, organizationID, input.ProjectID, input.Email, input.ProjectRole,
		input.OrganizationRole, digest[:], now.Add(input.ExpiresAfter), actor.ID, now,
	).Scan(
		&invitation.ID,
		&invitation.Email,
		&invitation.OrganizationRole,
		&invitation.ProjectID,
		&invitation.ProjectRole,
		&invitation.Status,
		&invitation.ExpiresAt,
		&invitation.CreatedAt,
		&invitation.CreatedByPrincipalID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "invitations_pending_email_uq" {
			return Invitation{}, ErrInvitationExists
		}
		return Invitation{}, fmt.Errorf("create user invitation: %w", err)
	}
	if input.ProjectID != "" {
		if err := tx.QueryRow(ctx, `
			SELECT name FROM identity.projects WHERE organization_id = $1 AND id = $2
		`, organizationID, input.ProjectID).Scan(&invitation.ProjectName); err != nil {
			return Invitation{}, fmt.Errorf("load invitation project name: %w", err)
		}
	}
	if err := tx.QueryRow(ctx, `
		SELECT display_name FROM identity.actors WHERE organization_id = $1 AND id = $2
	`, organizationID, actor.ID).Scan(&invitation.CreatedByDisplayName); err != nil {
		return Invitation{}, fmt.Errorf("load invitation creator: %w", err)
	}
	if err := insertEvent(ctx, tx, organizationID, actor.ID, "", "invitation.created", map[string]any{
		"invitation_id":     invitation.ID,
		"email":             input.Email,
		"organization_role": input.OrganizationRole,
		"project_id":        input.ProjectID,
		"project_role":      input.ProjectRole,
	}, now); err != nil {
		return Invitation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, fmt.Errorf("commit user invitation: %w", err)
	}
	return invitation, nil
}

func (r *PostgresRepository) RevokeInvitation(
	ctx context.Context,
	organizationID string,
	actor access.Principal,
	invitationID string,
	now time.Time,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin invitation revocation: %w", err)
	}
	defer rollback(tx)
	if err := lockUserAdministration(ctx, tx, organizationID); err != nil {
		return err
	}
	actorRole, err := authorizeAdministrator(ctx, tx, organizationID, actor.ID)
	if err != nil {
		return err
	}
	var invitationRole string
	var email string
	if err := tx.QueryRow(ctx, `
		SELECT organization_role, email
		FROM identity.invitations
		WHERE organization_id = $1 AND id = $2 AND status = 'pending' AND expires_at > $3
		FOR UPDATE
	`, organizationID, invitationID, now).Scan(&invitationRole, &email); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("load invitation for revocation: %w", err)
	}
	if actorRole != "owner" && invitationRole != "member" {
		return ErrForbidden
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.invitations
		SET status = 'revoked', revoked_at = $3
		WHERE organization_id = $1 AND id = $2
	`, organizationID, invitationID, now); err != nil {
		return fmt.Errorf("revoke user invitation: %w", err)
	}
	if err := insertEvent(ctx, tx, organizationID, actor.ID, "", "invitation.revoked", map[string]any{
		"invitation_id": invitationID, "email": email,
	}, now); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit invitation revocation: %w", err)
	}
	return nil
}

func authorizeAdministrator(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	actorID string,
) (string, error) {
	var role string
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT membership.role, account.status
		FROM identity.organization_memberships AS membership
		JOIN identity.local_accounts AS account
		  ON account.organization_id = membership.organization_id
		 AND account.principal_id = membership.principal_id
		WHERE membership.organization_id = $1 AND membership.principal_id = $2
	`, organizationID, actorID).Scan(&role, &status); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	} else if err != nil {
		return "", fmt.Errorf("authorize user administrator: %w", err)
	}
	if status != "active" || (role != "owner" && role != "admin") {
		return "", ErrForbidden
	}
	return role, nil
}

func authorizeTarget(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	actorID string,
	targetID string,
) (string, string, error) {
	actorRole, err := authorizeAdministrator(ctx, tx, organizationID, actorID)
	if err != nil {
		return "", "", err
	}
	var targetRole string
	var targetStatus string
	if err := tx.QueryRow(ctx, `
		SELECT membership.role, account.status
		FROM identity.organization_memberships AS membership
		JOIN identity.local_accounts AS account
		  ON account.organization_id = membership.organization_id
		 AND account.principal_id = membership.principal_id
		WHERE membership.organization_id = $1 AND membership.principal_id = $2
		FOR UPDATE OF membership, account
	`, organizationID, targetID).Scan(&targetRole, &targetStatus); errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	} else if err != nil {
		return "", "", fmt.Errorf("load administration target: %w", err)
	}
	if actorRole != "owner" && targetRole != "member" {
		return "", "", ErrForbidden
	}
	return targetRole, targetStatus, nil
}

func lockUserAdministration(ctx context.Context, tx pgx.Tx, organizationID string) error {
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
	`, "pact-user-administration:"+organizationID); err != nil {
		return fmt.Errorf("lock user administration: %w", err)
	}
	return nil
}

func revokeUserAccess(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	principalID string,
	now time.Time,
) error {
	if _, err := tx.Exec(ctx, `
		UPDATE identity.web_sessions
		SET status = 'revoked', revoked_at = $3
		WHERE organization_id = $1 AND principal_id = $2 AND status = 'active'
	`, organizationID, principalID, now); err != nil {
		return fmt.Errorf("revoke user web sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.device_credentials
		SET status = 'revoked', revoked_at = $3
		WHERE organization_id = $1 AND principal_id = $2 AND status = 'active'
	`, organizationID, principalID, now); err != nil {
		return fmt.Errorf("revoke user devices: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.sessions AS session
		SET status = 'closed', ended_at = $3, last_seen_at = LEAST(session.expires_at, GREATEST(session.started_at, $3))
		WHERE session.organization_id = $1
		  AND session.actor_id IN (
		      SELECT agent.id
		      FROM identity.agents AS agent
		      WHERE agent.organization_id = $1 AND agent.sponsor_principal_id = $2
		  )
		  AND session.status IN ('starting', 'active', 'stale')
	`, organizationID, principalID, now); err != nil {
		return fmt.Errorf("close sponsored agent sessions: %w", err)
	}
	return nil
}

func insertEvent(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	actorID string,
	targetID string,
	action string,
	details map[string]any,
	now time.Time,
) error {
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode user administration event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.user_admin_events (
			organization_id, actor_principal_id, target_principal_id,
			action, details, created_at
		) VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6)
	`, organizationID, actorID, targetID, action, encoded, now); err != nil {
		return fmt.Errorf("record user administration event: %w", err)
	}
	return nil
}

func updateDetails(input UpdateUserInput) map[string]any {
	details := make(map[string]any)
	if input.DisplayName != nil {
		details["display_name"] = *input.DisplayName
	}
	if input.Email != nil {
		details["email"] = *input.Email
	}
	if input.Username != nil {
		details["username"] = *input.Username
	}
	if input.Status != nil {
		details["status"] = *input.Status
	}
	if input.OrganizationRole != nil {
		details["organization_role"] = *input.OrganizationRole
	}
	return details
}

func accountWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.ConstraintName == "local_accounts_email_uq" || pgErr.ConstraintName == "local_accounts_username_uq") {
		return ErrAccountExists
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func rollback(tx pgx.Tx) {
	rollbackContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackContext)
}
