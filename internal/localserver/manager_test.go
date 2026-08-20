package localserver

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordedCommand struct {
	name string
	args []string
}

type fakeRunner struct {
	commands []recordedCommand
	services string
}

func (r *fakeRunner) Run(_ context.Context, _ string, _ io.Reader, stdout, _ io.Writer, name string, args ...string) error {
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
	if len(args) > 0 && args[len(args)-1] == "--services" {
		_, _ = io.WriteString(stdout, r.services)
	}
	return nil
}

func TestInstallWritesPrivateConfigurationAndStartsStack(t *testing.T) {
	runner := &fakeRunner{services: "postgres\npact-server\n"}
	manager := &Manager{Root: t.TempDir(), Runner: runner, Stdout: io.Discard, Stderr: io.Discard}

	result, err := manager.Install(context.Background(), InstallOptions{Port: 9080, Image: "ghcr.io/example/pact:v1"})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.SetupCode == "" || !result.Status.Installed || !result.Status.Running || result.Status.ServerURL != "http://127.0.0.1:9080" {
		t.Fatalf("Install() = %#v", result)
	}
	for _, name := range []string{composeName, environmentName, installationName} {
		info, err := os.Stat(filepath.Join(manager.Root, name))
		if err != nil {
			t.Fatal(err)
		}
		if os.PathSeparator != '\\' && info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %o", name, info.Mode().Perm())
		}
	}
	environment, err := os.ReadFile(manager.environmentPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(environment, []byte("PACT_SERVER_IMAGE=ghcr.io/example/pact:v1")) || bytes.Contains(environment, []byte("PASSWORD=\n")) {
		t.Fatalf("environment = %s", environment)
	}
	joined := commandText(runner.commands)
	for _, command := range []string{"docker info", "docker compose version", "pull", "up --detach --wait"} {
		if !strings.Contains(joined, command) {
			t.Errorf("commands do not contain %q: %s", command, joined)
		}
	}
}

func TestStatusReportsNotInstalledWithoutDocker(t *testing.T) {
	manager := &Manager{Root: t.TempDir(), Runner: &fakeRunner{}}
	status, err := manager.Status(context.Background())
	if err != nil || status.Installed {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
}

func TestForcedInstallPreservesExistingCredentials(t *testing.T) {
	runner := &fakeRunner{services: "pact-server\n"}
	manager := &Manager{Root: t.TempDir(), Runner: runner, Stdout: io.Discard, Stderr: io.Discard}
	if err := os.WriteFile(manager.installationPath(), []byte(`{"schema_version":1,"installed_at":"original","updated_at":"original","port":8080,"image":"old:v1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.environmentPath(), []byte("PACT_DB_PASSWORD=database-secret\nPACT_SETUP_TOKEN=setup-secret\nPACT_SERVER_IMAGE=old:v1\nPACT_HTTP_PORT=8080\nPACT_PUBLIC_URL=http://127.0.0.1:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := manager.Install(context.Background(), InstallOptions{Port: 9080, Image: "new:v2", Force: true})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.SetupCode != "setup-secret" {
		t.Fatalf("setup code = %q", result.SetupCode)
	}
	payload, err := os.ReadFile(manager.environmentPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "PACT_DB_PASSWORD=database-secret") || !strings.Contains(string(payload), "PACT_SETUP_TOKEN=setup-secret") {
		t.Fatalf("credentials changed during forced install: %s", payload)
	}
	metadata, err := manager.loadInstallation()
	if err != nil {
		t.Fatal(err)
	}
	if metadata.InstalledAt != "original" || metadata.Port != 9080 || metadata.Image != "new:v2" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestUpgradeChangesOnlyImageAndKeepsSecrets(t *testing.T) {
	runner := &fakeRunner{services: "pact-server\n"}
	manager := &Manager{Root: t.TempDir(), Runner: runner, Stdout: io.Discard, Stderr: io.Discard}
	if err := os.WriteFile(manager.installationPath(), []byte(`{"schema_version":1,"installed_at":"now","updated_at":"now","port":8080,"image":"old:v1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.environmentPath(), []byte("PACT_DB_PASSWORD=secret\nPACT_SETUP_TOKEN=setup\nPACT_SERVER_IMAGE=old:v1\nPACT_HTTP_PORT=8080\nPACT_PUBLIC_URL=http://127.0.0.1:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.composePath(), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Backup output is written by the fake runner as an empty but valid file.
	_, _, err := manager.Upgrade(context.Background(), "new:v2")
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	payload, err := os.ReadFile(manager.environmentPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "PACT_DB_PASSWORD=secret") || !strings.Contains(string(payload), "PACT_SERVER_IMAGE=new:v2") {
		t.Fatalf("environment after upgrade = %s", payload)
	}
}

func commandText(commands []recordedCommand) string {
	var value strings.Builder
	for _, command := range commands {
		value.WriteString(command.name)
		value.WriteByte(' ')
		value.WriteString(strings.Join(command.args, " "))
		value.WriteByte('\n')
	}
	return value.String()
}
