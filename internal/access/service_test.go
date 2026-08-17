package access

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	role string
}

func (f fakeRepository) Authenticate(context.Context, string, [sha256.Size]byte) (Principal, error) {
	return Principal{}, ErrUnauthorized
}
func (f fakeRepository) ProjectRole(context.Context, string, string, string) (string, error) {
	if f.role == "" {
		return "", ErrForbidden
	}
	return f.role, nil
}
func (f fakeRepository) VisibleProjectIDs(context.Context, string, string) (map[string]struct{}, error) {
	return map[string]struct{}{"project": {}}, nil
}
func (f fakeRepository) GetProjectAccess(context.Context, string, string, time.Time, time.Time) (ProjectAccess, error) {
	return ProjectAccess{ProjectID: "project"}, nil
}
func (f fakeRepository) CreateInvitation(context.Context, string, Principal, string, CreateInvitationInput, [sha256.Size]byte) (Invitation, error) {
	return Invitation{ID: "invitation", Email: "person@example.com", Role: "contributor"}, nil
}
func (f fakeRepository) AcceptInvitation(context.Context, string, AcceptInvitationInput, [sha256.Size]byte, [sha256.Size]byte, time.Time) (AcceptedInvitation, error) {
	return AcceptedInvitation{}, nil
}
func (f fakeRepository) RevokeInvitation(context.Context, string, Principal, string) error {
	return nil
}
func (f fakeRepository) RevokeToken(context.Context, string, Principal) error            { return nil }
func (f fakeRepository) GrantProjectOwner(context.Context, string, string, string) error { return nil }

func TestBootstrapAuthenticationAndProjectAuthorization(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", "bootstrap-secret", fakeRepository{role: "viewer"})
	principal, err := service.Authenticate(context.Background(), "bootstrap-secret")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !principal.Bootstrap || principal.OrganizationRole != "owner" {
		t.Fatalf("principal = %#v", principal)
	}
	if err := service.RequireProjectRole(context.Background(), principal, "project", "owner"); err != nil {
		t.Fatalf("bootstrap RequireProjectRole() error = %v", err)
	}
}

func TestContributorCannotCreateInvitations(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", "bootstrap-secret", fakeRepository{role: "contributor"})
	_, err := service.CreateInvitation(context.Background(), Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		OrganizationRole: "member",
	}, "project", CreateInvitationInput{Email: "person@example.com", Role: "viewer"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
}

func TestInvitationValidation(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", "bootstrap-secret", fakeRepository{role: "owner"})
	_, err := service.CreateInvitation(context.Background(), Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		OrganizationRole: "member",
	}, "project", CreateInvitationInput{Email: "not an email", Role: "owner"})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
}

func TestProjectAccessRequiresViewerRole(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", "bootstrap-secret", fakeRepository{})
	_, err := service.GetProjectAccess(context.Background(), Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		OrganizationRole: "member",
	}, "project")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetProjectAccess() error = %v", err)
	}
}
