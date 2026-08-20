package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jorgenuanzs/the-pact/internal/userconfig"
)

func TestLocalRuntimeIsExtractedAsAWorkingCLI(t *testing.T) {
	t.Setenv("PACT_DESKTOP_CONFIG_DIR", t.TempDir())

	path, version, err := ensureLocalRuntime()
	if err != nil {
		t.Fatalf("ensure local runtime: %v", err)
	}
	if version == "" {
		t.Fatal("expected a runtime version")
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

func TestLocalComputerStatusUsesEmptyJSONArrays(t *testing.T) {
	t.Setenv("PACT_DESKTOP_CONFIG_DIR", t.TempDir())
	t.Setenv("PACT_CONFIG_DIR", t.TempDir())

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
	const serverURL = "https://pact.example.com"
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
}
