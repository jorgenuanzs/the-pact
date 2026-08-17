package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

const (
	defaultInvitationLifetime = 24 * time.Hour
	maximumInvitationLifetime = 7 * 24 * time.Hour
)

type Repository interface {
	ProjectRole(context.Context, string, string, string) (string, error)
	VisibleProjectIDs(context.Context, string, string) (map[string]struct{}, error)
	GetProjectAccess(context.Context, string, string, time.Time, time.Time) (ProjectAccess, error)
	CreateInvitation(context.Context, string, Principal, string, CreateInvitationInput, [sha256.Size]byte) (Invitation, error)
	RevokeInvitation(context.Context, string, Principal, string) error
	GrantProjectOwner(context.Context, string, string, string) error
}

type Service struct {
	organizationID string
	repository     Repository
	now            func() time.Time
}

func NewService(organizationID string, repository Repository) *Service {
	return &Service{
		organizationID: organizationID,
		repository:     repository,
		now:            time.Now,
	}
}

func (s *Service) ProjectRole(ctx context.Context, principal Principal, projectID string) (string, error) {
	if principal.OrganizationID != s.organizationID {
		return "", ErrForbidden
	}
	if principal.OrganizationRole == "owner" || principal.OrganizationRole == "admin" {
		return "owner", nil
	}
	return s.repository.ProjectRole(ctx, s.organizationID, principal.ID, projectID)
}

func (s *Service) RequireProjectRole(ctx context.Context, principal Principal, projectID, minimum string) error {
	actual, err := s.ProjectRole(ctx, principal, projectID)
	if err != nil {
		return err
	}
	if projectRoleRank(actual) < projectRoleRank(minimum) {
		return ErrForbidden
	}
	return nil
}

func (s *Service) VisibleProjectIDs(ctx context.Context, principal Principal) (map[string]struct{}, error) {
	if principal.OrganizationRole == "owner" || principal.OrganizationRole == "admin" {
		return nil, nil
	}
	return s.repository.VisibleProjectIDs(ctx, s.organizationID, principal.ID)
}

func (s *Service) CanCreateProject(principal Principal) bool {
	return principal.OrganizationID == s.organizationID &&
		(principal.OrganizationRole == "owner" || principal.OrganizationRole == "admin")
}

func (s *Service) GetProjectAccess(
	ctx context.Context,
	principal Principal,
	projectID string,
) (ProjectAccess, error) {
	if err := s.RequireProjectRole(ctx, principal, projectID, "viewer"); err != nil {
		return ProjectAccess{}, err
	}
	now := s.now().UTC()
	return s.repository.GetProjectAccess(ctx, s.organizationID, projectID, now, now.Add(-30*time.Second))
}

func (s *Service) CreateInvitation(
	ctx context.Context,
	principal Principal,
	projectID string,
	input CreateInvitationInput,
) (CreatedInvitation, error) {
	input.Email = normalizeEmail(input.Email)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.OrganizationRole = strings.ToLower(strings.TrimSpace(input.OrganizationRole))
	if input.OrganizationRole == "" {
		if input.Role == "owner" {
			input.OrganizationRole = "owner"
		} else {
			input.OrganizationRole = "member"
		}
	}
	if input.ExpiresAfter == 0 {
		input.ExpiresAfter = defaultInvitationLifetime
	}
	if err := validateInvitationInput(input); err != nil {
		return CreatedInvitation{}, err
	}
	actualRole, err := s.ProjectRole(ctx, principal, projectID)
	if err != nil {
		return CreatedInvitation{}, err
	}
	if actualRole != "owner" && actualRole != "maintainer" {
		return CreatedInvitation{}, ErrForbidden
	}
	if input.Role == "owner" && principal.OrganizationRole != "owner" && principal.OrganizationRole != "admin" {
		return CreatedInvitation{}, ErrForbidden
	}
	if input.OrganizationRole == "owner" && principal.OrganizationRole != "owner" {
		return CreatedInvitation{}, ErrForbidden
	}
	if actualRole == "maintainer" && input.Role == "maintainer" {
		return CreatedInvitation{}, ErrForbidden
	}
	secret, digest, err := newSecret("pact_inv_")
	if err != nil {
		return CreatedInvitation{}, err
	}
	invitation, err := s.repository.CreateInvitation(
		ctx, s.organizationID, principal, projectID, input, digest,
	)
	if err != nil {
		return CreatedInvitation{}, err
	}
	return CreatedInvitation{Invitation: invitation, Secret: secret}, nil
}

func (s *Service) RevokeInvitation(ctx context.Context, principal Principal, invitationID string) error {
	return s.repository.RevokeInvitation(ctx, s.organizationID, principal, strings.TrimSpace(invitationID))
}

func (s *Service) GrantProjectOwner(ctx context.Context, principal Principal, projectID string) error {
	return s.repository.GrantProjectOwner(ctx, s.organizationID, projectID, principal.ID)
}

func normalizeEmail(raw string) string {
	parsed, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil || parsed.Address == "" {
		return ""
	}
	return strings.ToLower(parsed.Address)
}

func validateInvitationInput(input CreateInvitationInput) error {
	switch {
	case input.Email == "" || len(input.Email) > 320:
		return &ValidationError{Field: "email", Message: "must be a valid email address"}
	case input.OrganizationRole != "owner" && input.OrganizationRole != "member":
		return &ValidationError{Field: "organization_role", Message: "must be owner or member"}
	case input.Role != "owner" && input.Role != "maintainer" && input.Role != "contributor" && input.Role != "viewer":
		return &ValidationError{Field: "role", Message: "must be owner, maintainer, contributor, or viewer"}
	case input.ExpiresAfter < time.Hour || input.ExpiresAfter > maximumInvitationLifetime:
		return &ValidationError{Field: "expires_after", Message: "must be between 1 hour and 7 days"}
	}
	return nil
}

func projectRoleRank(role string) int {
	switch role {
	case "owner":
		return 4
	case "maintainer":
		return 3
	case "contributor":
		return 2
	case "viewer":
		return 1
	default:
		return 0
	}
}

func newSecret(prefix string) (string, [sha256.Size]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", [sha256.Size]byte{}, fmt.Errorf("generate access secret: %w", err)
	}
	secret := prefix + base64.RawURLEncoding.EncodeToString(raw)
	return secret, sha256.Sum256([]byte(secret)), nil
}
