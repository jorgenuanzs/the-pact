package workspaces

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("workspace not found")
	ErrProjectNotFound     = errors.New("project not found")
	ErrSlugTaken           = errors.New("workspace slug is already in use")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with another request")
	ErrCommandIncomplete   = errors.New("an earlier workspace command has not completed")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Message)
}
