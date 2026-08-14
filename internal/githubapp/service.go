package githubapp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const connectionLifetime = 10 * time.Minute

type RepositoryStore interface {
	CreateAttempt(context.Context, string, string, [sha256.Size]byte, time.Time) error
	SetAttemptInstallation(context.Context, [sha256.Size]byte, int64) error
	ConsumeAttempt(context.Context, [sha256.Size]byte) (string, string, int64, error)
	UpsertInstallation(context.Context, string, ProviderInstallation, []ProviderRepository) error
	UpdateInstallationStatus(context.Context, string, int64, string, time.Time) error
	Status(context.Context, string, bool) (Status, error)
	InstallationForRepository(context.Context, string, string) (int64, int64, error)
	BeginDelivery(context.Context, string, string, string, [sha256.Size]byte) (bool, error)
	CompleteDelivery(context.Context, string, string, string, string) error
}

type ProviderClient interface {
	ExchangeCode(context.Context, string, string) (string, error)
	GetUserInstallation(context.Context, string, int64) error
	GetInstallation(context.Context, int64) (ProviderInstallation, error)
	ListRepositories(context.Context, int64) ([]ProviderRepository, error)
	InstallationToken(context.Context, int64, int64) (string, time.Time, error)
}

type ServiceConfig struct {
	OrganizationID string
	AppSlug        string
	WebURL         string
	WebhookSecret  string
	ClientID       string
	RedirectURL    string
	OAuthSecret    string
	Configured     bool
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

type Service struct {
	organizationID string
	appSlug        string
	webURL         string
	webhookSecret  []byte
	clientID       string
	redirectURL    string
	oauthSecret    []byte
	configured     bool
	repository     RepositoryStore
	provider       ProviderClient
	now            func() time.Time
	tokenMu        sync.Mutex
	tokens         map[string]cachedToken
}

func NewService(config ServiceConfig, repository RepositoryStore, provider ProviderClient) *Service {
	return &Service{
		organizationID: strings.TrimSpace(config.OrganizationID),
		appSlug:        strings.TrimSpace(config.AppSlug),
		webURL:         strings.TrimRight(strings.TrimSpace(config.WebURL), "/"),
		webhookSecret:  []byte(config.WebhookSecret),
		clientID:       strings.TrimSpace(config.ClientID),
		redirectURL:    strings.TrimSpace(config.RedirectURL),
		oauthSecret:    []byte(config.OAuthSecret),
		configured:     config.Configured,
		repository:     repository, provider: provider, now: time.Now,
		tokens: make(map[string]cachedToken),
	}
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	return s.repository.Status(ctx, s.organizationID, s.configured)
}

func (s *Service) Connect(ctx context.Context, principalID string) (Connection, error) {
	if !s.configured || s.provider == nil {
		return Connection{}, ErrNotConfigured
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Connection{}, fmt.Errorf("generate GitHub connection state: %w", err)
	}
	state := hex.EncodeToString(raw)
	digest := sha256.Sum256([]byte(state))
	expiresAt := s.now().UTC().Add(connectionLifetime)
	if err := s.repository.CreateAttempt(ctx, s.organizationID, principalID, digest, expiresAt); err != nil {
		return Connection{}, err
	}
	installURL := s.webURL + "/apps/" + url.PathEscape(s.appSlug) + "/installations/new?state=" + url.QueryEscape(state)
	return Connection{InstallURL: installURL, ExpiresAt: expiresAt}, nil
}

func (s *Service) CompleteConnection(ctx context.Context, state, code string) error {
	if !s.configured || s.provider == nil {
		return ErrNotConfigured
	}
	state = strings.TrimSpace(state)
	code = strings.TrimSpace(code)
	if state == "" || code == "" {
		return ErrInvalidState
	}
	digest := sha256.Sum256([]byte(state))
	organizationID, _, installationID, err := s.repository.ConsumeAttempt(ctx, digest)
	if err != nil {
		return err
	}
	if organizationID != s.organizationID {
		return ErrInvalidState
	}
	userToken, err := s.provider.ExchangeCode(ctx, code, oauthCodeVerifier(s.oauthSecret, state))
	if err != nil {
		return err
	}
	if err := s.provider.GetUserInstallation(ctx, userToken, installationID); err != nil {
		return err
	}
	return s.syncInstallation(ctx, organizationID, installationID)
}

func (s *Service) BeginUserAuthorization(
	ctx context.Context, state string, installationID int64,
) (string, error) {
	if !s.configured || s.provider == nil {
		return "", ErrNotConfigured
	}
	state = strings.TrimSpace(state)
	if state == "" || installationID <= 0 {
		return "", ErrInvalidState
	}
	digest := sha256.Sum256([]byte(state))
	if err := s.repository.SetAttemptInstallation(ctx, digest, installationID); err != nil {
		return "", err
	}
	query := url.Values{
		"client_id":             []string{s.clientID},
		"redirect_uri":          []string{s.redirectURL},
		"state":                 []string{state},
		"code_challenge":        []string{oauthCodeChallenge(s.oauthSecret, state)},
		"code_challenge_method": []string{"S256"},
	}
	return s.webURL + "/login/oauth/authorize?" + query.Encode(), nil
}

func oauthCodeVerifier(secret []byte, state string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(state))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func oauthCodeChallenge(secret []byte, state string) string {
	digest := sha256.Sum256([]byte(oauthCodeVerifier(secret, state)))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (s *Service) TokenForRepository(ctx context.Context, fullName string) (string, error) {
	if !s.configured || s.provider == nil {
		return "", ErrNotConfigured
	}
	fullName = strings.ToLower(strings.TrimSpace(fullName))
	installationID, repositoryID, err := s.repository.InstallationForRepository(ctx, s.organizationID, fullName)
	if err != nil {
		return "", err
	}
	key := strconv.FormatInt(installationID, 10) + ":" + strconv.FormatInt(repositoryID, 10)
	s.tokenMu.Lock()
	cached, found := s.tokens[key]
	if found && s.now().Before(cached.expiresAt.Add(-time.Minute)) {
		s.tokenMu.Unlock()
		return cached.token, nil
	}
	s.tokenMu.Unlock()
	token, expiresAt, err := s.provider.InstallationToken(ctx, installationID, repositoryID)
	if err != nil {
		return "", err
	}
	s.tokenMu.Lock()
	s.tokens[key] = cachedToken{token: token, expiresAt: expiresAt}
	s.tokenMu.Unlock()
	return token, nil
}

func (s *Service) HandleWebhook(
	ctx context.Context, deliveryID, eventType, signature string, body []byte,
) error {
	if !s.configured || len(s.webhookSecret) == 0 {
		return ErrNotConfigured
	}
	if !validWebhookSignature(s.webhookSecret, signature, body) {
		return ErrWebhookSignature
	}
	deliveryID = strings.TrimSpace(deliveryID)
	eventType = strings.TrimSpace(eventType)
	if deliveryID == "" || eventType == "" {
		return errors.New("GitHub webhook delivery and event headers are required")
	}
	hash := sha256.Sum256(body)
	reserved, err := s.repository.BeginDelivery(ctx, s.organizationID, deliveryID, eventType, hash)
	if err != nil || !reserved {
		return err
	}
	processErr := s.processWebhook(ctx, eventType, body)
	if processErr != nil {
		_ = s.repository.CompleteDelivery(ctx, s.organizationID, deliveryID, "failed", "processing_failed")
		return processErr
	}
	return s.repository.CompleteDelivery(ctx, s.organizationID, deliveryID, "processed", "")
}

func (s *Service) processWebhook(ctx context.Context, eventType string, body []byte) error {
	if eventType == "ping" {
		return nil
	}
	var envelope struct {
		Action       string `json:"action"`
		Installation struct {
			ID          int64      `json:"id"`
			SuspendedAt *time.Time `json:"suspended_at"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode GitHub webhook: %w", err)
	}
	if envelope.Installation.ID <= 0 {
		return nil
	}
	s.clearTokenCache()
	switch eventType {
	case "installation":
		switch envelope.Action {
		case "deleted":
			return s.repository.UpdateInstallationStatus(
				ctx, s.organizationID, envelope.Installation.ID, "deleted", s.now().UTC(),
			)
		case "suspend":
			at := s.now().UTC()
			if envelope.Installation.SuspendedAt != nil {
				at = *envelope.Installation.SuspendedAt
			}
			return s.repository.UpdateInstallationStatus(
				ctx, s.organizationID, envelope.Installation.ID, "suspended", at,
			)
		case "created", "new_permissions_accepted", "unsuspend":
			return s.syncInstallation(ctx, s.organizationID, envelope.Installation.ID)
		}
	case "installation_repositories":
		return s.syncInstallation(ctx, s.organizationID, envelope.Installation.ID)
	}
	return nil
}

func (s *Service) syncInstallation(ctx context.Context, organizationID string, installationID int64) error {
	installation, err := s.provider.GetInstallation(ctx, installationID)
	if err != nil {
		return err
	}
	repositories, err := s.provider.ListRepositories(ctx, installationID)
	if err != nil {
		return err
	}
	if err := s.repository.UpsertInstallation(ctx, organizationID, installation, repositories); err != nil {
		return err
	}
	s.clearTokenCache()
	return nil
}

func (s *Service) clearTokenCache() {
	s.tokenMu.Lock()
	s.tokens = make(map[string]cachedToken)
	s.tokenMu.Unlock()
}

func validWebhookSignature(secret []byte, signature string, body []byte) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return false
	}
	received, err := hex.DecodeString(strings.TrimPrefix(signature, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hmac.Equal(received, mac.Sum(nil))
}
