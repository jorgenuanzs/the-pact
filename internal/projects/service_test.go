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
}

func (f fakeRepository) Create(ctx context.Context, organizationID, key string, hash [sha256.Size]byte, input CreateInput) (CreateResult, error) {
	return f.create(ctx, organizationID, key, hash, input)
}

func (f fakeRepository) Get(ctx context.Context, organizationID, projectID string) (Project, error) {
	return f.get(ctx, organizationID, projectID)
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
