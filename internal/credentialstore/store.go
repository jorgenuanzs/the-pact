package credentialstore

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound    = errors.New("PACT credential not found")
	ErrUnavailable = errors.New("PACT credential store unavailable")
)

// Store persists device credentials behind opaque references. Implementations
// must never log secret values.
type Store interface {
	Put(reference, secret string) error
	Get(reference string) (string, error)
	Delete(reference string) error
	Exists(reference string) (bool, error)
}

func validateReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", errors.New("credential reference is required")
	}
	if strings.ContainsAny(reference, "\r\n\x00") {
		return "", errors.New("credential reference contains invalid characters")
	}
	return reference, nil
}

func unavailable(operation string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrUnavailable, operation, err)
}
