package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateBuildsIsolatedIdempotentGitWorktree(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "pact@example.com")
	runGit(t, root, "config", "user.name", "Pact Test")
	writeFile(t, filepath.Join(root, ".gitignore"), ".pact/\n", 0o644)
	writeFile(t, filepath.Join(root, "README.md"), "base\n", 0o644)
	runGit(t, root, "add", ".gitignore", "README.md")
	runGit(t, root, "commit", "-m", "base")
	base := runGit(t, root, "rev-parse", "HEAD")

	writeFile(t, filepath.Join(root, ".pact", "config.json"), "{\"server_url\":\"https://pact.example\"}\n", 0o600)
	writeFile(t, filepath.Join(root, ".pact", "node.json"), "{\"node_id\":\"test\"}\n", 0o600)

	intentID := "018f75cb-a60a-7000-8000-000000000001"
	result, err := Create(context.Background(), root, intentID, "Improve API safety", base)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.PathRef != ".pact/worktrees/"+intentID {
		t.Fatalf("PathRef = %q", result.PathRef)
	}
	if result.Branch != "pact/018f75cba60a-improve-api-safety" {
		t.Fatalf("Branch = %q", result.Branch)
	}
	if got := runGit(t, result.Path, "rev-parse", "HEAD"); got != base {
		t.Fatalf("worktree HEAD = %q, want %q", got, base)
	}
	if got := runGit(t, result.Path, "branch", "--show-current"); got != result.Branch {
		t.Fatalf("worktree branch = %q, want %q", got, result.Branch)
	}
	for _, name := range []string{"config.json", "node.json"} {
		info, statErr := os.Stat(filepath.Join(result.Path, ".pact", name))
		if statErr != nil {
			t.Fatalf("worktree runtime %s: %v", name, statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("worktree runtime %s mode = %o", name, info.Mode().Perm())
		}
	}

	again, err := Create(context.Background(), root, intentID, "Improve API safety", base)
	if err != nil {
		t.Fatalf("idempotent Create() error = %v", err)
	}
	if again != result {
		t.Fatalf("second result = %#v, want %#v", again, result)
	}
	if got := runGit(t, root, "status", "--short"); got != "" {
		t.Fatalf("main worktree became dirty: %q", got)
	}
}

func TestCreateRejectsManagedPathSymlink(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "pact@example.com")
	runGit(t, root, "config", "user.name", "Pact Test")
	writeFile(t, filepath.Join(root, ".gitignore"), ".pact/\n", 0o644)
	writeFile(t, filepath.Join(root, "README.md"), "base\n", 0o644)
	runGit(t, root, "add", ".gitignore", "README.md")
	runGit(t, root, "commit", "-m", "base")
	base := runGit(t, root, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(root, ".pact", "config.json"), "{}\n", 0o600)

	intentID := "018f75cb-a60a-7000-8000-000000000002"
	managed := filepath.Join(root, ".pact", "worktrees", intentID)
	if err := os.MkdirAll(filepath.Dir(managed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), managed); err != nil {
		t.Fatal(err)
	}
	_, err := Create(context.Background(), root, intentID, "Unsafe", base)
	if err == nil || !strings.Contains(err.Error(), "not a real directory") {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRecoversBranchAfterLocalRuntimeFailure(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "pact@example.com")
	runGit(t, root, "config", "user.name", "Pact Test")
	writeFile(t, filepath.Join(root, ".gitignore"), ".pact/\n", 0o644)
	writeFile(t, filepath.Join(root, "README.md"), "base\n", 0o644)
	runGit(t, root, "add", ".gitignore", "README.md")
	runGit(t, root, "commit", "-m", "base")
	base := runGit(t, root, "rev-parse", "HEAD")

	intentID := "018f75cb-a60a-7000-8000-000000000003"
	if _, err := Create(context.Background(), root, intentID, "Recover", base); err == nil || !strings.Contains(err.Error(), "config.json") {
		t.Fatalf("first Create() error = %v", err)
	}
	writeFile(t, filepath.Join(root, ".pact", "config.json"), "{}\n", 0o600)
	result, err := Create(context.Background(), root, intentID, "Recover", base)
	if err != nil {
		t.Fatalf("recovered Create() error = %v", err)
	}
	if got := runGit(t, result.Path, "branch", "--show-current"); got != result.Branch {
		t.Fatalf("recovered branch = %q, want %q", got, result.Branch)
	}
}

func runGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
