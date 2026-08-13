package localproject

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitCreatesSharedManifestAndPrivateLocalState(t *testing.T) {
	root := newGitRepository(t, "ref: refs/heads/trunk\n")
	nested := filepath.Join(root, "internal", "example")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("bin/"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Init(InitOptions{
		StartPath: nested,
		Name:      "Example Project",
		ServerURL: "https://pact.example.com/",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if result.Root != root {
		t.Fatalf("Root = %q, want %q", result.Root, root)
	}
	if !result.ManifestCreated || !result.LocalConfigCreated {
		t.Fatalf("created flags = %v, %v", result.ManifestCreated, result.LocalConfigCreated)
	}
	if result.ServerURL != "https://pact.example.com" {
		t.Fatalf("ServerURL = %q", result.ServerURL)
	}

	manifest := readFile(t, filepath.Join(root, manifestName))
	for _, expected := range []string{
		"apiVersion: pact.dev/v1alpha1",
		"kind: Project",
		`name: "Example Project"`,
		`canonicalRef: "refs/heads/trunk"`,
		"governanceMode: observer",
	} {
		if !strings.Contains(manifest, expected) {
			t.Errorf("manifest does not contain %q:\n%s", expected, manifest)
		}
	}
	if strings.Contains(manifest, "pact.example.com") {
		t.Fatal("shared manifest contains the machine-specific server URL")
	}

	configPath := filepath.Join(root, localDirectory, localConfigName)
	var config localConfig
	if err := json.Unmarshal([]byte(readFile(t, configPath)), &config); err != nil {
		t.Fatalf("decode local config: %v", err)
	}
	if config.SchemaVersion != 1 || config.ServerURL != "https://pact.example.com" {
		t.Fatalf("local config = %#v", config)
	}
	if info, err := os.Stat(configPath); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
	if info, err := os.Stat(filepath.Join(root, localDirectory)); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("local directory mode = %o, want 700", info.Mode().Perm())
	}

	ignore := readFile(t, filepath.Join(root, ".gitignore"))
	if ignore != "bin/\n\n# Pact local runtime\n.pact/\n" {
		t.Fatalf(".gitignore = %q", ignore)
	}
}

func TestInitIsIdempotentAndDoesNotReplaceLocalBinding(t *testing.T) {
	root := newGitRepository(t, "ref: refs/heads/main\n")

	first, err := Init(InitOptions{StartPath: root})
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	manifestBefore := readFile(t, first.ManifestPath)
	configBefore := readFile(t, first.LocalConfigPath)

	second, err := Init(InitOptions{StartPath: root})
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	if second.ManifestCreated || second.LocalConfigCreated {
		t.Fatalf("second init created files: %#v", second)
	}
	if got := readFile(t, second.ManifestPath); got != manifestBefore {
		t.Fatal("second init changed the shared manifest")
	}
	if got := readFile(t, second.LocalConfigPath); got != configBefore {
		t.Fatal("second init changed the local configuration")
	}
	if count := strings.Count(readFile(t, filepath.Join(root, ".gitignore")), ".pact/"); count != 1 {
		t.Fatalf(".pact ignore count = %d", count)
	}

	_, err = Init(InitOptions{StartPath: root, ServerURL: "https://other.example.com"})
	if err == nil || !strings.Contains(err.Error(), "already linked locally") {
		t.Fatalf("Init() server replacement error = %v", err)
	}
}

func TestInitRejectsUnsafeOrInvalidLocalConfiguration(t *testing.T) {
	t.Run("outside Git", func(t *testing.T) {
		_, err := Init(InitOptions{StartPath: t.TempDir()})
		if err == nil || !strings.Contains(err.Error(), "no Git repository") {
			t.Fatalf("Init() error = %v", err)
		}
	})

	t.Run("credentials in URL", func(t *testing.T) {
		root := newGitRepository(t, "ref: refs/heads/main\n")
		_, err := Init(InitOptions{
			StartPath: root,
			ServerURL: "https://token@example.com",
		})
		if err == nil || !strings.Contains(err.Error(), "must not contain credentials") {
			t.Fatalf("Init() error = %v", err)
		}
	})

	t.Run("plaintext remote server", func(t *testing.T) {
		root := newGitRepository(t, "ref: refs/heads/main\n")
		_, err := Init(InitOptions{
			StartPath: root,
			ServerURL: "http://pact.example.com",
		})
		if err == nil || !strings.Contains(err.Error(), "must use https") {
			t.Fatalf("Init() error = %v", err)
		}
	})

	t.Run("local directory symlink", func(t *testing.T) {
		root := newGitRepository(t, "ref: refs/heads/main\n")
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(root, localDirectory)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, err := Init(InitOptions{StartPath: root})
		if err == nil || !strings.Contains(err.Error(), "not a real directory") {
			t.Fatalf("Init() error = %v", err)
		}
	})
}

func TestFindRootSupportsGitWorktreeMetadata(t *testing.T) {
	root := t.TempDir()
	gitDirectory := filepath.Join(t.TempDir(), "worktree-git")
	if err := os.MkdirAll(gitDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDirectory, "HEAD"), []byte("ref: refs/heads/feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".git"),
		[]byte("gitdir: "+gitDirectory+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	result, err := Init(InitOptions{StartPath: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if manifest := readFile(t, result.ManifestPath); !strings.Contains(manifest, `canonicalRef: "refs/heads/feature"`) {
		t.Fatalf("manifest does not use worktree HEAD:\n%s", manifest)
	}
}

func newGitRepository(t *testing.T, head string) string {
	t.Helper()
	root := t.TempDir()
	gitDirectory := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDirectory, "HEAD"), []byte(head), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
