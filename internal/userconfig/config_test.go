package userconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveAndLoadDeviceCredential(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("PACT_CONFIG_DIR", directory)
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	credential := "pact_device_" + strings.Repeat("a", 48)

	path, err := Save("https://pact.example.com/", credential)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.SchemaVersion != schemaVersion || loaded.ServerURL != "https://pact.example.com" || loaded.DeviceCredential != credential {
		t.Fatalf("Load() = %#v", loaded)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(content), credential) || strings.Contains(string(content), "device_credential") {
		t.Fatalf("profile registry contains credential material: %s", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if permissions := info.Mode().Perm(); runtime.GOOS != "windows" && permissions != 0o600 {
		t.Fatalf("config permissions = %o", permissions)
	}
}

func TestLoadRejectsRetiredPersonalTokenConfiguration(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("PACT_CONFIG_DIR", directory)
	t.Setenv("PACT_CREDENTIAL_STORE", "memory")
	legacy := `{"schema_version":1,"server_url":"https://pact.example.com","api_token":"pact_pat_retired"}`
	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "retired authentication model") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestNormalizeServerURLRequiresTLSOutsideLoopback(t *testing.T) {
	if _, err := NormalizeServerURL("http://pact.example.com"); err == nil {
		t.Fatal("remote HTTP URL was accepted")
	}
	if got, err := NormalizeServerURL("http://127.0.0.1:8080/"); err != nil || got != "http://127.0.0.1:8080" {
		t.Fatalf("loopback URL = %q, %v", got, err)
	}
}
