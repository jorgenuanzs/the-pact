package authn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jorgenuanzs/the-pact/internal/access"
)

const (
	maximumFailedLogins = 5
	loginLockDuration   = 15 * time.Minute
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

type Repository interface {
	SetupComplete(context.Context, string) (bool, error)
	CreateOwner(context.Context, string, AccountInput, [sha256.Size]byte, [sha256.Size]byte, time.Time, SessionMetadata) (WebSession, error)
	FindAccountByLogin(context.Context, string, string) (Account, error)
	FindAccountByPrincipal(context.Context, string, string) (Account, error)
	RecordFailedLogin(context.Context, string, string, int, *time.Time) error
	CreateWebSession(context.Context, string, string, [sha256.Size]byte, [sha256.Size]byte, time.Time, SessionMetadata) (WebSession, error)
	AuthenticateWebSession(context.Context, string, [sha256.Size]byte) (WebSession, error)
	RevokeWebSession(context.Context, string, string, string) error
	ChangePassword(context.Context, string, string, string, string) error
	PreviewInvitation(context.Context, string, [sha256.Size]byte) (InvitationPreview, error)
	RegisterInvitation(context.Context, string, InvitationRegistrationInput, [sha256.Size]byte, [sha256.Size]byte, [sha256.Size]byte, time.Time, SessionMetadata) (InvitationAcceptance, WebSession, error)
	AcceptInvitation(context.Context, string, access.Principal, [sha256.Size]byte) (InvitationAcceptance, error)
	CreateDeviceAuthorization(context.Context, string, string, [sha256.Size]byte, [sha256.Size]byte, string, time.Time) error
	ApproveDeviceAuthorization(context.Context, string, access.Principal, [sha256.Size]byte) error
	ExchangeDeviceAuthorization(context.Context, string, [sha256.Size]byte, [sha256.Size]byte, time.Time) (deviceExchangeRecord, error)
	AuthenticateDevice(context.Context, string, [sha256.Size]byte) (DevicePrincipal, error)
	RevokeDevice(context.Context, string, string, string) error
	ListDevices(context.Context, string, string) ([]Device, error)
	RevokeDeviceByID(context.Context, string, string, string) error
}

type Config struct {
	OrganizationID string
	SetupToken     string
	PublicURL      string
	Now            func() time.Time
}

type Service struct {
	organizationID string
	setupHash      [sha256.Size]byte
	setupAvailable bool
	publicURL      string
	repository     Repository
	now            func() time.Time
	dummyHash      string
}

func NewService(cfg Config, repository Repository) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	setupToken := strings.TrimSpace(cfg.SetupToken)
	return &Service{
		organizationID: cfg.OrganizationID,
		setupHash:      sha256.Sum256([]byte(setupToken)),
		setupAvailable: len(setupToken) >= 24,
		publicURL:      strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/"),
		repository:     repository,
		now:            now,
		dummyHash:      dummyPasswordHash(),
	}
}

func (s *Service) SetupStatus(ctx context.Context) (SetupStatus, error) {
	complete, err := s.repository.SetupComplete(ctx, s.organizationID)
	if err != nil {
		return SetupStatus{}, err
	}
	return SetupStatus{Required: !complete, Configured: s.setupAvailable}, nil
}

func (s *Service) Setup(ctx context.Context, input SetupInput, metadata SessionMetadata) (CreatedWebSession, error) {
	complete, err := s.repository.SetupComplete(ctx, s.organizationID)
	if err != nil {
		return CreatedWebSession{}, err
	}
	if complete {
		return CreatedWebSession{}, ErrAlreadyConfigured
	}
	if !s.setupAvailable {
		return CreatedWebSession{}, ErrSetupUnavailable
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(input.SetupCode)))
	if subtle.ConstantTimeCompare(digest[:], s.setupHash[:]) != 1 {
		return CreatedWebSession{}, ErrSetupUnavailable
	}
	account, err := normalizeAndValidateAccount(input.AccountInput)
	if err != nil {
		return CreatedWebSession{}, err
	}
	account.Password, err = hashPassword(account.Password)
	if err != nil {
		return CreatedWebSession{}, err
	}
	return s.createOwnerSession(ctx, account, metadata)
}

func (s *Service) createOwnerSession(ctx context.Context, account AccountInput, metadata SessionMetadata) (CreatedWebSession, error) {
	sessionSecret, sessionDigest, err := newSecret("pact_web_")
	if err != nil {
		return CreatedWebSession{}, err
	}
	csrfSecret, csrfDigest, err := newSecret("pact_csrf_")
	if err != nil {
		return CreatedWebSession{}, err
	}
	expiresAt := s.now().UTC().Add(WebSessionLifetime)
	session, err := s.repository.CreateOwner(
		ctx, s.organizationID, account,
		sessionDigest, csrfDigest, expiresAt, sanitizeMetadata(metadata),
	)
	if err != nil {
		return CreatedWebSession{}, err
	}
	return CreatedWebSession{Session: session, SessionSecret: sessionSecret, CSRFSecret: csrfSecret}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput, metadata SessionMetadata) (CreatedWebSession, error) {
	login := strings.ToLower(strings.TrimSpace(input.Login))
	if login == "" || input.Password == "" {
		return CreatedWebSession{}, ErrInvalidCredentials
	}
	account, err := s.repository.FindAccountByLogin(ctx, s.organizationID, login)
	if errors.Is(err, ErrNotFound) {
		_ = verifyPassword(input.Password, s.dummyHash)
		return CreatedWebSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return CreatedWebSession{}, err
	}
	now := s.now().UTC()
	if account.Status != "active" || (account.LockedUntil != nil && account.LockedUntil.After(now)) {
		_ = verifyPassword(input.Password, account.PasswordHash)
		return CreatedWebSession{}, ErrInvalidCredentials
	}
	if !verifyPassword(input.Password, account.PasswordHash) {
		attempts := account.FailedLoginAttempts + 1
		var lockedUntil *time.Time
		if attempts >= maximumFailedLogins {
			locked := now.Add(loginLockDuration)
			lockedUntil = &locked
		}
		if recordErr := s.repository.RecordFailedLogin(ctx, s.organizationID, account.Principal.ID, attempts, lockedUntil); recordErr != nil {
			return CreatedWebSession{}, recordErr
		}
		return CreatedWebSession{}, ErrInvalidCredentials
	}

	sessionSecret, sessionDigest, err := newSecret("pact_web_")
	if err != nil {
		return CreatedWebSession{}, err
	}
	csrfSecret, csrfDigest, err := newSecret("pact_csrf_")
	if err != nil {
		return CreatedWebSession{}, err
	}
	expiresAt := now.Add(WebSessionLifetime)
	session, err := s.repository.CreateWebSession(
		ctx, s.organizationID, account.Principal.ID,
		sessionDigest, csrfDigest, expiresAt, sanitizeMetadata(metadata),
	)
	if err != nil {
		return CreatedWebSession{}, err
	}
	return CreatedWebSession{Session: session, SessionSecret: sessionSecret, CSRFSecret: csrfSecret}, nil
}

func (s *Service) AuthenticateWeb(ctx context.Context, raw string) (WebSession, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "pact_web_") || len(raw) < 40 {
		return WebSession{}, ErrUnauthorized
	}
	return s.repository.AuthenticateWebSession(ctx, s.organizationID, sha256.Sum256([]byte(raw)))
}

func (s *Service) AuthenticateDevice(ctx context.Context, raw string) (DevicePrincipal, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "pact_device_") || len(raw) < 40 {
		return DevicePrincipal{}, ErrUnauthorized
	}
	return s.repository.AuthenticateDevice(ctx, s.organizationID, sha256.Sum256([]byte(raw)))
}

func (s *Service) ValidateCSRF(session WebSession, raw string) bool {
	digest := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return subtle.ConstantTimeCompare(digest[:], session.CSRFDigest[:]) == 1
}

func (s *Service) LogoutWeb(ctx context.Context, session WebSession) error {
	return s.repository.RevokeWebSession(ctx, s.organizationID, session.Principal.ID, session.ID)
}

func (s *Service) ChangePassword(ctx context.Context, session WebSession, input ChangePasswordInput) error {
	if err := validatePassword(input.NewPassword); err != nil {
		return err
	}
	account, err := s.repository.FindAccountByPrincipal(ctx, s.organizationID, session.Principal.ID)
	if err != nil {
		return err
	}
	if !verifyPassword(input.CurrentPassword, account.PasswordHash) {
		return ErrInvalidCredentials
	}
	if verifyPassword(input.NewPassword, account.PasswordHash) {
		return &ValidationError{Field: "new_password", Message: "must be different from the current password"}
	}
	hash, err := hashPassword(input.NewPassword)
	if err != nil {
		return err
	}
	return s.repository.ChangePassword(ctx, s.organizationID, session.Principal.ID, session.ID, hash)
}

func (s *Service) PreviewInvitation(ctx context.Context, secret string) (InvitationPreview, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, "pact_inv_") || len(secret) < 40 {
		return InvitationPreview{}, ErrInvitationInvalid
	}
	return s.repository.PreviewInvitation(ctx, s.organizationID, sha256.Sum256([]byte(secret)))
}

func (s *Service) RegisterInvitation(ctx context.Context, input InvitationRegistrationInput, metadata SessionMetadata) (CreatedInvitationSession, error) {
	input.Secret = strings.TrimSpace(input.Secret)
	if !strings.HasPrefix(input.Secret, "pact_inv_") || len(input.Secret) < 40 {
		return CreatedInvitationSession{}, ErrInvitationInvalid
	}
	account, err := normalizeAndValidateAccount(input.AccountInput)
	if err != nil {
		return CreatedInvitationSession{}, err
	}
	account.Password, err = hashPassword(account.Password)
	if err != nil {
		return CreatedInvitationSession{}, err
	}
	sessionSecret, sessionDigest, err := newSecret("pact_web_")
	if err != nil {
		return CreatedInvitationSession{}, err
	}
	csrfSecret, csrfDigest, err := newSecret("pact_csrf_")
	if err != nil {
		return CreatedInvitationSession{}, err
	}
	expiresAt := s.now().UTC().Add(WebSessionLifetime)
	input.AccountInput = account
	acceptance, session, err := s.repository.RegisterInvitation(
		ctx, s.organizationID, input,
		sha256.Sum256([]byte(input.Secret)), sessionDigest, csrfDigest,
		expiresAt, sanitizeMetadata(metadata),
	)
	if err != nil {
		return CreatedInvitationSession{}, err
	}
	return CreatedInvitationSession{
		Acceptance: acceptance, Session: session,
		SessionSecret: sessionSecret, CSRFSecret: csrfSecret,
	}, nil
}

func (s *Service) AcceptInvitation(ctx context.Context, principal access.Principal, secret string) (InvitationAcceptance, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, "pact_inv_") || len(secret) < 40 {
		return InvitationAcceptance{}, ErrInvitationInvalid
	}
	return s.repository.AcceptInvitation(ctx, s.organizationID, principal, sha256.Sum256([]byte(secret)))
}

func (s *Service) BeginDevice(ctx context.Context, input BeginDeviceInput) (DeviceAuthorization, error) {
	input.DeviceName = strings.TrimSpace(input.DeviceName)
	if input.DeviceName == "" || utf8.RuneCountInString(input.DeviceName) > 200 {
		return DeviceAuthorization{}, &ValidationError{Field: "device_name", Message: "must contain 1 to 200 characters"}
	}
	deviceCode, deviceDigest, err := newSecret("pact_device_code_")
	if err != nil {
		return DeviceAuthorization{}, err
	}
	userCode, err := newUserCode()
	if err != nil {
		return DeviceAuthorization{}, err
	}
	expiresAt := s.now().UTC().Add(DeviceCodeLifetime)
	if err := s.repository.CreateDeviceAuthorization(
		ctx, s.organizationID, input.DeviceName, deviceDigest,
		sha256.Sum256([]byte(userCode)), userCode, expiresAt,
	); err != nil {
		return DeviceAuthorization{}, err
	}
	verificationURI := s.publicURL + "/admin/#device=" + userCode
	if s.publicURL == "" {
		verificationURI = "/admin/#device=" + userCode
	}
	return DeviceAuthorization{
		DeviceCode: deviceCode, UserCode: userCode, VerificationURI: verificationURI,
		ExpiresAt: expiresAt, IntervalSeconds: 2,
	}, nil
}

func (s *Service) ApproveDevice(ctx context.Context, principal access.Principal, userCode string) error {
	userCode = normalizeUserCode(userCode)
	if len(userCode) != 9 {
		return ErrDeviceCodeInvalid
	}
	return s.repository.ApproveDeviceAuthorization(
		ctx, s.organizationID, principal, sha256.Sum256([]byte(userCode)),
	)
}

func (s *Service) ExchangeDevice(ctx context.Context, deviceCode string) (DeviceExchange, error) {
	deviceCode = strings.TrimSpace(deviceCode)
	if !strings.HasPrefix(deviceCode, "pact_device_code_") || len(deviceCode) < 48 {
		return DeviceExchange{}, ErrDeviceCodeInvalid
	}
	credential, credentialDigest, err := newSecret("pact_device_")
	if err != nil {
		return DeviceExchange{}, err
	}
	expiresAt := s.now().UTC().Add(DeviceLifetime)
	record, err := s.repository.ExchangeDeviceAuthorization(
		ctx, s.organizationID, sha256.Sum256([]byte(deviceCode)), credentialDigest, expiresAt,
	)
	if err != nil {
		return DeviceExchange{}, err
	}
	if record.Status != "authorized" {
		return DeviceExchange{Status: record.Status}, nil
	}
	return DeviceExchange{
		Status: "authorized", DeviceCredential: credential,
		CredentialID: record.CredentialID, Principal: &record.Principal,
		ExpiresAt: &record.ExpiresAt,
	}, nil
}

func (s *Service) RevokeCurrentDevice(ctx context.Context, device DevicePrincipal) error {
	return s.repository.RevokeDevice(ctx, s.organizationID, device.Principal.ID, device.CredentialID)
}

func (s *Service) ListDevices(ctx context.Context, principal access.Principal) ([]Device, error) {
	return s.repository.ListDevices(ctx, s.organizationID, principal.ID)
}

func (s *Service) RevokeDevice(ctx context.Context, principal access.Principal, deviceID string) error {
	return s.repository.RevokeDeviceByID(ctx, s.organizationID, principal.ID, strings.TrimSpace(deviceID))
}

func normalizeAndValidateAccount(input AccountInput) (AccountInput, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	if input.DisplayName == "" || utf8.RuneCountInString(input.DisplayName) > 200 {
		return AccountInput{}, &ValidationError{Field: "display_name", Message: "must contain 1 to 200 characters"}
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || !strings.EqualFold(address.Address, input.Email) || len(input.Email) > 320 {
		return AccountInput{}, &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	if !usernamePattern.MatchString(input.Username) {
		return AccountInput{}, &ValidationError{Field: "username", Message: "must contain 3 to 32 lowercase letters, numbers, dots, underscores, or hyphens"}
	}
	if err := validatePassword(input.Password); err != nil {
		return AccountInput{}, err
	}
	return input, nil
}

func sanitizeMetadata(metadata SessionMetadata) SessionMetadata {
	metadata.UserAgent = strings.TrimSpace(metadata.UserAgent)
	metadata.RemoteAddress = strings.TrimSpace(metadata.RemoteAddress)
	if len(metadata.UserAgent) > 500 {
		metadata.UserAgent = metadata.UserAgent[:500]
	}
	if len(metadata.RemoteAddress) > 100 {
		metadata.RemoteAddress = metadata.RemoteAddress[:100]
	}
	return metadata
}

func newSecret(prefix string) (string, [sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", digest, fmt.Errorf("generate authentication secret: %w", err)
	}
	secret := prefix + base64.RawURLEncoding.EncodeToString(raw)
	return secret, sha256.Sum256([]byte(secret)), nil
}

func newUserCode() (string, error) {
	raw := make([]byte, 5)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate device user code: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	return encoded[:4] + "-" + encoded[4:8], nil
}

func normalizeUserCode(raw string) string {
	compact := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(raw), "-", ""))
	if len(compact) != 8 {
		return compact
	}
	return compact[:4] + "-" + compact[4:]
}
