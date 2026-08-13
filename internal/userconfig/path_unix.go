//go:build !windows

package userconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "pact", "config.json"), nil
}
