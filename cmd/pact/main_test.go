package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

const cliTestToken = "this-is-a-long-cli-test-token"

func TestLoginInitAndConnectExistingProject(t *testing.T) {
	var (
		lock          sync.Mutex
		remoteProject *projects.Project
		sessionOpened bool
		sessionClosed bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+cliTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		lock.Lock()
		defer lock.Unlock()
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/projects":
			projectList := make([]projects.Project, 0)
			if remoteProject != nil {
				projectList = append(projectList, *remoteProject)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"projects": projectList}})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/projects":
			var input projects.CreateInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode create request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			remote := input.RootRepository.RemoteURL
			remoteProject = &projects.Project{
				ID:                "018f784a-68c1-7b0f-8f2a-cfc255f99e1d",
				Name:              input.Name,
				Slug:              input.Slug,
				Status:            "active",
				CanonicalRevision: input.CanonicalRevision,
				RootRepository: &projects.SourceRepository{
					ID:            "018f784a-68c1-7b0f-8f2a-cfc255f99e2e",
					Slug:          "primary",
					Name:          "Primary",
					VCSType:       "git",
					Status:        "active",
					RemoteURL:     &remote,
					DefaultBranch: input.RootRepository.DefaultBranch,
					ObjectFormat:  input.RootRepository.ObjectFormat,
					Version:       1,
				},
				Version: 1,
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": remoteProject})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/projects/018f784a-68c1-7b0f-8f2a-cfc255f99e1d/agent-sessions":
			var input agentsession.StartInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode agent session request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if input.AgentName != "Kimi" || input.AgentType != "kimi" || input.NodeKey == "" || input.ObserveGit {
				t.Errorf("agent session input = %#v", input)
			}
			sessionOpened = true
			now := time.Now().UTC()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": agentsession.Session{
				ID:         "018f784a-68c1-7b0f-8f2a-cfc255f99e3f",
				ProjectID:  "018f784a-68c1-7b0f-8f2a-cfc255f99e1d",
				ActorID:    "018f784a-68c1-7b0f-8f2a-cfc255f99e4a",
				ActorName:  "Kimi",
				NodeID:     "018f784a-68c1-7b0f-8f2a-cfc255f99e5b",
				NodeName:   "Test computer",
				ClientType: "kimi",
				Status:     "active",
				StartedAt:  now,
				LastSeenAt: now,
				ExpiresAt:  now.Add(45 * time.Second),
			}})
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/agent-sessions/018f784a-68c1-7b0f-8f2a-cfc255f99e3f":
			sessionClosed = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(
		[]string{"login", "--server", server.URL, "--token-stdin"},
		strings.NewReader(cliTestToken),
		&stdout,
		&stderr,
	); err != nil {
		t.Fatalf("login error = %v; stderr = %s", err, stderr.String())
	}

	ownerRoot := newRealGitRepository(t, "https://github.com/example/footfall.git")
	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"init", "--name", "Footfall", ownerRoot}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("init error = %v; stderr = %s", err, stderr.String())
	}
	for _, expected := range []string{
		"Created and connected Pact project",
		filepath.Join(ownerRoot, "pact.yaml"),
		"footfall (018f784a-68c1-7b0f-8f2a-cfc255f99e1d)",
		"No database credentials or API tokens",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("init stdout does not contain %q:\n%s", expected, stdout.String())
		}
	}

	collaboratorRoot := newRealGitRepository(t, "git@github.com:example/footfall.git")
	manifest, err := os.ReadFile(filepath.Join(ownerRoot, "pact.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collaboratorRoot, "pact.yaml"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"connect", collaboratorRoot}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("connect error = %v; stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Connected existing Pact project") {
		t.Fatalf("connect stdout = %s", stdout.String())
	}
	config, err := os.ReadFile(filepath.Join(collaboratorRoot, ".pact", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), remoteProject.ID) || strings.Contains(string(config), cliTestToken) {
		t.Fatalf("local config = %s", config)
	}

	stdout.Reset()
	stderr.Reset()
	if err := run(
		[]string{"agent", "run", "--client", "kimi", "--path", collaboratorRoot, "--", "true"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	); err != nil {
		t.Fatalf("agent run error = %v; stderr = %s", err, stderr.String())
	}
	if !sessionOpened || !sessionClosed || !strings.Contains(stdout.String(), "PACT agent session active: Kimi") {
		t.Fatalf("session opened=%v closed=%v stdout=%s", sessionOpened, sessionClosed, stdout.String())
	}
	nodeConfig, err := os.ReadFile(filepath.Join(collaboratorRoot, ".pact", "node.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(nodeConfig), cliTestToken) {
		t.Fatalf("node config contains an API token: %s", nodeConfig)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"destroy-everything"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %v", err)
	}
}

func newRealGitRepository(t *testing.T, remote string) string {
	t.Helper()
	root := t.TempDir()
	commands := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "pact@example.com"},
		{"config", "user.name", "Pact Test"},
		{"remote", "add", "origin", remote},
	}
	for _, arguments := range commands {
		command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"add", "README.md"}, {"commit", "-m", "Initial commit"}} {
		command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
		}
	}
	return root
}
