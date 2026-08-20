package access

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	role          string
	workspaceRole string
}

func (f fakeRepository) ProjectRole(context.Context, string, string, string) (string, error) {
	if f.role == "" {
		return "", ErrForbidden
	}
	return f.role, nil
}
func (f fakeRepository) WorkspaceRole(context.Context, string, string, string) (string, error) {
	if f.workspaceRole == "" {
		return "", ErrForbidden
	}
	return f.workspaceRole, nil
}
func (f fakeRepository) VisibleProjectIDs(context.Context, string, string) (map[string]struct{}, error) {
	return map[string]struct{}{"project": {}}, nil
}
func (f fakeRepository) GetProjectAccess(context.Context, string, string, time.Time, time.Time) (ProjectAccess, error) {
	return ProjectAccess{ProjectID: "project"}, nil
}
func (f fakeRepository) GetWorkspaceAccess(context.Context, string, string, time.Time, time.Time) (WorkspaceAccess, error) {
	return WorkspaceAccess{WorkspaceID: "workspace", Members: []WorkspaceMember{}, Agents: []ProjectAgent{}}, nil
}
func (f fakeRepository) CreateInvitation(context.Context, string, Principal, string, CreateInvitationInput, [sha256.Size]byte) (Invitation, error) {
	return Invitation{ID: "invitation", Email: "person@example.com", Role: "contributor"}, nil
}
func (f fakeRepository) RevokeInvitation(context.Context, string, Principal, string) error {
	return nil
}
func (f fakeRepository) GrantProjectOwner(context.Context, string, string, string) error { return nil }

func TestOrganizationOwnerProjectAuthorization(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", fakeRepository{role: "viewer"})
	principal := Principal{
		ID:               "018f784a-68c1-7b0f-8f2a-cfc255f99e1d",
		OrganizationID:   "00000000-0000-4000-8000-000000000001",
		OrganizationRole: "owner",
	}
	if err := service.RequireProjectRole(context.Background(), principal, "project", "owner"); err != nil {
		t.Fatalf("owner RequireProjectRole() error = %v", err)
	}
}

func TestContributorCannotCreateInvitations(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", fakeRepository{role: "contributor"})
	_, err := service.CreateInvitation(context.Background(), Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		OrganizationRole: "member",
	}, "project", CreateInvitationInput{Email: "person@example.com", Role: "viewer"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
}

func TestInvitationValidation(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", fakeRepository{role: "owner"})
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
	service := NewService("00000000-0000-4000-8000-000000000001", fakeRepository{})
	_, err := service.GetProjectAccess(context.Background(), Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		OrganizationRole: "member",
	}, "project")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetProjectAccess() error = %v", err)
	}
}

func TestOrganizationOwnerCanReadEmptyWorkspaceAccess(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", fakeRepository{})
	roster, err := service.GetWorkspaceAccess(context.Background(), Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		OrganizationRole: "owner",
	}, "workspace")
	if err != nil {
		t.Fatalf("GetWorkspaceAccess() error = %v", err)
	}
	if roster.WorkspaceID != "workspace" || roster.Members == nil || roster.Agents == nil {
		t.Fatalf("GetWorkspaceAccess() = %#v", roster)
	}
}

func TestWorkspaceAccessRequiresViewerRole(t *testing.T) {
	service := NewService("00000000-0000-4000-8000-000000000001", fakeRepository{})
	_, err := service.GetWorkspaceAccess(context.Background(), Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		OrganizationRole: "member",
	}, "workspace")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetWorkspaceAccess() error = %v", err)
	}
}
