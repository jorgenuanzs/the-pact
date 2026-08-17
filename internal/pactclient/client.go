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
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/contextpack"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/projectrepo"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/repositorysync"
	"github.com/jorgenuanzs/the-pact/internal/rooms"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

const maxResponseBody = 2 << 20

type Client struct {
	baseURL    *url.URL
	apiToken   string
	httpClient *http.Client
}

type Problem struct {
	Status   int                         `json:"status"`
	Code     string                      `json:"code"`
	Title    string                      `json:"title"`
	Detail   string                      `json:"detail"`
	Overlaps []coordination.ScopeOverlap `json:"overlaps,omitempty"`
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

func (c *Client) GetRepositorySync(ctx context.Context, projectID string) (repositorysync.State, error) {
	var response struct {
		Data repositorysync.State `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/repository-sync"
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return repositorysync.State{}, err
	}
	return response.Data, nil
}

func (c *Client) SyncRepository(
	ctx context.Context, projectID, idempotencyKey string,
) (repositorysync.Result, error) {
	var response struct {
		Data repositorysync.Result `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/repository-sync"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey},
		Body:    map[string]any{},
	}, &response); err != nil {
		return repositorysync.Result{}, err
	}
	return response.Data, nil
}

type ProjectRepositories struct {
	Repositories []projectrepo.Repository `json:"repositories"`
	SyncStates   []repositorysync.State   `json:"sync_states"`
}

func (c *Client) ListProjectRepositories(
	ctx context.Context, projectID string,
) (ProjectRepositories, error) {
	var response struct {
		Data ProjectRepositories `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/repositories"
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return ProjectRepositories{}, err
	}
	if response.Data.Repositories == nil {
		response.Data.Repositories = make([]projectrepo.Repository, 0)
	}
	if response.Data.SyncStates == nil {
		response.Data.SyncStates = make([]repositorysync.State, 0)
	}
	return response.Data, nil
}

func (c *Client) GetProjectRepositorySync(
	ctx context.Context, projectID, repositoryID string,
) (repositorysync.State, error) {
	var response struct {
		Data repositorysync.State `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/repositories/" +
		url.PathEscape(repositoryID) + "/sync"
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return repositorysync.State{}, err
	}
	return response.Data, nil
}

func (c *Client) SyncProjectRepository(
	ctx context.Context, projectID, repositoryID, idempotencyKey string,
) (repositorysync.Result, error) {
	var response struct {
		Data repositorysync.Result `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/repositories/" +
		url.PathEscape(repositoryID) + "/sync"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: map[string]any{},
	}, &response); err != nil {
		return repositorysync.Result{}, err
	}
	return response.Data, nil
}

func (c *Client) ListWorkspaces(ctx context.Context) ([]workspaces.Workspace, error) {
	var response struct {
		Data struct {
			Workspaces []workspaces.Workspace `json:"workspaces"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/workspaces", "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Workspaces == nil {
		response.Data.Workspaces = make([]workspaces.Workspace, 0)
	}
	return response.Data.Workspaces, nil
}

func (c *Client) GetWorkspace(ctx context.Context, reference string) (workspaces.Workspace, error) {
	var response struct {
		Data workspaces.Workspace `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(reference)
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return workspaces.Workspace{}, err
	}
	return response.Data, nil
}

func (c *Client) CreateWorkspace(
	ctx context.Context,
	idempotencyKey string,
	input workspaces.CreateInput,
) (workspaces.Workspace, error) {
	var response struct {
		Data workspaces.Workspace `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/workspaces", "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return workspaces.Workspace{}, err
	}
	return response.Data, nil
}

func (c *Client) AttachWorkspaceProject(
	ctx context.Context,
	workspaceID string,
	projectID string,
) (workspaces.Workspace, error) {
	var response struct {
		Data workspaces.Workspace `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/projects/" + url.PathEscape(projectID)
	if err := c.do(ctx, http.MethodPut, path, "", nil, &response); err != nil {
		return workspaces.Workspace{}, err
	}
	return response.Data, nil
}

func (c *Client) ListRooms(ctx context.Context, workspaceID string) ([]rooms.Room, error) {
	var response struct {
		Data struct {
			Rooms []rooms.Room `json:"rooms"`
		} `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/rooms"
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Rooms == nil {
		response.Data.Rooms = make([]rooms.Room, 0)
	}
	return response.Data.Rooms, nil
}

func (c *Client) ListRoomParticipants(ctx context.Context, workspaceID string) ([]rooms.Participant, error) {
	var response struct {
		Data struct {
			Participants []rooms.Participant `json:"participants"`
		} `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/participants"
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Participants == nil {
		response.Data.Participants = make([]rooms.Participant, 0)
	}
	return response.Data.Participants, nil
}

func (c *Client) ListRoomMessages(ctx context.Context, workspaceID, roomID string, options rooms.MessageListOptions) ([]rooms.Message, error) {
	var response struct {
		Data struct {
			Messages []rooms.Message `json:"messages"`
		} `json:"data"`
	}
	values := make(url.Values)
	if options.BeforeMessageID != "" {
		values.Set("before", options.BeforeMessageID)
	}
	if options.ThreadRootMessageID != "" {
		values.Set("thread_root", options.ThreadRootMessageID)
	}
	if options.Query != "" {
		values.Set("q", options.Query)
	}
	if options.Limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", options.Limit))
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/rooms/" + url.PathEscape(roomID) + "/messages"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Messages == nil {
		response.Data.Messages = make([]rooms.Message, 0)
	}
	return response.Data.Messages, nil
}

func (c *Client) CreateRoomMessage(ctx context.Context, workspaceID, roomID, idempotencyKey string, input rooms.CreateMessageInput) (rooms.Message, error) {
	var response struct {
		Data rooms.Message `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/rooms/" + url.PathEscape(roomID) + "/messages"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return rooms.Message{}, err
	}
	return response.Data, nil
}

func (c *Client) ListAgentRoomMentions(ctx context.Context, sessionID string, options rooms.InboxOptions) ([]rooms.Mention, error) {
	var response struct {
		Data struct {
			Mentions []rooms.Mention `json:"mentions"`
		} `json:"data"`
	}
	values := make(url.Values)
	if options.WorkspaceID != "" {
		values.Set("workspace_id", options.WorkspaceID)
	}
	if options.Status != "" {
		values.Set("status", options.Status)
	}
	if options.Limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", options.Limit))
	}
	path := "/v1/agent-sessions/" + url.PathEscape(sessionID) + "/room-mentions"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Mentions == nil {
		response.Data.Mentions = make([]rooms.Mention, 0)
	}
	return response.Data.Mentions, nil
}

func (c *Client) UpdateAgentRoomMention(ctx context.Context, sessionID, mentionID, status string) (rooms.Mention, error) {
	var response struct {
		Data rooms.Mention `json:"data"`
	}
	path := "/v1/agent-sessions/" + url.PathEscape(sessionID) + "/room-mentions/" + url.PathEscape(mentionID) + "/status"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Body: rooms.MentionStatusInput{Status: status},
	}, &response); err != nil {
		return rooms.Mention{}, err
	}
	return response.Data, nil
}

func (c *Client) ListResources(
	ctx context.Context, workspaceID string, options knowledge.ListOptions,
) ([]knowledge.Resource, error) {
	var response struct {
		Data struct {
			Resources []knowledge.Resource `json:"resources"`
		} `json:"data"`
	}
	query := knowledgeQuery(options, "kind")
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/resources"
	if query != "" {
		path += "?" + query
	}
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Resources == nil {
		response.Data.Resources = make([]knowledge.Resource, 0)
	}
	return response.Data.Resources, nil
}

func (c *Client) CreateResource(
	ctx context.Context, workspaceID, idempotencyKey string, input knowledge.CreateResourceInput,
) (knowledge.Resource, error) {
	var response struct {
		Data knowledge.Resource `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/resources"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return knowledge.Resource{}, err
	}
	return response.Data, nil
}

func (c *Client) ListRecords(
	ctx context.Context, workspaceID string, options knowledge.ListOptions,
) ([]knowledge.Record, error) {
	var response struct {
		Data struct {
			Records []knowledge.Record `json:"records"`
		} `json:"data"`
	}
	query := knowledgeQuery(options, "type")
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/records"
	if query != "" {
		path += "?" + query
	}
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Records == nil {
		response.Data.Records = make([]knowledge.Record, 0)
	}
	return response.Data.Records, nil
}

func (c *Client) CreateRecord(
	ctx context.Context, workspaceID, idempotencyKey string, input knowledge.CreateRecordInput,
) (knowledge.Record, error) {
	var response struct {
		Data knowledge.Record `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/records"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return knowledge.Record{}, err
	}
	return response.Data, nil
}

func (c *Client) UpdateRecordStatus(
	ctx context.Context, workspaceID, recordID, idempotencyKey string, input knowledge.RecordStatusInput,
) (knowledge.Record, error) {
	var response struct {
		Data knowledge.Record `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/records/" + url.PathEscape(recordID) + "/status"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return knowledge.Record{}, err
	}
	return response.Data, nil
}

func (c *Client) WorkspaceContext(ctx context.Context, workspaceID string) (knowledge.WorkspaceContext, error) {
	var response struct {
		Data knowledge.WorkspaceContext `json:"data"`
	}
	path := "/v1/workspaces/" + url.PathEscape(workspaceID) + "/context"
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return knowledge.WorkspaceContext{}, err
	}
	return response.Data, nil
}

func knowledgeQuery(options knowledge.ListOptions, kindKey string) string {
	values := make(url.Values)
	if strings.TrimSpace(options.Query) != "" {
		values.Set("q", options.Query)
	}
	if strings.TrimSpace(options.Kind) != "" {
		values.Set(kindKey, options.Kind)
	}
	if strings.TrimSpace(options.Status) != "" {
		values.Set("status", options.Status)
	}
	if options.Limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", options.Limit))
	}
	return values.Encode()
}

func (c *Client) GetProjectOverview(ctx context.Context, projectID string) (projects.Project, backoffice.Overview, error) {
	var response struct {
		Data struct {
			Project projects.Project `json:"project"`
			backoffice.Overview
		} `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/overview"
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return projects.Project{}, backoffice.Overview{}, err
	}
	return response.Data.Project, response.Data.Overview, nil
}

func (c *Client) CheckScopes(
	ctx context.Context,
	projectID string,
	scopes []coordination.ScopeInput,
) (coordination.ScopeCheckResult, error) {
	var response struct {
		Data coordination.ScopeCheckResult `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/scope-checks"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Body: map[string]any{"scopes": scopes},
	}, &response); err != nil {
		return coordination.ScopeCheckResult{}, err
	}
	return response.Data, nil
}

func (c *Client) StartWork(
	ctx context.Context,
	projectID string,
	idempotencyKey string,
	input coordination.StartInput,
) (coordination.StartResult, error) {
	var response struct {
		Data coordination.StartResult `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/work-items"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return coordination.StartResult{}, err
	}
	return response.Data, nil
}

func (c *Client) ListHandoffs(
	ctx context.Context, projectID, intentID string,
) ([]coordination.Handoff, error) {
	var response struct {
		Data struct {
			Handoffs []coordination.Handoff `json:"handoffs"`
		} `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/handoffs"
	if strings.TrimSpace(intentID) != "" {
		path += "?intent_id=" + url.QueryEscape(intentID)
	}
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.Handoffs == nil {
		response.Data.Handoffs = make([]coordination.Handoff, 0)
	}
	return response.Data.Handoffs, nil
}

func (c *Client) OfferHandoff(
	ctx context.Context, projectID, intentID, idempotencyKey string, input coordination.OfferHandoffInput,
) (coordination.HandoffResult, error) {
	var response struct {
		Data coordination.HandoffResult `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/intents/" +
		url.PathEscape(intentID) + "/handoffs"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return coordination.HandoffResult{}, err
	}
	return response.Data, nil
}

func (c *Client) UpdateHandoffStatus(
	ctx context.Context, projectID, intentID, handoffID, idempotencyKey string,
	input coordination.HandoffStatusInput,
) (coordination.HandoffResult, error) {
	var response struct {
		Data coordination.HandoffResult `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/intents/" +
		url.PathEscape(intentID) + "/handoffs/" + url.PathEscape(handoffID) + "/status"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return coordination.HandoffResult{}, err
	}
	return response.Data, nil
}

func (c *Client) CompileContextPack(
	ctx context.Context, projectID, intentID, idempotencyKey string, input contextpack.CompileInput,
) (contextpack.CompileResult, error) {
	var response struct {
		Data contextpack.CompileResult `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/intents/" +
		url.PathEscape(intentID) + "/context-packs"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return contextpack.CompileResult{}, err
	}
	return response.Data, nil
}

func (c *Client) GetContextPack(
	ctx context.Context, projectID, contextPackID string,
) (contextpack.ContextPack, error) {
	var response struct {
		Data contextpack.ContextPack `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/context-packs/" + url.PathEscape(contextPackID)
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return contextpack.ContextPack{}, err
	}
	return response.Data, nil
}

func (c *Client) AttachWorkspace(
	ctx context.Context,
	intentID string,
	idempotencyKey string,
	input coordination.WorkspaceInput,
) (coordination.WorkspaceResult, error) {
	var response struct {
		Data coordination.WorkspaceResult `json:"data"`
	}
	path := "/v1/intents/" + url.PathEscape(intentID) + "/workspace"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return coordination.WorkspaceResult{}, err
	}
	return response.Data, nil
}

func (c *Client) AttachWorktree(
	ctx context.Context,
	intentID string,
	idempotencyKey string,
	input coordination.WorktreeInput,
) (coordination.WorktreeResult, error) {
	var response struct {
		Data coordination.WorktreeResult `json:"data"`
	}
	path := "/v1/intents/" + url.PathEscape(intentID) + "/worktree"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return coordination.WorktreeResult{}, err
	}
	return response.Data, nil
}

func (c *Client) UpdateWorkStatus(
	ctx context.Context,
	intentID string,
	idempotencyKey string,
	input coordination.StatusInput,
) (coordination.StatusResult, error) {
	var response struct {
		Data coordination.StatusResult `json:"data"`
	}
	path := "/v1/intents/" + url.PathEscape(intentID) + "/status"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey}, Body: input,
	}, &response); err != nil {
		return coordination.StatusResult{}, err
	}
	return response.Data, nil
}

func (c *Client) ListWork(ctx context.Context, projectID string) ([]coordination.WorkItem, error) {
	var response struct {
		Data struct {
			WorkItems []coordination.WorkItem `json:"work_items"`
		} `json:"data"`
	}
	path := "/v1/projects/" + url.PathEscape(projectID) + "/work-items"
	if err := c.do(ctx, http.MethodGet, path, "", nil, &response); err != nil {
		return nil, err
	}
	if response.Data.WorkItems == nil {
		response.Data.WorkItems = make([]coordination.WorkItem, 0)
	}
	return response.Data.WorkItems, nil
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

func (c *Client) ObserveRepository(
	ctx context.Context,
	sessionID string,
	idempotencyKey string,
	input agentsession.ObservationInput,
) (agentsession.ObservationResult, error) {
	var response struct {
		Data agentsession.ObservationResult `json:"data"`
	}
	path := "/v1/agent-sessions/" + url.PathEscape(sessionID) + "/repository-observations"
	if err := c.do(ctx, http.MethodPost, path, "application/json", request{
		Headers: map[string]string{"Idempotency-Key": idempotencyKey},
		Body:    input,
	}, &response); err != nil {
		return agentsession.ObservationResult{}, err
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
	pathOnly, rawQuery, _ := strings.Cut(path, "?")
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + pathOnly
	endpoint.RawQuery = rawQuery
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
