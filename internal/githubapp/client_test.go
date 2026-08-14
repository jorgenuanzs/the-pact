package githubapp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInstallationTokenIsScopedToOneRepository(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/app/installations/11/access_tokens" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		verifyAppJWT(t, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), &privateKey.PublicKey)
		var body struct {
			RepositoryIDs []int64           `json:"repository_ids"`
			Permissions   map[string]string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if len(body.RepositoryIDs) != 1 || body.RepositoryIDs[0] != 33 {
			t.Errorf("repository_ids = %#v", body.RepositoryIDs)
		}
		if body.Permissions["contents"] != "read" || body.Permissions["metadata"] != "read" {
			t.Errorf("permissions = %#v", body.Permissions)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": "scoped-token", "expires_at": time.Now().UTC().Add(time.Hour),
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		AppID: 123, ClientID: "client", ClientSecret: "secret",
		PrivateKeyBase64: encodePrivateKey(privateKey), APIURL: server.URL,
		WebURL: server.URL, RedirectURL: "https://pact.example/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, expiresAt, err := client.InstallationToken(context.Background(), 11, 33)
	if err != nil {
		t.Fatal(err)
	}
	if token != "scoped-token" || expiresAt.Before(time.Now().Add(50*time.Minute)) {
		t.Fatalf("token = %q; expires_at = %s", token, expiresAt)
	}
}

func TestExchangeCodeSendsPKCEVerifier(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login/oauth/access_token" {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if body["code"] != "oauth-code" || body["code_verifier"] != "pkce-verifier" {
			t.Errorf("OAuth body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"user-token"}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		AppID: 123, ClientID: "client", ClientSecret: "secret",
		PrivateKeyBase64: encodePrivateKey(privateKey), APIURL: server.URL,
		WebURL: server.URL, RedirectURL: "https://pact.example/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := client.ExchangeCode(context.Background(), "oauth-code", "pkce-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if token != "user-token" {
		t.Fatalf("token = %q", token)
	}
}

func encodePrivateKey(key *rsa.PrivateKey) string {
	block := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return base64.StdEncoding.EncodeToString(block)
}

func verifyAppJWT(t *testing.T, token string, publicKey *rsa.PublicKey) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT has %d parts", len(parts))
	}
	unsigned := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(unsigned))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("verify JWT signature: %v", err)
	}
	encodedPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Issuer int64 `json:"iss"`
		Issued int64 `json:"iat"`
		Expiry int64 `json:"exp"`
	}
	if err := json.Unmarshal(encodedPayload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Issuer != 123 || payload.Expiry <= payload.Issued || payload.Expiry-payload.Issued > 10*60 {
		t.Fatalf("JWT claims = %#v", payload)
	}
}
