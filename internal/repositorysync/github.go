package repositorysync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	defaultGitHubAPIURL     = "https://api.github.com"
	defaultGitHubAPIVersion = "2026-03-10"
	maxGitHubResponseBody   = 2 << 20
)

var githubNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type Provider interface {
	Fetch(context.Context, Reference) (Snapshot, error)
}

type GitHubClient struct {
	baseURL    *url.URL
	token      string
	apiVersion string
	httpClient *http.Client
	userAgent  string
}

type GitHubOptions struct {
	APIURL     string
	Token      string
	APIVersion string
	Timeout    time.Duration
	UserAgent  string
	HTTPClient *http.Client
}

func NewGitHubClient(options GitHubOptions) (*GitHubClient, error) {
	apiURL := strings.TrimSpace(options.APIURL)
	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API URL: %w", err)
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil, errors.New("GitHub API URL must be an absolute http or https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("GitHub API URL must not contain credentials, a query, or a fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if options.Timeout <= 0 {
		options.Timeout = 10 * time.Second
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: options.Timeout,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many GitHub API redirects")
				}
				if !strings.EqualFold(request.URL.Host, parsed.Host) {
					return errors.New("GitHub API redirect changed host")
				}
				return nil
			},
		}
	}
	apiVersion := strings.TrimSpace(options.APIVersion)
	if apiVersion == "" {
		apiVersion = defaultGitHubAPIVersion
	}
	userAgent := strings.TrimSpace(options.UserAgent)
	if userAgent == "" {
		userAgent = "the-pact"
	}
	return &GitHubClient{
		baseURL: parsed, token: strings.TrimSpace(options.Token), apiVersion: apiVersion,
		httpClient: client, userAgent: userAgent,
	}, nil
}

func ParseGitHubRemote(raw string) (Reference, error) {
	remote := strings.TrimSpace(raw)
	if remote == "" {
		return Reference{}, ErrUnsupportedRemote
	}

	var owner, name string
	if strings.HasPrefix(remote, "git@github.com:") {
		owner, name = splitGitHubPath(strings.TrimPrefix(remote, "git@github.com:"))
	} else {
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Hostname(), "github.com") {
			return Reference{}, ErrUnsupportedRemote
		}
		if parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "ssh" && parsed.Scheme != "git" {
			return Reference{}, ErrUnsupportedRemote
		}
		if parsed.User != nil && parsed.Scheme != "ssh" {
			return Reference{}, &ValidationError{Field: "remote_url", Message: "must not contain credentials"}
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return Reference{}, &ValidationError{Field: "remote_url", Message: "must not contain a query or fragment"}
		}
		owner, name = splitGitHubPath(parsed.Path)
	}
	name = strings.TrimSuffix(name, ".git")
	if owner == "" || name == "" || len(owner) > 100 || len(name) > 100 ||
		!githubNamePattern.MatchString(owner) || !githubNamePattern.MatchString(name) {
		return Reference{}, &ValidationError{Field: "remote_url", Message: "must identify one GitHub owner and repository"}
	}
	return Reference{Owner: owner, Name: name, FullName: owner + "/" + name}, nil
}

func splitGitHubPath(raw string) (string, string) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(raw), "/"), "/")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func (c *GitHubClient) Fetch(ctx context.Context, reference Reference) (Snapshot, error) {
	var repository struct {
		FullName      string     `json:"full_name"`
		DefaultBranch string     `json:"default_branch"`
		Visibility    string     `json:"visibility"`
		Private       bool       `json:"private"`
		UpdatedAt     *time.Time `json:"updated_at"`
	}
	path := "/repos/" + url.PathEscape(reference.Owner) + "/" + url.PathEscape(reference.Name)
	if err := c.get(ctx, path, &repository); err != nil {
		return Snapshot{}, err
	}
	if strings.TrimSpace(repository.DefaultBranch) == "" {
		return Snapshot{}, &ProviderError{Code: "invalid_response", Err: errors.New("GitHub returned an empty default branch")}
	}
	repository.DefaultBranch = strings.TrimSpace(repository.DefaultBranch)
	if len(repository.DefaultBranch) > 255 {
		return Snapshot{}, &ProviderError{Code: "invalid_response", Err: errors.New("GitHub returned an oversized default branch")}
	}

	var referenceState struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := c.get(ctx, path+"/git/ref/heads/"+escapePath(repository.DefaultBranch), &referenceState); err != nil {
		return Snapshot{}, err
	}
	referenceState.Object.SHA = strings.ToLower(strings.TrimSpace(referenceState.Object.SHA))
	if !revisionPattern.MatchString(referenceState.Object.SHA) {
		return Snapshot{}, &ProviderError{Code: "invalid_response", Err: errors.New("GitHub returned an invalid commit object ID")}
	}
	visibility := strings.ToLower(strings.TrimSpace(repository.Visibility))
	if visibility != "public" && visibility != "private" && visibility != "internal" {
		if repository.Private {
			visibility = "private"
		} else {
			visibility = "public"
		}
	}
	fullName := strings.TrimSpace(repository.FullName)
	if fullName == "" {
		fullName = reference.FullName
	}
	if len(fullName) > 202 {
		return Snapshot{}, &ProviderError{Code: "invalid_response", Err: errors.New("GitHub returned an oversized repository name")}
	}
	return Snapshot{
		Provider: "github", RepositoryFullName: fullName,
		DefaultBranch: repository.DefaultBranch, CanonicalRevision: referenceState.Object.SHA,
		Visibility: visibility, ProviderUpdatedAt: repository.UpdatedAt,
	}, nil
}

func escapePath(value string) string {
	parts := strings.Split(value, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func (c *GitHubClient) get(ctx context.Context, path string, output any) error {
	requestURL := strings.TrimRight(c.baseURL.String(), "/") + path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return &ProviderError{Code: "request_failed", Err: err}
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", c.apiVersion)
	request.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return &ProviderError{Code: "upstream_unavailable", Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxGitHubResponseBody))
		code := githubErrorCode(response.StatusCode, response.Header)
		return &ProviderError{
			Code: code, StatusCode: response.StatusCode,
			RetryAfter: strings.TrimSpace(response.Header.Get("Retry-After")),
		}
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxGitHubResponseBody))
	if err := decoder.Decode(output); err != nil {
		return &ProviderError{Code: "invalid_response", Err: err}
	}
	return nil
}

func githubErrorCode(status int, header http.Header) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_required"
	case http.StatusForbidden:
		if strings.TrimSpace(header.Get("X-RateLimit-Remaining")) == "0" || header.Get("Retry-After") != "" {
			return "rate_limited"
		}
		return "forbidden"
	case http.StatusNotFound:
		return "repository_not_found_or_inaccessible"
	case http.StatusTooManyRequests:
		return "rate_limited"
	default:
		if status >= 500 {
			return "upstream_unavailable"
		}
		return "unexpected_status"
	}
}

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
