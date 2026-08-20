package workspaces

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
)

type fakeRepository struct {
	create func(context.Context, string, string, [sha256.Size]byte, CreateInput) (CreateResult, error)
	get    func(context.Context, string, string) (Workspace, error)
	list   func(context.Context, string) ([]Workspace, error)
	update func(context.Context, string, string, UpdateInput) (Workspace, error)
	attach func(context.Context, string, string, string) (Workspace, error)
}

func (f fakeRepository) Create(ctx context.Context, organizationID, key string, hash [sha256.Size]byte, input CreateInput) (CreateResult, error) {
	return f.create(ctx, organizationID, key, hash, input)
}

func (f fakeRepository) Get(ctx context.Context, organizationID, reference string) (Workspace, error) {
	return f.get(ctx, organizationID, reference)
}

func (f fakeRepository) List(ctx context.Context, organizationID string) ([]Workspace, error) {
	return f.list(ctx, organizationID)
}

func (f fakeRepository) Update(ctx context.Context, organizationID, reference string, input UpdateInput) (Workspace, error) {
	return f.update(ctx, organizationID, reference, input)
}

func (f fakeRepository) AttachProject(ctx context.Context, organizationID, workspaceID, projectID string) (Workspace, error) {
	return f.attach(ctx, organizationID, workspaceID, projectID)
}

func TestCreateNormalizesAndDeduplicatesProjects(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	var received CreateInput
	service := NewService("org", fakeRepository{create: func(_ context.Context, _, key string, _ [sha256.Size]byte, input CreateInput) (CreateResult, error) {
		if key != "key" {
			t.Fatalf("key = %q", key)
		}
		received = input
		return CreateResult{Workspace: Workspace{ID: "workspace"}}, nil
	}})

	_, err := service.Create(context.Background(), " key ", CreateInput{
		Name: " Footfall ", Slug: " FOOTFALL-APP ", Description: " Shared product ",
		ProjectIDs: []string{" " + projectID + " ", projectID},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if received.Name != "Footfall" || received.Slug != "footfall-app" || received.Description != "Shared product" || received.Color != DefaultColor {
		t.Fatalf("normalized input = %#v", received)
	}
	if len(received.ProjectIDs) != 1 || received.ProjectIDs[0] != projectID {
		t.Fatalf("project IDs = %#v", received.ProjectIDs)
	}
}

func TestUpdateNormalizesWorkspaceDetails(t *testing.T) {
	var received UpdateInput
	service := NewService("org", fakeRepository{update: func(_ context.Context, organizationID, reference string, input UpdateInput) (Workspace, error) {
		if organizationID != "org" || reference != "footfall" {
			t.Fatalf("organization=%q reference=%q", organizationID, reference)
		}
		received = input
		return Workspace{Name: input.Name, Description: input.Description, Color: input.Color}, nil
	}})

	workspace, err := service.Update(context.Background(), " footfall ", UpdateInput{
		Name: " Footfall Platform ", Description: " Shared context ", Color: " #3877DC ",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if received.Name != "Footfall Platform" || received.Description != "Shared context" || received.Color != "#3877dc" {
		t.Fatalf("normalized input = %#v", received)
	}
	if workspace.Color != "#3877dc" {
		t.Fatalf("workspace = %#v", workspace)
	}
}

func TestUpdateRejectsInvalidColor(t *testing.T) {
	service := NewService("org", fakeRepository{update: func(context.Context, string, string, UpdateInput) (Workspace, error) {
		t.Fatal("repository should not be called")
		return Workspace{}, nil
	}})

	_, err := service.Update(context.Background(), "footfall", UpdateInput{Name: "Footfall", Color: "lime"})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "color" {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	service := NewService("org", fakeRepository{create: func(context.Context, string, string, [sha256.Size]byte, CreateInput) (CreateResult, error) {
		t.Fatal("repository should not be called")
		return CreateResult{}, nil
	}})

	_, err := service.Create(context.Background(), "key", CreateInput{Name: "Workspace", Slug: "Not Valid"})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "slug" {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestGetAcceptsSlugAndAttachRequiresUUIDs(t *testing.T) {
	service := NewService("org", fakeRepository{
		get: func(_ context.Context, organizationID, reference string) (Workspace, error) {
			if organizationID != "org" || reference != "footfall" {
				t.Fatalf("organization=%q reference=%q", organizationID, reference)
			}
			return Workspace{Slug: reference}, nil
		},
		attach: func(context.Context, string, string, string) (Workspace, error) {
			t.Fatal("repository should not be called")
			return Workspace{}, nil
		},
	})
	if _, err := service.Get(context.Background(), "footfall"); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if _, err := service.AttachProject(context.Background(), "footfall", "project"); err == nil {
		t.Fatal("AttachProject() accepted non-UUID references")
	}
}
