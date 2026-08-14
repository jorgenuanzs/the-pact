package knowledge

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("knowledge record not found")
	ErrResourceNotFound    = errors.New("knowledge resource not found")
	ErrResourceExists      = errors.New("the resource is already registered in this workspace")
	ErrVersionConflict     = errors.New("the knowledge record changed since it was read")
	ErrInvalidTransition   = errors.New("the requested knowledge record transition is not allowed")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with another request")
	ErrCommandIncomplete   = errors.New("an earlier knowledge command has not completed")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
