package localproject

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/atomicfile"
	"github.com/jorgenuanzs/the-pact/internal/filelock"
)

const LocalBindingSchemaVersion = 2

type localConfig struct {
	SchemaVersion        int        `json:"schema_version"`
	ServerURL            string     `json:"server_url"`
	WorkspaceID          string     `json:"workspace_id,omitempty"`
	RepositoryID         string     `json:"repository_id,omitempty"`
	ProjectID            string     `json:"project_id,omitempty"`
	GitRemoteFingerprint string     `json:"git_remote_fingerprint,omitempty"`
	ConfiguredAt         *time.Time `json:"configured_at,omitempty"`
}

type Binding struct {
	SchemaVersion        int
	Root                 string
	ServerURL            string
	WorkspaceID          string
	RepositoryID         string
	ProjectID            string
	GitRemoteFingerprint string
	ConfiguredAt         time.Time
	NeedsMigration       bool
}

type BindOptions struct {
	ServerURL    string
	WorkspaceID  string
	RepositoryID string
	ProjectID    string
	Rebind       bool
}

// Bind writes only schema v2. A legacy v1 binding is enriched in place when
// the resolved project is unchanged; changing any existing destination
// requires the explicit Rebind option.
func Bind(startPath string, options BindOptions) (Binding, error) {
	return bind(startPath, options, writeLocalConfig)
}

func bind(
	startPath string,
	options BindOptions,
	writeConfig func(string, localConfig) error,
) (Binding, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return Binding{}, err
	}
	serverURL, err := normalizeServerURL(options.ServerURL)
	if err != nil {
		return Binding{}, err
	}
	if !validUUID(strings.TrimSpace(options.WorkspaceID)) {
		return Binding{}, errors.New("remote workspace ID must be a UUID")
	}
	if !validUUID(strings.TrimSpace(options.RepositoryID)) {
		return Binding{}, errors.New("remote repository ID must be a UUID")
	}
	if !validUUID(strings.TrimSpace(options.ProjectID)) {
		return Binding{}, errors.New("remote project ID must be a UUID")
	}
	fingerprint, err := GitRemoteFingerprint(root)
	if err != nil {
		return Binding{}, err
	}
	configPath := filepath.Join(root, localDirectory, localConfigName)
	release, err := acquireBindingLock(configPath)
	if err != nil {
		return Binding{}, err
	}
	defer release()

	current, err := readLocalConfig(configPath)
	if err != nil {
		return Binding{}, err
	}
	currentServer, err := normalizeServerURL(current.ServerURL)
	if err != nil {
		return Binding{}, fmt.Errorf("invalid server URL in local Pact configuration: %w", err)
	}
	serverChanged := currentServer != serverURL
	if serverChanged && !options.Rebind {
		return Binding{}, fmt.Errorf(
			"this checkout is already bound to %s; use pact connect --rebind --server %s to change it",
			currentServer, serverURL,
		)
	}

	switch current.SchemaVersion {
	case 1:
		if current.WorkspaceID != "" || current.RepositoryID != "" || current.GitRemoteFingerprint != "" || current.ConfiguredAt != nil {
			return Binding{}, errors.New("legacy local Pact configuration contains unexpected v2 fields")
		}
		if current.ProjectID != "" && current.ProjectID != options.ProjectID && !options.Rebind {
			return Binding{}, fmt.Errorf(
				"this checkout is already connected to project %s; use pact connect --rebind to change it",
				current.ProjectID,
			)
		}
	case LocalBindingSchemaVersion:
		complete, shapeErr := completeV2Config(current)
		if shapeErr != nil {
			return Binding{}, shapeErr
		}
		if complete {
			sameDestination := currentServer == serverURL &&
				current.WorkspaceID == options.WorkspaceID &&
				current.RepositoryID == options.RepositoryID &&
				current.ProjectID == options.ProjectID
			sameRemote := current.GitRemoteFingerprint == fingerprint
			if (!sameDestination || !sameRemote) && !options.Rebind {
				return Binding{}, errors.New(
					"this checkout already has a different PACT binding; use pact connect --rebind to replace it",
				)
			}
			if sameDestination && sameRemote {
				if err := reconcileExistingNodeIdentity(root, serverURL, false); err != nil {
					return Binding{}, err
				}
				return loadBindingFromConfig(root, current)
			}
		}
	default:
		return Binding{}, fmt.Errorf("unsupported local Pact schema version %d", current.SchemaVersion)
	}

	configuredAt := time.Now().UTC()
	target := localConfig{
		SchemaVersion:        LocalBindingSchemaVersion,
		ServerURL:            serverURL,
		WorkspaceID:          strings.TrimSpace(options.WorkspaceID),
		RepositoryID:         strings.TrimSpace(options.RepositoryID),
		ProjectID:            strings.TrimSpace(options.ProjectID),
		GitRemoteFingerprint: fingerprint,
		ConfiguredAt:         &configuredAt,
	}
	if err := writeConfig(configPath, target); err != nil {
		return Binding{}, fmt.Errorf("write local Pact binding: %w", err)
	}
	if err := reconcileExistingNodeIdentity(root, serverURL, serverChanged); err != nil {
		return Binding{}, fmt.Errorf("binding was updated but node identity could not be reconciled: %w", err)
	}
	return loadBindingFromConfig(root, target)
}

func LoadBinding(startPath string) (Binding, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return Binding{}, err
	}
	configPath := filepath.Join(root, localDirectory, localConfigName)
	config, err := readLocalConfig(configPath)
	if err != nil {
		return Binding{}, err
	}
	return loadBindingFromConfig(root, config)
}

// FindBinding distinguishes an absent binding from malformed or incomplete
// state. Callers must never treat an invalid existing file as permission to
// fall back to another server profile.
func FindBinding(startPath string) (Binding, bool, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return Binding{}, false, nil
	}
	configPath := filepath.Join(root, localDirectory, localConfigName)
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		return Binding{}, false, nil
	} else if err != nil {
		return Binding{}, true, fmt.Errorf("inspect local Pact configuration: %w", err)
	}
	binding, err := LoadBinding(root)
	return binding, true, err
}

func GitRemoteFingerprint(startPath string) (string, error) {
	root, err := FindRoot(startPath)
	if err != nil {
		return "", err
	}
	remote, err := gitOutput(root, "config", "--get", "remote.origin.url")
	if err != nil || strings.TrimSpace(remote) == "" {
		return "", errors.New("Git remote origin is required before binding the checkout to PACT")
	}
	normalized, err := NormalizeGitRemote(remote)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func loadBindingFromConfig(root string, config localConfig) (Binding, error) {
	serverURL, err := normalizeServerURL(config.ServerURL)
	if err != nil {
		return Binding{}, err
	}
	binding := Binding{
		SchemaVersion: config.SchemaVersion, Root: root, ServerURL: serverURL,
		WorkspaceID: config.WorkspaceID, RepositoryID: config.RepositoryID,
		ProjectID: config.ProjectID, GitRemoteFingerprint: config.GitRemoteFingerprint,
	}
	if config.ConfiguredAt != nil {
		binding.ConfiguredAt = *config.ConfiguredAt
	}
	switch config.SchemaVersion {
	case 1:
		if !validUUID(config.ProjectID) {
			return Binding{}, errors.New("project is not connected to a valid remote project")
		}
		if config.WorkspaceID != "" || config.RepositoryID != "" || config.GitRemoteFingerprint != "" || config.ConfiguredAt != nil {
			return Binding{}, errors.New("legacy local Pact configuration contains unexpected v2 fields")
		}
		binding.NeedsMigration = true
		if fingerprint, fingerprintErr := GitRemoteFingerprint(root); fingerprintErr == nil {
			binding.GitRemoteFingerprint = fingerprint
		}
	case LocalBindingSchemaVersion:
		complete, err := completeV2Config(config)
		if err != nil {
			return Binding{}, err
		}
		if !complete {
			return Binding{}, errors.New("project binding is incomplete; finish pact init or pact connect")
		}
		currentFingerprint, err := GitRemoteFingerprint(root)
		if err != nil {
			return Binding{}, err
		}
		if currentFingerprint != config.GitRemoteFingerprint {
			return Binding{}, errors.New(
				"Git remote origin changed after this checkout was bound; run pact connect --rebind to confirm the new destination",
			)
		}
	default:
		return Binding{}, fmt.Errorf("unsupported local Pact schema version %d", config.SchemaVersion)
	}
	if err := ensurePlatformGitConfig(root); err != nil {
		return Binding{}, err
	}
	configPath := filepath.Join(root, localDirectory, localConfigName)
	if err := os.Chmod(configPath, 0o600); err != nil {
		return Binding{}, fmt.Errorf("secure local Pact configuration: %w", err)
	}
	return binding, nil
}

func completeV2Config(config localConfig) (bool, error) {
	values := []string{config.WorkspaceID, config.RepositoryID, config.ProjectID, config.GitRemoteFingerprint}
	present := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			present++
		}
	}
	if present == 0 && config.ConfiguredAt == nil {
		return false, nil
	}
	if present != len(values) || config.ConfiguredAt == nil || config.ConfiguredAt.IsZero() {
		return false, errors.New("local Pact binding v2 is partially configured")
	}
	if !validUUID(config.WorkspaceID) || !validUUID(config.RepositoryID) || !validUUID(config.ProjectID) {
		return false, errors.New("local Pact binding v2 contains an invalid workspace, repository, or project ID")
	}
	if !validRemoteFingerprint(config.GitRemoteFingerprint) {
		return false, errors.New("local Pact binding v2 contains an invalid Git remote fingerprint")
	}
	return true, nil
}

func validRemoteFingerprint(value string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil
}

func readLocalConfig(path string) (localConfig, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return localConfig{}, errors.New("project is not connected; run pact init or pact connect")
	}
	if err != nil {
		return localConfig{}, fmt.Errorf("read local Pact configuration: %w", err)
	}
	var config localConfig
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return localConfig{}, fmt.Errorf("decode local Pact configuration: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return localConfig{}, errors.New("decode local Pact configuration: unexpected trailing data")
	}
	return config, nil
}

func writeLocalConfig(path string, config localConfig) error {
	payload, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode local Pact configuration: %w", err)
	}
	payload = append(payload, '\n')
	return atomicfile.Write(path, payload, 0o600)
}

func acquireBindingLock(configPath string) (func(), error) {
	release, err := filelock.Acquire(configPath + ".lock")
	if err != nil {
		return nil, fmt.Errorf("lock local Pact binding: %w", err)
	}
	return func() { _ = release() }, nil
}
