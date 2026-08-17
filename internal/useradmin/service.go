package useradmin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jorgenuanzs/the-pact/internal/access"
)

const (
	defaultInvitationLifetime = 24 * time.Hour
	maximumInvitationLifetime = 7 * 24 * time.Hour
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

type Repository interface {
	Directory(context.Context, string, time.Time) (Directory, error)
	GetUser(context.Context, string, string, time.Time) (User, error)
	UpdateUser(context.Context, string, access.Principal, string, UpdateUserInput, time.Time) (User, error)
	SetProjectPermission(context.Context, string, access.Principal, string, string, string, time.Time) (User, error)
	RemoveProjectPermission(context.Context, string, access.Principal, string, string, time.Time) (User, error)
	RevokeUserSessions(context.Context, string, access.Principal, string, time.Time) (User, error)
	CreateInvitation(context.Context, string, access.Principal, CreateInvitationInput, [sha256.Size]byte, time.Time) (Invitation, error)
	RevokeInvitation(context.Context, string, access.Principal, string, time.Time) error
}

type Service struct {
	organizationID string
	repository     Repository
	now            func() time.Time
}

func NewService(organizationID string, repository Repository) *Service {
	return &Service{organizationID: organizationID, repository: repository, now: time.Now}
}

func (s *Service) Directory(ctx context.Context, actor access.Principal) (Directory, error) {
	if err := s.requireAdministrator(actor); err != nil {
		return Directory{}, err
	}
	return s.repository.Directory(ctx, s.organizationID, s.now().UTC())
}

func (s *Service) GetUser(ctx context.Context, actor access.Principal, principalID string) (User, error) {
	if err := s.requireAdministrator(actor); err != nil {
		return User{}, err
	}
	user, err := s.repository.GetUser(ctx, s.organizationID, strings.TrimSpace(principalID), s.now().UTC())
	if err != nil {
		return User{}, err
	}
	if err := canManageTarget(actor, user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) UpdateUser(
	ctx context.Context,
	actor access.Principal,
	principalID string,
	input UpdateUserInput,
) (User, error) {
	if err := s.requireAdministrator(actor); err != nil {
		return User{}, err
	}
	target, err := s.repository.GetUser(ctx, s.organizationID, strings.TrimSpace(principalID), s.now().UTC())
	if err != nil {
		return User{}, err
	}
	if err := canManageTarget(actor, target); err != nil {
		return User{}, err
	}
	normalized, err := normalizeUpdate(input)
	if err != nil {
		return User{}, err
	}
	if normalized.Status == nil && normalized.OrganizationRole == nil && normalized.DisplayName == nil && normalized.Email == nil && normalized.Username == nil {
		return User{}, &ValidationError{Field: "user", Message: "must include at least one editable field"}
	}
	if actor.ID == target.PrincipalID {
		if (normalized.Status != nil && *normalized.Status != "active") ||
			(normalized.OrganizationRole != nil && *normalized.OrganizationRole != target.OrganizationRole) {
			return User{}, ErrSelfManagement
		}
	}
	if normalized.OrganizationRole != nil {
		if actor.OrganizationRole != "owner" && *normalized.OrganizationRole != "member" {
			return User{}, ErrForbidden
		}
	}
	return s.repository.UpdateUser(
		ctx, s.organizationID, actor, target.PrincipalID, normalized, s.now().UTC(),
	)
}

func (s *Service) SetProjectPermission(
	ctx context.Context,
	actor access.Principal,
	principalID string,
	projectID string,
	role string,
) (User, error) {
	target, err := s.manageableProjectTarget(ctx, actor, principalID)
	if err != nil {
		return User{}, err
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if !validProjectRole(role) {
		return User{}, &ValidationError{Field: "role", Message: "must be owner, maintainer, contributor, or viewer"}
	}
	return s.repository.SetProjectPermission(
		ctx, s.organizationID, actor, target.PrincipalID, strings.TrimSpace(projectID), role, s.now().UTC(),
	)
}

func (s *Service) RemoveProjectPermission(
	ctx context.Context,
	actor access.Principal,
	principalID string,
	projectID string,
) (User, error) {
	target, err := s.manageableProjectTarget(ctx, actor, principalID)
	if err != nil {
		return User{}, err
	}
	return s.repository.RemoveProjectPermission(
		ctx, s.organizationID, actor, target.PrincipalID, strings.TrimSpace(projectID), s.now().UTC(),
	)
}

func (s *Service) RevokeUserSessions(
	ctx context.Context,
	actor access.Principal,
	principalID string,
) (User, error) {
	if err := s.requireAdministrator(actor); err != nil {
		return User{}, err
	}
	target, err := s.repository.GetUser(ctx, s.organizationID, strings.TrimSpace(principalID), s.now().UTC())
	if err != nil {
		return User{}, err
	}
	if err := canManageTarget(actor, target); err != nil {
		return User{}, err
	}
	if actor.ID == target.PrincipalID {
		return User{}, ErrSelfManagement
	}
	return s.repository.RevokeUserSessions(
		ctx, s.organizationID, actor, target.PrincipalID, s.now().UTC(),
	)
}

func (s *Service) CreateInvitation(
	ctx context.Context,
	actor access.Principal,
	input CreateInvitationInput,
) (CreatedInvitation, error) {
	if err := s.requireAdministrator(actor); err != nil {
		return CreatedInvitation{}, err
	}
	normalized, err := normalizeInvitation(input)
	if err != nil {
		return CreatedInvitation{}, err
	}
	if actor.OrganizationRole != "owner" && normalized.OrganizationRole != "member" {
		return CreatedInvitation{}, ErrForbidden
	}
	secret, digest, err := newInvitationSecret()
	if err != nil {
		return CreatedInvitation{}, err
	}
	invitation, err := s.repository.CreateInvitation(
		ctx, s.organizationID, actor, normalized, digest, s.now().UTC(),
	)
	if err != nil {
		return CreatedInvitation{}, err
	}
	return CreatedInvitation{Invitation: invitation, Secret: secret}, nil
}

func (s *Service) RevokeInvitation(
	ctx context.Context,
	actor access.Principal,
	invitationID string,
) error {
	if err := s.requireAdministrator(actor); err != nil {
		return err
	}
	return s.repository.RevokeInvitation(
		ctx, s.organizationID, actor, strings.TrimSpace(invitationID), s.now().UTC(),
	)
}

func (s *Service) manageableProjectTarget(
	ctx context.Context,
	actor access.Principal,
	principalID string,
) (User, error) {
	if err := s.requireAdministrator(actor); err != nil {
		return User{}, err
	}
	target, err := s.repository.GetUser(ctx, s.organizationID, strings.TrimSpace(principalID), s.now().UTC())
	if err != nil {
		return User{}, err
	}
	if err := canManageTarget(actor, target); err != nil {
		return User{}, err
	}
	if target.OrganizationRole == "owner" || target.OrganizationRole == "admin" {
		return User{}, ErrGlobalProjectRole
	}
	if target.Status != "active" {
		return User{}, ErrInactiveUser
	}
	return target, nil
}

func (s *Service) requireAdministrator(actor access.Principal) error {
	if actor.OrganizationID != s.organizationID || (actor.OrganizationRole != "owner" && actor.OrganizationRole != "admin") {
		return ErrForbidden
	}
	return nil
}

func canManageTarget(actor access.Principal, target User) error {
	if actor.OrganizationRole == "owner" {
		return nil
	}
	if actor.OrganizationRole == "admin" && target.OrganizationRole == "member" {
		return nil
	}
	return ErrForbidden
}

func normalizeUpdate(input UpdateUserInput) (UpdateUserInput, error) {
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if value == "" || utf8.RuneCountInString(value) > 200 {
			return UpdateUserInput{}, &ValidationError{Field: "display_name", Message: "must contain 1 to 200 characters"}
		}
		input.DisplayName = &value
	}
	if input.Email != nil {
		value := strings.ToLower(strings.TrimSpace(*input.Email))
		address, err := mail.ParseAddress(value)
		if err != nil || !strings.EqualFold(address.Address, value) || len(value) > 320 {
			return UpdateUserInput{}, &ValidationError{Field: "email", Message: "must be a valid email address"}
		}
		input.Email = &value
	}
	if input.Username != nil {
		value := strings.ToLower(strings.TrimSpace(*input.Username))
		if !usernamePattern.MatchString(value) {
			return UpdateUserInput{}, &ValidationError{Field: "username", Message: "must contain 3 to 32 lowercase letters, numbers, dots, underscores, or hyphens"}
		}
		input.Username = &value
	}
	if input.Status != nil {
		value := strings.ToLower(strings.TrimSpace(*input.Status))
		if value != "active" && value != "disabled" {
			return UpdateUserInput{}, &ValidationError{Field: "status", Message: "must be active or disabled"}
		}
		input.Status = &value
	}
	if input.OrganizationRole != nil {
		value := strings.ToLower(strings.TrimSpace(*input.OrganizationRole))
		if !validOrganizationRole(value) {
			return UpdateUserInput{}, &ValidationError{Field: "organization_role", Message: "must be owner, admin, or member"}
		}
		input.OrganizationRole = &value
	}
	return input, nil
}

func normalizeInvitation(input CreateInvitationInput) (CreateInvitationInput, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.OrganizationRole = strings.ToLower(strings.TrimSpace(input.OrganizationRole))
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.ProjectRole = strings.ToLower(strings.TrimSpace(input.ProjectRole))
	if input.ExpiresAfter == 0 {
		input.ExpiresAfter = defaultInvitationLifetime
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || !strings.EqualFold(address.Address, input.Email) || len(input.Email) > 320 {
		return CreateInvitationInput{}, &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	if !validOrganizationRole(input.OrganizationRole) {
		return CreateInvitationInput{}, &ValidationError{Field: "organization_role", Message: "must be owner, admin, or member"}
	}
	if (input.ProjectID == "") != (input.ProjectRole == "") {
		return CreateInvitationInput{}, &ValidationError{Field: "project", Message: "project_id and project_role must be provided together"}
	}
	if input.ProjectRole != "" && !validProjectRole(input.ProjectRole) {
		return CreateInvitationInput{}, &ValidationError{Field: "project_role", Message: "must be owner, maintainer, contributor, or viewer"}
	}
	if input.OrganizationRole != "member" && input.ProjectID != "" {
		return CreateInvitationInput{}, &ValidationError{Field: "project_id", Message: "must be empty because owners and administrators already have global project access"}
	}
	if input.ExpiresAfter < time.Hour || input.ExpiresAfter > maximumInvitationLifetime {
		return CreateInvitationInput{}, &ValidationError{Field: "expires_in_hours", Message: "must be between 1 hour and 7 days"}
	}
	return input, nil
}

func validOrganizationRole(role string) bool {
	return role == "owner" || role == "admin" || role == "member"
}

func validProjectRole(role string) bool {
	return role == "owner" || role == "maintainer" || role == "contributor" || role == "viewer"
}

func newInvitationSecret() (string, [sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", digest, fmt.Errorf("generate invitation secret: %w", err)
	}
	secret := "pact_inv_" + base64.RawURLEncoding.EncodeToString(raw)
	return secret, sha256.Sum256([]byte(secret)), nil
}
