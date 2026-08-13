//go:build windows

package localproject

import (
	"os/exec"
	"strings"
	"testing"
)

func TestEnsurePlatformGitConfigEnablesLongPaths(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := ensurePlatformGitConfig(root); err != nil {
		t.Fatalf("ensurePlatformGitConfig() error = %v", err)
	}
	output, err := exec.Command("git", "-C", root, "config", "--local", "--get", "core.longpaths").CombinedOutput()
	if err != nil {
		t.Fatalf("read core.longpaths: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "true" {
		t.Fatalf("core.longpaths = %q, want true", output)
	}
}
