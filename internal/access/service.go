package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultInvitationLifetime = 24 * time.Hour
	maximumInvitationLifetime = 7 * 24 * time.Hour
	personalTokenLifetime     = 30 * 24 * time.Hour
)

type Repository interface {
	Authenticate(context.Context, string, [sha256.Size]byte) (Principal, error)
	ProjectRole(context.Context, string, string, string) (string, error)
	VisibleProjectIDs(context.Context, string, string) (map[string]struct{}, error)
	CreateInvitation(context.Context, string, Principal, string, CreateInvitationInput, [sha256.Size]byte) (Invitation, error)
	AcceptInvitation(context.Context, string, AcceptInvitationInput, [sha256.Size]byte, [sha256.Size]byte, time.Time) (AcceptedInvitation, error)
	RevokeInvitation(context.Context, string, Principal, string) error
	RevokeToken(context.Context, string, Principal) error
	GrantProjectOwner(context.Context, string, string, string) error
}

type Service struct {
	organizationID string
	bootstrapHash  [sha256.Size]byte
	repository     Repository
	now            func() time.Time
}

func NewService(organizationID, bootstrapToken string, repository Repository) *Service {
	return &Service{
		organizationID: organizationID,
		bootstrapHash:  sha256.Sum256([]byte(bootstrapToken)),
		repository:     repository,
		now:            time.Now,
	}
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (Principal, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return Principal{}, ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(rawToken))
	if subtle.ConstantTimeCompare(digest[:], s.bootstrapHash[:]) == 1 {
		return Principal{
			ID: BootstrapPrincipalID, OrganizationID: s.organizationID,
			DisplayName: "Local administrator", PrincipalType: "human",
			OrganizationRole: "owner", Bootstrap: true,
		}, nil
	}
	return s.repository.Authenticate(ctx, s.organizationID, digest)
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

func (s *Service) CreateInvitation(
	ctx context.Context,
	principal Principal,
	projectID string,
	input CreateInvitationInput,
) (CreatedInvitation, error) {
	input.Email = normalizeEmail(input.Email)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
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
	if input.Role == "owner" && !principal.Bootstrap {
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

func (s *Service) AcceptInvitation(ctx context.Context, input AcceptInvitationInput) (AcceptedInvitation, error) {
	input.Secret = strings.TrimSpace(input.Secret)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.TokenName = strings.TrimSpace(input.TokenName)
	if !strings.HasPrefix(input.Secret, "pact_inv_") || len(input.Secret) < 40 {
		return AcceptedInvitation{}, ErrInvitationInvalid
	}
	if input.DisplayName == "" || utf8.RuneCountInString(input.DisplayName) > 200 {
		return AcceptedInvitation{}, &ValidationError{Field: "display_name", Message: "must contain 1 to 200 characters"}
	}
	if input.TokenName == "" || utf8.RuneCountInString(input.TokenName) > 200 {
		return AcceptedInvitation{}, &ValidationError{Field: "token_name", Message: "must contain 1 to 200 characters"}
	}
	accessToken, accessDigest, err := newSecret("pact_pat_")
	if err != nil {
		return AcceptedInvitation{}, err
	}
	inviteDigest := sha256.Sum256([]byte(input.Secret))
	expiresAt := s.now().UTC().Add(personalTokenLifetime)
	result, err := s.repository.AcceptInvitation(
		ctx, s.organizationID, input, inviteDigest, accessDigest, expiresAt,
	)
	if err != nil {
		return AcceptedInvitation{}, err
	}
	result.AccessToken = accessToken
	return result, nil
}

func (s *Service) RevokeInvitation(ctx context.Context, principal Principal, invitationID string) error {
	return s.repository.RevokeInvitation(ctx, s.organizationID, principal, strings.TrimSpace(invitationID))
}

func (s *Service) RevokeCurrentToken(ctx context.Context, principal Principal) error {
	if principal.Bootstrap || principal.TokenID == "" {
		return &ValidationError{Field: "token", Message: "the bootstrap credential cannot be revoked through the API"}
	}
	return s.repository.RevokeToken(ctx, s.organizationID, principal)
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
