package gitobserve

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCaptureDetectsWorktreeChangesWithoutExposingPaths(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "pact@example.com")
	git(t, root, "config", "user.name", "Pact Test")
	write(t, filepath.Join(root, "README.md"), "initial\n")
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-m", "initial")

	clean, err := Capture(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if clean.Dirty || clean.ChangedPaths != 0 || clean.HeadRevision == "" || clean.Branch != "main" {
		t.Fatalf("clean snapshot = %#v", clean)
	}
	write(t, filepath.Join(root, "README.md"), "modified\n")
	modified, err := Capture(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !modified.Dirty || modified.ChangedPaths != 1 || modified.Fingerprint == clean.Fingerprint {
		t.Fatalf("modified snapshot = %#v; clean = %#v", modified, clean)
	}
	if len(modified.Fingerprint) != 64 {
		t.Fatalf("fingerprint = %q", modified.Fingerprint)
	}

	write(t, filepath.Join(root, "private-name.txt"), "secret content\n")
	untracked, err := Capture(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !untracked.Dirty || untracked.ChangedPaths != 2 || untracked.Fingerprint == modified.Fingerprint {
		t.Fatalf("untracked snapshot = %#v", untracked)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}
