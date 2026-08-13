package pactclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/projects"
)

const maxResponseBody = 2 << 20

type Client struct {
	baseURL    *url.URL
	apiToken   string
	httpClient *http.Client
}

type Problem struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func (p *Problem) Error() string {
	if p.Detail != "" {
		return p.Detail
	}
	if p.Title != "" {
		return p.Title
	}
	return fmt.Sprintf("Pact Server returned HTTP %d", p.Status)
}

func New(serverURL, apiToken string) (*Client, error) {
	client, err := newUnauthenticated(serverURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiToken) == "" {
		return nil, errors.New("Pact API token is required")
	}
	client.apiToken = apiToken
	return client, nil
}

func newUnauthenticated(serverURL string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil {
		return nil, fmt.Errorf("parse Pact Server URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("Pact Server URL must be an absolute http or https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Pact Server URL must not contain credentials, a query, or a fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return &Client{
		baseURL: parsed,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, nil
}

func (c *Client) ListProjects(ctx context.Context) ([]projects.Project, error) {
	var response struct {
		Data struct {
			Projects []projects.Project `json:"projects"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/projects", "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Projects == nil {
		response.Data.Projects = make([]projects.Project, 0)
	}
	return response.Data.Projects, nil
}

func (c *Client) CreateProject(
	ctx context.Context,
	idempotencyKey string,
	input projects.CreateInput,
) (projects.Project, error) {
	var response struct {
		Data projects.Project `json:"data"`
	}
	headers := map[string]string{"Idempotency-Key": idempotencyKey}
	if err := c.do(ctx, http.MethodPost, "/v1/projects", "application/json", request{
		Headers: headers,
		Body:    input,
	}, &response); err != nil {
		return projects.Project{}, err
	}
	return response.Data, nil
}

func (c *Client) StartAgentSession(
	ctx context.Context,
	projectID string,
	input agentsession.StartInput,
) (agentsession.Session, error) {
	var response struct {
		Data agentsession.Session `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/agent-sessions"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{Body: input}, &response); err != nil {
		return agentsession.Session{}, err
	}
	return response.Data, nil
}

func (c *Client) HeartbeatAgentSession(ctx context.Context, sessionID string) (agentsession.Session, error) {
	var response struct {
		Data agentsession.Session `json:"data"`
	}
	path := "/v1/agent-sessions/" + url.PathEscape(sessionID) + "/heartbeat"
	if err := c.do(ctx, http.MethodPost, path, "", nil, &response); err != nil {
		return agentsession.Session{}, err
	}
	return response.Data, nil
}

func (c *Client) CloseAgentSession(ctx context.Context, sessionID string) error {
	path := "/v1/agent-sessions/" + url.PathEscape(sessionID)
	return c.do(ctx, http.MethodDelete, path, "", nil, nil)
}

func (c *Client) Me(ctx context.Context) (access.Principal, error) {
	var response struct {
		Data access.Principal `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/me", "", nil, &response); err != nil {
		return access.Principal{}, err
	}
	return response.Data, nil
}

func (c *Client) CreateInvitation(
	ctx context.Context,
	projectID string,
	email string,
	role string,
	expiresInHours int,
) (access.CreatedInvitation, error) {
	var response struct {
		Data access.CreatedInvitation `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/invitations"
	body := map[string]any{"email": email, "role": role, "expires_in_hours": expiresInHours}
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{Body: body}, &response); err != nil {
		return access.CreatedInvitation{}, err
	}
	return response.Data, nil
}

func AcceptInvitation(
	ctx context.Context,
	serverURL string,
	input access.AcceptInvitationInput,
) (access.AcceptedInvitation, error) {
	client, err := newUnauthenticated(serverURL)
	if err != nil {
		return access.AcceptedInvitation{}, err
	}
	var response struct {
		Data access.AcceptedInvitation `json:"data"`
	}
	if err := client.do(ctx, http.MethodPost, "/v1/invitation-acceptances", "application/json", request{Body: input}, &response); err != nil {
		return access.AcceptedInvitation{}, err
	}
	return response.Data, nil
}

func (c *Client) RevokeCurrentToken(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/v1/me/tokens/current", "", nil, nil)
}

type request struct {
	Headers map[string]string
	Body    any
}

func (c *Client) do(
	ctx context.Context,
	method string,
	path string,
	contentType string,
	payload any,
	target any,
) error {
	var body io.Reader
	headers := make(map[string]string)
	if requestPayload, ok := payload.(request); ok {
		encoded, err := json.Marshal(requestPayload.Body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
		for key, value := range requestPayload.Headers {
			headers[key] = value
		}
	}

	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	httpRequest, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")
	if c.apiToken != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+c.apiToken)
	}
	if contentType != "" {
		httpRequest.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		httpRequest.Header.Set(key, value)
	}

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("contact Pact Server: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	if err != nil {
		return fmt.Errorf("read Pact Server response: %w", err)
	}
	if len(responseBody) > maxResponseBody {
		return errors.New("Pact Server response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		problem := &Problem{Status: response.StatusCode}
		if err := json.Unmarshal(responseBody, problem); err != nil {
			problem.Detail = strings.TrimSpace(string(responseBody))
		}
		return problem
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return fmt.Errorf("decode Pact Server response: %w", err)
	}
	return nil
}
