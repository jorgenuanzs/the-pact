package useradmin

import (
	"errors"
	"fmt"
)

var (
	ErrForbidden         = errors.New("the principal cannot administer organization users")
	ErrNotFound          = errors.New("user administration resource not found")
	ErrAccountExists     = errors.New("an account already exists for this email or username")
	ErrInvitationExists  = errors.New("a pending invitation already exists for this email")
	ErrLastOwner         = errors.New("the last active organization owner cannot be disabled or demoted")
	ErrSelfManagement    = errors.New("the current account cannot revoke, disable, or demote itself")
	ErrGlobalProjectRole = errors.New("organization owners and administrators already have global project access")
	ErrInactiveUser      = errors.New("the user account is disabled")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
