package repositorybinding

import (
	"context"
	"testing"
)

type fakeStore struct{ candidates []Candidate }

func (f fakeStore) ListCandidates(context.Context, string) ([]Candidate, error) {
	return f.candidates, nil
}

func TestResolveMatchesEquivalentRemotesAndWorkspaceFilter(t *testing.T) {
	const workspaceID = "018f784a-68c1-7b0f-8f2a-cfc255f99e3f"
	service := NewService("organization", fakeStore{candidates: []Candidate{
		{Match: Match{WorkspaceID: workspaceID, RepositoryID: "repository"}, RemoteURL: "https://github.com/example/repository.git"},
		{Match: Match{WorkspaceID: "028f784a-68c1-7b0f-8f2a-cfc255f99e3f", RepositoryID: "other"}, RemoteURL: "https://github.com/example/repository"},
	}})
	matches, err := service.Resolve(context.Background(), ResolveInput{
		RemoteURL: "git@github.com:example/repository.git", WorkspaceID: workspaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].RepositoryID != "repository" || matches[0].Match != "exact" {
		t.Fatalf("matches = %#v", matches)
	}
}
