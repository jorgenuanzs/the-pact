package repositorysync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseGitHubRemote(t *testing.T) {
	tests := map[string]string{
		"https://github.com/jorgenuanzs/the-pact.git": "jorgenuanzs/the-pact",
		"git@github.com:jorgenuanzs/the-pact.git":     "jorgenuanzs/the-pact",
		"ssh://git@github.com/jorgenuanzs/the-pact":   "jorgenuanzs/the-pact",
		"git://github.com/jorgenuanzs/the-pact.git":   "jorgenuanzs/the-pact",
	}
	for remote, expected := range tests {
		reference, err := ParseGitHubRemote(remote)
		if err != nil {
			t.Fatalf("ParseGitHubRemote(%q) error = %v", remote, err)
		}
		if reference.FullName != expected {
			t.Fatalf("ParseGitHubRemote(%q) = %q", remote, reference.FullName)
		}
	}
}

func TestParseGitHubRemoteRejectsCredentialsAndOtherProviders(t *testing.T) {
	for _, remote := range []string{
		"https://secret@github.com/owner/repo.git",
		"https://gitlab.com/owner/repo.git",
		"file:///tmp/repo",
		"https://github.com/owner/repo/extra",
	} {
		if _, err := ParseGitHubRemote(remote); err == nil {
			t.Fatalf("ParseGitHubRemote(%q) succeeded", remote)
		}
	}
}

func TestGitHubClientFetchesCanonicalState(t *testing.T) {
	updatedAt := time.Date(2026, time.August, 14, 10, 30, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("X-GitHub-Api-Version") != defaultGitHubAPIVersion {
			t.Errorf("X-GitHub-Api-Version = %q", r.Header.Get("X-GitHub-Api-Version"))
		}
		if r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.EscapedPath() {
		case "/repos/owner/repository":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"full_name":"owner/repository","default_branch":"release/v1","visibility":"private","private":true,"updated_at":"2026-08-14T10:30:00Z"}`))
		case "/repos/owner/repository/git/ref/heads/release/v1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":{"type":"commit","sha":"37ab373144cb17d18e77c52c03f5f6e18e1fb3c5"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewGitHubClient(GitHubOptions{
		APIURL: server.URL, Token: "provider-secret", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.Fetch(context.Background(), Reference{
		Owner: "owner", Name: "repository", FullName: "owner/repository",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CanonicalRevision != "37ab373144cb17d18e77c52c03f5f6e18e1fb3c5" ||
		snapshot.DefaultBranch != "release/v1" || snapshot.Visibility != "private" ||
		snapshot.ProviderUpdatedAt == nil || !snapshot.ProviderUpdatedAt.Equal(updatedAt) {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestGitHubClientMapsPrivateRepositoryNotFoundWithoutReturningBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "sensitive provider detail", http.StatusNotFound)
	}))
	defer server.Close()
	client, err := NewGitHubClient(GitHubOptions{APIURL: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(context.Background(), Reference{Owner: "owner", Name: "private", FullName: "owner/private"})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "repository_not_found_or_inaccessible" {
		t.Fatalf("Fetch() error = %#v", err)
	}
	if providerErr.Error() == "sensitive provider detail" {
		t.Fatal("provider response body leaked")
	}
}
