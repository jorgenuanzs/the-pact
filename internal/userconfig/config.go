package userconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jorgenuanzs/the-pact/internal/credentialstore"
	"github.com/jorgenuanzs/the-pact/internal/serverprofile"
)

const schemaVersion = serverprofile.SchemaVersion

// Config is the compatibility view used by existing CLI, Desktop and runtime
// code. It is never persisted directly: the registry stores profile metadata
// and the device credential remains in CredentialStore.
type Config struct {
	SchemaVersion    int    `json:"schema_version"`
	ServerURL        string `json:"server_url"`
	DeviceCredential string `json:"-"`
}

type PrincipalMetadata struct {
	ID    string
	Label string
}

type AuthorizedMetadata struct {
	ProfileLabel   string
	PrincipalID    string
	PrincipalLabel string
}

func Load() (Config, error) {
	manager, _, err := profileManager()
	if err != nil {
		return Config{}, err
	}
	profile, err := manager.Active()
	if errors.Is(err, serverprofile.ErrNoActiveProfile) {
		return Config{}, errors.New("not logged in; run pact login --server <url>")
	}
	if err != nil {
		return Config{}, err
	}
	return Config{
		SchemaVersion:    schemaVersion,
		ServerURL:        profile.ServerURL,
		DeviceCredential: profile.DeviceCredential,
	}, nil
}

func Save(serverURL, deviceCredential string) (string, error) {
	return SaveAuthorized(serverURL, deviceCredential, PrincipalMetadata{})
}

func SaveAuthorized(serverURL, deviceCredential string, principal PrincipalMetadata) (string, error) {
	return SaveAuthorizedProfile(serverURL, deviceCredential, AuthorizedMetadata{
		PrincipalID: principal.ID, PrincipalLabel: principal.Label,
	})
}

func SaveAuthorizedProfile(serverURL, deviceCredential string, metadata AuthorizedMetadata) (string, error) {
	manager, path, err := profileManager()
	if err != nil {
		return "", err
	}
	if _, err := manager.UpsertAuthorized(serverprofile.AuthorizedInput{
		ServerURL:        serverURL,
		Label:            strings.TrimSpace(metadata.ProfileLabel),
		PrincipalID:      strings.TrimSpace(metadata.PrincipalID),
		PrincipalLabel:   strings.TrimSpace(metadata.PrincipalLabel),
		DeviceCredential: deviceCredential,
	}); err != nil {
		return "", err
	}
	return path, nil
}

// Delete removes only the active profile. Other authorized servers remain
// available and the most recently used remaining profile becomes active.
func Delete() error {
	manager, _, err := profileManager()
	if err != nil {
		return err
	}
	active, err := manager.Active()
	if errors.Is(err, serverprofile.ErrNoActiveProfile) {
		return nil
	}
	if err != nil {
		return err
	}
	return manager.Remove(active.ID)
}

func ListProfiles() ([]serverprofile.Profile, error) {
	manager, _, err := profileManager()
	if err != nil {
		return nil, err
	}
	return manager.List()
}

func ActiveProfile() (serverprofile.Profile, error) {
	manager, _, err := profileManager()
	if err != nil {
		return serverprofile.Profile{}, err
	}
	return manager.ActiveMetadata()
}

func FindProfileByURL(serverURL string) (serverprofile.Profile, error) {
	manager, _, err := profileManager()
	if err != nil {
		return serverprofile.Profile{}, err
	}
	return manager.FindByURL(serverURL)
}

func FindProfile(identifier string) (serverprofile.Profile, error) {
	manager, _, err := profileManager()
	if err != nil {
		return serverprofile.Profile{}, err
	}
	return manager.Get(identifier)
}

func AuthorizedForServer(serverURL string) (serverprofile.AuthorizedProfile, error) {
	manager, _, err := profileManager()
	if err != nil {
		return serverprofile.AuthorizedProfile{}, err
	}
	return manager.AuthorizedForURL(serverURL)
}

func AuthorizedProfile(identifier string) (serverprofile.AuthorizedProfile, error) {
	manager, _, err := profileManager()
	if err != nil {
		return serverprofile.AuthorizedProfile{}, err
	}
	return manager.Authorized(identifier)
}

func SetActiveProfile(identifier string) error {
	manager, _, err := profileManager()
	if err != nil {
		return err
	}
	return manager.SetActive(identifier)
}

func RemoveProfile(identifier string) error {
	manager, _, err := profileManager()
	if err != nil {
		return err
	}
	return manager.Remove(identifier)
}

func NormalizeServerURL(raw string) (string, error) {
	return serverprofile.NormalizeServerURL(raw)
}

func profileManager() (*serverprofile.Manager, string, error) {
	path, err := configPath()
	if err != nil {
		return nil, "", err
	}
	store, err := credentialstore.Default(filepath.Dir(path))
	if err != nil {
		return nil, "", err
	}
	return serverprofile.NewManager(path, store), path, nil
}

func configPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PACT_CONFIG_DIR")); override != "" {
		absolute, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve PACT_CONFIG_DIR: %w", err)
		}
		return filepath.Join(absolute, "config.json"), nil
	}
	return defaultConfigPath()
}
