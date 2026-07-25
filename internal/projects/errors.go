package projects

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("project not found")
	ErrSlugTaken           = errors.New("project slug is already in use")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with a different request")
	ErrCommandIncomplete   = errors.New("previous command did not store a reusable result")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
