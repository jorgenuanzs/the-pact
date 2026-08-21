package serverprofile

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/atomicfile"
	"github.com/jorgenuanzs/the-pact/internal/credentialstore"
	"github.com/jorgenuanzs/the-pact/internal/filelock"
)

var (
	ErrNoActiveProfile = errors.New("no active PACT Server profile")
	ErrProfileNotFound = errors.New("PACT Server profile not found")
	managerLocks       sync.Map
)

type Manager struct {
	path        string
	credentials credentialstore.Store
	now         func() time.Time
}

func NewManager(path string, credentials credentialstore.Store) *Manager {
	return &Manager{path: path, credentials: credentials, now: time.Now}
}

func (m *Manager) List() ([]Profile, error) {
	state, err := m.load()
	if err != nil {
		return nil, err
	}
	profiles := append([]Profile(nil), state.Profiles...)
	sort.SliceStable(profiles, func(i, j int) bool {
		return profiles[i].LastUsedAt.After(profiles[j].LastUsedAt)
	})
	return profiles, nil
}

func (m *Manager) Get(identifier string) (Profile, error) {
	state, err := m.load()
	if err != nil {
		return Profile{}, err
	}
	index, err := profileIndex(state, identifier)
	if err != nil {
		return Profile{}, err
	}
	return state.Profiles[index], nil
}

func (m *Manager) FindByURL(serverURL string) (Profile, error) {
	normalized, err := NormalizeServerURL(serverURL)
	if err != nil {
		return Profile{}, err
	}
	return m.Get(normalized)
}

func (m *Manager) Active() (AuthorizedProfile, error) {
	profile, err := m.ActiveMetadata()
	if err != nil {
		return AuthorizedProfile{}, err
	}
	return m.authorize(profile)
}

// ActiveMetadata returns the preferred profile without reading its secret.
// Listing and selecting profiles must keep working even when a native
// credential store entry has been removed outside PACT.
func (m *Manager) ActiveMetadata() (Profile, error) {
	state, err := m.load()
	if err != nil {
		return Profile{}, err
	}
	if state.ActiveProfileID == "" {
		return Profile{}, ErrNoActiveProfile
	}
	index, err := profileIndex(state, state.ActiveProfileID)
	if err != nil {
		return Profile{}, ErrNoActiveProfile
	}
	return state.Profiles[index], nil
}

func (m *Manager) AuthorizedForURL(serverURL string) (AuthorizedProfile, error) {
	profile, err := m.FindByURL(serverURL)
	if err != nil {
		return AuthorizedProfile{}, err
	}
	return m.authorize(profile)
}

// Authorized resolves a profile ID or normalized server URL and retrieves its
// device credential. It never falls back to the active profile.
func (m *Manager) Authorized(identifier string) (AuthorizedProfile, error) {
	profile, err := m.Get(identifier)
	if err != nil {
		return AuthorizedProfile{}, err
	}
	return m.authorize(profile)
}

func (m *Manager) UpsertAuthorized(input AuthorizedInput) (Profile, error) {
	unlock, err := m.lock()
	if err != nil {
		return Profile{}, err
	}
	defer unlock()

	normalized, err := NormalizeServerURL(input.ServerURL)
	if err != nil {
		return Profile{}, err
	}
	credential := strings.TrimSpace(input.DeviceCredential)
	if err := validateDeviceCredential(credential); err != nil {
		return Profile{}, err
	}
	state, err := m.loadLocked()
	if err != nil {
		return Profile{}, err
	}

	now := m.now().UTC()
	index := findURLIndex(state, normalized)
	profile := Profile{
		ID:             profileID(normalized),
		Label:          defaultLabel(normalized),
		ServerURL:      normalized,
		Kind:           inferredKind(normalized),
		PrincipalID:    strings.TrimSpace(input.PrincipalID),
		PrincipalLabel: strings.TrimSpace(input.PrincipalLabel),
		CreatedAt:      now,
		LastUsedAt:     now,
	}
	if index >= 0 {
		profile = state.Profiles[index]
		profile.LastUsedAt = now
		if strings.TrimSpace(input.PrincipalID) != "" {
			profile.PrincipalID = strings.TrimSpace(input.PrincipalID)
		}
		if strings.TrimSpace(input.PrincipalLabel) != "" {
			profile.PrincipalLabel = strings.TrimSpace(input.PrincipalLabel)
		}
	}
	if label := strings.TrimSpace(input.Label); label != "" {
		profile.Label = label
	}
	if input.Kind != "" {
		profile.Kind = input.Kind
	}
	profile.CredentialRef = credentialReference(profile.ID)
	if err := validateProfile(profile); err != nil {
		return Profile{}, err
	}

	rollback, err := m.replaceCredential(profile.CredentialRef, credential)
	if err != nil {
		return Profile{}, err
	}
	if index >= 0 {
		state.Profiles[index] = profile
	} else {
		state.Profiles = append(state.Profiles, profile)
	}
	state.ActiveProfileID = profile.ID
	if err := m.writeLocked(state); err != nil {
		return Profile{}, errors.Join(err, rollback())
	}
	return profile, nil
}

func (m *Manager) SetActive(identifier string) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()

	state, err := m.loadLocked()
	if err != nil {
		return err
	}
	index, err := profileIndex(state, identifier)
	if err != nil {
		return err
	}
	if _, err := m.credentials.Get(state.Profiles[index].CredentialRef); err != nil {
		return fmt.Errorf("read credential for profile %q: %w", state.Profiles[index].Label, err)
	}
	state.Profiles[index].LastUsedAt = m.now().UTC()
	state.ActiveProfileID = state.Profiles[index].ID
	return m.writeLocked(state)
}

func (m *Manager) Remove(identifier string) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()

	state, err := m.loadLocked()
	if err != nil {
		return err
	}
	index, err := profileIndex(state, identifier)
	if err != nil {
		return err
	}
	profile := state.Profiles[index]
	previousSecret, getErr := m.credentials.Get(profile.CredentialRef)
	if getErr != nil && !errors.Is(getErr, credentialstore.ErrNotFound) {
		return fmt.Errorf("read credential before removing profile: %w", getErr)
	}
	if err := m.credentials.Delete(profile.CredentialRef); err != nil {
		return fmt.Errorf("delete profile credential: %w", err)
	}

	state.Profiles = append(state.Profiles[:index], state.Profiles[index+1:]...)
	if state.ActiveProfileID == profile.ID {
		state.ActiveProfileID = mostRecentlyUsedProfile(state.Profiles)
	}
	if err := m.writeLocked(state); err != nil {
		if getErr == nil {
			return errors.Join(err, m.credentials.Put(profile.CredentialRef, previousSecret))
		}
		return err
	}
	return nil
}

func (m *Manager) load() (State, error) {
	unlock, err := m.lock()
	if err != nil {
		return State{}, err
	}
	defer unlock()
	return m.loadLocked()
}

func (m *Manager) loadLocked() (State, error) {
	content, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read PACT Server profile registry: %w", err)
	}
	if err := os.Chmod(m.path, 0o600); err != nil {
		return State{}, fmt.Errorf("secure PACT Server profile registry: %w", err)
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(content, &header); err != nil {
		return State{}, fmt.Errorf("decode PACT Server profile registry: %w", err)
	}
	switch header.SchemaVersion {
	case 1:
		return State{}, errors.New("the stored token belongs to PACT's retired authentication model; run pact login --server <url> again")
	case 2:
		return m.migrateV2(content)
	case SchemaVersion:
		var state State
		if err := decodeStrict(content, &state); err != nil {
			return State{}, fmt.Errorf("decode PACT Server profile registry: %w", err)
		}
		if err := validateState(state); err != nil {
			return State{}, fmt.Errorf("validate PACT Server profile registry: %w", err)
		}
		return state, nil
	default:
		return State{}, fmt.Errorf("unsupported PACT Server profile registry version %d", header.SchemaVersion)
	}
}

func (m *Manager) migrateV2(content []byte) (State, error) {
	var legacy struct {
		SchemaVersion    int    `json:"schema_version"`
		ServerURL        string `json:"server_url"`
		DeviceCredential string `json:"device_credential"`
		LegacyAPIToken   string `json:"api_token,omitempty"`
	}
	if err := decodeStrict(content, &legacy); err != nil {
		return State{}, fmt.Errorf("decode legacy PACT user configuration: %w", err)
	}
	if legacy.LegacyAPIToken != "" {
		return State{}, errors.New("the stored token belongs to PACT's retired authentication model; run pact login --server <url> again")
	}
	normalized, err := NormalizeServerURL(legacy.ServerURL)
	if err != nil {
		return State{}, err
	}
	credential := strings.TrimSpace(legacy.DeviceCredential)
	if err := validateDeviceCredential(credential); err != nil {
		return State{}, fmt.Errorf("migrate PACT user configuration: %w", err)
	}
	now := m.now().UTC()
	profile := Profile{
		ID:         profileID(normalized),
		Label:      defaultLabel(normalized),
		ServerURL:  normalized,
		Kind:       inferredKind(normalized),
		CreatedAt:  now,
		LastUsedAt: now,
	}
	profile.CredentialRef = credentialReference(profile.ID)
	state := State{SchemaVersion: SchemaVersion, ActiveProfileID: profile.ID, Profiles: []Profile{profile}}

	rollback, err := m.replaceCredential(profile.CredentialRef, credential)
	if err != nil {
		return State{}, fmt.Errorf("migrate PACT credential: %w", err)
	}
	if err := m.writeLocked(state); err != nil {
		return State{}, fmt.Errorf("migrate PACT profile registry: %w", errors.Join(err, rollback()))
	}
	return state, nil
}

func (m *Manager) replaceCredential(reference, secret string) (func() error, error) {
	previous, previousErr := m.credentials.Get(reference)
	if previousErr != nil && !errors.Is(previousErr, credentialstore.ErrNotFound) {
		return nil, fmt.Errorf("read previous credential: %w", previousErr)
	}
	restore := func() error {
		if previousErr == nil {
			return m.credentials.Put(reference, previous)
		}
		return m.credentials.Delete(reference)
	}
	if err := m.credentials.Put(reference, secret); err != nil {
		_ = restore()
		return nil, fmt.Errorf("store credential: %w", err)
	}
	stored, err := m.credentials.Get(reference)
	if err != nil {
		_ = restore()
		return nil, fmt.Errorf("verify stored credential: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(secret)) != 1 {
		_ = restore()
		return nil, errors.New("verify stored credential: value did not round-trip")
	}
	return restore, nil
}

func (m *Manager) authorize(profile Profile) (AuthorizedProfile, error) {
	credential, err := m.credentials.Get(profile.CredentialRef)
	if errors.Is(err, credentialstore.ErrNotFound) {
		return AuthorizedProfile{}, fmt.Errorf("credential for PACT Server profile %q is missing; authorize it again", profile.Label)
	}
	if err != nil {
		return AuthorizedProfile{}, fmt.Errorf("read credential for PACT Server profile %q: %w", profile.Label, err)
	}
	if err := validateDeviceCredential(credential); err != nil {
		return AuthorizedProfile{}, fmt.Errorf("stored credential for PACT Server profile %q is invalid", profile.Label)
	}
	return AuthorizedProfile{Profile: profile, DeviceCredential: credential}, nil
}

func (m *Manager) writeLocked(state State) error {
	if err := validateState(state); err != nil {
		return fmt.Errorf("validate PACT Server profile registry: %w", err)
	}
	directory := filepath.Dir(m.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create PACT configuration directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure PACT configuration directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode PACT Server profile registry: %w", err)
	}
	if strings.Contains(string(payload), "pact_device_") {
		return errors.New("refusing to write a device credential into the PACT Server profile registry")
	}
	payload = append(payload, '\n')
	if err := atomicfile.Write(m.path, payload, 0o600); err != nil {
		return fmt.Errorf("write PACT Server profile registry: %w", err)
	}
	return nil
}

func (m *Manager) lock() (func(), error) {
	key, err := filepath.Abs(m.path)
	if err != nil {
		key = m.path
	}
	value, _ := managerLocks.LoadOrStore(key, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	directory := filepath.Dir(m.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		mutex.Unlock()
		return nil, fmt.Errorf("create PACT configuration directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		mutex.Unlock()
		return nil, fmt.Errorf("secure PACT configuration directory: %w", err)
	}
	releaseFile, err := filelock.Acquire(m.path + ".lock")
	if err != nil {
		mutex.Unlock()
		return nil, fmt.Errorf("lock PACT Server profile registry: %w", err)
	}
	return func() {
		_ = releaseFile()
		mutex.Unlock()
	}, nil
}

func emptyState() State {
	return State{SchemaVersion: SchemaVersion, Profiles: []Profile{}}
}

func validateState(state State) error {
	if state.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema version must be %d", SchemaVersion)
	}
	ids := make(map[string]struct{}, len(state.Profiles))
	urls := make(map[string]struct{}, len(state.Profiles))
	activeFound := state.ActiveProfileID == ""
	for _, profile := range state.Profiles {
		if err := validateProfile(profile); err != nil {
			return fmt.Errorf("profile %q: %w", profile.ID, err)
		}
		if _, duplicate := ids[profile.ID]; duplicate {
			return fmt.Errorf("duplicate profile ID %q", profile.ID)
		}
		ids[profile.ID] = struct{}{}
		if _, duplicate := urls[profile.ServerURL]; duplicate {
			return fmt.Errorf("duplicate server URL %q", profile.ServerURL)
		}
		urls[profile.ServerURL] = struct{}{}
		if profile.ID == state.ActiveProfileID {
			activeFound = true
		}
	}
	if !activeFound {
		return fmt.Errorf("active profile %q does not exist", state.ActiveProfileID)
	}
	return nil
}

func validateProfile(profile Profile) error {
	if profile.ID == "" || profile.ID != profileID(profile.ServerURL) {
		return errors.New("profile ID does not match its server URL")
	}
	normalized, err := NormalizeServerURL(profile.ServerURL)
	if err != nil || normalized != profile.ServerURL {
		return errors.New("server URL is not normalized")
	}
	if strings.TrimSpace(profile.Label) == "" {
		return errors.New("profile label is required")
	}
	if profile.Kind != KindRemote && profile.Kind != KindManagedLocal {
		return fmt.Errorf("unsupported profile kind %q", profile.Kind)
	}
	parsed, _ := url.Parse(profile.ServerURL)
	if profile.Kind == KindManagedLocal && !IsLoopbackHost(parsed.Hostname()) {
		return errors.New("managed local profile must use a loopback address")
	}
	if profile.Kind == KindRemote && parsed.Scheme != "https" {
		return errors.New("remote profile must use HTTPS")
	}
	if profile.CredentialRef != credentialReference(profile.ID) {
		return errors.New("credential reference does not match profile ID")
	}
	if profile.CreatedAt.IsZero() || profile.LastUsedAt.IsZero() {
		return errors.New("profile timestamps are required")
	}
	return nil
}

func validateDeviceCredential(credential string) error {
	if !strings.HasPrefix(credential, "pact_device_") || len(credential) < 40 {
		return errors.New("PACT device credential is invalid")
	}
	return nil
}

func profileIndex(state State, identifier string) (int, error) {
	identifier = strings.TrimSpace(identifier)
	for index, profile := range state.Profiles {
		if profile.ID == identifier || profile.ServerURL == identifier {
			return index, nil
		}
	}
	if normalized, err := NormalizeServerURL(identifier); err == nil {
		for index, profile := range state.Profiles {
			if profile.ServerURL == normalized {
				return index, nil
			}
		}
	}
	return -1, fmt.Errorf("%w: %q", ErrProfileNotFound, identifier)
}

func findURLIndex(state State, normalizedURL string) int {
	for index, profile := range state.Profiles {
		if profile.ServerURL == normalizedURL {
			return index
		}
	}
	return -1
}

func profileID(normalizedURL string) string {
	digest := sha256.Sum256([]byte("PACT Server profile\x00" + normalizedURL))
	bytes := append([]byte(nil), digest[:16]...)
	bytes[6] = (bytes[6] & 0x0f) | 0x50
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	hexValue := hex.EncodeToString(bytes)
	return hexValue[0:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:32]
}

func credentialReference(profileID string) string {
	return "pact/server/" + profileID
}

func defaultLabel(serverURL string) string {
	parsed, err := url.Parse(serverURL)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return serverURL
}

func inferredKind(serverURL string) Kind {
	parsed, err := url.Parse(serverURL)
	if err == nil && IsLoopbackHost(parsed.Hostname()) {
		return KindManagedLocal
	}
	return KindRemote
}

func mostRecentlyUsedProfile(profiles []Profile) string {
	if len(profiles) == 0 {
		return ""
	}
	selected := profiles[0]
	for _, profile := range profiles[1:] {
		if profile.LastUsedAt.After(selected.LastUsedAt) {
			selected = profile
		}
	}
	return selected.ID
}

func decodeStrict(content []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing data")
	}
	return nil
}
