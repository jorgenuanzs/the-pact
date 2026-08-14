package repositorysync

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/jorgenuanzs/the-pact/internal/projects"
)

const (
	testOrganizationID = "00000000-0000-4000-8000-000000000001"
	testPrincipalID    = "00000000-0000-4000-8000-000000000002"
	testProjectID      = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	testRepositoryID   = "018f784a-68c1-7b0f-8f2a-cfc255f99e1e"
)

type fakeProjectCatalog struct{ project projects.Project }

func (f fakeProjectCatalog) Get(context.Context, string) (projects.Project, error) {
	return f.project, nil
}

func (f fakeProjectCatalog) List(context.Context) ([]projects.Project, error) {
	return []projects.Project{f.project}, nil
}

type fakeProvider struct {
	snapshot Snapshot
	err      error
}

func (f fakeProvider) Fetch(context.Context, Reference) (Snapshot, error) {
	return f.snapshot, f.err
}

type fakeRepository struct {
	state       State
	found       bool
	result      Result
	failureCode string
}

func (f *fakeRepository) Get(context.Context, string, string, string) (State, bool, error) {
	return f.state, f.found, nil
}

func (f *fakeRepository) Replay(context.Context, string, string, string, [sha256.Size]byte) (Result, bool, error) {
	return Result{}, false, nil
}

func (f *fakeRepository) Apply(context.Context, string, string, string, string, string, [sha256.Size]byte, Snapshot) (Result, error) {
	return f.result, nil
}

func (f *fakeRepository) ApplyScheduled(context.Context, string, string, string, Snapshot) (Result, error) {
	return f.result, nil
}

func (f *fakeRepository) RecordFailure(_ context.Context, _, _, _, _, code string) (State, error) {
	f.failureCode = code
	return State{}, nil
}

func TestServiceGetDescribesNeverSyncedGitHubRepository(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(testOrganizationID, fakeProjectCatalog{project: githubProject()}, repository, fakeProvider{})
	state, err := service.Get(context.Background(), testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusNever || state.Provider != "github" || state.RepositoryFullName != "owner/repository" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestServiceSyncRecordsSanitizedProviderFailure(t *testing.T) {
	repository := &fakeRepository{}
	providerErr := &ProviderError{Code: "authentication_required", Err: errors.New("private detail")}
	service := NewService(testOrganizationID, fakeProjectCatalog{project: githubProject()}, repository, fakeProvider{err: providerErr})
	_, err := service.Sync(context.Background(), testPrincipalID, testProjectID, "sync-once")
	if !errors.Is(err, providerErr) {
		t.Fatalf("Sync() error = %v", err)
	}
	if repository.failureCode != "authentication_required" {
		t.Fatalf("failure code = %q", repository.failureCode)
	}
}

func githubProject() projects.Project {
	remote := "https://github.com/owner/repository.git"
	return projects.Project{
		ID: testProjectID, Status: "active",
		RootRepository: &projects.SourceRepository{
			ID: testRepositoryID, Status: "active", RemoteURL: &remote,
			DefaultBranch: "main", ObjectFormat: "sha1",
		},
	}
}
