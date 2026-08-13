//go:build windows

package localproject

import (
	"fmt"
	"os/exec"
	"strings"
)

func ensurePlatformGitConfig(root string) error {
	command := exec.Command("git", "-C", root, "config", "--local", "core.longpaths", "true")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("enable long Git paths for Windows: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
