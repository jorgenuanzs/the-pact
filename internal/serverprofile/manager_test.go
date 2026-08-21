package serverprofile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/credentialstore"
)

func TestMigratesV2WithoutLeavingCredentialInJSON(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	credential := "pact_device_" + strings.Repeat("a", 48)
	legacy := `{"schema_version":2,"server_url":"https://pact.example.com/","device_credential":"` + credential + `"}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store := credentialstore.NewMemory()
	manager := NewManager(path, store)
	manager.now = func() time.Time { return time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC) }

	active, err := manager.Active()
	if err != nil {
		t.Fatalf("Active() error = %v", err)
	}
	if active.ServerURL != "https://pact.example.com" || active.DeviceCredential != credential {
		t.Fatalf("Active() = %#v", active)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), credential) || strings.Contains(string(content), "device_credential") {
		t.Fatalf("migrated registry contains credential material: %s", content)
	}
	var state State
	if err := json.Unmarshal(content, &state); err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != SchemaVersion || len(state.Profiles) != 1 || state.ActiveProfileID != active.ID {
		t.Fatalf("migrated state = %#v", state)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if permissions := info.Mode().Perm(); permissions != 0o600 {
			t.Fatalf("registry permissions = %o", permissions)
		}
	}
}

func TestFailedMigrationKeepsLegacyConfigurationUntouched(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	credential := "pact_device_" + strings.Repeat("b", 48)
	legacy := `{"schema_version":2,"server_url":"https://pact.example.com","device_credential":"` + credential + `"}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(path, failingStore{err: errors.New("keychain unavailable")})
	if _, err := manager.Active(); err == nil {
		t.Fatal("Active() succeeded with failing credential store")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != legacy {
		t.Fatalf("legacy configuration changed after failed migration: %s", content)
	}
}

func TestProfilesAreUniqueByNormalizedURLAndKeepSeparateCredentials(t *testing.T) {
	manager, store := testManager(t)
	firstCredential := "pact_device_" + strings.Repeat("c", 48)
	secondCredential := "pact_device_" + strings.Repeat("d", 48)
	updatedCredential := "pact_device_" + strings.Repeat("e", 48)

	first, err := manager.UpsertAuthorized(AuthorizedInput{
		ServerURL: "https://one.example.com/", DeviceCredential: firstCredential,
		PrincipalID: "principal-one", PrincipalLabel: "One User",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.UpsertAuthorized(AuthorizedInput{
		ServerURL: "https://two.example.com", DeviceCredential: secondCredential,
	})
	if err != nil {
		t.Fatal(err)
	}
	reauthorized, err := manager.UpsertAuthorized(AuthorizedInput{
		ServerURL: "https://one.example.com", DeviceCredential: updatedCredential,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reauthorized.ID != first.ID {
		t.Fatalf("reauthorization changed profile ID: %q != %q", reauthorized.ID, first.ID)
	}
	profiles, err := manager.List()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("List() = %d profiles, %v", len(profiles), err)
	}
	if active, err := manager.Active(); err != nil || active.ID != first.ID || active.DeviceCredential != updatedCredential {
		t.Fatalf("Active() after reauthorization = %#v, %v", active, err)
	}
	if authorized, err := manager.AuthorizedForURL(second.ServerURL); err != nil || authorized.DeviceCredential != secondCredential {
		t.Fatalf("AuthorizedForURL(second) = %#v, %v", authorized, err)
	}
	if firstSecret, _ := store.Get(first.CredentialRef); firstSecret != updatedCredential {
		t.Fatalf("first credential = %q", firstSecret)
	}
	if secondSecret, _ := store.Get(second.CredentialRef); secondSecret != secondCredential {
		t.Fatalf("second credential = %q", secondSecret)
	}
}

func TestSetActiveAndRemoveSelectMostRecentlyUsedRemainingProfile(t *testing.T) {
	manager, _ := testManager(t)
	clock := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time {
		clock = clock.Add(time.Minute)
		return clock
	}
	first, err := manager.UpsertAuthorized(AuthorizedInput{
		ServerURL: "https://one.example.com", DeviceCredential: "pact_device_" + strings.Repeat("f", 48),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.UpsertAuthorized(AuthorizedInput{
		ServerURL: "https://two.example.com", DeviceCredential: "pact_device_" + strings.Repeat("g", 48),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetActive(first.ServerURL + "/"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(first.ID); err != nil {
		t.Fatal(err)
	}
	active, err := manager.Active()
	if err != nil || active.ID != second.ID {
		t.Fatalf("Active() after Remove() = %#v, %v", active, err)
	}
	if err := manager.Remove(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Active(); !errors.Is(err, ErrNoActiveProfile) {
		t.Fatalf("Active() on empty registry error = %v", err)
	}
}

func TestRegistryRefusesCredentialShapedMetadata(t *testing.T) {
	manager, _ := testManager(t)
	_, err := manager.UpsertAuthorized(AuthorizedInput{
		ServerURL:        "https://pact.example.com",
		Label:            "pact_device_" + strings.Repeat("x", 48),
		DeviceCredential: "pact_device_" + strings.Repeat("h", 48),
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to write") {
		t.Fatalf("UpsertAuthorized() error = %v", err)
	}
}

func testManager(t *testing.T) (*Manager, *credentialstore.Memory) {
	t.Helper()
	store := credentialstore.NewMemory()
	manager := NewManager(filepath.Join(t.TempDir(), "config.json"), store)
	manager.now = func() time.Time { return time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC) }
	return manager, store
}

type failingStore struct {
	err error
}

func (f failingStore) Put(string, string) error    { return f.err }
func (f failingStore) Get(string) (string, error)  { return "", credentialstore.ErrNotFound }
func (f failingStore) Delete(string) error         { return nil }
func (f failingStore) Exists(string) (bool, error) { return false, f.err }
