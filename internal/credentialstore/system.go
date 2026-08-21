package credentialstore

import (
	"errors"

	keyring "github.com/zalando/go-keyring"
)

const systemService = "com.nuanzs.pact.device-credentials"

type keyringBackend interface {
	Set(service, account, secret string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

type packageKeyring struct{}

func (packageKeyring) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (packageKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (packageKeyring) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

// System stores credentials in macOS Keychain, Windows Credential Manager or
// the platform's native user-scoped keyring. PACT Desktop, CLI and pact-local
// intentionally share the same service because they must resolve the same
// profile without copying its secret into configuration files.
type System struct {
	backend keyringBackend
}

func NewSystem() *System {
	return &System{backend: packageKeyring{}}
}

func (s *System) Put(reference, secret string) error {
	reference, err := validateReference(reference)
	if err != nil {
		return err
	}
	if err := s.backend.Set(systemService, reference, secret); err != nil {
		return mapSystemError("store credential", err)
	}
	return nil
}

func (s *System) Get(reference string) (string, error) {
	reference, err := validateReference(reference)
	if err != nil {
		return "", err
	}
	secret, err := s.backend.Get(systemService, reference)
	if err != nil {
		return "", mapSystemError("read credential", err)
	}
	return secret, nil
}

func (s *System) Delete(reference string) error {
	reference, err := validateReference(reference)
	if err != nil {
		return err
	}
	if err := s.backend.Delete(systemService, reference); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return mapSystemError("delete credential", err)
	}
	return nil
}

func (s *System) Exists(reference string) (bool, error) {
	_, err := s.Get(reference)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

func mapSystemError(operation string, err error) error {
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return unavailable(operation, err)
}
