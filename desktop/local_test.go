package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/userconfig"
)

func TestBindLocalFolderRegistersRepositoryDirectlyInSelectedWorkspace(t *testing.T) {
	t.Setenv("PACT_DESKTOP_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")

	const (
		workspaceID  = "019ffb8b-b422-7f7e-bf1a-54af07cba391"
		projectID    = "019ffb8b-b422-7f7e-bf1a-54af07cba392"
		repositoryID = "019ffb8b-b422-7f7e-bf1a-54af07cba393"
	)
	var createBody []byte
	var enrollmentBody []byte
	var legacyAttachCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/repository-bindings/resolve":
			_, _ = writer.Write([]byte(`{"data":{"matches":[]}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/projects":
			_, _ = writer.Write([]byte(`{"data":{"projects":[]}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/projects":
			createBody, _ = io.ReadAll(request.Body)
			var input struct {
				RootRepository struct {
					RemoteURL     string `json:"remote_url"`
					DefaultBranch string `json:"default_branch"`
					ObjectFormat  string `json:"object_format"`
				} `json:"root_repository"`
			}
			if err := json.Unmarshal(createBody, &input); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			response, _ := json.Marshal(map[string]any{"data": map[string]any{
				"id": projectID, "name": "magi", "slug": "magi", "status": "active", "version": 1,
				"root_repository": map[string]any{
					"id": repositoryID, "slug": "primary", "name": "Primary", "vcs_type": "git",
					"status": "active", "remote_url": input.RootRepository.RemoteURL,
					"default_branch": input.RootRepository.DefaultBranch, "object_format": input.RootRepository.ObjectFormat, "version": 1,
				},
			}})
			_, _ = writer.Write(response)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/workspaces/"+workspaceID+"/agent-enrollments":
			enrollmentBody, _ = io.ReadAll(request.Body)
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"data":{"created":true,"enrollment":{"id":"019ffb8b-b422-7f7e-bf1a-54af07cba394","workspace_id":"` + workspaceID + `","project_id":"` + projectID + `","agent_id":"019ffb8b-b422-7f7e-bf1a-54af07cba395","agent_name":"Codex","sponsor_principal_id":"019ffb8b-b422-7f7e-bf1a-54af07cba396","agent_type":"codex","client_type":"codex-mcp","status":"pending","created_at":"2026-08-24T12:00:00Z","updated_at":"2026-08-24T12:00:00Z"}}}`))
		case request.Method == http.MethodPut && strings.Contains(request.URL.Path, "/projects/"):
			legacyAttachCalls.Add(1)
			http.Error(writer, "legacy attach must not be called", http.StatusInternalServerError)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	if _, err := userconfig.Save(server.URL, "pact_device_"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	profile, err := userconfig.ActiveProfile()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "magi")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "PACT Test"},
		{"commit", "--allow-empty", "--quiet", "-m", "initial"},
		{"remote", "add", "origin", "https://github.com/example/magi.git"},
	}
	for _, arguments := range commands {
		command := exec.Command("git", arguments...)
		command.Dir = root
		if output, runErr := command.CombinedOutput(); runErr != nil {
			t.Fatalf("git %v: %v: %s", arguments, runErr, output)
		}
	}

	result, err := NewDesktop().BindLocalFolder(BindLocalFolderInput{
		ProjectRoot: root, ProfileID: profile.ID, WorkspaceID: workspaceID, CreateIfNeeded: true,
		Clients: []string{"codex"},
	})
	if err != nil {
		t.Fatalf("BindLocalFolder() error = %v", err)
	}
	if !result.Created || result.Folder.WorkspaceID != workspaceID || result.Folder.ProjectID != projectID {
		t.Fatalf("unexpected binding result: %+v", result)
	}
	if legacyAttachCalls.Load() != 0 {
		t.Fatalf("legacy workspace attachment calls = %d", legacyAttachCalls.Load())
	}
	if !bytes.Contains(createBody, []byte(`"workspace_id":"`+workspaceID+`"`)) {
		t.Fatalf("project creation did not target selected workspace: %s", createBody)
	}
	if len(result.Clients) != 1 || result.Clients[0].EnrollmentStatus != "pending" || result.Clients[0].Warning != "" {
		t.Fatalf("agent was not enrolled as pending: %+v", result.Clients)
	}
	if !bytes.Contains(enrollmentBody, []byte(`"project_id":"`+projectID+`"`)) || !bytes.Contains(enrollmentBody, []byte(`"agent_type":"codex"`)) {
		t.Fatalf("unexpected enrollment request: %s", enrollmentBody)
	}
}

func TestLocalRuntimeIsExtractedAsAWorkingCLI(t *testing.T) {
	configDirectory := t.TempDir()
	t.Setenv("PACT_DESKTOP_CONFIG_DIR", configDirectory)

	path, version, err := ensureLocalRuntime()
	if err != nil {
		t.Fatalf("ensure local runtime: %v", err)
	}
	if version == "" {
		t.Fatal("expected a runtime version")
	}
	launcherName := "pact-mcp"
	if runtime.GOOS == "windows" {
		launcherName += ".exe"
	}
	expectedLauncher := filepath.Join(configDirectory, "bin", launcherName)
	if path != expectedLauncher {
		t.Fatalf("launcher path = %q, want stable path %q", path, expectedLauncher)
	}
	active, err := os.ReadFile(filepath.Join(configDirectory, "runtime", "active"))
	if err != nil {
		t.Fatalf("read active runtime pointer: %v", err)
	}
	if strings.TrimSpace(string(active)) != version {
		t.Fatalf("active runtime = %q, want %q", strings.TrimSpace(string(active)), version)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("inspect extracted runtime: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("runtime is not a regular file: %s", path)
	}
	output, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("execute extracted runtime: %v: %s", err, output)
	}
	if !strings.Contains(string(output), `"version"`) {
		t.Fatalf("unexpected runtime version output: %s", output)
	}
}

func TestVersionCompatibility(t *testing.T) {
	if got := protocolCompatibility(buildinfo.Info{ProtocolVersion: 1, MinProtocolVersion: 1}); got != "compatible" {
		t.Fatalf("compatible protocol = %q", got)
	}
	if got := protocolCompatibility(buildinfo.Info{}); got != "unknown" {
		t.Fatalf("legacy protocol = %q", got)
	}
	if !versionOlderThan("0.15.9", "0.16.0") || versionOlderThan("0.17.0", "0.16.0") {
		t.Fatal("semantic version comparison is incorrect")
	}
}

func TestInspectServerVersionsReportsCompatibilityAndUpdates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"version":"v0.16.9","commit":"abcdef012345","date":"2026-08-24T00:00:00Z","protocol_version":1,"min_protocol_version":1}}`))
	}))
	defer server.Close()
	originalVersion := currentVersion
	currentVersion = "0.17.0"
	t.Cleanup(func() { currentVersion = originalVersion })

	profiles := inspectServerVersions([]DesktopServerProfile{{ID: "server", ServerURL: server.URL}})
	if len(profiles) != 1 || !profiles[0].Reachable || profiles[0].Compatibility != "compatible" || !profiles[0].UpdateAvailable {
		t.Fatalf("unexpected server inspection: %+v", profiles)
	}
	if profiles[0].Version != "0.16.9" || profiles[0].Commit != "abcdef012345" {
		t.Fatalf("unexpected server build: %+v", profiles[0])
	}
}

func TestLocalComputerStatusUsesEmptyJSONArrays(t *testing.T) {
	t.Setenv("PACT_DESKTOP_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")

	status := NewDesktop().LocalComputerStatus()
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document["clients"].([]any); !ok {
		t.Fatalf("clients must be a JSON array: %s", payload)
	}
	if folders, ok := document["folders"].([]any); !ok || len(folders) != 0 {
		t.Fatalf("local status must encode empty collections as arrays: %s", payload)
	}
}

func TestConnectLocalAgentWritesProjectScopedCodexConfiguration(t *testing.T) {
	desktopConfig := t.TempDir()
	userConfig := t.TempDir()
	t.Setenv("PACT_DESKTOP_CONFIG_DIR", desktopConfig)
	t.Setenv("PACT_CONFIG_DIR", userConfig)
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	const serverURL = "http://127.0.0.1:1"
	if _, err := userconfig.Save(serverURL, "pact_device_"+strings.Repeat("a", 64)); err != nil {
		t.Fatalf("save device login: %v", err)
	}

	root := filepath.Join(t.TempDir(), "footfall")
	if err := os.MkdirAll(filepath.Join(root, ".pact"), 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("initialize Git repository: %v: %s", err, output)
	}
	binding := map[string]any{
		"schema_version": 1,
		"server_url":     serverURL,
		"project_id":     "019ffb8b-b422-7f7e-bf1a-54af07cba39d",
	}
	payload, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pact", "config.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	desktop := NewDesktop()
	result, err := desktop.ConnectLocalAgent(ConnectLocalAgentInput{Client: "codex", ProjectRoot: root})
	if err != nil {
		t.Fatalf("connect Codex: %v", err)
	}
	if !result.Changed || !result.RestartNeeded {
		t.Fatalf("unexpected connection result: %+v", result)
	}
	if result.EnrollmentStatus != "deferred" || result.Warning == "" {
		t.Fatalf("unreachable server must defer enrollment without discarding local configuration: %+v", result)
	}
	content, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read generated Codex configuration: %v", err)
	}
	expectedRuntimePath := result.RuntimePath
	if filepath.Separator == '\\' {
		// TOML string literals escape Windows path separators. Compare against
		// the serialized value instead of the filesystem representation.
		expectedRuntimePath = strings.ReplaceAll(expectedRuntimePath, `\`, `\\`)
	}
	if !strings.Contains(string(content), expectedRuntimePath) || !strings.Contains(string(content), `"mcp"`) {
		t.Fatalf("generated configuration does not reference PACT runtime: %s", content)
	}

	status := desktop.LocalComputerStatus()
	if len(status.Folders) != 1 || status.Folders[0].Root != root {
		t.Fatalf("expected the connected folder in local state: %+v", status.Folders)
	}
	if len(status.Folders[0].Clients) != 1 || status.Folders[0].Clients[0] != "codex" {
		t.Fatalf("expected Codex on the connected folder: %+v", status.Folders[0].Clients)
	}

	legacyRuntimePath := filepath.Join(desktopConfig, "runtime", "000000000000", "pact-local")
	serializedLegacyPath := legacyRuntimePath
	if filepath.Separator == '\\' {
		serializedLegacyPath = strings.ReplaceAll(serializedLegacyPath, `\`, `\\`)
	}
	legacyContent := strings.Replace(string(content), expectedRuntimePath, serializedLegacyPath, 1)
	if legacyContent == string(content) {
		t.Fatal("test could not replace the managed launcher path")
	}
	if err := os.WriteFile(result.ConfigPath, []byte(legacyContent), 0o600); err != nil {
		t.Fatal(err)
	}
	migrated := desktop.LocalComputerStatus()
	if migrated.MCPMigrated != 1 || len(migrated.MCPMigrationErrors) != 0 {
		t.Fatalf("unexpected MCP migration result: %+v", migrated)
	}
	migratedContent, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migratedContent), expectedRuntimePath) || strings.Contains(string(migratedContent), serializedLegacyPath) {
		t.Fatalf("managed configuration was not migrated to the stable launcher: %s", migratedContent)
	}
}

func TestConnectLocalAgentUsesFolderProfileInsteadOfActiveProfile(t *testing.T) {
	t.Setenv("PACT_DESKTOP_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	const folderServer = "https://folder.pact.example.com"
	if _, err := userconfig.Save(folderServer, "pact_device_"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := userconfig.Save("https://active.pact.example.com", "pact_device_"+strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(root, ".pact"), 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("initialize Git repository: %v: %s", err, output)
	}
	payload, _ := json.Marshal(map[string]any{
		"schema_version": 1, "server_url": folderServer,
		"project_id": "019ffb8b-b422-7f7e-bf1a-54af07cba39d",
	})
	if err := os.WriteFile(filepath.Join(root, ".pact", "config.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewDesktop().ConnectLocalAgent(ConnectLocalAgentInput{Client: "codex", ProjectRoot: root}); err != nil {
		t.Fatalf("folder profile must not depend on active profile: %v", err)
	}
}

func TestInspectLocalFolderTreatsUnboundCheckoutAsOnboardingState(t *testing.T) {
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	root := filepath.Join(t.TempDir(), "project")
	commands := [][]string{
		{"init", "--quiet", root},
		{"-C", root, "config", "user.email", "test@example.com"},
		{"-C", root, "config", "user.name", "Test"},
		{"-C", root, "commit", "--quiet", "--allow-empty", "-m", "initial"},
		{"-C", root, "remote", "add", "origin", "git@github.com:nuanzs/project.git"},
	}
	for _, arguments := range commands {
		if output, err := exec.Command("git", arguments...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
	}

	inspection := inspectLocalFolder(root)
	if inspection.Error != "" || inspection.Connected {
		t.Fatalf("unbound checkout must be selectable without an error: %+v", inspection)
	}
	if inspection.RemoteURL != "https://github.com/nuanzs/project" {
		t.Fatalf("unexpected checkout metadata: %+v", inspection)
	}
}
