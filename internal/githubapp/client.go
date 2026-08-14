package githubapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBody = 4 << 20

type ClientConfig struct {
	AppID            int64
	ClientID         string
	ClientSecret     string
	PrivateKeyBase64 string
	APIURL           string
	WebURL           string
	RedirectURL      string
	APIVersion       string
	UserAgent        string
	Timeout          time.Duration
	HTTPClient       *http.Client
}

type Client struct {
	appID        int64
	clientID     string
	clientSecret string
	privateKey   *rsa.PrivateKey
	apiURL       *url.URL
	webURL       *url.URL
	redirectURL  string
	apiVersion   string
	userAgent    string
	httpClient   *http.Client
	now          func() time.Time
}

func NewClient(config ClientConfig) (*Client, error) {
	if config.AppID <= 0 || strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.ClientSecret) == "" {
		return nil, errors.New("GitHub App ID, client ID, and client secret are required")
	}
	key, err := parsePrivateKey(config.PrivateKeyBase64)
	if err != nil {
		return nil, err
	}
	apiURL, err := parseBaseURL(config.APIURL, "GitHub API URL")
	if err != nil {
		return nil, err
	}
	webURL, err := parseBaseURL(config.WebURL, "GitHub web URL")
	if err != nil {
		return nil, err
	}
	redirectURL, err := url.Parse(strings.TrimSpace(config.RedirectURL))
	if err != nil || redirectURL.Scheme == "" || redirectURL.Host == "" {
		return nil, errors.New("GitHub App redirect URL must be absolute")
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many GitHub redirects")
				}
				if len(via) > 0 && !strings.EqualFold(request.URL.Host, via[0].URL.Host) {
					return errors.New("GitHub redirect changed host")
				}
				return nil
			},
		}
	}
	apiVersion := strings.TrimSpace(config.APIVersion)
	if apiVersion == "" {
		apiVersion = "2026-03-10"
	}
	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		userAgent = "the-pact"
	}
	return &Client{
		appID: config.AppID, clientID: strings.TrimSpace(config.ClientID),
		clientSecret: strings.TrimSpace(config.ClientSecret), privateKey: key,
		apiURL: apiURL, webURL: webURL, redirectURL: redirectURL.String(),
		apiVersion: apiVersion, userAgent: userAgent, httpClient: httpClient,
		now: time.Now,
	}, nil
}

func parsePrivateKey(encoded string) (*rsa.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode PACT_GITHUB_APP_PRIVATE_KEY_BASE64: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("PACT_GITHUB_APP_PRIVATE_KEY_BASE64 does not contain a PEM private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App private key must be RSA")
	}
	return key, nil
}

func parseBaseURL(raw, name string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%s must be absolute", name)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("%s must use http or https", name)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%s must not contain credentials, a query, or a fragment", name)
	}
	return parsed, nil
}

func (c *Client) ExchangeCode(ctx context.Context, code, codeVerifier string) (string, error) {
	body := map[string]string{
		"client_id": c.clientID, "client_secret": c.clientSecret,
		"code": strings.TrimSpace(code), "redirect_uri": c.redirectURL,
		"code_verifier": strings.TrimSpace(codeVerifier),
	}
	var response struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.webURL.String()+"/login/oauth/access_token", "", body, &response, http.StatusOK); err != nil {
		return "", err
	}
	if response.Error != "" || strings.TrimSpace(response.AccessToken) == "" {
		return "", &ProviderError{Code: "oauth_exchange_failed", Err: errors.New(response.ErrorDescription)}
	}
	return response.AccessToken, nil
}

func (c *Client) GetUserInstallation(ctx context.Context, userToken string, installationID int64) error {
	var response struct {
		ID int64 `json:"id"`
	}
	path := "/user/installations/" + strconv.FormatInt(installationID, 10)
	if err := c.api(ctx, http.MethodGet, path, userToken, nil, &response, http.StatusOK); err != nil {
		var providerErr *ProviderError
		if errors.As(err, &providerErr) && (providerErr.StatusCode == http.StatusNotFound || providerErr.StatusCode == http.StatusForbidden) {
			return ErrInstallationDenied
		}
		return err
	}
	if response.ID != installationID {
		return ErrInstallationDenied
	}
	return nil
}

func (c *Client) GetInstallation(ctx context.Context, installationID int64) (ProviderInstallation, error) {
	jwt, err := c.appJWT()
	if err != nil {
		return ProviderInstallation{}, err
	}
	var response struct {
		ID      int64 `json:"id"`
		Account struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
		RepositorySelection string            `json:"repository_selection"`
		Permissions         map[string]string `json:"permissions"`
		SuspendedAt         *time.Time        `json:"suspended_at"`
	}
	path := "/app/installations/" + strconv.FormatInt(installationID, 10)
	if err := c.api(ctx, http.MethodGet, path, jwt, nil, &response, http.StatusOK); err != nil {
		return ProviderInstallation{}, err
	}
	return ProviderInstallation{
		ID: response.ID, AccountID: response.Account.ID, AccountLogin: response.Account.Login,
		AccountType: response.Account.Type, RepositorySelection: response.RepositorySelection,
		Permissions: response.Permissions, SuspendedAt: response.SuspendedAt,
	}, nil
}

func (c *Client) ListRepositories(ctx context.Context, installationID int64) ([]ProviderRepository, error) {
	token, _, err := c.InstallationToken(ctx, installationID, 0)
	if err != nil {
		return nil, err
	}
	result := make([]ProviderRepository, 0)
	for page := 1; ; page++ {
		var response struct {
			Repositories []struct {
				ID            int64      `json:"id"`
				Name          string     `json:"name"`
				FullName      string     `json:"full_name"`
				Private       bool       `json:"private"`
				Visibility    string     `json:"visibility"`
				DefaultBranch string     `json:"default_branch"`
				HTMLURL       string     `json:"html_url"`
				CloneURL      string     `json:"clone_url"`
				UpdatedAt     *time.Time `json:"updated_at"`
				Owner         struct {
					Login string `json:"login"`
				} `json:"owner"`
			} `json:"repositories"`
		}
		path := "/installation/repositories?per_page=100&page=" + strconv.Itoa(page)
		if err := c.api(ctx, http.MethodGet, path, token, nil, &response, http.StatusOK); err != nil {
			return nil, err
		}
		for _, repository := range response.Repositories {
			visibility := strings.ToLower(strings.TrimSpace(repository.Visibility))
			if visibility != "public" && visibility != "private" && visibility != "internal" {
				if repository.Private {
					visibility = "private"
				} else {
					visibility = "public"
				}
			}
			result = append(result, ProviderRepository{
				ID: repository.ID, InstallationID: installationID, OwnerLogin: repository.Owner.Login,
				Name: repository.Name, FullName: repository.FullName, Private: repository.Private,
				Visibility: visibility, DefaultBranch: repository.DefaultBranch,
				HTMLURL: repository.HTMLURL, CloneURL: repository.CloneURL,
				ProviderUpdatedAt: repository.UpdatedAt,
			})
		}
		if len(response.Repositories) < 100 {
			break
		}
	}
	return result, nil
}

func (c *Client) InstallationToken(
	ctx context.Context, installationID, repositoryID int64,
) (string, time.Time, error) {
	jwt, err := c.appJWT()
	if err != nil {
		return "", time.Time{}, err
	}
	body := map[string]any{"permissions": map[string]string{"metadata": "read", "contents": "read"}}
	if repositoryID > 0 {
		body["repository_ids"] = []int64{repositoryID}
	}
	var response struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	path := "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens"
	if err := c.api(ctx, http.MethodPost, path, jwt, body, &response, http.StatusCreated); err != nil {
		return "", time.Time{}, err
	}
	if strings.TrimSpace(response.Token) == "" || response.ExpiresAt.IsZero() {
		return "", time.Time{}, &ProviderError{Code: "invalid_response", Err: errors.New("GitHub returned an invalid installation token")}
	}
	return response.Token, response.ExpiresAt, nil
}

func (c *Client) appJWT() (string, error) {
	now := c.now().UTC()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	payload, _ := json.Marshal(map[string]int64{
		"iat": now.Add(-60 * time.Second).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": c.appID,
	})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (c *Client) api(
	ctx context.Context, method, path, token string, body, output any, expected int,
) error {
	return c.doJSON(ctx, method, strings.TrimRight(c.apiURL.String(), "/")+path, token, body, output, expected)
}

func (c *Client) doJSON(
	ctx context.Context, method, endpoint, token string, body, output any, expected int,
) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return &ProviderError{Code: "request_failed", Err: err}
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", c.apiVersion)
	request.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return &ProviderError{Code: "upstream_unavailable", Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != expected {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBody))
		return &ProviderError{Code: providerErrorCode(response.StatusCode), StatusCode: response.StatusCode}
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBody)).Decode(output); err != nil {
		return &ProviderError{Code: "invalid_response", Err: err}
	}
	return nil
}

func providerErrorCode(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_required"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusUnprocessableEntity:
		return "invalid_request"
	case http.StatusTooManyRequests:
		return "rate_limited"
	default:
		if status >= 500 {
			return "upstream_unavailable"
		}
		return "unexpected_status"
	}
}
