package authn

import (
	"crypto/sha256"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
)

const (
	WebSessionCookie   = "pact_session"
	CSRFCookie         = "pact_csrf"
	WebSessionLifetime = 12 * time.Hour
	DeviceLifetime     = 30 * 24 * time.Hour
	DeviceCodeLifetime = 10 * time.Minute
)

type SetupStatus struct {
	Required   bool `json:"required"`
	Configured bool `json:"configured"`
}

type AccountInput struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

type SetupInput struct {
	SetupCode string `json:"setup_code"`
	AccountInput
}

type LoginInput struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type SessionMetadata struct {
	UserAgent     string
	RemoteAddress string
}

type Account struct {
	Principal           access.Principal
	Email               string
	Username            string
	PasswordHash        string
	Status              string
	FailedLoginAttempts int
	LockedUntil         *time.Time
}

type WebSession struct {
	ID         string            `json:"id"`
	Principal  access.Principal  `json:"principal"`
	ExpiresAt  time.Time         `json:"expires_at"`
	CSRFDigest [sha256.Size]byte `json:"-"`
}

type CreatedWebSession struct {
	Session       WebSession
	SessionSecret string
	CSRFSecret    string
}

type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type InvitationPreview struct {
	Email            string    `json:"email"`
	OrganizationRole string    `json:"organization_role"`
	Role             string    `json:"role,omitempty"`
	ProjectID        string    `json:"project_id,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type InvitationRegistrationInput struct {
	Secret string `json:"secret"`
	AccountInput
}

type InvitationAcceptance struct {
	Principal        access.Principal `json:"principal"`
	OrganizationRole string           `json:"organization_role"`
	ProjectID        string           `json:"project_id,omitempty"`
	ProjectRole      string           `json:"project_role,omitempty"`
	ExpiresAt        time.Time        `json:"expires_at"`
}

type CreatedInvitationSession struct {
	Acceptance    InvitationAcceptance
	Session       WebSession
	SessionSecret string
	CSRFSecret    string
}

type BeginDeviceInput struct {
	DeviceName string `json:"device_name"`
}

type DeviceAuthorization struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	IntervalSeconds int       `json:"interval_seconds"`
}

type DeviceExchange struct {
	Status           string            `json:"status"`
	DeviceCredential string            `json:"device_credential,omitempty"`
	CredentialID     string            `json:"credential_id,omitempty"`
	Principal        *access.Principal `json:"principal,omitempty"`
	ExpiresAt        *time.Time        `json:"expires_at,omitempty"`
}

type DevicePrincipal struct {
	CredentialID string
	Principal    access.Principal
}

type Device struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type deviceExchangeRecord struct {
	Status       string
	CredentialID string
	Principal    access.Principal
	ExpiresAt    time.Time
}
