package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/contextpack"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

const testToken = "this-is-a-long-local-test-token"

type fakeAccessService struct {
	require      func(context.Context, access.Principal, string, string) error
	visible      func(context.Context, access.Principal) (map[string]struct{}, error)
	accept       func(context.Context, access.AcceptInvitationInput) (access.AcceptedInvitation, error)
	createInvite func(context.Context, access.Principal, string, access.CreateInvitationInput) (access.CreatedInvitation, error)
}

func (fakeAccessService) Authenticate(_ context.Context, token string) (access.Principal, error) {
	if token != testToken {
		return access.Principal{}, access.ErrUnauthorized
	}
	return access.Principal{
		ID: access.BootstrapPrincipalID, OrganizationID: "00000000-0000-4000-8000-000000000001",
		DisplayName: "Test administrator", PrincipalType: "human", OrganizationRole: "owner", Bootstrap: true,
	}, nil
}

func (f fakeAccessService) RequireProjectRole(ctx context.Context, principal access.Principal, projectID, role string) error {
	if f.require != nil {
		return f.require(ctx, principal, projectID, role)
	}
	return nil
}

func (f fakeAccessService) VisibleProjectIDs(ctx context.Context, principal access.Principal) (map[string]struct{}, error) {
	if f.visible != nil {
		return f.visible(ctx, principal)
	}
	return nil, nil
}

func (fakeAccessService) CanCreateProject(access.Principal) bool { return true }

func (f fakeAccessService) CreateInvitation(ctx context.Context, principal access.Principal, projectID string, input access.CreateInvitationInput) (access.CreatedInvitation, error) {
	if f.createInvite != nil {
		return f.createInvite(ctx, principal, projectID, input)
	}
	return access.CreatedInvitation{}, nil
}

func (f fakeAccessService) AcceptInvitation(ctx context.Context, input access.AcceptInvitationInput) (access.AcceptedInvitation, error) {
	if f.accept != nil {
		return f.accept(ctx, input)
	}
	return access.AcceptedInvitation{}, nil
}

func (fakeAccessService) RevokeInvitation(context.Context, access.Principal, string) error {
	return nil
}
func (fakeAccessService) RevokeCurrentToken(context.Context, access.Principal) error { return nil }
func (fakeAccessService) GrantProjectOwner(context.Context, access.Principal, string) error {
	return nil
}

type fakeProjectService struct {
	create func(context.Context, string, projects.CreateInput) (projects.CreateResult, error)
	get    func(context.Context, string) (projects.Project, error)
	list   func(context.Context) ([]projects.Project, error)
}

type fakeWorkspaceService struct {
	create func(context.Context, string, workspaces.CreateInput) (workspaces.CreateResult, error)
	get    func(context.Context, string) (workspaces.Workspace, error)
	list   func(context.Context) ([]workspaces.Workspace, error)
	attach func(context.Context, string, string) (workspaces.Workspace, error)
}

func (f fakeWorkspaceService) Create(ctx context.Context, key string, input workspaces.CreateInput) (workspaces.CreateResult, error) {
	return f.create(ctx, key, input)
}

func (f fakeWorkspaceService) Get(ctx context.Context, reference string) (workspaces.Workspace, error) {
	return f.get(ctx, reference)
}

func (f fakeWorkspaceService) List(ctx context.Context) ([]workspaces.Workspace, error) {
	return f.list(ctx)
}

func (f fakeWorkspaceService) AttachProject(ctx context.Context, workspaceID, projectID string) (workspaces.Workspace, error) {
	return f.attach(ctx, workspaceID, projectID)
}

func (f fakeProjectService) Create(ctx context.Context, key string, input projects.CreateInput) (projects.CreateResult, error) {
	return f.create(ctx, key, input)
}

func (f fakeProjectService) Get(ctx context.Context, projectID string) (projects.Project, error) {
	return f.get(ctx, projectID)
}

func (f fakeProjectService) List(ctx context.Context) ([]projects.Project, error) {
	return f.list(ctx)
}

type fakeBackofficeReader struct {
	get func(context.Context, string, string) (backoffice.Overview, error)
}

func (f fakeBackofficeReader) Get(
	ctx context.Context,
	organizationID string,
	projectID string,
) (backoffice.Overview, error) {
	return f.get(ctx, organizationID, projectID)
}

type fakeEventReader struct {
	list func(context.Context, string, string, int64, int) ([]eventlog.Event, error)
}

type fakeAgentSessionService struct {
	observe func(context.Context, string, string, string, agentsession.ObservationInput) (agentsession.ObservationResult, error)
}

type fakeCoordinationService struct {
	check     func(context.Context, string, []coordination.ScopeInput) (coordination.ScopeCheckResult, error)
	start     func(context.Context, string, bool, string, string, coordination.StartInput) (coordination.StartResult, error)
	workspace func(context.Context, string, bool, string, string, coordination.WorkspaceInput) (coordination.WorkspaceResult, error)
	status    func(context.Context, string, bool, string, string, coordination.StatusInput) (coordination.StatusResult, error)
	list      func(context.Context, string) ([]coordination.WorkItem, error)
}

type fakeHandoffService struct {
	offer  func(context.Context, string, bool, string, string, string, coordination.OfferHandoffInput) (coordination.HandoffResult, error)
	list   func(context.Context, string, string) ([]coordination.Handoff, error)
	status func(context.Context, string, bool, string, string, string, string, coordination.HandoffStatusInput) (coordination.HandoffResult, error)
}

func (f fakeHandoffService) OfferHandoff(ctx context.Context, principalID string, allowAll bool, projectID, intentID, key string, input coordination.OfferHandoffInput) (coordination.HandoffResult, error) {
	return f.offer(ctx, principalID, allowAll, projectID, intentID, key, input)
}

func (f fakeHandoffService) ListHandoffs(ctx context.Context, projectID, intentID string) ([]coordination.Handoff, error) {
	return f.list(ctx, projectID, intentID)
}

func (f fakeHandoffService) UpdateHandoffStatus(ctx context.Context, principalID string, allowAll bool, projectID, intentID, handoffID, key string, input coordination.HandoffStatusInput) (coordination.HandoffResult, error) {
	return f.status(ctx, principalID, allowAll, projectID, intentID, handoffID, key, input)
}

type fakeContextPackService struct {
	compile func(context.Context, string, bool, string, string, string, contextpack.CompileInput) (contextpack.CompileResult, error)
	get     func(context.Context, string, string) (contextpack.ContextPack, error)
}

func (f fakeContextPackService) Compile(ctx context.Context, principalID string, allowAll bool, projectID, intentID, key string, input contextpack.CompileInput) (contextpack.CompileResult, error) {
	return f.compile(ctx, principalID, allowAll, projectID, intentID, key, input)
}

func (f fakeContextPackService) Get(ctx context.Context, projectID, packID string) (contextpack.ContextPack, error) {
	return f.get(ctx, projectID, packID)
}

func (f fakeCoordinationService) CheckScopes(ctx context.Context, projectID string, scopes []coordination.ScopeInput) (coordination.ScopeCheckResult, error) {
	return f.check(ctx, projectID, scopes)
}

func (f fakeCoordinationService) Start(ctx context.Context, principalID string, allowAll bool, projectID, key string, input coordination.StartInput) (coordination.StartResult, error) {
	return f.start(ctx, principalID, allowAll, projectID, key, input)
}

func (f fakeCoordinationService) AttachWorkspace(ctx context.Context, principalID string, allowAll bool, intentID, key string, input coordination.WorkspaceInput) (coordination.WorkspaceResult, error) {
	return f.workspace(ctx, principalID, allowAll, intentID, key, input)
}

func (f fakeCoordinationService) UpdateStatus(ctx context.Context, principalID string, allowAll bool, intentID, key string, input coordination.StatusInput) (coordination.StatusResult, error) {
	return f.status(ctx, principalID, allowAll, intentID, key, input)
}

func (f fakeCoordinationService) List(ctx context.Context, projectID string) ([]coordination.WorkItem, error) {
	return f.list(ctx, projectID)
}

func (fakeAgentSessionService) Start(context.Context, string, string, agentsession.StartInput) (agentsession.Session, error) {
	return agentsession.Session{}, nil
}

func (fakeAgentSessionService) Heartbeat(context.Context, string, bool, string) (agentsession.Session, error) {
	return agentsession.Session{}, nil
}

func (f fakeAgentSessionService) Observe(ctx context.Context, principalID, sessionID, key string, input agentsession.ObservationInput) (agentsession.ObservationResult, error) {
	return f.observe(ctx, principalID, sessionID, key, input)
}

func (fakeAgentSessionService) Close(context.Context, string, bool, string) error { return nil }

func (f fakeEventReader) List(ctx context.Context, organizationID, projectID string, after int64, limit int) ([]eventlog.Event, error) {
	return f.list(ctx, organizationID, projectID, after, limit)
}

func TestLiveDoesNotRequireAuthentication(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})
	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID is empty")
	}
}

func TestProtectedEndpointRequiresToken(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})
	request := httptest.NewRequest(http.MethodPost, "/v1/projects", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q", response.Header().Get("WWW-Authenticate"))
	}
}

func TestListWorkspacesFiltersProjectsByVisibility(t *testing.T) {
	const visibleProjectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{},
		WorkspaceService: fakeWorkspaceService{list: func(context.Context) ([]workspaces.Workspace, error) {
			return []workspaces.Workspace{
				{ID: "workspace-visible", Slug: "visible", Projects: []workspaces.Project{{ID: visibleProjectID}, {ID: "hidden"}}},
				{ID: "workspace-hidden", Slug: "hidden", Projects: []workspaces.Project{{ID: "hidden"}}},
			}, nil
		}},
		AccessService: fakeAccessService{visible: func(context.Context, access.Principal) (map[string]struct{}, error) {
			return map[string]struct{}{visibleProjectID: {}}, nil
		}},
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }},
		EventReader:      fakeEventReader{},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/workspaces", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Workspaces []workspaces.Workspace `json:"workspaces"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Workspaces) != 1 || len(body.Data.Workspaces[0].Projects) != 1 || body.Data.Workspaces[0].Projects[0].ID != visibleProjectID {
		t.Fatalf("workspaces = %#v", body.Data.Workspaces)
	}
}

func TestCreateWorkspaceUsesIdempotencyAndReturnsLocation(t *testing.T) {
	const workspaceID = "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{},
		WorkspaceService: fakeWorkspaceService{create: func(_ context.Context, key string, input workspaces.CreateInput) (workspaces.CreateResult, error) {
			if key != "workspace-1" || input.Name != "Footfall" || input.Slug != "footfall" {
				t.Fatalf("key=%q input=%#v", key, input)
			}
			return workspaces.CreateResult{Workspace: workspaces.Workspace{ID: workspaceID, Name: input.Name, Slug: input.Slug}, Replayed: true}, nil
		}},
		AccessService:    fakeAccessService{},
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }},
		EventReader:      fakeEventReader{},
	})
	request := authenticatedRequest(http.MethodPost, "/v1/workspaces", `{"name":"Footfall","slug":"footfall"}`)
	request.Header.Set("Idempotency-Key", "workspace-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get("Location") != "/v1/workspaces/"+workspaceID || response.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("status=%d location=%q replayed=%q body=%s", response.Code, response.Header().Get("Location"), response.Header().Get("Idempotency-Replayed"), response.Body.String())
	}
}

func TestAttachWorktreeUsesNewVocabulary(t *testing.T) {
	const (
		intentID   = "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
		worktreeID = "018f784a-68c1-7b0f-8f2a-cfc255f99e4a"
	)
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{}, AccessService: fakeAccessService{},
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }},
		EventReader:      fakeEventReader{},
		CoordinationService: fakeCoordinationService{workspace: func(_ context.Context, principalID string, allowAll bool, receivedIntentID, key string, input coordination.WorkspaceInput) (coordination.WorkspaceResult, error) {
			if principalID != access.BootstrapPrincipalID || !allowAll || receivedIntentID != intentID || key != "worktree-1" {
				t.Fatalf("principal=%q allowAll=%v intent=%q key=%q", principalID, allowAll, receivedIntentID, key)
			}
			return coordination.WorkspaceResult{
				Workspace: coordination.Worktree{ID: worktreeID, SessionID: input.SessionID, PathRef: input.PathRef},
				EventID:   "018f784a-68c1-7b0f-8f2a-cfc255f99e5b",
				Replayed:  true,
			}, nil
		}},
	})
	request := authenticatedRequest(http.MethodPost, "/v1/intents/"+intentID+"/worktree", `{"session_id":"018f784a-68c1-7b0f-8f2a-cfc255f99e3f","base_revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","path_ref":".pact/worktrees/test","git_branch":"pact/test"}`)
	request.Header.Set("Idempotency-Key", "worktree-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get("Location") != "/v1/intents/"+intentID+"/worktree" {
		t.Fatalf("status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"worktree":{"id":"`+worktreeID+`"`) || strings.Contains(response.Body.String(), `"workspace":`) {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestRepositoryObservationUsesAuthenticatedSessionAndIdempotency(t *testing.T) {
	const sessionID = "018f784a-68c1-7b0f-8f2a-cfc255f99e3f"
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"},
		Readiness:      func(context.Context) error { return nil },
		ProjectService: fakeProjectService{},
		AgentSessionService: fakeAgentSessionService{observe: func(_ context.Context, principalID, receivedSessionID, key string, input agentsession.ObservationInput) (agentsession.ObservationResult, error) {
			if principalID != access.BootstrapPrincipalID || receivedSessionID != sessionID || key != "observation-1" {
				t.Fatalf("principal=%q session=%q key=%q", principalID, receivedSessionID, key)
			}
			if !input.Dirty || input.ChangedPaths != 1 || len(input.DiffFingerprint) != 64 {
				t.Fatalf("input = %#v", input)
			}
			return agentsession.ObservationResult{Replayed: true}, nil
		}},
		AccessService:    fakeAccessService{},
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }},
		EventReader:      fakeEventReader{},
	})
	request := authenticatedRequest(
		http.MethodPost,
		"/v1/agent-sessions/"+sessionID+"/repository-observations",
		`{"dirty":true,"diff_fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","changed_paths":1}`,
	)
	request.Header.Set("Idempotency-Key", "observation-1")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("status = %d, replayed = %q, body = %s", response.Code, response.Header().Get("Idempotency-Replayed"), response.Body.String())
	}
}

func TestStartWorkUsesAuthenticatedPrincipalAndReturnsLocation(t *testing.T) {
	const (
		projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
		intentID  = "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
		sessionID = "018f784a-68c1-7b0f-8f2a-cfc255f99e3f"
	)
	handler := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build: buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{}, AccessService: fakeAccessService{},
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }},
		EventReader:      fakeEventReader{},
		CoordinationService: fakeCoordinationService{start: func(_ context.Context, principalID string, allowAll bool, receivedProjectID, key string, input coordination.StartInput) (coordination.StartResult, error) {
			if principalID != access.BootstrapPrincipalID || !allowAll || receivedProjectID != projectID || key != "work-1" {
				t.Fatalf("principal=%q allowAll=%v project=%q key=%q", principalID, allowAll, receivedProjectID, key)
			}
			if input.SessionID != sessionID || input.Title != "Change API" || len(input.Scopes) != 1 {
				t.Fatalf("input = %#v", input)
			}
			return coordination.StartResult{Intent: coordination.Intent{ID: intentID, ProjectID: projectID, Status: "active", Version: 1}, Replayed: true}, nil
		}},
	})
	request := authenticatedRequest(http.MethodPost, "/v1/projects/"+projectID+"/work-items", `{"session_id":"`+sessionID+`","title":"Change API","goal":"Safer API","base_revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","scopes":[{"kind":"path","locator":"internal/api"}]}`)
	request.Header.Set("Idempotency-Key", "work-1")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || response.Header().Get("Location") != "/v1/intents/"+intentID || response.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("status=%d location=%q replayed=%q body=%s", response.Code, response.Header().Get("Location"), response.Header().Get("Idempotency-Replayed"), response.Body.String())
	}
}

func TestStartWorkReturnsStructuredScopeConflict(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	overlap := coordination.ScopeOverlap{
		Requested:        coordination.ScopeInput{Kind: "file", Locator: "internal/api.go", Mode: "exclusive"},
		ExistingIntentID: "018f784a-68c1-7b0f-8f2a-cfc255f99e2e", ExistingActor: "Kimi",
		ExistingScope: coordination.ScopeInput{Kind: "path", Locator: "internal", Mode: "exclusive"}, Blocking: true,
	}
	handler := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build: buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{}, AccessService: fakeAccessService{},
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }},
		EventReader:      fakeEventReader{},
		CoordinationService: fakeCoordinationService{start: func(context.Context, string, bool, string, string, coordination.StartInput) (coordination.StartResult, error) {
			return coordination.StartResult{}, &coordination.ScopeConflictError{Overlaps: []coordination.ScopeOverlap{overlap}}
		}},
	})
	request := authenticatedRequest(http.MethodPost, "/v1/projects/"+projectID+"/work-items", `{}`)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"scope_conflict"`) || !strings.Contains(response.Body.String(), `"existing_actor":"Kimi"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestOfferHandoffUsesAuthenticatedPrincipalAndIdempotency(t *testing.T) {
	const (
		projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
		intentID  = "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
		sessionID = "018f784a-68c1-7b0f-8f2a-cfc255f99e3f"
		handoffID = "018f784a-68c1-7b0f-8f2a-cfc255f99e4a"
	)
	handler := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build: buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{}, AccessService: fakeAccessService{},
		HandoffService: fakeHandoffService{offer: func(_ context.Context, principalID string, allowAll bool, receivedProjectID, receivedIntentID, key string, input coordination.OfferHandoffInput) (coordination.HandoffResult, error) {
			if principalID != access.BootstrapPrincipalID || !allowAll || receivedProjectID != projectID || receivedIntentID != intentID || key != "handoff-1" {
				t.Fatalf("principal=%q allowAll=%v project=%q intent=%q key=%q", principalID, allowAll, receivedProjectID, receivedIntentID, key)
			}
			if input.SessionID != sessionID || input.Summary != "Ready for review" || len(input.NextSteps) != 1 {
				t.Fatalf("input = %#v", input)
			}
			return coordination.HandoffResult{Handoff: coordination.Handoff{ID: handoffID, ProjectID: projectID, IntentID: intentID, Status: "offered", Version: 1}, Replayed: true}, nil
		}},
	})
	request := authenticatedRequest(http.MethodPost, "/v1/projects/"+projectID+"/intents/"+intentID+"/handoffs", `{"session_id":"`+sessionID+`","summary":"Ready for review","next_steps":["Run tests"]}`)
	request.Header.Set("Idempotency-Key", "handoff-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get("Idempotency-Replayed") != "true" || !strings.Contains(response.Body.String(), handoffID) {
		t.Fatalf("status=%d replayed=%q body=%s", response.Code, response.Header().Get("Idempotency-Replayed"), response.Body.String())
	}
}

func TestCompileContextPackUsesAgentSessionAndReturnsLocation(t *testing.T) {
	const (
		projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
		intentID  = "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
		sessionID = "018f784a-68c1-7b0f-8f2a-cfc255f99e3f"
		packID    = "018f784a-68c1-7b0f-8f2a-cfc255f99e5b"
	)
	handler := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build: buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{}, AccessService: fakeAccessService{},
		ContextPackService: fakeContextPackService{compile: func(_ context.Context, principalID string, allowAll bool, receivedProjectID, receivedIntentID, key string, input contextpack.CompileInput) (contextpack.CompileResult, error) {
			if principalID != access.BootstrapPrincipalID || !allowAll || receivedProjectID != projectID || receivedIntentID != intentID || key != "context-1" {
				t.Fatalf("principal=%q allowAll=%v project=%q intent=%q key=%q", principalID, allowAll, receivedProjectID, receivedIntentID, key)
			}
			if input.SessionID != sessionID || input.Type != "review" || input.TTLMinutes != 10 {
				t.Fatalf("input = %#v", input)
			}
			return contextpack.CompileResult{Pack: contextpack.ContextPack{ID: packID, ProjectID: projectID, IntentID: intentID, Type: "review"}}, nil
		}},
	})
	request := authenticatedRequest(http.MethodPost, "/v1/projects/"+projectID+"/intents/"+intentID+"/context-packs", `{"session_id":"`+sessionID+`","type":"review","ttl_minutes":10}`)
	request.Header.Set("Idempotency-Key", "context-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get("Location") != "/v1/projects/"+projectID+"/context-packs/"+packID {
		t.Fatalf("status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
}

func TestAdminShellIsPublicButContainsNoCredentials(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})
	request := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if policy := response.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "form-action 'none'") {
		t.Fatalf("Content-Security-Policy does not disable form submissions: %q", policy)
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q", cacheControl)
	}
	if strings.Contains(response.Body.String(), testToken) {
		t.Fatal("admin shell contains the API token")
	}
	if strings.Contains(response.Body.String(), `name="token"`) {
		t.Fatal("admin shell allows the browser to serialize the token field")
	}
}

func TestAdminUsesCanonicalRouteAndNoSPAFallback(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})

	redirectRequest := httptest.NewRequest(http.MethodGet, "/admin", nil)
	redirectResponse := httptest.NewRecorder()
	handler.ServeHTTP(redirectResponse, redirectRequest)
	if redirectResponse.Code != http.StatusPermanentRedirect ||
		redirectResponse.Header().Get("Location") != "/admin/" {
		t.Fatalf(
			"redirect status=%d location=%q",
			redirectResponse.Code,
			redirectResponse.Header().Get("Location"),
		)
	}

	missingRequest := httptest.NewRequest(http.MethodGet, "/admin/not-an-asset", nil)
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missingRequest)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, body = %s", missingResponse.Code, missingResponse.Body.String())
	}
	if strings.Contains(strings.ToLower(missingResponse.Body.String()), "<!doctype html>") {
		t.Fatal("missing asset fell back to the application shell")
	}
}

func TestListProjectsReturnsAnEmptyArray(t *testing.T) {
	service := fakeProjectService{
		list: func(context.Context) ([]projects.Project, error) {
			return []projects.Project{}, nil
		},
	}
	handler := testHandler(t, service, fakeEventReader{})
	request := authenticatedRequest(http.MethodGet, "/v1/projects", "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Projects []projects.Project `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Projects == nil || len(body.Data.Projects) != 0 {
		t.Fatalf("projects = %#v", body.Data.Projects)
	}
}

func TestListProjectsFiltersInaccessibleProjects(t *testing.T) {
	service := fakeProjectService{
		list: func(context.Context) ([]projects.Project, error) {
			return []projects.Project{{ID: "visible"}, {ID: "hidden"}}, nil
		},
	}
	handler := testHandlerWithAccess(t, service, fakeAccessService{
		visible: func(context.Context, access.Principal) (map[string]struct{}, error) {
			return map[string]struct{}{"visible": {}}, nil
		},
	})
	request := authenticatedRequest(http.MethodGet, "/v1/projects", "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "hidden") || !strings.Contains(response.Body.String(), "visible") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAcceptInvitationDoesNotRequireAuthenticationAndDisablesCaching(t *testing.T) {
	handler := testHandlerWithAccess(t, fakeProjectService{}, fakeAccessService{
		accept: func(_ context.Context, input access.AcceptInvitationInput) (access.AcceptedInvitation, error) {
			if input.Secret != "pact_inv_test" || input.DisplayName != "Ada" {
				t.Fatalf("input = %#v", input)
			}
			return access.AcceptedInvitation{AccessToken: "pact_pat_secret"}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/invitation-acceptances", bytes.NewBufferString(`{
		"secret":"pact_inv_test","display_name":"Ada","token_name":"Laptop"
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d, Cache-Control = %q, body = %s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "pact_pat_secret") {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestProjectOverviewKeepsCodeActivityUnobserved(t *testing.T) {
	const (
		organizationID = "00000000-0000-4000-8000-000000000001"
		projectID      = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	)
	service := fakeProjectService{
		get: func(_ context.Context, id string) (projects.Project, error) {
			if id != projectID {
				t.Fatalf("project ID = %q", id)
			}
			return projects.Project{ID: projectID, Name: "Pact", Slug: "pact"}, nil
		},
	}
	reader := fakeBackofficeReader{
		get: func(_ context.Context, receivedOrganizationID, receivedProjectID string) (backoffice.Overview, error) {
			if receivedOrganizationID != organizationID || receivedProjectID != projectID {
				t.Fatalf("Get(org=%q, project=%q)", receivedOrganizationID, receivedProjectID)
			}
			return backoffice.Overview{
				CodeActivity: backoffice.CodeActivity{
					State:  backoffice.CodeActivityUnobserved,
					Reason: backoffice.ReasonNoConnectedObserver,
				},
				ActiveWork:   []backoffice.ActiveWork{},
				RecentEvents: []backoffice.RecentEvent{},
				GeneratedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	handler := testHandlerWithBackoffice(t, service, reader, fakeEventReader{})
	request := authenticatedRequest(http.MethodGet, "/v1/projects/"+projectID+"/overview", "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			CodeActivity backoffice.CodeActivity  `json:"code_activity"`
			ActiveWork   []backoffice.ActiveWork  `json:"active_work"`
			RecentEvents []backoffice.RecentEvent `json:"recent_events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.CodeActivity.State != backoffice.CodeActivityUnobserved ||
		body.Data.CodeActivity.Reason != backoffice.ReasonNoConnectedObserver {
		t.Fatalf("code activity = %#v", body.Data.CodeActivity)
	}
	if body.Data.ActiveWork == nil || body.Data.RecentEvents == nil {
		t.Fatalf("overview arrays must not be null: %#v", body.Data)
	}
}

func TestProjectOverviewDoesNotReadAcrossProjectBoundary(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	service := fakeProjectService{
		get: func(context.Context, string) (projects.Project, error) {
			return projects.Project{}, projects.ErrNotFound
		},
	}
	reader := fakeBackofficeReader{
		get: func(context.Context, string, string) (backoffice.Overview, error) {
			t.Fatal("backoffice reader should not be called")
			return backoffice.Overview{}, nil
		},
	}
	handler := testHandlerWithBackoffice(t, service, reader, fakeEventReader{})
	request := authenticatedRequest(http.MethodGet, "/v1/projects/"+projectID+"/overview", "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUnknownRoutesAndMethodsUseProblemDetails(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
		wantAllow  string
	}{
		{
			name:       "unknown route",
			method:     http.MethodGet,
			target:     "/unknown",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "known route with unsupported method",
			method:     http.MethodPost,
			target:     "/livez",
			wantStatus: http.StatusMethodNotAllowed,
			wantAllow:  http.MethodGet,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
				t.Fatalf("Content-Type = %q", contentType)
			}
			if allow := response.Header().Get("Allow"); allow != test.wantAllow {
				t.Fatalf("Allow = %q", allow)
			}
		})
	}
}

func TestCreateProjectRejectsUnknownJSONField(t *testing.T) {
	service := fakeProjectService{
		create: func(context.Context, string, projects.CreateInput) (projects.CreateResult, error) {
			t.Fatal("service should not be called")
			return projects.CreateResult{}, nil
		},
	}
	handler := testHandler(t, service, fakeEventReader{})
	request := authenticatedRequest(http.MethodPost, "/v1/projects", `{"name":"Pact","slug":"pact","surprise":true}`)
	request.Header.Set("Idempotency-Key", "create-pact")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestCreateProjectReturnsStoredResponseOnReplay(t *testing.T) {
	project := projects.Project{
		ID:             "018f784a-68c1-7b0f-8f2a-cfc255f99e1d",
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Name:           "Pact",
		Slug:           "pact",
		Status:         "initializing",
		Version:        1,
		CreatedAt:      time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	service := fakeProjectService{
		create: func(_ context.Context, key string, input projects.CreateInput) (projects.CreateResult, error) {
			if key != "create-pact" || input.Slug != "pact" {
				t.Fatalf("unexpected command: key=%q input=%#v", key, input)
			}
			return projects.CreateResult{Project: project, Replayed: true}, nil
		},
	}
	handler := testHandler(t, service, fakeEventReader{})
	request := authenticatedRequest(http.MethodPost, "/v1/projects", `{"name":"Pact","slug":"pact"}`)
	request.Header.Set("Idempotency-Key", "create-pact")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("Idempotency-Replayed = %q", response.Header().Get("Idempotency-Replayed"))
	}
	if response.Header().Get("Location") != "/v1/projects/"+project.ID {
		t.Fatalf("Location = %q", response.Header().Get("Location"))
	}
}

func TestListEventsPassesCursorAndLimit(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	service := fakeProjectService{
		get: func(_ context.Context, id string) (projects.Project, error) {
			if id != projectID {
				t.Fatalf("project ID = %q", id)
			}
			return projects.Project{ID: id}, nil
		},
	}
	reader := fakeEventReader{
		list: func(_ context.Context, organizationID, id string, after int64, limit int) ([]eventlog.Event, error) {
			if organizationID != "00000000-0000-4000-8000-000000000001" || id != projectID || after != 41 || limit != 21 {
				t.Fatalf("List(org=%q, project=%q, after=%d, limit=%d)", organizationID, id, after, limit)
			}
			return []eventlog.Event{{ProjectSequence: 42, Type: "project.created"}}, nil
		},
	}
	handler := testHandler(t, service, reader)
	request := authenticatedRequest(http.MethodGet, "/v1/projects/"+projectID+"/events?after=41&limit=20", "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			NextCursor *string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.NextCursor == nil || *body.Data.NextCursor != "42" {
		t.Fatalf("next_cursor = %v", body.Data.NextCursor)
	}
}

func TestStreamReplaysFromLastEventIDAndStopsOnCancellation(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	ctx, cancel := context.WithCancel(context.Background())
	service := fakeProjectService{
		get: func(_ context.Context, id string) (projects.Project, error) {
			return projects.Project{ID: id}, nil
		},
	}
	call := 0
	reader := fakeEventReader{
		list: func(_ context.Context, _, _ string, after int64, _ int) ([]eventlog.Event, error) {
			call++
			switch call {
			case 1:
				if after != 41 {
					t.Fatalf("first cursor = %d", after)
				}
				return []eventlog.Event{{
					ID:               "018f784a-68c1-7b0f-8f2a-cfc255f99e1e",
					ProjectID:        projectID,
					ProjectSequence:  42,
					Type:             "pact.project.created.v1",
					Version:          1,
					AggregateType:    "project",
					AggregateID:      projectID,
					AggregateVersion: 1,
					CommandID:        "018f784a-68c1-7b0f-8f2a-cfc255f99e1f",
					CorrelationID:    "018f784a-68c1-7b0f-8f2a-cfc255f99e1f",
					OccurredAt:       time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
					RecordedAt:       time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
					Payload:          json.RawMessage(`{"name":"Pact"}`),
				}}, nil
			default:
				if after != 42 {
					t.Fatalf("resumed cursor = %d", after)
				}
				cancel()
				return nil, nil
			}
		},
	}
	handler := testHandler(t, service, reader)
	request := authenticatedRequest(
		http.MethodGet,
		"/v1/projects/"+projectID+"/events/stream",
		"",
	).WithContext(ctx)
	request.Header.Set("Last-Event-ID", "41")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{
		"id: 42\n",
		"event: pact.project.created.v1\n",
		`"sequence":"42"`,
		`"data":{"name":"Pact"}`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("stream body does not contain %q: %s", expected, body)
		}
	}
}

func TestPanicIsRecoveredWithRequestID(t *testing.T) {
	service := fakeProjectService{
		get: func(context.Context, string) (projects.Project, error) {
			panic("unexpected failure")
		},
	}
	handler := testHandler(t, service, fakeEventReader{})
	request := authenticatedRequest(
		http.MethodGet,
		"/v1/projects/018f784a-68c1-7b0f-8f2a-cfc255f99e1d",
		"",
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RequestID == "" || body.RequestID != response.Header().Get("X-Request-ID") {
		t.Fatalf("request_id = %q, header = %q", body.RequestID, response.Header().Get("X-Request-ID"))
	}
}

func TestSSEEventNameRejectsProtocolInjection(t *testing.T) {
	if got := sseEventName("project.created\nid: forged"); got != "pact.event" {
		t.Fatalf("sseEventName() = %q", got)
	}
	if got := sseEventName("pact.project.created.v1"); got != "pact.project.created.v1" {
		t.Fatalf("sseEventName() = %q", got)
	}
}

func testHandler(t *testing.T, projectService ProjectService, eventReader eventlog.Reader) http.Handler {
	t.Helper()
	return testHandlerWithBackoffice(
		t,
		projectService,
		fakeBackofficeReader{
			get: func(context.Context, string, string) (backoffice.Overview, error) {
				return backoffice.Overview{}, nil
			},
		},
		eventReader,
	)
}

func testHandlerWithBackoffice(
	t *testing.T,
	projectService ProjectService,
	backofficeReader backoffice.Reader,
	eventReader eventlog.Reader,
) http.Handler {
	t.Helper()
	return New(Config{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID:   "00000000-0000-4000-8000-000000000001",
		Build:            buildinfo.Info{Version: "test"},
		Readiness:        func(context.Context) error { return nil },
		ProjectService:   projectService,
		AccessService:    fakeAccessService{},
		BackofficeReader: backofficeReader,
		EventReader:      eventReader,
	})
}

func testHandlerWithAccess(t *testing.T, projectService ProjectService, accessService AccessService) http.Handler {
	t.Helper()
	return New(Config{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID:   "00000000-0000-4000-8000-000000000001",
		Build:            buildinfo.Info{Version: "test"},
		Readiness:        func(context.Context) error { return nil },
		ProjectService:   projectService,
		AccessService:    accessService,
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }},
		EventReader:      fakeEventReader{},
	})
}

func authenticatedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}
