package agentconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableCodexCreatesIdempotentMachineLocalConfiguration(t *testing.T) {
	root := newGitRepository(t)
	pactCommand := newExecutable(t, "pact-v1")

	first, err := EnableCodex(CodexOptions{ProjectRoot: root, PactCommand: pactCommand})
	if err != nil {
		t.Fatalf("EnableCodex() error = %v", err)
	}
	if !first.Created || !first.Changed || !first.Excluded {
		t.Fatalf("first result = %#v", first)
	}
	content := readTestFile(t, first.ConfigPath)
	for _, expected := range []string{
		managedStart,
		"[mcp_servers.pact]",
		`command = "` + pactCommand + `"`,
		`"--client", "codex"`,
		`cwd = "` + root + `"`,
		managedEnd,
	} {
		if !strings.Contains(content, expected) {
			t.Errorf("config does not contain %q:\n%s", expected, content)
		}
	}
	if strings.Count(content, "[mcp_servers.pact]") != 1 {
		t.Fatalf("managed table count = %d", strings.Count(content, "[mcp_servers.pact]"))
	}
	exclude := readTestFile(t, filepath.Join(root, ".git", "info", "exclude"))
	if !strings.Contains(exclude, "/.codex/config.toml") {
		t.Fatalf("Git exclude = %q", exclude)
	}
	if status := gitOutput(t, root, "status", "--short"); status != "" {
		t.Fatalf("Git status = %q, want clean", status)
	}

	second, err := EnableCodex(CodexOptions{ProjectRoot: root, PactCommand: pactCommand})
	if err != nil {
		t.Fatalf("second EnableCodex() error = %v", err)
	}
	if second.Created || second.Changed || second.Excluded {
		t.Fatalf("second result = %#v", second)
	}
	if updated := readTestFile(t, second.ConfigPath); updated != content {
		t.Fatalf("idempotent enable changed config:\n%s", updated)
	}
}

func TestEnableCodexPreservesExistingConfigurationAndUpdatesManagedBlock(t *testing.T) {
	root := newGitRepository(t)
	directory := filepath.Join(root, ".codex")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(configPath, []byte("[features]\nweb_search = true\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	firstCommand := newExecutable(t, "pact-v1")
	if _, err := EnableCodex(CodexOptions{ProjectRoot: root, PactCommand: firstCommand}); err != nil {
		t.Fatal(err)
	}
	secondCommand := newExecutable(t, "pact-v2")
	result, err := EnableCodex(CodexOptions{ProjectRoot: root, PactCommand: secondCommand})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Created {
		t.Fatalf("result = %#v", result)
	}
	content := readTestFile(t, configPath)
	if !strings.Contains(content, "[features]\nweb_search = true") || !strings.Contains(content, secondCommand) {
		t.Fatalf("updated config = %s", content)
	}
	if strings.Contains(content, firstCommand) || strings.Count(content, managedStart) != 1 {
		t.Fatalf("stale or duplicate managed block:\n%s", content)
	}
	if info, err := os.Stat(configPath); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o640 {
		t.Fatalf("config mode = %o, want 640", info.Mode().Perm())
	}
}

func TestEnableCodexRejectsConflictingOrUnsafeConfiguration(t *testing.T) {
	t.Run("existing Pact table", func(t *testing.T) {
		root := newGitRepository(t)
		if err := os.Mkdir(filepath.Join(root, ".codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(root, ".codex", "config.toml"),
			[]byte("[mcp_servers.pact]\ncommand = \"custom\"\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		_, err := EnableCodex(CodexOptions{ProjectRoot: root, PactCommand: newExecutable(t, "pact")})
		if err == nil || !strings.Contains(err.Error(), "already defines") {
			t.Fatalf("EnableCodex() error = %v", err)
		}
	})

	t.Run("symlinked directory", func(t *testing.T) {
		root := newGitRepository(t)
		if err := os.Symlink(t.TempDir(), filepath.Join(root, ".codex")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, err := EnableCodex(CodexOptions{ProjectRoot: root, PactCommand: newExecutable(t, "pact")})
		if err == nil || !strings.Contains(err.Error(), "not a real directory") {
			t.Fatalf("EnableCodex() error = %v", err)
		}
	})
}

func newGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	command := exec.Command("git", "init", "-q", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	return root
}

func newExecutable(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
