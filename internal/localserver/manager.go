package localserver

import (
	"bufio"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
)

const (
	defaultPort       = 8080
	defaultProject    = "pact-local"
	composeName       = "compose.yaml"
	environmentName   = ".env"
	installationName  = "installation.json"
	serverImagePrefix = "ghcr.io/jorgenuanzs/the-pact:"
)

//go:embed assets/compose.yaml
var assets embed.FS

type Runner interface {
	Run(context.Context, string, io.Reader, io.Writer, io.Writer, string, ...string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, directory string, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(append([]string{name}, args...), " "), err)
	}
	return nil
}

type Manager struct {
	Root   string
	Runner Runner
	Stdout io.Writer
	Stderr io.Writer
	Client *http.Client
}

type InstallOptions struct {
	Port  int
	Image string
	Force bool
}

type InstallResult struct {
	Status    Status `json:"status"`
	SetupCode string `json:"setup_code"`
}

type Status struct {
	Installed     bool   `json:"installed"`
	Running       bool   `json:"running"`
	Ready         bool   `json:"ready"`
	ServerURL     string `json:"server_url,omitempty"`
	Image         string `json:"image,omitempty"`
	Version       string `json:"version,omitempty"`
	DataDirectory string `json:"data_directory,omitempty"`
	Error         string `json:"error,omitempty"`
}

type installation struct {
	SchemaVersion int    `json:"schema_version"`
	InstalledAt   string `json:"installed_at"`
	UpdatedAt     string `json:"updated_at"`
	Port          int    `json:"port"`
	Image         string `json:"image"`
}

func NewDefault(stdout, stderr io.Writer) (*Manager, error) {
	root, err := DefaultRoot()
	if err != nil {
		return nil, err
	}
	return &Manager{
		Root: root, Runner: ExecRunner{}, Stdout: stdout, Stderr: stderr,
		Client: &http.Client{Timeout: 2 * time.Second},
	}, nil
}

func DefaultRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PACT_SERVER_DIR")); override != "" {
		absolute, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve PACT_SERVER_DIR: %w", err)
		}
		return absolute, nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user configuration directory: %w", err)
	}
	return filepath.Join(config, "Pact", "server"), nil
}

func DefaultImage() string {
	version := strings.TrimSpace(buildinfo.Current().Version)
	if version == "" || version == "dev" || version == "unknown" {
		version = "edge"
	}
	return serverImagePrefix + version
}

func (m *Manager) Install(ctx context.Context, options InstallOptions) (InstallResult, error) {
	if err := m.defaults(); err != nil {
		return InstallResult{}, err
	}
	if options.Port == 0 {
		options.Port = defaultPort
	}
	if options.Port < 1 || options.Port > 65535 {
		return InstallResult{}, errors.New("PACT Server port must be between 1 and 65535")
	}
	if strings.TrimSpace(options.Image) == "" {
		options.Image = DefaultImage()
	}
	if strings.ContainsAny(options.Image, "\r\n") {
		return InstallResult{}, errors.New("PACT Server image is invalid")
	}
	existingMetadata, existingErr := m.loadInstallation()
	if existingErr == nil && !options.Force {
		return InstallResult{}, errors.New("PACT Server is already installed; use pact server upgrade or --force")
	} else if existingErr != nil && !errors.Is(existingErr, os.ErrNotExist) {
		return InstallResult{}, fmt.Errorf("inspect PACT Server installation: %w", existingErr)
	}
	if err := os.MkdirAll(m.Root, 0o700); err != nil {
		return InstallResult{}, fmt.Errorf("create PACT Server directory: %w", err)
	}
	if err := os.Chmod(m.Root, 0o700); err != nil && os.PathSeparator != '\\' {
		return InstallResult{}, fmt.Errorf("secure PACT Server directory: %w", err)
	}
	if err := m.preflight(ctx); err != nil {
		return InstallResult{}, err
	}
	compose, err := assets.ReadFile("assets/compose.yaml")
	if err != nil {
		return InstallResult{}, fmt.Errorf("read embedded compose definition: %w", err)
	}
	setupCode := ""
	databasePassword := ""
	if existingErr == nil {
		values, readErr := readEnvironment(m.environmentPath())
		if readErr != nil {
			return InstallResult{}, fmt.Errorf("preserve existing PACT Server credentials: %w", readErr)
		}
		setupCode = values["PACT_SETUP_TOKEN"]
		databasePassword = values["PACT_DB_PASSWORD"]
		if setupCode == "" || databasePassword == "" {
			return InstallResult{}, errors.New("existing PACT Server credentials are incomplete")
		}
	} else {
		setupCode, err = randomSecret(32)
		if err != nil {
			return InstallResult{}, err
		}
		databasePassword, err = randomSecret(32)
		if err != nil {
			return InstallResult{}, err
		}
	}
	serverURL := "http://127.0.0.1:" + strconv.Itoa(options.Port)
	environment := strings.Join([]string{
		"PACT_DB_PASSWORD=" + databasePassword,
		"PACT_SETUP_TOKEN=" + setupCode,
		"PACT_SERVER_IMAGE=" + strings.TrimSpace(options.Image),
		"PACT_HTTP_PORT=" + strconv.Itoa(options.Port),
		"PACT_PUBLIC_URL=" + serverURL,
		"",
	}, "\n")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	installedAt := now
	if existingErr == nil {
		installedAt = existingMetadata.InstalledAt
	}
	metadata := installation{SchemaVersion: 1, InstalledAt: installedAt, UpdatedAt: now, Port: options.Port, Image: strings.TrimSpace(options.Image)}
	metadataPayload, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return InstallResult{}, fmt.Errorf("encode PACT Server installation: %w", err)
	}
	metadataPayload = append(metadataPayload, '\n')
	for _, file := range []struct {
		path    string
		content []byte
		mode    os.FileMode
	}{
		{m.composePath(), compose, 0o600},
		{m.environmentPath(), []byte(environment), 0o600},
		{m.installationPath(), metadataPayload, 0o600},
	} {
		if err := writeAtomic(file.path, file.content, file.mode); err != nil {
			return InstallResult{}, fmt.Errorf("write %s: %w", filepath.Base(file.path), err)
		}
	}
	if err := m.compose(ctx, nil, m.Stdout, "pull"); err != nil {
		return InstallResult{}, fmt.Errorf("download PACT Server images: %w", err)
	}
	if err := m.compose(ctx, nil, m.Stdout, "up", "--detach", "--wait"); err != nil {
		return InstallResult{}, fmt.Errorf("start PACT Server: %w", err)
	}
	status, err := m.Status(ctx)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Status: status, SetupCode: setupCode}, nil
}

func (m *Manager) Start(ctx context.Context) (Status, error) {
	if err := m.requireInstallation(); err != nil {
		return Status{}, err
	}
	if err := m.preflight(ctx); err != nil {
		return Status{}, err
	}
	if err := m.compose(ctx, nil, m.Stdout, "up", "--detach", "--wait"); err != nil {
		return Status{}, err
	}
	return m.Status(ctx)
}

func (m *Manager) Stop(ctx context.Context) (Status, error) {
	if err := m.requireInstallation(); err != nil {
		return Status{}, err
	}
	if err := m.compose(ctx, nil, m.Stdout, "stop"); err != nil {
		return Status{}, err
	}
	return m.Status(ctx)
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	if err := m.defaults(); err != nil {
		return Status{}, err
	}
	metadata, err := m.loadInstallation()
	if errors.Is(err, os.ErrNotExist) {
		return Status{Installed: false}, nil
	}
	if err != nil {
		return Status{}, err
	}
	result := Status{
		Installed: true, ServerURL: "http://127.0.0.1:" + strconv.Itoa(metadata.Port),
		Image: metadata.Image, Version: imageVersion(metadata.Image), DataDirectory: m.Root,
	}
	if err := m.preflight(ctx); err != nil {
		result.Error = err.Error()
		return result, nil
	}
	var services strings.Builder
	if err := m.compose(ctx, nil, &services, "ps", "--status", "running", "--services"); err != nil {
		result.Error = err.Error()
		return result, nil
	}
	for _, service := range strings.Fields(services.String()) {
		if service == "pact-server" {
			result.Running = true
			break
		}
	}
	if !result.Running {
		return result, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, result.ServerURL+"/readyz", nil)
	if err != nil {
		return result, nil
	}
	response, err := m.Client.Do(request)
	if err == nil {
		defer response.Body.Close()
		result.Ready = response.StatusCode == http.StatusOK
	}
	return result, nil
}

func (m *Manager) Logs(ctx context.Context, follow bool, output io.Writer) error {
	if err := m.requireInstallation(); err != nil {
		return err
	}
	args := []string{"logs", "--tail", "250"}
	if follow {
		args = append(args, "--follow")
	}
	return m.compose(ctx, nil, output, args...)
}

func (m *Manager) Backup(ctx context.Context, destination string) (string, error) {
	if err := m.requireInstallation(); err != nil {
		return "", err
	}
	if strings.TrimSpace(destination) == "" {
		destination = filepath.Join(m.Root, "backups", "pact-"+time.Now().UTC().Format("20060102T150405Z")+".dump")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", fmt.Errorf("resolve backup destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(absolute), ".pact-backup-*")
	if err != nil {
		return "", fmt.Errorf("create backup file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", err
	}
	runErr := m.compose(ctx, nil, temporary, "exec", "-T", "postgres", "pg_dump", "-U", "pact", "-d", "pact", "-Fc")
	closeErr := temporary.Close()
	if runErr != nil {
		return "", fmt.Errorf("create PostgreSQL backup: %w", runErr)
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(temporaryPath, absolute); err != nil {
		return "", fmt.Errorf("commit backup: %w", err)
	}
	return absolute, nil
}

func (m *Manager) Restore(ctx context.Context, source string) error {
	if err := m.requireInstallation(); err != nil {
		return err
	}
	file, err := os.Open(strings.TrimSpace(source))
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer file.Close()
	return m.compose(ctx, file, m.Stdout, "exec", "-T", "postgres", "pg_restore", "-U", "pact", "-d", "pact", "--clean", "--if-exists", "--no-owner")
}

func (m *Manager) Upgrade(ctx context.Context, image string) (Status, string, error) {
	metadata, err := m.loadInstallation()
	if err != nil {
		return Status{}, "", err
	}
	if strings.TrimSpace(image) == "" {
		image = DefaultImage()
	}
	backup, err := m.Backup(ctx, "")
	if err != nil {
		return Status{}, "", fmt.Errorf("back up PACT before upgrade: %w", err)
	}
	values, err := readEnvironment(m.environmentPath())
	if err != nil {
		return Status{}, backup, err
	}
	values["PACT_SERVER_IMAGE"] = strings.TrimSpace(image)
	if err := writeEnvironment(m.environmentPath(), values); err != nil {
		return Status{}, backup, err
	}
	metadata.Image = strings.TrimSpace(image)
	metadata.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := m.writeInstallation(metadata); err != nil {
		return Status{}, backup, err
	}
	if err := m.compose(ctx, nil, m.Stdout, "pull"); err != nil {
		return Status{}, backup, err
	}
	if err := m.compose(ctx, nil, m.Stdout, "up", "--detach", "--wait"); err != nil {
		return Status{}, backup, err
	}
	status, err := m.Status(ctx)
	return status, backup, err
}

func (m *Manager) Uninstall(ctx context.Context, removeData bool) error {
	if err := m.requireInstallation(); err != nil {
		return err
	}
	args := []string{"down", "--remove-orphans"}
	if removeData {
		args = append(args, "--volumes")
	}
	if err := m.compose(ctx, nil, m.Stdout, args...); err != nil {
		return err
	}
	if removeData {
		return os.RemoveAll(m.Root)
	}
	return nil
}

func (m *Manager) defaults() error {
	if strings.TrimSpace(m.Root) == "" {
		return errors.New("PACT Server directory is required")
	}
	if m.Runner == nil {
		m.Runner = ExecRunner{}
	}
	if m.Stdout == nil {
		m.Stdout = io.Discard
	}
	if m.Stderr == nil {
		m.Stderr = io.Discard
	}
	if m.Client == nil {
		m.Client = &http.Client{Timeout: 2 * time.Second}
	}
	return nil
}

func (m *Manager) preflight(ctx context.Context) error {
	if err := m.defaults(); err != nil {
		return err
	}
	if err := m.Runner.Run(ctx, m.Root, nil, io.Discard, m.Stderr, "docker", "info"); err != nil {
		return errors.New("Docker is not installed or its daemon is not running")
	}
	if err := m.Runner.Run(ctx, m.Root, nil, io.Discard, m.Stderr, "docker", "compose", "version"); err != nil {
		return errors.New("Docker Compose v2 is required")
	}
	return nil
}

func (m *Manager) compose(ctx context.Context, stdin io.Reader, stdout io.Writer, arguments ...string) error {
	if err := m.defaults(); err != nil {
		return err
	}
	args := []string{"compose", "--project-name", defaultProject, "--env-file", m.environmentPath(), "--file", m.composePath()}
	args = append(args, arguments...)
	return m.Runner.Run(ctx, m.Root, stdin, stdout, m.Stderr, "docker", args...)
}

func (m *Manager) requireInstallation() error {
	if err := m.defaults(); err != nil {
		return err
	}
	if _, err := os.Stat(m.installationPath()); errors.Is(err, os.ErrNotExist) {
		return errors.New("PACT Server is not installed; run pact server install")
	} else if err != nil {
		return fmt.Errorf("inspect PACT Server installation: %w", err)
	}
	return nil
}

func (m *Manager) loadInstallation() (installation, error) {
	payload, err := os.ReadFile(m.installationPath())
	if err != nil {
		return installation{}, err
	}
	var metadata installation
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return installation{}, fmt.Errorf("decode PACT Server installation: %w", err)
	}
	if metadata.SchemaVersion != 1 || metadata.Port < 1 || metadata.Image == "" {
		return installation{}, errors.New("PACT Server installation metadata is invalid")
	}
	return metadata, nil
}

func (m *Manager) writeInstallation(metadata installation) error {
	payload, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(m.installationPath(), append(payload, '\n'), 0o600)
}

func (m *Manager) composePath() string      { return filepath.Join(m.Root, composeName) }
func (m *Manager) environmentPath() string  { return filepath.Join(m.Root, environmentName) }
func (m *Manager) installationPath() string { return filepath.Join(m.Root, installationName) }

func randomSecret(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate local PACT secret: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func imageVersion(image string) string {
	if index := strings.LastIndex(image, ":"); index >= 0 && index < len(image)-1 {
		return image[index+1:]
	}
	return image
}

func readEnvironment(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open PACT Server environment: %w", err)
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, errors.New("PACT Server environment contains an invalid line")
		}
		values[strings.TrimSpace(key)] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func writeEnvironment(path string, values map[string]string) error {
	order := []string{"PACT_DB_PASSWORD", "PACT_SETUP_TOKEN", "PACT_SERVER_IMAGE", "PACT_HTTP_PORT", "PACT_PUBLIC_URL"}
	var content strings.Builder
	for _, key := range order {
		value, ok := values[key]
		if !ok || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("PACT Server environment value %s is invalid", key)
		}
		fmt.Fprintf(&content, "%s=%s\n", key, value)
	}
	return writeAtomic(path, []byte(content.String()), 0o600)
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pact-server-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
