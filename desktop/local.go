package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/agentconfig"
	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/localserver"
	"github.com/jorgenuanzs/the-pact/internal/userconfig"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const localStateSchemaVersion = 1

type LocalClientStatus struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Detected         bool   `json:"detected"`
	Detection        string `json:"detection,omitempty"`
	ConnectedFolders int    `json:"connected_folders"`
}

type LocalFolder struct {
	Root       string   `json:"root"`
	Name       string   `json:"name"`
	ServerURL  string   `json:"server_url"`
	ProjectID  string   `json:"project_id"`
	Clients    []string `json:"clients"`
	Available  bool     `json:"available"`
	Status     string   `json:"status,omitempty"`
	Configured string   `json:"configured_at,omitempty"`
}

type LocalComputerStatus struct {
	Hostname        string              `json:"hostname"`
	OperatingSystem string              `json:"operating_system"`
	Architecture    string              `json:"architecture"`
	RuntimeReady    bool                `json:"runtime_ready"`
	RuntimePath     string              `json:"runtime_path,omitempty"`
	RuntimeVersion  string              `json:"runtime_version,omitempty"`
	RuntimeError    string              `json:"runtime_error,omitempty"`
	ServerURL       string              `json:"server_url,omitempty"`
	Clients         []LocalClientStatus `json:"clients"`
	Folders         []LocalFolder       `json:"folders"`
	ManagedServer   localserver.Status  `json:"managed_server"`
}

type LocalFolderInspection struct {
	Canceled  bool     `json:"canceled"`
	Connected bool     `json:"connected"`
	Root      string   `json:"root,omitempty"`
	Name      string   `json:"name,omitempty"`
	ServerURL string   `json:"server_url,omitempty"`
	ProjectID string   `json:"project_id,omitempty"`
	Clients   []string `json:"clients,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type ConnectLocalAgentInput struct {
	Client      string `json:"client"`
	ProjectRoot string `json:"project_root"`
}

type ConnectLocalAgentResult struct {
	Client        string `json:"client"`
	ProjectRoot   string `json:"project_root"`
	ConfigPath    string `json:"config_path"`
	RuntimePath   string `json:"runtime_path"`
	Changed       bool   `json:"changed"`
	RestartNeeded bool   `json:"restart_needed"`
}

type localState struct {
	SchemaVersion int                    `json:"schema_version"`
	Folders       map[string]localRecord `json:"folders"`
}

type localRecord struct {
	Root         string   `json:"root"`
	Name         string   `json:"name"`
	ServerURL    string   `json:"server_url"`
	ProjectID    string   `json:"project_id"`
	Clients      []string `json:"clients"`
	ConfiguredAt string   `json:"configured_at"`
}

func (d *Desktop) LocalComputerStatus() LocalComputerStatus {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "Este computador"
	}
	result := LocalComputerStatus{
		Hostname:        hostname,
		OperatingSystem: goruntime.GOOS,
		Architecture:    goruntime.GOARCH,
		Clients:         make([]LocalClientStatus, 0),
		Folders:         make([]LocalFolder, 0),
	}
	if config, loadErr := userconfig.Load(); loadErr == nil {
		result.ServerURL = config.ServerURL
	}
	runtimePath, runtimeVersion, runtimeErr := ensureLocalRuntime()
	if runtimeErr != nil {
		result.RuntimeError = runtimeErr.Error()
	} else {
		result.RuntimeReady = true
		result.RuntimePath = runtimePath
		result.RuntimeVersion = runtimeVersion
	}

	state, stateErr := loadLocalState()
	if stateErr != nil {
		result.RuntimeError = joinLocalError(result.RuntimeError, stateErr.Error())
	}
	for _, record := range state.Folders {
		result.Folders = append(result.Folders, inspectLocalRecord(record))
	}
	sort.Slice(result.Folders, func(left, right int) bool {
		return strings.ToLower(result.Folders[left].Name) < strings.ToLower(result.Folders[right].Name)
	})
	result.Clients = detectLocalClients(result.Folders)
	if manager, managerErr := desktopLocalServerManager(); managerErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		result.ManagedServer, _ = manager.Status(ctx)
		cancel()
	}
	return result
}

func (d *Desktop) SelectLocalProjectFolder() (LocalFolderInspection, error) {
	ctx := d.appContext()
	if ctx == nil {
		return LocalFolderInspection{}, errors.New("desktop window is not ready")
	}
	selected, err := wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{
		Title:                "Selecciona una carpeta Git conectada a PACT",
		CanCreateDirectories: false,
		ResolvesAliases:      true,
	})
	if err != nil {
		return LocalFolderInspection{}, fmt.Errorf("select project folder: %w", err)
	}
	if strings.TrimSpace(selected) == "" {
		return LocalFolderInspection{Canceled: true}, nil
	}
	return inspectLocalFolder(selected), nil
}

func (d *Desktop) InspectLocalProjectFolder(path string) LocalFolderInspection {
	return inspectLocalFolder(path)
}

func (d *Desktop) ConnectLocalAgent(input ConnectLocalAgentInput) (ConnectLocalAgentResult, error) {
	clientID := strings.ToLower(strings.TrimSpace(input.Client))
	if clientID != "codex" && clientID != "claude" {
		return ConnectLocalAgentResult{}, errors.New("client must be codex or claude")
	}
	binding, err := localproject.LoadBinding(strings.TrimSpace(input.ProjectRoot))
	if err != nil {
		return ConnectLocalAgentResult{}, err
	}
	login, err := userconfig.Load()
	if err != nil {
		return ConnectLocalAgentResult{}, errors.New("PACT Desktop is not connected to a server")
	}
	if binding.ServerURL != login.ServerURL {
		return ConnectLocalAgentResult{}, fmt.Errorf("this folder belongs to %s, but PACT Desktop is connected to %s", binding.ServerURL, login.ServerURL)
	}
	runtimePath, _, err := ensureLocalRuntime()
	if err != nil {
		return ConnectLocalAgentResult{}, err
	}

	result := ConnectLocalAgentResult{
		Client:        clientID,
		ProjectRoot:   binding.Root,
		RuntimePath:   runtimePath,
		RestartNeeded: true,
	}
	switch clientID {
	case "codex":
		configured, configureErr := agentconfig.EnableCodex(agentconfig.CodexOptions{
			ProjectRoot: binding.Root,
			PactCommand: runtimePath,
		})
		if configureErr != nil {
			return ConnectLocalAgentResult{}, configureErr
		}
		result.ConfigPath = configured.ConfigPath
		result.Changed = configured.Changed
	case "claude":
		configured, configureErr := agentconfig.EnableClaude(agentconfig.ClaudeOptions{
			ProjectRoot: binding.Root,
			PactCommand: runtimePath,
		})
		if configureErr != nil {
			return ConnectLocalAgentResult{}, configureErr
		}
		result.ConfigPath = configured.ConfigPath
		result.Changed = configured.Changed
	}
	if err := rememberLocalConnection(binding, clientID); err != nil {
		return ConnectLocalAgentResult{}, err
	}
	return result, nil
}

func inspectLocalFolder(path string) LocalFolderInspection {
	root, err := localproject.FindRoot(strings.TrimSpace(path))
	if err != nil {
		return LocalFolderInspection{Error: err.Error()}
	}
	result := LocalFolderInspection{Root: root, Name: filepath.Base(root)}
	if descriptor, describeErr := localproject.Describe(root); describeErr == nil && strings.TrimSpace(descriptor.Name) != "" {
		result.Name = descriptor.Name
	}
	binding, err := localproject.LoadBinding(root)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Connected = true
	result.ServerURL = binding.ServerURL
	result.ProjectID = binding.ProjectID
	result.Clients = configuredClients(root)
	return result
}

func inspectLocalRecord(record localRecord) LocalFolder {
	result := LocalFolder{
		Root: record.Root, Name: record.Name, ServerURL: record.ServerURL,
		ProjectID: record.ProjectID, Clients: append([]string(nil), record.Clients...),
		Configured: record.ConfiguredAt,
	}
	info, err := os.Stat(record.Root)
	if err != nil || !info.IsDir() {
		result.Status = "La carpeta ya no está disponible"
		return result
	}
	inspection := inspectLocalFolder(record.Root)
	if !inspection.Connected {
		result.Status = inspection.Error
		return result
	}
	result.Available = true
	result.Name = inspection.Name
	result.ServerURL = inspection.ServerURL
	result.ProjectID = inspection.ProjectID
	result.Clients = inspection.Clients
	return result
}

func configuredClients(root string) []string {
	clients := make([]string, 0, 2)
	if fileContains(filepath.Join(root, ".codex", "config.toml"), "# >>> PACT managed Codex MCP") {
		clients = append(clients, "codex")
	}
	if fileContains(filepath.Join(root, ".mcp.json"), "pact.enable.claude/v1") {
		clients = append(clients, "claude")
	}
	return clients
}

func fileContains(path, marker string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 1<<20))
	return err == nil && bytes.Contains(content, []byte(marker))
}

func detectLocalClients(folders []LocalFolder) []LocalClientStatus {
	clients := []LocalClientStatus{
		{ID: "codex", Name: "Codex"},
		{ID: "claude", Name: "Claude Code"},
	}
	for index := range clients {
		if executable, err := exec.LookPath(clients[index].ID); err == nil {
			clients[index].Detected = true
			clients[index].Detection = executable
		}
		if clients[index].ID == "codex" && !clients[index].Detected && applicationExists("Codex.app") {
			clients[index].Detected = true
			clients[index].Detection = "Codex.app"
		}
		for _, folder := range folders {
			for _, configured := range folder.Clients {
				if configured == clients[index].ID {
					clients[index].ConnectedFolders++
				}
			}
		}
	}
	return clients
}

func applicationExists(name string) bool {
	if goruntime.GOOS != "darwin" {
		return false
	}
	paths := []string{filepath.Join("/Applications", name)}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, "Applications", name))
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func ensureLocalRuntime() (string, string, error) {
	name := "pact-local"
	if goruntime.GOOS == "windows" {
		name += ".exe"
	}
	payload, err := localHelperAssets.ReadFile(filepath.ToSlash(filepath.Join("localhelper", name)))
	if err != nil || len(payload) == 0 {
		return "", "", errors.New("the PACT local runtime is not bundled in this development build")
	}
	digest := sha256.Sum256(payload)
	version := hex.EncodeToString(digest[:])[:12]
	configDirectory, err := desktopConfigDirectory()
	if err != nil {
		return "", "", fmt.Errorf("resolve local application directory: %w", err)
	}
	directory := filepath.Join(configDirectory, "runtime", version)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", fmt.Errorf("create local runtime directory: %w", err)
	}
	path := filepath.Join(directory, name)
	if current, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(current, payload) {
		if goruntime.GOOS != "windows" {
			_ = os.Chmod(path, 0o700)
		}
		return path, version, nil
	}
	if err := writeLocalAtomic(path, payload, 0o700); err != nil {
		return "", "", fmt.Errorf("install local PACT runtime: %w", err)
	}
	return path, version, nil
}

func rememberLocalConnection(binding localproject.Binding, clientID string) error {
	state, err := loadLocalState()
	if err != nil {
		return err
	}
	if state.Folders == nil {
		state.Folders = make(map[string]localRecord)
	}
	record := state.Folders[binding.Root]
	record.Root = binding.Root
	record.Name = filepath.Base(binding.Root)
	if descriptor, describeErr := localproject.Describe(binding.Root); describeErr == nil && descriptor.Name != "" {
		record.Name = descriptor.Name
	}
	record.ServerURL = binding.ServerURL
	record.ProjectID = binding.ProjectID
	record.ConfiguredAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !containsString(record.Clients, clientID) {
		record.Clients = append(record.Clients, clientID)
		sort.Strings(record.Clients)
	}
	state.Folders[binding.Root] = record
	return saveLocalState(state)
}

func loadLocalState() (localState, error) {
	path, err := localStatePath()
	if err != nil {
		return localState{SchemaVersion: localStateSchemaVersion, Folders: make(map[string]localRecord)}, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return localState{SchemaVersion: localStateSchemaVersion, Folders: make(map[string]localRecord)}, nil
	}
	if err != nil {
		return localState{}, fmt.Errorf("read desktop local state: %w", err)
	}
	var state localState
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return localState{}, fmt.Errorf("decode desktop local state: %w", err)
	}
	if state.SchemaVersion != localStateSchemaVersion {
		return localState{}, fmt.Errorf("unsupported desktop local state version %d", state.SchemaVersion)
	}
	if state.Folders == nil {
		state.Folders = make(map[string]localRecord)
	}
	return state, nil
}

func saveLocalState(state localState) error {
	path, err := localStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create desktop local state directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode desktop local state: %w", err)
	}
	return writeLocalAtomic(path, append(payload, '\n'), 0o600)
}

func localStatePath() (string, error) {
	directory, err := desktopConfigDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "desktop-local.json"), nil
}

func desktopConfigDirectory() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PACT_DESKTOP_CONFIG_DIR")); override != "" {
		absolute, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve PACT_DESKTOP_CONFIG_DIR: %w", err)
		}
		return absolute, nil
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user configuration directory: %w", err)
	}
	return filepath.Join(directory, "Pact"), nil
}

func writeLocalAtomic(path string, payload []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pact-desktop-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func joinLocalError(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}
