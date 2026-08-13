package coordination

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound              = errors.New("coordinated work not found")
	ErrRepositoryUnavailable = errors.New("project does not have an active root repository")
	ErrForbidden             = errors.New("the session cannot modify this work")
	ErrScopeConflict         = errors.New("one or more requested scopes are already reserved")
	ErrVersionConflict       = errors.New("the intent version changed")
	ErrInvalidTransition     = errors.New("the requested intent status transition is invalid")
	ErrWorkspaceExists       = errors.New("the intent already has a live workspace")
	ErrIdempotencyConflict   = errors.New("idempotency key was already used with different input")
	ErrCommandIncomplete     = errors.New("the previous coordination command has not completed")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ScopeConflictError struct {
	Overlaps []ScopeOverlap
}

func (e *ScopeConflictError) Error() string { return ErrScopeConflict.Error() }
func (e *ScopeConflictError) Unwrap() error { return ErrScopeConflict }
