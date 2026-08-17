package authn

import (
	"errors"
	"fmt"
)

var (
	ErrUnauthorized        = errors.New("authentication required")
	ErrInvalidCredentials  = errors.New("invalid username, email, or password")
	ErrSetupUnavailable    = errors.New("initial setup is unavailable")
	ErrAlreadyConfigured   = errors.New("initial setup has already been completed")
	ErrAccountExists       = errors.New("an account already exists for this email or username")
	ErrInvitationInvalid   = errors.New("invitation is invalid, expired, accepted, or revoked")
	ErrInvitationMismatch  = errors.New("invitation belongs to a different account")
	ErrDeviceCodeInvalid   = errors.New("device authorization is invalid or expired")
	ErrAuthorizationDenied = errors.New("device authorization was denied")
	ErrNotFound            = errors.New("authentication resource not found")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
