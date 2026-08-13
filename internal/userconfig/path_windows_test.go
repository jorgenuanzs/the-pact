//go:build windows

package userconfig

import (
	"path/filepath"
	"testing"
)

func TestConfigPathUsesWindowsAppData(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	t.Setenv("PACT_CONFIG_DIR", "")

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath() error = %v", err)
	}
	want := filepath.Join(appData, "Pact", "config.json")
	if path != want {
		t.Fatalf("configPath() = %q, want %q", path, want)
	}
}
