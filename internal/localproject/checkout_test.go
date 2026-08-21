package localproject

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInspectCheckoutDoesNotRequirePactManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "My Project")
	runGit(t, "init", "--quiet", root)
	runGit(t, "-C", root, "config", "user.email", "test@example.com")
	runGit(t, "-C", root, "config", "user.name", "Test")
	runGit(t, "-C", root, "commit", "--quiet", "--allow-empty", "-m", "initial")
	runGit(t, "-C", root, "remote", "add", "origin", "git@github.com:nuanzs/example.git")

	checkout, err := InspectCheckout(root)
	if err != nil {
		t.Fatalf("inspect checkout: %v", err)
	}
	if checkout.Root != root || checkout.Name != "My Project" || checkout.Slug != "my-project" {
		t.Fatalf("unexpected checkout identity: %+v", checkout)
	}
	if checkout.RemoteURL != "https://github.com/nuanzs/example" {
		t.Fatalf("unexpected normalized remote: %q", checkout.RemoteURL)
	}
	if checkout.CanonicalRevision == "" || checkout.ObjectFormat == "" {
		t.Fatalf("missing Git metadata: %+v", checkout)
	}
}

func runGit(t *testing.T, arguments ...string) {
	t.Helper()
	if output, err := exec.Command("git", arguments...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}
