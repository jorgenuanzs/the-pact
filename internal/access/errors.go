package access

import (
	"errors"
	"fmt"
)

var (
	ErrUnauthorized      = errors.New("authentication required")
	ErrForbidden         = errors.New("the principal does not have permission for this operation")
	ErrInvitationInvalid = errors.New("invitation is invalid, expired, accepted, or revoked")
	ErrInvitationExists  = errors.New("a pending invitation already exists for this email and project")
	ErrNotFound          = errors.New("access resource not found")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
