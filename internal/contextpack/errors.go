package contextpack

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("context pack or intent not found")
	ErrForbidden           = errors.New("the current identity cannot compile this context pack")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with different input")
	ErrCommandIncomplete   = errors.New("the previous context compilation has not completed")
	ErrIntegrity           = errors.New("the stored context pack failed its integrity check")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
