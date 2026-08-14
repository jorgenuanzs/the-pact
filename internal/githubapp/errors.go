package githubapp

import "errors"

var (
	ErrNotConfigured      = errors.New("GitHub App integration is not configured")
	ErrInvalidState       = errors.New("GitHub connection state is invalid or expired")
	ErrInstallationDenied = errors.New("the GitHub user did not authorize this installation")
	ErrNotFound           = errors.New("GitHub installation or repository was not found")
	ErrWebhookSignature   = errors.New("GitHub webhook signature is invalid")
)

type ProviderError struct {
	Code       string
	StatusCode int
	Err        error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return "GitHub App provider error: " + e.Code + ": " + e.Err.Error()
	}
	return "GitHub App provider error: " + e.Code
}

func (e *ProviderError) Unwrap() error { return e.Err }
