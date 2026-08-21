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
	"github.com/jorgenuanzs/the-pact/internal/gitremote"
	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/localserver"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/repositorybinding"
	"github.com/jorgenuanzs/the-pact/internal/userconfig"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const localStateSchemaVersion = 2

type DesktopServerProfile struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	ServerURL      string `json:"server_url"`
	Kind           string `json:"kind"`
	PrincipalLabel string `json:"principal_label,omitempty"`
	Active         bool   `json:"active"`
}

type LocalClientStatus struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Detected         bool   `json:"detected"`
	Detection        string `json:"detection,omitempty"`
	ConnectedFolders int    `json:"connected_folders"`
}

type LocalFolder struct {
	Root         string   `json:"root"`
	Name         string   `json:"name"`
	ProfileID    string   `json:"profile_id,omitempty"`
	ServerURL    string   `json:"server_url"`
	WorkspaceID  string   `json:"workspace_id,omitempty"`
	RepositoryID string   `json:"repository_id,omitempty"`
	ProjectID    string   `json:"project_id"`
	Clients      []string `json:"clients"`
	Available    bool     `json:"available"`
	Status       string   `json:"status,omitempty"`
	Configured   string   `json:"configured_at,omitempty"`
}

type LocalComputerStatus struct {
	Hostname        string                 `json:"hostname"`
	OperatingSystem string                 `json:"operating_system"`
	Architecture    string                 `json:"architecture"`
	RuntimeReady    bool                   `json:"runtime_ready"`
	RuntimePath     string                 `json:"runtime_path,omitempty"`
	RuntimeVersion  string                 `json:"runtime_version,omitempty"`
	RuntimeError    string                 `json:"runtime_error,omitempty"`
	ServerURL       string                 `json:"server_url,omitempty"`
	ActiveProfileID string                 `json:"active_profile_id,omitempty"`
	Profiles        []DesktopServerProfile `json:"profiles"`
	Clients         []LocalClientStatus    `json:"clients"`
	Folders         []LocalFolder          `json:"folders"`
	ManagedServer   localserver.Status     `json:"managed_server"`
}

type LocalFolderInspection struct {
	Canceled     bool     `json:"canceled"`
	Connected    bool     `json:"connected"`
	Root         string   `json:"root,omitempty"`
	Name         string   `json:"name,omitempty"`
	RemoteURL    string   `json:"remote_url,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	Revision     string   `json:"revision,omitempty"`
	ProfileID    string   `json:"profile_id,omitempty"`
	ServerURL    string   `json:"server_url,omitempty"`
	WorkspaceID  string   `json:"workspace_id,omitempty"`
	RepositoryID string   `json:"repository_id,omitempty"`
	ProjectID    string   `json:"project_id,omitempty"`
	Clients      []string `json:"clients,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type ResolveLocalFolderInput struct {
	ProjectRoot string `json:"project_root"`
	ProfileID   string `json:"profile_id"`
}

type LocalFolderResolution struct {
	Folder     LocalFolderInspection     `json:"folder"`
	Profile    DesktopServerProfile      `json:"profile"`
	Workspaces []workspaces.Workspace    `json:"workspaces"`
	Matches    []repositorybinding.Match `json:"matches"`
}

type BindLocalFolderInput struct {
	ProjectRoot    string   `json:"project_root"`
	ProfileID      string   `json:"profile_id"`
	WorkspaceID    string   `json:"workspace_id"`
	ProjectID      string   `json:"project_id,omitempty"`
	RepositoryID   string   `json:"repository_id,omitempty"`
	CreateIfNeeded bool     `json:"create_if_needed"`
	Rebind         bool     `json:"rebind"`
	Clients        []string `json:"clients"`
}

type BindLocalFolderResult struct {
	Folder  LocalFolderInspection     `json:"folder"`
	Clients []ConnectLocalAgentResult `json:"clients"`
	Created bool                      `json:"created"`
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
	ProfileID    string   `json:"profile_id,omitempty"`
	ServerURL    string   `json:"server_url"`
	WorkspaceID  string   `json:"workspace_id,omitempty"`
	RepositoryID string   `json:"repository_id,omitempty"`
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
		Profiles:        make([]DesktopServerProfile, 0),
		Clients:         make([]LocalClientStatus, 0),
		Folders:         make([]LocalFolder, 0),
	}
	if profiles, profileErr := userconfig.ListProfiles(); profileErr == nil {
		active, _ := userconfig.ActiveProfile()
		result.ActiveProfileID = active.ID
		for _, profile := range profiles {
			presentation := desktopServerProfile(profile.ID, profile.Label, profile.ServerURL, string(profile.Kind), profile.PrincipalLabel, profile.ID == active.ID)
			result.Profiles = append(result.Profiles, presentation)
			if presentation.Active {
				result.ServerURL = presentation.ServerURL
			}
		}
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
	app := d.application()
	if app == nil {
		return LocalFolderInspection{}, errors.New("desktop window is not ready")
	}
	selected, err := app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                "Selecciona una carpeta Git",
		CanChooseDirectories: true,
		CanChooseFiles:       false,
		CanCreateDirectories: false,
		ResolvesAliases:      true,
	}).PromptForSingleSelection()
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

func (d *Desktop) ListServerProfiles() ([]DesktopServerProfile, error) {
	profiles, err := userconfig.ListProfiles()
	if err != nil {
		return nil, err
	}
	active, _ := userconfig.ActiveProfile()
	result := make([]DesktopServerProfile, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, desktopServerProfile(
			profile.ID, profile.Label, profile.ServerURL, string(profile.Kind),
			profile.PrincipalLabel, profile.ID == active.ID,
		))
	}
	return result, nil
}

func (d *Desktop) UseServerProfile(identifier string) (DesktopStatus, error) {
	if err := userconfig.SetActiveProfile(strings.TrimSpace(identifier)); err != nil {
		return DesktopStatus{}, err
	}
	return d.Status(), nil
}

func (d *Desktop) ResolveLocalFolder(input ResolveLocalFolderInput) (LocalFolderResolution, error) {
	inspection := inspectLocalFolder(input.ProjectRoot)
	if inspection.Error != "" {
		return LocalFolderResolution{}, errors.New(inspection.Error)
	}
	profile, err := userconfig.AuthorizedProfile(strings.TrimSpace(input.ProfileID))
	if err != nil {
		return LocalFolderResolution{}, fmt.Errorf("la conexión PACT seleccionada no está autorizada: %w", err)
	}
	client, err := pactclient.New(profile.ServerURL, profile.DeviceCredential)
	if err != nil {
		return LocalFolderResolution{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	workspaceList, err := client.ListWorkspaces(ctx)
	if err != nil {
		return LocalFolderResolution{}, fmt.Errorf("cargar workspaces de %s: %w", profile.Label, err)
	}
	matches, err := client.ResolveRepositoryBinding(ctx, repositorybinding.ResolveInput{RemoteURL: inspection.RemoteURL})
	if err != nil {
		return LocalFolderResolution{}, fmt.Errorf("resolver repositorio en %s: %w", profile.Label, err)
	}
	inspection.ProfileID = profile.ID
	return LocalFolderResolution{
		Folder:     inspection,
		Profile:    desktopServerProfile(profile.ID, profile.Label, profile.ServerURL, string(profile.Kind), profile.PrincipalLabel, true),
		Workspaces: workspaceList,
		Matches:    matches,
	}, nil
}

func (d *Desktop) BindLocalFolder(input BindLocalFolderInput) (BindLocalFolderResult, error) {
	checkout, err := localproject.InspectCheckout(strings.TrimSpace(input.ProjectRoot))
	if err != nil {
		return BindLocalFolderResult{}, err
	}
	profile, err := userconfig.AuthorizedProfile(strings.TrimSpace(input.ProfileID))
	if err != nil {
		return BindLocalFolderResult{}, fmt.Errorf("la conexión PACT seleccionada no está autorizada: %w", err)
	}
	client, err := pactclient.New(profile.ServerURL, profile.DeviceCredential)
	if err != nil {
		return BindLocalFolderResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	projectID := strings.TrimSpace(input.ProjectID)
	repositoryID := strings.TrimSpace(input.RepositoryID)
	created := false
	if projectID == "" || repositoryID == "" {
		if !input.CreateIfNeeded {
			return BindLocalFolderResult{}, errors.New("selecciona un repositorio registrado o autoriza su creación en el workspace")
		}
		project, wasCreated, resolveErr := ensureDesktopProject(ctx, client, checkout)
		if resolveErr != nil {
			return BindLocalFolderResult{}, resolveErr
		}
		if _, attachErr := client.AttachWorkspaceProject(ctx, strings.TrimSpace(input.WorkspaceID), project.ID); attachErr != nil {
			return BindLocalFolderResult{}, fmt.Errorf("vincular proyecto al workspace: %w", attachErr)
		}
		if project.RootRepository == nil {
			return BindLocalFolderResult{}, errors.New("el proyecto PACT no tiene un repositorio raíz")
		}
		projectID = project.ID
		repositoryID = project.RootRepository.ID
		created = wasCreated
	} else {
		matches, resolveErr := client.ResolveRepositoryBinding(ctx, repositorybinding.ResolveInput{
			RemoteURL: checkout.RemoteURL, WorkspaceID: strings.TrimSpace(input.WorkspaceID),
		})
		if resolveErr != nil {
			return BindLocalFolderResult{}, resolveErr
		}
		if !containsBindingMatch(matches, projectID, repositoryID) {
			return BindLocalFolderResult{}, errors.New("el repositorio seleccionado ya no coincide con esta carpeta o ya no es accesible")
		}
	}

	if _, err := localproject.Init(localproject.InitOptions{
		StartPath: checkout.Root, Name: checkout.Name, ServerURL: profile.ServerURL,
		AllowServerChange: input.Rebind,
	}); err != nil {
		return BindLocalFolderResult{}, err
	}
	binding, err := localproject.Bind(checkout.Root, localproject.BindOptions{
		ServerURL: profile.ServerURL, WorkspaceID: strings.TrimSpace(input.WorkspaceID),
		RepositoryID: repositoryID, ProjectID: projectID, Rebind: input.Rebind,
	})
	if err != nil {
		return BindLocalFolderResult{}, err
	}
	results := make([]ConnectLocalAgentResult, 0, len(input.Clients))
	for _, clientID := range uniqueLocalClients(input.Clients) {
		configured, configureErr := connectLocalAgent(binding, clientID)
		if configureErr != nil {
			return BindLocalFolderResult{}, fmt.Errorf("configurar %s: %w", clientID, configureErr)
		}
		results = append(results, configured)
	}
	if err := rememberLocalBinding(binding, profile.ID, uniqueLocalClients(input.Clients)); err != nil {
		return BindLocalFolderResult{}, err
	}
	return BindLocalFolderResult{Folder: inspectLocalFolder(binding.Root), Clients: results, Created: created}, nil
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
	profile, err := userconfig.FindProfileByURL(binding.ServerURL)
	if err != nil {
		return ConnectLocalAgentResult{}, fmt.Errorf("esta carpeta pertenece a %s, pero este computador no tiene una conexión autorizada para ese servidor", binding.ServerURL)
	}
	result, err := connectLocalAgent(binding, clientID)
	if err != nil {
		return ConnectLocalAgentResult{}, err
	}
	if err := rememberLocalBinding(binding, profile.ID, configuredClients(binding.Root)); err != nil {
		return ConnectLocalAgentResult{}, err
	}
	return result, nil
}

func connectLocalAgent(binding localproject.Binding, clientID string) (ConnectLocalAgentResult, error) {
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
	return result, nil
}

func inspectLocalFolder(path string) LocalFolderInspection {
	checkout, err := localproject.InspectCheckout(strings.TrimSpace(path))
	if err != nil {
		return LocalFolderInspection{Error: err.Error()}
	}
	result := LocalFolderInspection{
		Root: checkout.Root, Name: checkout.Name, RemoteURL: checkout.RemoteURL,
		Branch: checkout.DefaultBranch, Revision: checkout.CanonicalRevision,
		Clients: configuredClients(checkout.Root),
	}
	if descriptor, describeErr := localproject.Describe(checkout.Root); describeErr == nil && strings.TrimSpace(descriptor.Name) != "" {
		result.Name = descriptor.Name
	}
	binding, found, err := localproject.FindBinding(checkout.Root)
	if !found {
		return result
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Connected = true
	result.ServerURL = binding.ServerURL
	result.WorkspaceID = binding.WorkspaceID
	result.RepositoryID = binding.RepositoryID
	result.ProjectID = binding.ProjectID
	if profile, profileErr := userconfig.FindProfileByURL(binding.ServerURL); profileErr == nil {
		result.ProfileID = profile.ID
	}
	return result
}

func inspectLocalRecord(record localRecord) LocalFolder {
	result := LocalFolder{
		Root: record.Root, Name: record.Name, ProfileID: record.ProfileID, ServerURL: record.ServerURL,
		WorkspaceID: record.WorkspaceID, RepositoryID: record.RepositoryID,
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
	result.ProfileID = inspection.ProfileID
	result.ServerURL = inspection.ServerURL
	result.WorkspaceID = inspection.WorkspaceID
	result.RepositoryID = inspection.RepositoryID
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

func rememberLocalBinding(binding localproject.Binding, profileID string, clients []string) error {
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
	record.ProfileID = profileID
	record.ServerURL = binding.ServerURL
	record.WorkspaceID = binding.WorkspaceID
	record.RepositoryID = binding.RepositoryID
	record.ProjectID = binding.ProjectID
	record.ConfiguredAt = time.Now().UTC().Format(time.RFC3339Nano)
	record.Clients = uniqueLocalClients(append(record.Clients, clients...))
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
	if err := json.Unmarshal(content, &state); err != nil {
		return localState{}, fmt.Errorf("decode desktop local state: %w", err)
	}
	if state.SchemaVersion != 1 && state.SchemaVersion != localStateSchemaVersion {
		return localState{}, fmt.Errorf("unsupported desktop local state version %d", state.SchemaVersion)
	}
	state.SchemaVersion = localStateSchemaVersion
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

func uniqueLocalClients(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		clientID := strings.ToLower(strings.TrimSpace(value))
		if (clientID != "codex" && clientID != "claude") || seen[clientID] {
			continue
		}
		seen[clientID] = true
		result = append(result, clientID)
	}
	sort.Strings(result)
	return result
}

func desktopServerProfile(id, label, serverURL, kind, principalLabel string, active bool) DesktopServerProfile {
	return DesktopServerProfile{
		ID: id, Label: label, ServerURL: serverURL, Kind: kind,
		PrincipalLabel: principalLabel, Active: active,
	}
}

func containsBindingMatch(matches []repositorybinding.Match, projectID, repositoryID string) bool {
	for _, match := range matches {
		if match.ProjectID == projectID && match.RepositoryID == repositoryID {
			return true
		}
	}
	return false
}

func ensureDesktopProject(
	ctx context.Context,
	client *pactclient.Client,
	checkout localproject.Checkout,
) (projects.Project, bool, error) {
	projectList, err := client.ListProjects(ctx)
	if err != nil {
		return projects.Project{}, false, err
	}
	for _, project := range projectList {
		if project.RootRepository == nil || project.RootRepository.RemoteURL == nil {
			continue
		}
		registered, normalizeErr := gitremote.Normalize(*project.RootRepository.RemoteURL)
		if normalizeErr == nil && registered == checkout.RemoteURL {
			return project, false, nil
		}
	}
	revision := checkout.CanonicalRevision
	input := projects.CreateInput{
		Name: checkout.Name, Slug: checkout.Slug, CanonicalRevision: &revision,
		RootRepository: &projects.SourceRepositoryInput{
			Slug: "primary", Name: "Primary", RemoteURL: checkout.RemoteURL,
			DefaultBranch: checkout.DefaultBranch, ObjectFormat: checkout.ObjectFormat,
		},
	}
	digest := sha256.Sum256([]byte("desktop.project.init\x00" + checkout.RemoteURL))
	project, err := client.CreateProject(ctx, "pact-desktop-init-"+hex.EncodeToString(digest[:]), input)
	if err != nil {
		return projects.Project{}, false, fmt.Errorf("registrar repositorio en PACT Server: %w", err)
	}
	return project, true, nil
}

func joinLocalError(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}
