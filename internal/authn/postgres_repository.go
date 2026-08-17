package authn

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
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

func (r *PostgresRepository) SetupComplete(ctx context.Context, organizationID string) (bool, error) {
	var complete bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM identity.local_accounts AS account
			JOIN identity.organization_memberships AS membership
			  ON membership.organization_id = account.organization_id
			 AND membership.principal_id = account.principal_id
			JOIN identity.actors AS actor
			  ON actor.organization_id = account.organization_id
			 AND actor.id = account.principal_id
			WHERE account.organization_id = $1
			  AND account.status = 'active'
			  AND actor.status = 'active'
			  AND membership.role IN ('owner', 'admin')
		)
	`, organizationID).Scan(&complete)
	if err != nil {
		return false, fmt.Errorf("read initial setup state: %w", err)
	}
	return complete, nil
}

func (r *PostgresRepository) CreateOwner(
	ctx context.Context,
	organizationID string,
	input AccountInput,
	sessionDigest [sha256.Size]byte,
	csrfDigest [sha256.Size]byte,
	expiresAt time.Time,
	metadata SessionMetadata,
) (WebSession, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return WebSession{}, fmt.Errorf("begin owner setup: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "pact-setup:"+organizationID); err != nil {
		return WebSession{}, fmt.Errorf("lock owner setup: %w", err)
	}
	var complete bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM identity.local_accounts AS account
			JOIN identity.organization_memberships AS membership
			  ON membership.organization_id = account.organization_id
			 AND membership.principal_id = account.principal_id
			WHERE account.organization_id = $1
			  AND account.status = 'active'
			  AND membership.role IN ('owner', 'admin')
		)
	`, organizationID).Scan(&complete); err != nil {
		return WebSession{}, fmt.Errorf("verify owner setup: %w", err)
	}
	if complete {
		return WebSession{}, ErrAlreadyConfigured
	}

	principal, err := createLocalPrincipal(ctx, tx, organizationID, input)
	if err != nil {
		return WebSession{}, err
	}
	principal.OrganizationRole = "owner"
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.organization_memberships (organization_id, principal_id, role)
		VALUES ($1, $2, 'owner')
	`, organizationID, principal.ID); err != nil {
		return WebSession{}, fmt.Errorf("create owner membership: %w", err)
	}

	session, err := insertWebSession(ctx, tx, organizationID, principal, sessionDigest, csrfDigest, expiresAt, metadata)
	if err != nil {
		return WebSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WebSession{}, fmt.Errorf("commit owner setup: %w", err)
	}
	return session, nil
}

func (r *PostgresRepository) FindAccountByLogin(ctx context.Context, organizationID, login string) (Account, error) {
	return scanAccount(r.pool.QueryRow(ctx, `
		SELECT principal.id, principal.organization_id, actor.display_name,
		       principal.principal_type, membership.role,
		       account.email, account.username, account.password_hash,
		       account.status, account.failed_login_attempts, account.locked_until
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
		  AND (account.email = $2 OR account.username = $2)
		  AND actor.status = 'active'
	`, organizationID, login), "find local account")
}

func (r *PostgresRepository) FindAccountByPrincipal(ctx context.Context, organizationID, principalID string) (Account, error) {
	return scanAccount(r.pool.QueryRow(ctx, `
		SELECT principal.id, principal.organization_id, actor.display_name,
		       principal.principal_type, membership.role,
		       account.email, account.username, account.password_hash,
		       account.status, account.failed_login_attempts, account.locked_until
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
		  AND account.principal_id = $2
		  AND actor.status = 'active'
	`, organizationID, principalID), "find principal account")
}

func (r *PostgresRepository) RecordFailedLogin(
	ctx context.Context,
	organizationID string,
	principalID string,
	attempts int,
	lockedUntil *time.Time,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE identity.local_accounts
		SET failed_login_attempts = $3,
		    locked_until = $4,
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND principal_id = $2
	`, organizationID, principalID, attempts, lockedUntil)
	if err != nil {
		return fmt.Errorf("record failed login: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateWebSession(
	ctx context.Context,
	organizationID string,
	principalID string,
	sessionDigest [sha256.Size]byte,
	csrfDigest [sha256.Size]byte,
	expiresAt time.Time,
	metadata SessionMetadata,
) (WebSession, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return WebSession{}, fmt.Errorf("begin web session: %w", err)
	}
	defer rollback(tx)
	account, err := scanAccount(tx.QueryRow(ctx, `
		SELECT principal.id, principal.organization_id, actor.display_name,
		       principal.principal_type, membership.role,
		       account.email, account.username, account.password_hash,
		       account.status, account.failed_login_attempts, account.locked_until
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
		  AND account.principal_id = $2
		  AND account.status = 'active'
		  AND actor.status = 'active'
		FOR UPDATE OF account
	`, organizationID, principalID), "load account for session")
	if err != nil {
		return WebSession{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.local_accounts
		SET failed_login_attempts = 0, locked_until = NULL,
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND principal_id = $2
	`, organizationID, principalID); err != nil {
		return WebSession{}, fmt.Errorf("reset login failures: %w", err)
	}
	session, err := insertWebSession(ctx, tx, organizationID, account.Principal, sessionDigest, csrfDigest, expiresAt, metadata)
	if err != nil {
		return WebSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WebSession{}, fmt.Errorf("commit web session: %w", err)
	}
	return session, nil
}

func (r *PostgresRepository) AuthenticateWebSession(
	ctx context.Context,
	organizationID string,
	digest [sha256.Size]byte,
) (WebSession, error) {
	var session WebSession
	var csrf []byte
	err := r.pool.QueryRow(ctx, `
		UPDATE identity.web_sessions AS session
		SET last_used_at = transaction_timestamp()
		FROM identity.local_accounts AS account,
		     identity.principals AS principal,
		     identity.actors AS actor,
		     identity.organization_memberships AS membership
		WHERE session.organization_id = $1
		  AND session.session_digest = $2
		  AND session.status = 'active'
		  AND session.expires_at > transaction_timestamp()
		  AND account.organization_id = session.organization_id
		  AND account.principal_id = session.principal_id
		  AND account.status = 'active'
		  AND principal.organization_id = account.organization_id
		  AND principal.id = account.principal_id
		  AND actor.organization_id = principal.organization_id
		  AND actor.id = principal.id
		  AND actor.status = 'active'
		  AND membership.organization_id = principal.organization_id
		  AND membership.principal_id = principal.id
		RETURNING session.id, session.expires_at, session.csrf_digest,
		          principal.id, principal.organization_id, actor.display_name,
		          principal.principal_type, membership.role
	`, organizationID, digest[:]).Scan(
		&session.ID, &session.ExpiresAt, &csrf,
		&session.Principal.ID, &session.Principal.OrganizationID,
		&session.Principal.DisplayName, &session.Principal.PrincipalType,
		&session.Principal.OrganizationRole,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebSession{}, ErrUnauthorized
	}
	if err != nil {
		return WebSession{}, fmt.Errorf("authenticate web session: %w", err)
	}
	if len(csrf) != sha256.Size {
		return WebSession{}, fmt.Errorf("authenticate web session: invalid csrf digest")
	}
	copy(session.CSRFDigest[:], csrf)
	return session, nil
}

func (r *PostgresRepository) RevokeWebSession(ctx context.Context, organizationID, principalID, sessionID string) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE identity.web_sessions
		SET status = 'revoked', revoked_at = transaction_timestamp()
		WHERE organization_id = $1 AND principal_id = $2 AND id = $3 AND status = 'active'
	`, organizationID, principalID, sessionID)
	if err != nil {
		return fmt.Errorf("revoke web session: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ChangePassword(ctx context.Context, organizationID, principalID, currentSessionID, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password change: %w", err)
	}
	defer rollback(tx)
	command, err := tx.Exec(ctx, `
		UPDATE identity.local_accounts
		SET password_hash = $3, password_changed_at = transaction_timestamp(),
		    failed_login_attempts = 0, locked_until = NULL,
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND principal_id = $2 AND status = 'active'
	`, organizationID, principalID, passwordHash)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.web_sessions
		SET status = 'revoked', revoked_at = transaction_timestamp()
		WHERE organization_id = $1 AND principal_id = $2
		  AND id <> $3 AND status = 'active'
	`, organizationID, principalID, currentSessionID); err != nil {
		return fmt.Errorf("revoke other web sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.device_credentials
		SET status = 'revoked', revoked_at = transaction_timestamp()
		WHERE organization_id = $1 AND principal_id = $2 AND status = 'active'
	`, organizationID, principalID); err != nil {
		return fmt.Errorf("revoke devices after password change: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password change: %w", err)
	}
	return nil
}

func (r *PostgresRepository) PreviewInvitation(
	ctx context.Context,
	organizationID string,
	digest [sha256.Size]byte,
) (InvitationPreview, error) {
	var preview InvitationPreview
	err := r.pool.QueryRow(ctx, `
		SELECT email, organization_role,
		       COALESCE(role, ''), COALESCE(project_id::text, ''), expires_at
		FROM identity.invitations
		WHERE organization_id = $1 AND token_digest = $2
		  AND status = 'pending' AND expires_at > transaction_timestamp()
	`, organizationID, digest[:]).Scan(
		&preview.Email, &preview.OrganizationRole, &preview.Role,
		&preview.ProjectID, &preview.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return InvitationPreview{}, ErrInvitationInvalid
	}
	if err != nil {
		return InvitationPreview{}, fmt.Errorf("preview invitation: %w", err)
	}
	return preview, nil
}

func (r *PostgresRepository) RegisterInvitation(
	ctx context.Context,
	organizationID string,
	input InvitationRegistrationInput,
	inviteDigest [sha256.Size]byte,
	sessionDigest [sha256.Size]byte,
	csrfDigest [sha256.Size]byte,
	sessionExpiresAt time.Time,
	metadata SessionMetadata,
) (InvitationAcceptance, WebSession, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return InvitationAcceptance{}, WebSession{}, fmt.Errorf("begin invitation registration: %w", err)
	}
	defer rollback(tx)

	invitation, err := lockInvitation(ctx, tx, organizationID, inviteDigest)
	if err != nil {
		return InvitationAcceptance{}, WebSession{}, err
	}
	if !strings.EqualFold(invitation.Email, input.Email) {
		return InvitationAcceptance{}, WebSession{}, ErrInvitationMismatch
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "pact-account:"+organizationID+":"+input.Email); err != nil {
		return InvitationAcceptance{}, WebSession{}, fmt.Errorf("lock invited account: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM identity.local_accounts
			WHERE organization_id = $1 AND (email = $2 OR username = $3)
		)
	`, organizationID, input.Email, input.Username).Scan(&exists); err != nil {
		return InvitationAcceptance{}, WebSession{}, fmt.Errorf("check invited account: %w", err)
	}
	if exists {
		return InvitationAcceptance{}, WebSession{}, ErrAccountExists
	}

	principal, err := findOrCreateInvitedPrincipal(ctx, tx, organizationID, input.AccountInput)
	if err != nil {
		return InvitationAcceptance{}, WebSession{}, err
	}
	principal.OrganizationRole = invitation.OrganizationRole
	if err := upsertMemberships(ctx, tx, organizationID, principal.ID, invitation.ProjectID, invitation.OrganizationRole, invitation.Role); err != nil {
		return InvitationAcceptance{}, WebSession{}, err
	}
	if err := consumeInvitation(ctx, tx, organizationID, invitation.ID, principal.ID); err != nil {
		return InvitationAcceptance{}, WebSession{}, err
	}
	session, err := insertWebSession(ctx, tx, organizationID, principal, sessionDigest, csrfDigest, sessionExpiresAt, metadata)
	if err != nil {
		return InvitationAcceptance{}, WebSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return InvitationAcceptance{}, WebSession{}, fmt.Errorf("commit invitation registration: %w", err)
	}
	return InvitationAcceptance{
		Principal: principal, ProjectID: invitation.ProjectID,
		OrganizationRole: invitation.OrganizationRole,
		ProjectRole:      invitation.Role, ExpiresAt: invitation.ExpiresAt,
	}, session, nil
}

func (r *PostgresRepository) AcceptInvitation(
	ctx context.Context,
	organizationID string,
	principal access.Principal,
	inviteDigest [sha256.Size]byte,
) (InvitationAcceptance, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return InvitationAcceptance{}, fmt.Errorf("begin invitation acceptance: %w", err)
	}
	defer rollback(tx)

	var email string
	if err := tx.QueryRow(ctx, `
		SELECT email FROM identity.local_accounts
		WHERE organization_id = $1 AND principal_id = $2 AND status = 'active'
	`, organizationID, principal.ID).Scan(&email); errors.Is(err, pgx.ErrNoRows) {
		return InvitationAcceptance{}, ErrInvitationMismatch
	} else if err != nil {
		return InvitationAcceptance{}, fmt.Errorf("load account for invitation: %w", err)
	}
	invitation, err := lockInvitation(ctx, tx, organizationID, inviteDigest)
	if err != nil {
		return InvitationAcceptance{}, err
	}
	if !strings.EqualFold(invitation.Email, email) {
		return InvitationAcceptance{}, ErrInvitationMismatch
	}
	if err := upsertMemberships(ctx, tx, organizationID, principal.ID, invitation.ProjectID, invitation.OrganizationRole, invitation.Role); err != nil {
		return InvitationAcceptance{}, err
	}
	if err := consumeInvitation(ctx, tx, organizationID, invitation.ID, principal.ID); err != nil {
		return InvitationAcceptance{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return InvitationAcceptance{}, fmt.Errorf("commit invitation acceptance: %w", err)
	}
	principal.OrganizationRole = strongerOrganizationRole(principal.OrganizationRole, invitation.OrganizationRole)
	return InvitationAcceptance{
		Principal: principal, ProjectID: invitation.ProjectID,
		OrganizationRole: invitation.OrganizationRole,
		ProjectRole:      invitation.Role, ExpiresAt: invitation.ExpiresAt,
	}, nil
}

func (r *PostgresRepository) CreateDeviceAuthorization(
	ctx context.Context,
	organizationID string,
	deviceName string,
	deviceDigest [sha256.Size]byte,
	userDigest [sha256.Size]byte,
	userCode string,
	expiresAt time.Time,
) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO identity.device_authorizations (
			organization_id, device_name, device_code_digest,
			user_code_digest, user_code_hint, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, organizationID, deviceName, deviceDigest[:], userDigest[:], userCode, expiresAt)
	if err != nil {
		return fmt.Errorf("create device authorization: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ApproveDeviceAuthorization(
	ctx context.Context,
	organizationID string,
	principal access.Principal,
	userDigest [sha256.Size]byte,
) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE identity.device_authorizations
		SET status = 'approved', principal_id = $3,
		    approved_at = transaction_timestamp()
		WHERE organization_id = $1 AND user_code_digest = $2
		  AND status = 'pending' AND expires_at > transaction_timestamp()
	`, organizationID, userDigest[:], principal.ID)
	if err != nil {
		return fmt.Errorf("approve device authorization: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrDeviceCodeInvalid
	}
	return nil
}

func (r *PostgresRepository) ExchangeDeviceAuthorization(
	ctx context.Context,
	organizationID string,
	deviceDigest [sha256.Size]byte,
	credentialDigest [sha256.Size]byte,
	credentialExpiresAt time.Time,
) (deviceExchangeRecord, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return deviceExchangeRecord{}, fmt.Errorf("begin device exchange: %w", err)
	}
	defer rollback(tx)

	var authorizationID, deviceName, status string
	var principalID *string
	var authorizationExpiresAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT id, device_name, status, principal_id, expires_at
		FROM identity.device_authorizations
		WHERE organization_id = $1 AND device_code_digest = $2
		FOR UPDATE
	`, organizationID, deviceDigest[:]).Scan(
		&authorizationID, &deviceName, &status, &principalID, &authorizationExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return deviceExchangeRecord{}, ErrDeviceCodeInvalid
	}
	if err != nil {
		return deviceExchangeRecord{}, fmt.Errorf("load device authorization: %w", err)
	}
	if !authorizationExpiresAt.After(time.Now().UTC()) {
		if status == "pending" || status == "approved" {
			if _, err := tx.Exec(ctx, `
				UPDATE identity.device_authorizations SET status = 'expired'
				WHERE organization_id = $1 AND id = $2
			`, organizationID, authorizationID); err != nil {
				return deviceExchangeRecord{}, fmt.Errorf("expire device authorization: %w", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return deviceExchangeRecord{}, fmt.Errorf("commit device expiry: %w", err)
		}
		return deviceExchangeRecord{}, ErrDeviceCodeInvalid
	}
	switch status {
	case "pending":
		return deviceExchangeRecord{Status: "pending"}, nil
	case "denied":
		return deviceExchangeRecord{}, ErrAuthorizationDenied
	case "approved":
		if principalID == nil {
			return deviceExchangeRecord{}, ErrDeviceCodeInvalid
		}
	case "consumed", "expired":
		return deviceExchangeRecord{}, ErrDeviceCodeInvalid
	default:
		return deviceExchangeRecord{}, ErrDeviceCodeInvalid
	}

	account, err := scanAccount(tx.QueryRow(ctx, `
		SELECT principal.id, principal.organization_id, actor.display_name,
		       principal.principal_type, membership.role,
		       account.email, account.username, account.password_hash,
		       account.status, account.failed_login_attempts, account.locked_until
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
		WHERE account.organization_id = $1 AND account.principal_id = $2
		  AND account.status = 'active' AND actor.status = 'active'
	`, organizationID, *principalID), "load device principal")
	if err != nil {
		return deviceExchangeRecord{}, err
	}
	var credentialID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO identity.device_credentials (
			organization_id, principal_id, name, credential_digest, expires_at
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, organizationID, *principalID, deviceName, credentialDigest[:], credentialExpiresAt).Scan(&credentialID); err != nil {
		return deviceExchangeRecord{}, fmt.Errorf("create device credential: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.device_authorizations
		SET status = 'consumed', consumed_at = transaction_timestamp()
		WHERE organization_id = $1 AND id = $2 AND status = 'approved'
	`, organizationID, authorizationID); err != nil {
		return deviceExchangeRecord{}, fmt.Errorf("consume device authorization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return deviceExchangeRecord{}, fmt.Errorf("commit device exchange: %w", err)
	}
	return deviceExchangeRecord{
		Status: "authorized", CredentialID: credentialID,
		Principal: account.Principal, ExpiresAt: credentialExpiresAt,
	}, nil
}

func (r *PostgresRepository) AuthenticateDevice(
	ctx context.Context,
	organizationID string,
	digest [sha256.Size]byte,
) (DevicePrincipal, error) {
	var device DevicePrincipal
	err := r.pool.QueryRow(ctx, `
		UPDATE identity.device_credentials AS credential
		SET last_used_at = transaction_timestamp()
		FROM identity.local_accounts AS account,
		     identity.principals AS principal,
		     identity.actors AS actor,
		     identity.organization_memberships AS membership
		WHERE credential.organization_id = $1
		  AND credential.credential_digest = $2
		  AND credential.status = 'active'
		  AND credential.expires_at > transaction_timestamp()
		  AND account.organization_id = credential.organization_id
		  AND account.principal_id = credential.principal_id
		  AND account.status = 'active'
		  AND principal.organization_id = account.organization_id
		  AND principal.id = account.principal_id
		  AND actor.organization_id = principal.organization_id
		  AND actor.id = principal.id
		  AND actor.status = 'active'
		  AND membership.organization_id = principal.organization_id
		  AND membership.principal_id = principal.id
		RETURNING credential.id, principal.id, principal.organization_id,
		          actor.display_name, principal.principal_type, membership.role
	`, organizationID, digest[:]).Scan(
		&device.CredentialID, &device.Principal.ID, &device.Principal.OrganizationID,
		&device.Principal.DisplayName, &device.Principal.PrincipalType,
		&device.Principal.OrganizationRole,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DevicePrincipal{}, ErrUnauthorized
	}
	if err != nil {
		return DevicePrincipal{}, fmt.Errorf("authenticate device: %w", err)
	}
	return device, nil
}

func (r *PostgresRepository) RevokeDevice(ctx context.Context, organizationID, principalID, credentialID string) error {
	return r.revokeDevice(ctx, organizationID, principalID, credentialID)
}

func (r *PostgresRepository) ListDevices(ctx context.Context, organizationID, principalID string) ([]Device, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, status, expires_at, last_used_at, created_at
		FROM identity.device_credentials
		WHERE organization_id = $1 AND principal_id = $2
		ORDER BY created_at DESC
	`, organizationID, principalID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	devices := make([]Device, 0)
	for rows.Next() {
		var device Device
		if err := rows.Scan(&device.ID, &device.Name, &device.Status, &device.ExpiresAt, &device.LastUsedAt, &device.CreatedAt); err != nil {
			return nil, fmt.Errorf("read device: %w", err)
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return devices, nil
}

func (r *PostgresRepository) RevokeDeviceByID(ctx context.Context, organizationID, principalID, credentialID string) error {
	return r.revokeDevice(ctx, organizationID, principalID, credentialID)
}

func (r *PostgresRepository) revokeDevice(ctx context.Context, organizationID, principalID, credentialID string) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE identity.device_credentials
		SET status = 'revoked', revoked_at = transaction_timestamp()
		WHERE organization_id = $1 AND principal_id = $2 AND id = $3 AND status = 'active'
	`, organizationID, principalID, credentialID)
	if err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type invitationRecord struct {
	ID               string
	ProjectID        string
	Email            string
	Role             string
	OrganizationRole string
	ExpiresAt        time.Time
}

func lockInvitation(ctx context.Context, tx pgx.Tx, organizationID string, digest [sha256.Size]byte) (invitationRecord, error) {
	var invitation invitationRecord
	err := tx.QueryRow(ctx, `
		SELECT id, COALESCE(project_id::text, ''), email,
		       COALESCE(role, ''), organization_role, expires_at
		FROM identity.invitations
		WHERE organization_id = $1 AND token_digest = $2
		  AND status = 'pending' AND expires_at > transaction_timestamp()
		FOR UPDATE
	`, organizationID, digest[:]).Scan(
		&invitation.ID, &invitation.ProjectID, &invitation.Email,
		&invitation.Role, &invitation.OrganizationRole, &invitation.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return invitationRecord{}, ErrInvitationInvalid
	}
	if err != nil {
		return invitationRecord{}, fmt.Errorf("load invitation: %w", err)
	}
	return invitation, nil
}

func consumeInvitation(ctx context.Context, tx pgx.Tx, organizationID, invitationID, principalID string) error {
	command, err := tx.Exec(ctx, `
		UPDATE identity.invitations
		SET status = 'accepted', accepted_by_principal_id = $3,
		    accepted_at = transaction_timestamp()
		WHERE organization_id = $1 AND id = $2 AND status = 'pending'
	`, organizationID, invitationID, principalID)
	if err != nil {
		return fmt.Errorf("consume invitation: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrInvitationInvalid
	}
	return nil
}

func createLocalPrincipal(ctx context.Context, tx pgx.Tx, organizationID string, input AccountInput) (access.Principal, error) {
	principal := access.Principal{
		OrganizationID: organizationID, DisplayName: input.DisplayName,
		PrincipalType: "human",
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO identity.actors (organization_id, kind, display_name, attributes)
		VALUES ($1, 'principal', $2, jsonb_build_object('email', $3::text, 'username', $4::text))
		RETURNING id
	`, organizationID, input.DisplayName, input.Email, input.Username).Scan(&principal.ID); err != nil {
		return access.Principal{}, fmt.Errorf("create local actor: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.principals (
			id, organization_id, principal_type, external_issuer, external_subject
		) VALUES ($1, $2, 'human', 'pact.local', $3)
	`, principal.ID, organizationID, input.Email); err != nil {
		return access.Principal{}, accountWriteError("create local principal", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.local_accounts (
			organization_id, principal_id, email, username, password_hash
		) VALUES ($1, $2, $3, $4, $5)
	`, organizationID, principal.ID, input.Email, input.Username, input.Password); err != nil {
		return access.Principal{}, accountWriteError("create local account", err)
	}
	return principal, nil
}

func findOrCreateInvitedPrincipal(ctx context.Context, tx pgx.Tx, organizationID string, input AccountInput) (access.Principal, error) {
	var principal access.Principal
	principal.OrganizationID = organizationID
	principal.DisplayName = input.DisplayName
	principal.PrincipalType = "human"
	err := tx.QueryRow(ctx, `
		SELECT principal.id
		FROM identity.principals AS principal
		JOIN identity.actors AS actor
		  ON actor.organization_id = principal.organization_id AND actor.id = principal.id
		WHERE principal.organization_id = $1
		  AND principal.external_issuer = 'pact.invitation'
		  AND lower(principal.external_subject) = lower($2)
		  AND actor.status = 'active'
		FOR UPDATE OF principal
	`, organizationID, input.Email).Scan(&principal.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return createLocalPrincipal(ctx, tx, organizationID, input)
	}
	if err != nil {
		return access.Principal{}, fmt.Errorf("find invited principal: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.actors
		SET display_name = $3,
		    attributes = attributes || jsonb_build_object('email', $4::text, 'username', $5::text),
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, principal.ID, input.DisplayName, input.Email, input.Username); err != nil {
		return access.Principal{}, fmt.Errorf("update invited actor: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.principals
		SET external_issuer = 'pact.local', external_subject = $3,
		    updated_at = transaction_timestamp()
		WHERE organization_id = $1 AND id = $2
	`, organizationID, principal.ID, input.Email); err != nil {
		return access.Principal{}, accountWriteError("convert invited principal", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.local_accounts (
			organization_id, principal_id, email, username, password_hash
		) VALUES ($1, $2, $3, $4, $5)
	`, organizationID, principal.ID, input.Email, input.Username, input.Password); err != nil {
		return access.Principal{}, accountWriteError("create invited account", err)
	}
	return principal, nil
}

func upsertMemberships(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	principalID string,
	projectID string,
	organizationRole string,
	projectRole string,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.organization_memberships (organization_id, principal_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id, principal_id) DO UPDATE
		SET role = CASE
			WHEN identity.organization_memberships.role = 'owner' THEN 'owner'
			WHEN EXCLUDED.role = 'owner' THEN 'owner'
			WHEN identity.organization_memberships.role = 'admin' THEN 'admin'
			WHEN EXCLUDED.role = 'admin' THEN 'admin'
			ELSE 'member'
		END,
		updated_at = transaction_timestamp()
	`, organizationID, principalID, organizationRole); err != nil {
		return fmt.Errorf("upsert organization membership: %w", err)
	}
	if projectID == "" {
		return nil
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
	`, organizationID, projectID, principalID, projectRole); err != nil {
		return fmt.Errorf("upsert project membership: %w", err)
	}
	return nil
}

func insertWebSession(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	principal access.Principal,
	sessionDigest [sha256.Size]byte,
	csrfDigest [sha256.Size]byte,
	expiresAt time.Time,
	metadata SessionMetadata,
) (WebSession, error) {
	session := WebSession{Principal: principal, ExpiresAt: expiresAt, CSRFDigest: csrfDigest}
	if err := tx.QueryRow(ctx, `
		INSERT INTO identity.web_sessions (
			organization_id, principal_id, session_digest, csrf_digest,
			user_agent, remote_address, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, organizationID, principal.ID, sessionDigest[:], csrfDigest[:],
		metadata.UserAgent, metadata.RemoteAddress, expiresAt).Scan(&session.ID); err != nil {
		return WebSession{}, fmt.Errorf("create web session: %w", err)
	}
	return session, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanAccount(row rowScanner, operation string) (Account, error) {
	var account Account
	err := row.Scan(
		&account.Principal.ID, &account.Principal.OrganizationID,
		&account.Principal.DisplayName, &account.Principal.PrincipalType,
		&account.Principal.OrganizationRole, &account.Email, &account.Username,
		&account.PasswordHash, &account.Status, &account.FailedLoginAttempts,
		&account.LockedUntil,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("%s: %w", operation, err)
	}
	return account, nil
}

func strongerOrganizationRole(current, candidate string) string {
	if current == "owner" || candidate == "owner" {
		return "owner"
	}
	if current == "admin" || candidate == "admin" {
		return "admin"
	}
	return "member"
}

func accountWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || strings.Contains(pgErr.ConstraintName, "local_accounts_")) {
		return ErrAccountExists
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func rollback(tx pgx.Tx) {
	rollbackContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(rollbackContext)
}
