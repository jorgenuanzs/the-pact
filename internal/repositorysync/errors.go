package repositorysync

import (
	"errors"
	"fmt"
)

var (
	ErrRepositoryUnavailable = errors.New("project does not have an active root repository")
	ErrUnsupportedRemote     = errors.New("root repository is not hosted on github.com")
	ErrNotFound              = errors.New("repository sync state not found")
	ErrIdempotencyConflict   = errors.New("idempotency key was already used with a different request")
	ErrCommandIncomplete     = errors.New("previous command did not store a reusable result")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ProviderError struct {
	Code       string
	StatusCode int
	RetryAfter string
	Err        error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("GitHub repository synchronization failed (%s): %v", e.Code, e.Err)
	}
	return fmt.Sprintf("GitHub repository synchronization failed (%s)", e.Code)
}

func (e *ProviderError) Unwrap() error { return e.Err }
