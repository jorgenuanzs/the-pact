package githubapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"testing"
	"time"
)

type fakeStore struct {
	digest         [sha256.Size]byte
	organizationID string
	principalID    string
	installationID int64
	repositoryID   int64
	upserted       bool
	deliveryNew    bool
	deliveryStatus string
}

func (f *fakeStore) CreateAttempt(_ context.Context, organizationID, principalID string, digest [sha256.Size]byte, _ time.Time) error {
	f.organizationID, f.principalID, f.digest = organizationID, principalID, digest
	return nil
}
func (f *fakeStore) SetAttemptInstallation(_ context.Context, digest [sha256.Size]byte, installationID int64) error {
	if digest != f.digest {
		return ErrInvalidState
	}
	f.installationID = installationID
	return nil
}
func (f *fakeStore) ConsumeAttempt(_ context.Context, digest [sha256.Size]byte) (string, string, int64, error) {
	if digest != f.digest {
		return "", "", 0, ErrInvalidState
	}
	return f.organizationID, f.principalID, f.installationID, nil
}
func (f *fakeStore) UpsertInstallation(_ context.Context, _ string, installation ProviderInstallation, repositories []ProviderRepository) error {
	f.upserted = installation.ID > 0 && len(repositories) > 0
	return nil
}
func (*fakeStore) UpdateInstallationStatus(context.Context, string, int64, string, time.Time) error {
	return nil
}
func (*fakeStore) Status(context.Context, string, bool) (Status, error) { return Status{}, nil }
func (f *fakeStore) InstallationForRepository(context.Context, string, string) (int64, int64, error) {
	return f.installationID, f.repositoryID, nil
}
func (f *fakeStore) BeginDelivery(context.Context, string, string, string, [sha256.Size]byte) (bool, error) {
	return f.deliveryNew, nil
}
func (f *fakeStore) CompleteDelivery(_ context.Context, _, _, status, _ string) error {
	f.deliveryStatus = status
	return nil
}

type fakeProvider struct {
	userVerified           bool
	installationTokenCalls int
}

func (*fakeProvider) ExchangeCode(_ context.Context, _, verifier string) (string, error) {
	if verifier == "" {
		return "", errors.New("missing PKCE verifier")
	}
	return "user-token", nil
}
func (f *fakeProvider) GetUserInstallation(_ context.Context, token string, installationID int64) error {
	if token != "user-token" || installationID <= 0 {
		return ErrInstallationDenied
	}
	f.userVerified = true
	return nil
}
func (f *fakeProvider) GetInstallation(_ context.Context, installationID int64) (ProviderInstallation, error) {
	if !f.userVerified {
		return ProviderInstallation{}, errors.New("installation read before user verification")
	}
	return ProviderInstallation{
		ID: installationID, AccountID: 22, AccountLogin: "acme", AccountType: "Organization",
		RepositorySelection: "selected", Permissions: map[string]string{"contents": "read"},
	}, nil
}
func (f *fakeProvider) ListRepositories(_ context.Context, installationID int64) ([]ProviderRepository, error) {
	return []ProviderRepository{{ID: 33, InstallationID: installationID, FullName: "acme/app"}}, nil
}
func (f *fakeProvider) InstallationToken(context.Context, int64, int64) (string, time.Time, error) {
	f.installationTokenCalls++
	return "installation-token", time.Now().Add(time.Hour), nil
}

func TestConnectionVerifiesUserBeforePersistingInstallation(t *testing.T) {
	store := &fakeStore{}
	provider := &fakeProvider{}
	service := NewService(ServiceConfig{
		OrganizationID: "organization", AppSlug: "the-pact", WebURL: "https://github.com",
		WebhookSecret: "secret", ClientID: "client-id", RedirectURL: "https://pact.example/callback",
		OAuthSecret: "oauth-secret",
		Configured:  true,
	}, store, provider)
	connection, err := service.Connect(context.Background(), "principal")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(connection.InstallURL)
	if err != nil {
		t.Fatal(err)
	}
	state := parsed.Query().Get("state")
	if state == "" || parsed.Path != "/apps/the-pact/installations/new" {
		t.Fatalf("install URL = %q", connection.InstallURL)
	}
	authorizationURL, err := service.BeginUserAuthorization(context.Background(), state, 11)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := url.Parse(authorizationURL)
	if err != nil || authorization.Path != "/login/oauth/authorize" || authorization.Query().Get("state") != state ||
		authorization.Query().Get("code_challenge_method") != "S256" || authorization.Query().Get("code_challenge") == "" {
		t.Fatalf("authorization URL = %q; error = %v", authorizationURL, err)
	}
	if err := service.CompleteConnection(context.Background(), state, "oauth-code"); err != nil {
		t.Fatal(err)
	}
	if !provider.userVerified || !store.upserted {
		t.Fatalf("user verified = %v; installation persisted = %v", provider.userVerified, store.upserted)
	}
}

func TestRepositoryTokenIsCachedBeforeExpiry(t *testing.T) {
	store := &fakeStore{installationID: 11, repositoryID: 33}
	provider := &fakeProvider{}
	service := NewService(ServiceConfig{OrganizationID: "organization", Configured: true}, store, provider)
	first, err := service.TokenForRepository(context.Background(), "acme/app")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.TokenForRepository(context.Background(), "ACME/APP")
	if err != nil {
		t.Fatal(err)
	}
	if first != "installation-token" || second != first || provider.installationTokenCalls != 1 {
		t.Fatalf("tokens = %q, %q; calls = %d", first, second, provider.installationTokenCalls)
	}
}

func TestWebhookRejectsInvalidSignatureAndDeduplicatesDelivery(t *testing.T) {
	store := &fakeStore{deliveryNew: true}
	provider := &fakeProvider{userVerified: true}
	service := NewService(ServiceConfig{
		OrganizationID: "organization", WebhookSecret: "webhook-secret", Configured: true,
	}, store, provider)
	body := []byte(`{"zen":"safe"}`)
	if err := service.HandleWebhook(context.Background(), "delivery", "ping", "sha256=00", body); !errors.Is(err, ErrWebhookSignature) {
		t.Fatalf("invalid signature error = %v", err)
	}
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if err := service.HandleWebhook(context.Background(), "delivery", "ping", signature, body); err != nil {
		t.Fatal(err)
	}
	if store.deliveryStatus != "processed" {
		t.Fatalf("delivery status = %q", store.deliveryStatus)
	}
	store.deliveryNew = false
	if err := service.HandleWebhook(context.Background(), "delivery", "ping", signature, body); err != nil {
		t.Fatal(err)
	}
}
