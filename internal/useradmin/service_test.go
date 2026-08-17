package useradmin

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
)

type fakeRepository struct {
	user User
}

func (f fakeRepository) Directory(context.Context, string, time.Time) (Directory, error) {
	return Directory{Users: []User{f.user}}, nil
}
func (f fakeRepository) GetUser(context.Context, string, string, time.Time) (User, error) {
	return f.user, nil
}
func (f fakeRepository) UpdateUser(context.Context, string, access.Principal, string, UpdateUserInput, time.Time) (User, error) {
	return f.user, nil
}
func (f fakeRepository) SetProjectPermission(context.Context, string, access.Principal, string, string, string, time.Time) (User, error) {
	return f.user, nil
}
func (f fakeRepository) RemoveProjectPermission(context.Context, string, access.Principal, string, string, time.Time) (User, error) {
	return f.user, nil
}
func (f fakeRepository) RevokeUserSessions(context.Context, string, access.Principal, string, time.Time) (User, error) {
	return f.user, nil
}
func (f fakeRepository) CreateInvitation(context.Context, string, access.Principal, CreateInvitationInput, [sha256.Size]byte, time.Time) (Invitation, error) {
	return Invitation{ID: "invitation"}, nil
}
func (f fakeRepository) RevokeInvitation(context.Context, string, access.Principal, string, time.Time) error {
	return nil
}

func TestAdministratorCannotManageOwner(t *testing.T) {
	service := NewService("organization", fakeRepository{user: User{
		PrincipalID: "owner", OrganizationRole: "owner", Status: "active",
	}})
	_, err := service.UpdateUser(context.Background(), access.Principal{
		ID: "admin", OrganizationID: "organization", OrganizationRole: "admin",
	}, "owner", UpdateUserInput{DisplayName: stringPointer("Changed")})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("UpdateUser() error = %v, want ErrForbidden", err)
	}
}

func TestOwnerCannotDemoteCurrentAccount(t *testing.T) {
	service := NewService("organization", fakeRepository{user: User{
		PrincipalID: "owner", OrganizationRole: "owner", Status: "active",
	}})
	_, err := service.UpdateUser(context.Background(), access.Principal{
		ID: "owner", OrganizationID: "organization", OrganizationRole: "owner",
	}, "owner", UpdateUserInput{OrganizationRole: stringPointer("member")})
	if !errors.Is(err, ErrSelfManagement) {
		t.Fatalf("UpdateUser() error = %v, want ErrSelfManagement", err)
	}
}

func TestGlobalRoleRejectsDirectProjectPermission(t *testing.T) {
	service := NewService("organization", fakeRepository{user: User{
		PrincipalID: "admin", OrganizationRole: "admin", Status: "active",
	}})
	_, err := service.SetProjectPermission(context.Background(), access.Principal{
		ID: "owner", OrganizationID: "organization", OrganizationRole: "owner",
	}, "admin", "project", "viewer")
	if !errors.Is(err, ErrGlobalProjectRole) {
		t.Fatalf("SetProjectPermission() error = %v, want ErrGlobalProjectRole", err)
	}
}

func TestAdminCanOnlyInviteMembers(t *testing.T) {
	service := NewService("organization", fakeRepository{})
	_, err := service.CreateInvitation(context.Background(), access.Principal{
		ID: "admin", OrganizationID: "organization", OrganizationRole: "admin",
	}, CreateInvitationInput{Email: "person@example.com", OrganizationRole: "admin"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreateInvitation() error = %v, want ErrForbidden", err)
	}
}

func TestInvitationRequiresCompleteProjectScope(t *testing.T) {
	service := NewService("organization", fakeRepository{})
	_, err := service.CreateInvitation(context.Background(), access.Principal{
		ID: "owner", OrganizationID: "organization", OrganizationRole: "owner",
	}, CreateInvitationInput{
		Email: "person@example.com", OrganizationRole: "member", ProjectID: "project",
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("CreateInvitation() error = %v, want ValidationError", err)
	}
}

func stringPointer(value string) *string { return &value }
