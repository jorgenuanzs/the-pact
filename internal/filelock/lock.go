package filelock

import (
	"fmt"
	"os"
)

// Acquire holds an exclusive OS lock until the returned release function is
// called. The operating system also releases it if the process exits.
func Acquire(path string) (func() error, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure lock file: %w", err)
	}
	if err := lock(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire file lock: %w", err)
	}
	return func() error {
		unlockErr := unlock(file)
		closeErr := file.Close()
		if unlockErr != nil {
			return fmt.Errorf("release file lock: %w", unlockErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close lock file: %w", closeErr)
		}
		return nil
	}, nil
}
