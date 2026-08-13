//go:build windows

package userconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

func defaultConfigPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user configuration directory: %w", err)
	}
	return filepath.Join(directory, "Pact", "config.json"), nil
}
