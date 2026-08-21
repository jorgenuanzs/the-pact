package credentialstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jorgenuanzs/the-pact/internal/atomicfile"
)

// File is an explicit fallback for headless CLI environments without a native
// secret store. It relies on OS file permissions and must never be selected
// automatically.
type File struct {
	directory string
}

func NewFile(directory string) (*File, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve credential directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create credential directory: %w", err)
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("secure credential directory: %w", err)
	}
	return &File{directory: absolute}, nil
}

func (f *File) Put(reference, secret string) error {
	path, err := f.path(reference)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(path, []byte(secret), 0o600); err != nil {
		return fmt.Errorf("write credential fallback: %w", err)
	}
	return nil
}

func (f *File) Get(reference string) (string, error) {
	path, err := f.path(reference)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read credential fallback: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("secure credential fallback: %w", err)
	}
	return string(content), nil
}

func (f *File) Delete(reference string) error {
	path, err := f.path(reference)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete credential fallback: %w", err)
	}
	return nil
}

func (f *File) Exists(reference string) (bool, error) {
	_, err := f.Get(reference)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (f *File) path(reference string) (string, error) {
	reference, err := validateReference(reference)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(reference))
	return filepath.Join(f.directory, hex.EncodeToString(digest[:])+".credential"), nil
}
