package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	"github.com/jorgenuanzs/the-pact/internal/authn"
	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/userconfig"
)

const cliTestToken = "pact_device_this-is-a-long-cli-test-device-credential"

func TestLoginInitAndConnectExistingProject(t *testing.T) {
	var (
		lock                sync.Mutex
		remoteProject       *projects.Project
		sessionOpened       bool
		sessionClosed       bool
		observationReported bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/v1/auth/devices" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": authn.DeviceAuthorization{
				DeviceCode: "pact_device_code_test-device-authorization-secret",
				UserCode:   "ABCD-2345", VerificationURI: "/admin/#device=ABCD-2345",
				ExpiresAt: time.Now().Add(10 * time.Second), IntervalSeconds: 1,
			}})
			return
		}
		if request.Method == http.MethodPost && request.URL.Path == "/v1/auth/devices/exchange" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": authn.DeviceExchange{
				Status: "authorized", DeviceCredential: cliTestToken,
			}})
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+cliTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		lock.Lock()
		defer lock.Unlock()
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": "00000000-0000-4000-8000-000000000002", "organization_id": "00000000-0000-4000-8000-000000000001",
				"display_name": "Test administrator", "principal_type": "human", "organization_role": "owner", "bootstrap": true,
			}})
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
			if input.AgentName != "Kimi" || input.AgentType != "kimi" || input.NodeKey == "" || !input.ObserveGit {
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
		case request.Method == http.MethodPost && request.URL.Path == "/v1/agent-sessions/018f784a-68c1-7b0f-8f2a-cfc255f99e3f/repository-observations":
			var input agentsession.ObservationInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode repository observation: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if request.Header.Get("Idempotency-Key") == "" || len(input.DiffFingerprint) != 64 || input.Dirty != (input.ChangedPaths > 0) {
				t.Errorf("repository observation = %#v; idempotency=%q", input, request.Header.Get("Idempotency-Key"))
			}
			observationReported = true
			_ = json.NewEncoder(w).Encode(map[string]any{"data": agentsession.ObservationResult{
				Observation: agentsession.RepositoryObservation{
					ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e6c", ProjectID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d",
					SessionID: "018f784a-68c1-7b0f-8f2a-cfc255f99e3f", DiffFingerprint: input.DiffFingerprint,
				},
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
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(
		[]string{"login", "--server", server.URL, "--name", "Local test", "--no-browser"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	); err != nil {
		t.Fatalf("login error = %v; stderr = %s", err, stderr.String())
	}
	profile, err := userconfig.FindProfileByURL(server.URL)
	if err != nil || profile.Label != "Local test" {
		t.Fatalf("login profile = %#v, %v", profile, err)
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
		"No database credentials, passwords, or device credentials",
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
		[]string{"agent", "run", "--client", "kimi", "--path", collaboratorRoot, "--", "git", "--version"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	); err != nil {
		t.Fatalf("agent run error = %v; stderr = %s", err, stderr.String())
	}
	if !sessionOpened || !sessionClosed || !observationReported || !strings.Contains(stdout.String(), "PACT agent session active: Kimi") {
		t.Fatalf("session opened=%v closed=%v observed=%v stdout=%s", sessionOpened, sessionClosed, observationReported, stdout.String())
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

func TestServersLifecycleKeepsProfilesIndependent(t *testing.T) {
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	firstURL := "https://one.pact.example.com"
	secondURL := "https://two.pact.example.com"
	firstCredential := "pact_device_" + strings.Repeat("1", 48)
	secondCredential := "pact_device_" + strings.Repeat("2", 48)
	if _, err := userconfig.SaveAuthorizedProfile(firstURL, firstCredential, userconfig.AuthorizedMetadata{
		ProfileLabel: "Client One", PrincipalID: "one", PrincipalLabel: "One User",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := userconfig.SaveAuthorizedProfile(secondURL, secondCredential, userconfig.AuthorizedMetadata{
		ProfileLabel: "Client Two", PrincipalID: "two", PrincipalLabel: "Two User",
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"servers", "list", "--json"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("servers list error = %v; stderr = %s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "credential_ref") || strings.Contains(stdout.String(), firstCredential) || strings.Contains(stdout.String(), secondCredential) {
		t.Fatalf("servers list exposed credential material: %s", stdout.String())
	}
	var profiles []serverProfileOutput
	if err := json.Unmarshal(stdout.Bytes(), &profiles); err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || !profiles[0].Active || profiles[0].ServerURL != secondURL {
		t.Fatalf("profiles = %#v", profiles)
	}

	stdout.Reset()
	if err := run([]string{"servers", "use", firstURL}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	active, err := userconfig.Load()
	if err != nil || active.ServerURL != firstURL || active.DeviceCredential != firstCredential {
		t.Fatalf("active after use = %#v, %v", active, err)
	}
	if err := run([]string{"servers", "remove", "--local-only", firstURL}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	active, err = userconfig.Load()
	if err != nil || active.ServerURL != secondURL || active.DeviceCredential != secondCredential {
		t.Fatalf("active after remove = %#v, %v", active, err)
	}
	if err := run([]string{"logout", "--server", secondURL, "--local-only"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	remaining, err := userconfig.ListProfiles()
	if err != nil || len(remaining) != 0 {
		t.Fatalf("remaining profiles = %#v, %v", remaining, err)
	}
}

func TestBoundFolderUsesItsProfileWithoutActiveFallback(t *testing.T) {
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	firstURL := "https://folder.pact.example.com"
	secondURL := "https://active.pact.example.com"
	firstCredential := "pact_device_" + strings.Repeat("3", 48)
	secondCredential := "pact_device_" + strings.Repeat("4", 48)
	if _, err := userconfig.Save(firstURL, firstCredential); err != nil {
		t.Fatal(err)
	}
	if _, err := userconfig.Save(secondURL, secondCredential); err != nil {
		t.Fatal(err)
	}
	root := newRealGitRepository(t, "https://github.com/example/multi-server.git")
	if _, err := localproject.Init(localproject.InitOptions{StartPath: root, ServerURL: firstURL}); err != nil {
		t.Fatal(err)
	}
	if err := localproject.Bind(root, firstURL, "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"); err != nil {
		t.Fatal(err)
	}
	login, err := loginForProjectOrActive(root, "")
	if err != nil || login.ServerURL != firstURL || login.DeviceCredential != firstCredential {
		t.Fatalf("folder login = %#v, %v", login, err)
	}
	var wait sync.WaitGroup
	errorsFound := make(chan error, 32)
	for index := 0; index < 16; index++ {
		for _, expected := range []struct {
			url        string
			credential string
		}{{firstURL, firstCredential}, {secondURL, secondCredential}} {
			wait.Add(1)
			go func(expectedURL, expectedCredential string) {
				defer wait.Done()
				resolved, resolveErr := loginForServer(expectedURL)
				if resolveErr != nil {
					errorsFound <- resolveErr
					return
				}
				if resolved.ServerURL != expectedURL || resolved.DeviceCredential != expectedCredential {
					errorsFound <- fmt.Errorf("resolved %#v for %s", resolved, expectedURL)
				}
			}(expected.url, expected.credential)
		}
	}
	wait.Wait()
	close(errorsFound)
	for concurrentErr := range errorsFound {
		t.Error(concurrentErr)
	}
	if err := userconfig.RemoveProfile(firstURL); err != nil {
		t.Fatal(err)
	}
	login, err = loginForProjectOrActive(root, "")
	if err == nil || login.ServerURL != "" || !strings.Contains(err.Error(), firstURL) || !strings.Contains(err.Error(), "pact login --server") {
		t.Fatalf("missing bound profile login = %#v, %v", login, err)
	}
	active, err := userconfig.Load()
	if err != nil || active.ServerURL != secondURL {
		t.Fatalf("active profile changed = %#v, %v", active, err)
	}
}

func TestLogoutRevokesOnlyRequestedServer(t *testing.T) {
	firstRevoked := false
	firstCredential := "pact_device_" + strings.Repeat("5", 48)
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/v1/auth/device/current" || request.Header.Get("Authorization") != "Bearer "+firstCredential {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		firstRevoked = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer firstServer.Close()
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	if _, err := userconfig.Save(firstServer.URL, firstCredential); err != nil {
		t.Fatal(err)
	}
	secondURL := "https://active.pact.example.com"
	if _, err := userconfig.Save(secondURL, "pact_device_"+strings.Repeat("6", 48)); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"logout", "--server", firstServer.URL}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !firstRevoked {
		t.Fatal("requested server was not revoked")
	}
	active, err := userconfig.Load()
	if err != nil || active.ServerURL != secondURL {
		t.Fatalf("active = %#v, %v", active, err)
	}
}

func TestStatusReportsBoundServerWorkspaceAndRepository(t *testing.T) {
	credential := "pact_device_" + strings.Repeat("7", 48)
	projectID := "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	repositoryID := "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
	workspaceID := "018f784a-68c1-7b0f-8f2a-cfc255f99e3f"
	remoteURL := "https://github.com/example/status"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+credential {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"projects": []map[string]any{{
				"id": projectID, "name": "Status project", "slug": "status-project", "status": "active",
			}}}})
		case "/v1/workspaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"workspaces": []map[string]any{{
				"id": workspaceID, "name": "Status workspace", "slug": "status-workspace", "status": "active",
				"projects": []map[string]any{{"id": projectID, "name": "Status project", "slug": "status-project", "status": "active"}},
			}}}})
		case "/v1/projects/" + projectID + "/repositories":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"repositories": []map[string]any{{
					"id": repositoryID, "project_id": projectID, "name": "status", "slug": "status",
					"remote_url": remoteURL, "primary": true, "required": true, "status": "active",
				}},
				"sync_states": []any{},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	if _, err := userconfig.SaveAuthorizedProfile(server.URL, credential, userconfig.AuthorizedMetadata{ProfileLabel: "Status Server"}); err != nil {
		t.Fatal(err)
	}
	root := newRealGitRepository(t, remoteURL+".git")
	if _, err := localproject.Init(localproject.InitOptions{StartPath: root, Name: "Status project", ServerURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	if err := localproject.Bind(root, server.URL, projectID); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"status", "--path", root, "--json"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("status error = %v; stderr = %s", err, stderr.String())
	}
	var status folderStatusOutput
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Server.URL != server.URL || status.Workspace == nil || status.Workspace.ID != workspaceID || status.Repository == nil || status.Repository.ID != repositoryID {
		t.Fatalf("status = %#v", status)
	}
}

func TestEnableCodexConfiguresConnectedProject(t *testing.T) {
	root := newRealGitRepository(t, "https://github.com/example/enabled.git")
	serverURL := "https://pact.example.com"
	if _, err := localproject.Init(localproject.InitOptions{StartPath: root, ServerURL: serverURL}); err != nil {
		t.Fatal(err)
	}
	if err := localproject.Bind(root, serverURL, "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	if _, err := userconfig.Save(serverURL, cliTestToken); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(
		[]string{"enable", "codex", "--path", root},
		strings.NewReader(""),
		&stdout,
		&stderr,
	); err != nil {
		t.Fatalf("enable error = %v; stderr = %s", err, stderr.String())
	}
	configPath := filepath.Join(root, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "[mcp_servers.pact]") ||
		!strings.Contains(stdout.String(), "Codex MCP enabled") ||
		!strings.Contains(stdout.String(), configPath) {
		t.Fatalf("stdout = %s\nconfig = %s", stdout.String(), content)
	}
	stdout.Reset()
	if err := run(
		[]string{"enable", "codex", "--path", root},
		strings.NewReader(""),
		&stdout,
		&stderr,
	); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "already enabled") {
		t.Fatalf("second enable stdout = %s", stdout.String())
	}
}

func TestEnableClaudeConfiguresConnectedProject(t *testing.T) {
	root := newRealGitRepository(t, "https://github.com/example/claude-enabled.git")
	serverURL := "https://pact.example.com"
	if _, err := localproject.Init(localproject.InitOptions{StartPath: root, ServerURL: serverURL}); err != nil {
		t.Fatal(err)
	}
	if err := localproject.Bind(root, serverURL, "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	if _, err := userconfig.Save(serverURL, cliTestToken); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(
		[]string{"enable", "claude", "--path", root},
		strings.NewReader(""),
		&stdout,
		&stderr,
	); err != nil {
		t.Fatalf("enable error = %v; stderr = %s", err, stderr.String())
	}
	configPath := filepath.Join(root, ".mcp.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"mcpServers"`) ||
		!strings.Contains(string(content), `"--client"`) ||
		!strings.Contains(string(content), `"claude"`) ||
		!strings.Contains(stdout.String(), "Claude MCP enabled") ||
		!strings.Contains(stdout.String(), configPath) {
		t.Fatalf("stdout = %s\nconfig = %s", stdout.String(), content)
	}
	stdout.Reset()
	if err := run(
		[]string{"enable", "claude", "--path", root},
		strings.NewReader(""),
		&stdout,
		&stderr,
	); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "already enabled") {
		t.Fatalf("second enable stdout = %s", stdout.String())
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
