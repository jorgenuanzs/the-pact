package credentialstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	keyring "github.com/zalando/go-keyring"
)

func TestMemoryStoreContract(t *testing.T) {
	store := NewMemory()
	const reference = "pact/server/test"

	if exists, err := store.Exists(reference); err != nil || exists {
		t.Fatalf("Exists() before Put = %v, %v", exists, err)
	}
	if err := store.Put(reference, "first"); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Put(reference, "second"); err != nil {
		t.Fatalf("Put() upsert error = %v", err)
	}
	if got, err := store.Get(reference); err != nil || got != "second" {
		t.Fatalf("Get() = %q, %v", got, err)
	}
	if err := store.Delete(reference); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := store.Delete(reference); err != nil {
		t.Fatalf("Delete() idempotent error = %v", err)
	}
	if _, err := store.Get(reference); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after Delete error = %v", err)
	}
}

func TestFileStoreIsExplicitAndUsesPrivatePermissions(t *testing.T) {
	directory := t.TempDir()
	store, err := NewFile(filepath.Join(directory, "credentials"))
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}
	const reference = "pact/server/file-test"
	const secret = "pact_device_file_secret"
	if err := store.Put(reference, secret); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if got, err := store.Get(reference); err != nil || got != secret {
		t.Fatalf("Get() = %q, %v", got, err)
	}

	entries, err := os.ReadDir(filepath.Join(directory, "credentials"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("ReadDir() = %d entries, %v", len(entries), err)
	}
	if strings.Contains(entries[0].Name(), "file-test") {
		t.Fatalf("credential filename exposes its reference: %q", entries[0].Name())
	}
	if runtime.GOOS != "windows" {
		info, err := entries[0].Info()
		if err != nil {
			t.Fatal(err)
		}
		if permissions := info.Mode().Perm(); permissions != 0o600 {
			t.Fatalf("credential permissions = %o", permissions)
		}
	}
}

func TestDefaultNeverSilentlySelectsFileFallback(t *testing.T) {
	t.Setenv(BackendEnvironment, "")
	if _, ok := mustDefault(t, t.TempDir()).(*System); !ok {
		t.Fatal("Default() did not select the native system store")
	}

	t.Setenv(BackendEnvironment, "file")
	if _, ok := mustDefault(t, t.TempDir()).(*File); !ok {
		t.Fatal("explicit file backend did not select File")
	}
}

func TestSystemMapsBackendErrorsWithoutSecretMaterial(t *testing.T) {
	backend := &fakeKeyring{getErr: keyring.ErrNotFound}
	store := &System{backend: backend}
	if _, err := store.Get("pact/server/missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v", err)
	}

	backend.getErr = errors.New("backend offline")
	if _, err := store.Get("pact/server/offline"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Get() unavailable error = %v", err)
	}
}

func TestSystemKeyringRoundTrip(t *testing.T) {
	if os.Getenv("PACT_SYSTEM_KEYRING_TEST") != "1" {
		t.Skip("set PACT_SYSTEM_KEYRING_TEST=1 to exercise the OS credential store")
	}
	store := NewSystem()
	reference := "pact/server/integration-" + time.Now().UTC().Format("20060102T150405.000000000")
	secret := "pact_device_" + strings.Repeat("k", 48)
	t.Cleanup(func() { _ = store.Delete(reference) })
	if err := store.Put(reference, secret); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if got, err := store.Get(reference); err != nil || got != secret {
		t.Fatalf("Get() = %q, %v", got, err)
	}
}

func mustDefault(t *testing.T, directory string) Store {
	t.Helper()
	store, err := Default(directory)
	if err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	return store
}

type fakeKeyring struct {
	secret string
	getErr error
}

func (f *fakeKeyring) Set(_, _, secret string) error {
	f.secret = secret
	return nil
}

func (f *fakeKeyring) Get(_, _ string) (string, error) {
	return f.secret, f.getErr
}

func (f *fakeKeyring) Delete(_, _ string) error {
	f.secret = ""
	return nil
}
