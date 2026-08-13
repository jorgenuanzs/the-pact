package projects

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
)

type fakeRepository struct {
	create func(context.Context, string, string, [sha256.Size]byte, CreateInput) (CreateResult, error)
	get    func(context.Context, string, string) (Project, error)
	list   func(context.Context, string) ([]Project, error)
}

func (f fakeRepository) Create(ctx context.Context, organizationID, key string, hash [sha256.Size]byte, input CreateInput) (CreateResult, error) {
	return f.create(ctx, organizationID, key, hash, input)
}

func (f fakeRepository) Get(ctx context.Context, organizationID, projectID string) (Project, error) {
	return f.get(ctx, organizationID, projectID)
}

func (f fakeRepository) List(ctx context.Context, organizationID string) ([]Project, error) {
	return f.list(ctx, organizationID)
}

func TestCreateNormalizesInputBeforeHashing(t *testing.T) {
	var received CreateInput
	revision := " ABC123F "
	repository := fakeRepository{
		create: func(_ context.Context, _, _ string, _ [sha256.Size]byte, input CreateInput) (CreateResult, error) {
			received = input
			return CreateResult{Project: Project{ID: "project"}}, nil
		},
	}
	service := NewService("00000000-0000-4000-8000-000000000001", repository)

	_, err := service.Create(context.Background(), " key ", CreateInput{
		Name:              " Pact ",
		Slug:              " pact ",
		CanonicalRevision: &revision,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if received.Name != "Pact" ||
		received.Slug != "pact" ||
		received.CanonicalRevision == nil ||
		*received.CanonicalRevision != "abc123f" {
		t.Fatalf("received input = %#v", received)
	}
}

func TestCreateRejectsInvalidSlug(t *testing.T) {
	service := NewService("org", fakeRepository{
		create: func(context.Context, string, string, [sha256.Size]byte, CreateInput) (CreateResult, error) {
			t.Fatal("repository should not be called")
			return CreateResult{}, nil
		},
	})

	_, err := service.Create(context.Background(), "key", CreateInput{Name: "Pact", Slug: "The Pact"})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "slug" {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsSymbolicRevision(t *testing.T) {
	revision := "main"
	service := NewService("org", fakeRepository{
		create: func(context.Context, string, string, [sha256.Size]byte, CreateInput) (CreateResult, error) {
			t.Fatal("repository should not be called")
			return CreateResult{}, nil
		},
	})

	_, err := service.Create(
		context.Background(),
		"key",
		CreateInput{Name: "Pact", Slug: "pact", CanonicalRevision: &revision},
	)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "canonical_revision" {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateNormalizesAndValidatesRootRepository(t *testing.T) {
	var received CreateInput
	repository := fakeRepository{
		create: func(_ context.Context, _, _ string, _ [sha256.Size]byte, input CreateInput) (CreateResult, error) {
			received = input
			return CreateResult{Project: Project{ID: "project"}}, nil
		},
	}
	service := NewService("org", repository)
	revision := "ABC123F"
	_, err := service.Create(context.Background(), "key", CreateInput{
		Name:              "Footfall",
		Slug:              "footfall",
		CanonicalRevision: &revision,
		RootRepository: &SourceRepositoryInput{
			Slug:          " primary ",
			Name:          " Primary ",
			RemoteURL:     " https://github.com/example/footfall ",
			DefaultBranch: " main ",
			ObjectFormat:  " SHA1 ",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if received.RootRepository == nil ||
		received.RootRepository.RemoteURL != "https://github.com/example/footfall" ||
		received.RootRepository.ObjectFormat != "sha1" {
		t.Fatalf("root repository = %#v", received.RootRepository)
	}

	_, err = service.Create(context.Background(), "other-key", CreateInput{
		Name: "Invalid",
		Slug: "invalid",
		RootRepository: &SourceRepositoryInput{
			Slug:          "primary",
			Name:          "Primary",
			RemoteURL:     "https://github.com/example/invalid",
			DefaultBranch: "main",
			ObjectFormat:  "md5",
		},
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "root_repository.object_format" {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestGetRejectsInvalidID(t *testing.T) {
	service := NewService("org", fakeRepository{
		get: func(context.Context, string, string) (Project, error) {
			t.Fatal("repository should not be called")
			return Project{}, nil
		},
	})

	_, err := service.Get(context.Background(), "not-an-id")
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestListUsesConfiguredOrganization(t *testing.T) {
	const organizationID = "00000000-0000-4000-8000-000000000001"
	repository := fakeRepository{
		list: func(_ context.Context, receivedOrganizationID string) ([]Project, error) {
			if receivedOrganizationID != organizationID {
				t.Fatalf("organization ID = %q", receivedOrganizationID)
			}
			return []Project{}, nil
		},
	}

	service := NewService(organizationID, repository)
	projectList, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if projectList == nil {
		t.Fatal("List() returned a nil slice")
	}
}
