package agentconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnableClaudeCreatesIdempotentMachineLocalConfiguration(t *testing.T) {
	root := newGitRepository(t)
	pactCommand := newExecutable(t, "pact-v1")

	first, err := EnableClaude(ClaudeOptions{ProjectRoot: root, PactCommand: pactCommand})
	if err != nil {
		t.Fatalf("EnableClaude() error = %v", err)
	}
	if !first.Created || !first.Changed || !first.Excluded {
		t.Fatalf("first result = %#v", first)
	}
	content := readTestFile(t, first.ConfigPath)
	var document struct {
		MCPServers map[string]claudeServer `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		t.Fatal(err)
	}
	server := document.MCPServers["pact"]
	if server.Type != "stdio" || server.Command != pactCommand ||
		server.Env["PACT_MANAGED_CONFIG"] != claudeManagedMarker ||
		!strings.Contains(strings.Join(server.Args, " "), "--client claude") ||
		server.Args[len(server.Args)-1] != root {
		t.Fatalf("Claude Pact server = %#v", server)
	}
	exclude := readTestFile(t, filepath.Join(root, ".git", "info", "exclude"))
	if !strings.Contains(exclude, "/.mcp.json") {
		t.Fatalf("Git exclude = %q", exclude)
	}
	if status := gitOutput(t, root, "status", "--short"); status != "" {
		t.Fatalf("Git status = %q, want clean", status)
	}

	second, err := EnableClaude(ClaudeOptions{ProjectRoot: root, PactCommand: pactCommand})
	if err != nil {
		t.Fatalf("second EnableClaude() error = %v", err)
	}
	if second.Created || second.Changed || second.Excluded {
		t.Fatalf("second result = %#v", second)
	}
	if updated := readTestFile(t, second.ConfigPath); updated != content {
		t.Fatalf("idempotent enable changed config:\n%s", updated)
	}
}

func TestEnableClaudePreservesExistingServersAndUpdatesManagedServer(t *testing.T) {
	root := newGitRepository(t)
	configPath := filepath.Join(root, ".mcp.json")
	if err := os.WriteFile(configPath, []byte(`{
  "mcpServers": {
    "other": {"type":"stdio","command":"other","args":[]}
  },
  "custom": true
}
`), 0o640); err != nil {
		t.Fatal(err)
	}
	firstCommand := newExecutable(t, "pact-v1")
	if _, err := EnableClaude(ClaudeOptions{ProjectRoot: root, PactCommand: firstCommand}); err != nil {
		t.Fatal(err)
	}
	secondCommand := newExecutable(t, "pact-v2")
	result, err := EnableClaude(ClaudeOptions{ProjectRoot: root, PactCommand: secondCommand})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Created {
		t.Fatalf("result = %#v", result)
	}
	content := readTestFile(t, configPath)
	if !strings.Contains(content, `"other"`) || !strings.Contains(content, `"custom": true`) ||
		!strings.Contains(content, secondCommand) || strings.Contains(content, firstCommand) {
		t.Fatalf("updated config = %s", content)
	}
	if info, err := os.Stat(configPath); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("config mode = %o, want 640", info.Mode().Perm())
	}
}

func TestEnableClaudeRejectsConflictingOrUnsafeConfiguration(t *testing.T) {
	t.Run("existing unmanaged Pact server", func(t *testing.T) {
		root := newGitRepository(t)
		if err := os.WriteFile(
			filepath.Join(root, ".mcp.json"),
			[]byte(`{"mcpServers":{"pact":{"type":"stdio","command":"custom","args":[]}}}`),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		_, err := EnableClaude(ClaudeOptions{ProjectRoot: root, PactCommand: newExecutable(t, "pact")})
		if err == nil || !strings.Contains(err.Error(), "outside PACT management") {
			t.Fatalf("EnableClaude() error = %v", err)
		}
	})

	t.Run("symlinked configuration", func(t *testing.T) {
		root := newGitRepository(t)
		target := filepath.Join(t.TempDir(), "mcp.json")
		if err := os.WriteFile(target, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, ".mcp.json")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, err := EnableClaude(ClaudeOptions{ProjectRoot: root, PactCommand: newExecutable(t, "pact")})
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("EnableClaude() error = %v", err)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		root := newGitRepository(t)
		if err := os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(`{"mcpServers":`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := EnableClaude(ClaudeOptions{ProjectRoot: root, PactCommand: newExecutable(t, "pact")})
		if err == nil || !strings.Contains(err.Error(), "parse .mcp.json") {
			t.Fatalf("EnableClaude() error = %v", err)
		}
	})
}
