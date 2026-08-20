package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/authn"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/contextpack"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/githubapp"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/repositorysync"
	"github.com/jorgenuanzs/the-pact/internal/rooms"
	"github.com/jorgenuanzs/the-pact/internal/useradmin"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

const testToken = "this-is-a-long-local-test-token"

type fakeAccessService struct {
	require            func(context.Context, access.Principal, string, string) error
	visible            func(context.Context, access.Principal) (map[string]struct{}, error)
	projectAccess      func(context.Context, access.Principal, string) (access.ProjectAccess, error)
	workspaceAccess    func(context.Context, access.Principal, string) (access.WorkspaceAccess, error)
	register           func(context.Context, authn.InvitationRegistrationInput, authn.SessionMetadata) (authn.CreatedInvitationSession, error)
	createInvite       func(context.Context, access.Principal, string, access.CreateInvitationInput) (access.CreatedInvitation, error)
	authenticateWeb    func(context.Context, string) (authn.WebSession, error)
	authenticateDevice func(context.Context, string) (authn.DevicePrincipal, error)
	principal          *access.Principal
	webSession         *authn.WebSession
	csrfSecret         string
}

func (f fakeAccessService) Authenticate(_ context.Context, token string) (access.Principal, error) {
	if token != testToken {
		return access.Principal{}, access.ErrUnauthorized
	}
	if f.principal != nil {
		return *f.principal, nil
	}
	return access.Principal{
		ID: access.BootstrapPrincipalID, OrganizationID: "00000000-0000-4000-8000-000000000001",
		DisplayName: "Test administrator", PrincipalType: "human", OrganizationRole: "owner", Bootstrap: true,
	}, nil
}

func (fakeAccessService) SetupStatus(context.Context) (authn.SetupStatus, error) {
	return authn.SetupStatus{}, nil
}
func (fakeAccessService) Setup(context.Context, authn.SetupInput, authn.SessionMetadata) (authn.CreatedWebSession, error) {
	return authn.CreatedWebSession{}, nil
}
func (fakeAccessService) Login(context.Context, authn.LoginInput, authn.SessionMetadata) (authn.CreatedWebSession, error) {
	return authn.CreatedWebSession{}, nil
}
func (f fakeAccessService) AuthenticateWeb(ctx context.Context, secret string) (authn.WebSession, error) {
	if f.authenticateWeb != nil {
		return f.authenticateWeb(ctx, secret)
	}
	if f.webSession != nil && secret == "pact_web_test_secret" {
		return *f.webSession, nil
	}
	return authn.WebSession{}, authn.ErrUnauthorized
}
func (f fakeAccessService) AuthenticateDevice(ctx context.Context, credential string) (authn.DevicePrincipal, error) {
	if f.authenticateDevice != nil {
		return f.authenticateDevice(ctx, credential)
	}
	principal, err := f.Authenticate(ctx, credential)
	return authn.DevicePrincipal{CredentialID: "test-device", Principal: principal}, err
}
func (f fakeAccessService) ValidateCSRF(_ authn.WebSession, secret string) bool {
	return f.csrfSecret == "" || secret == f.csrfSecret
}
func (fakeAccessService) LogoutWeb(context.Context, authn.WebSession) error { return nil }
func (fakeAccessService) ChangePassword(context.Context, authn.WebSession, authn.ChangePasswordInput) error {
	return nil
}
func (fakeAccessService) PreviewInvitation(context.Context, string) (authn.InvitationPreview, error) {
	return authn.InvitationPreview{}, nil
}

func (f fakeAccessService) RegisterInvitation(ctx context.Context, input authn.InvitationRegistrationInput, metadata authn.SessionMetadata) (authn.CreatedInvitationSession, error) {
	if f.register != nil {
		return f.register(ctx, input, metadata)
	}
	return authn.CreatedInvitationSession{}, nil
}
func (fakeAccessService) AcceptInvitation(context.Context, access.Principal, string) (authn.InvitationAcceptance, error) {
	return authn.InvitationAcceptance{}, nil
}
func (fakeAccessService) BeginDevice(context.Context, authn.BeginDeviceInput) (authn.DeviceAuthorization, error) {
	return authn.DeviceAuthorization{}, nil
}
func (fakeAccessService) ApproveDevice(context.Context, access.Principal, string) error { return nil }
func (fakeAccessService) ExchangeDevice(context.Context, string) (authn.DeviceExchange, error) {
	return authn.DeviceExchange{}, nil
}
func (fakeAccessService) RevokeCurrentDevice(context.Context, authn.DevicePrincipal) error {
	return nil
}
func (fakeAccessService) ListDevices(context.Context, access.Principal) ([]authn.Device, error) {
	return []authn.Device{}, nil
}
func (fakeAccessService) RevokeDevice(context.Context, access.Principal, string) error { return nil }

func (f fakeAccessService) RequireProjectRole(ctx context.Context, principal access.Principal, projectID, role string) error {
	if f.require != nil {
		return f.require(ctx, principal, projectID, role)
	}
	return nil
}

func (f fakeAccessService) RequireWorkspaceRole(ctx context.Context, principal access.Principal, workspaceID, role string) error {
	if f.require != nil {
		return f.require(ctx, principal, workspaceID, role)
	}
	return nil
}

func (f fakeAccessService) VisibleProjectIDs(ctx context.Context, principal access.Principal) (map[string]struct{}, error) {
	if f.visible != nil {
		return f.visible(ctx, principal)
	}
	return nil, nil
}

func (f fakeAccessService) GetProjectAccess(ctx context.Context, principal access.Principal, projectID string) (access.ProjectAccess, error) {
	if f.projectAccess != nil {
		return f.projectAccess(ctx, principal, projectID)
	}
	return access.ProjectAccess{ProjectID: projectID, Members: []access.ProjectMember{}, Agents: []access.ProjectAgent{}}, nil
}

func (f fakeAccessService) GetWorkspaceAccess(ctx context.Context, principal access.Principal, workspaceID string) (access.WorkspaceAccess, error) {
	if f.workspaceAccess != nil {
		return f.workspaceAccess(ctx, principal, workspaceID)
	}
	return access.WorkspaceAccess{WorkspaceID: workspaceID, Members: []access.WorkspaceMember{}, Agents: []access.ProjectAgent{}}, nil
}

func (fakeAccessService) CanCreateProject(access.Principal) bool { return true }

func (f fakeAccessService) CreateInvitation(ctx context.Context, principal access.Principal, projectID string, input access.CreateInvitationInput) (access.CreatedInvitation, error) {
	if f.createInvite != nil {
		return f.createInvite(ctx, principal, projectID, input)
	}
	return access.CreatedInvitation{}, nil
}

func (fakeAccessService) RevokeInvitation(context.Context, access.Principal, string) error {
	return nil
}
func (fakeAccessService) GrantProjectOwner(context.Context, access.Principal, string) error {
	return nil
}

type fakeProjectService struct {
	create func(context.Context, string, projects.CreateInput) (projects.CreateResult, error)
	get    func(context.Context, string) (projects.Project, error)
	list   func(context.Context) ([]projects.Project, error)
}

type fakeUserAdminService struct {
	directory func(context.Context, access.Principal) (useradmin.Directory, error)
	update    func(context.Context, access.Principal, string, useradmin.UpdateUserInput) (useradmin.User, error)
}

func (f fakeUserAdminService) Directory(ctx context.Context, principal access.Principal) (useradmin.Directory, error) {
	return f.directory(ctx, principal)
}

func (fakeUserAdminService) GetUser(context.Context, access.Principal, string) (useradmin.User, error) {
	return useradmin.User{}, nil
}

func (f fakeUserAdminService) UpdateUser(ctx context.Context, principal access.Principal, principalID string, input useradmin.UpdateUserInput) (useradmin.User, error) {
	return f.update(ctx, principal, principalID, input)
}

func (fakeUserAdminService) SetProjectPermission(context.Context, access.Principal, string, string, string) (useradmin.User, error) {
	return useradmin.User{}, nil
}

func (fakeUserAdminService) RemoveProjectPermission(context.Context, access.Principal, string, string) (useradmin.User, error) {
	return useradmin.User{}, nil
}

func (fakeUserAdminService) RevokeUserSessions(context.Context, access.Principal, string) (useradmin.User, error) {
	return useradmin.User{}, nil
}

func (fakeUserAdminService) CreateInvitation(context.Context, access.Principal, useradmin.CreateInvitationInput) (useradmin.CreatedInvitation, error) {
	return useradmin.CreatedInvitation{}, nil
}

func (fakeUserAdminService) RevokeInvitation(context.Context, access.Principal, string) error {
	return nil
}

type fakeRepositorySyncService struct {
	get  func(context.Context, string) (repositorysync.State, error)
	sync func(context.Context, string, string, string) (repositorysync.Result, error)
}

func (f fakeRepositorySyncService) Get(ctx context.Context, projectID string) (repositorysync.State, error) {
	return f.get(ctx, projectID)
}

func (f fakeRepositorySyncService) Sync(ctx context.Context, principalID, projectID, key string) (repositorysync.Result, error) {
	return f.sync(ctx, principalID, projectID, key)
}

func (f fakeRepositorySyncService) GetRepository(ctx context.Context, projectID, _ string) (repositorysync.State, error) {
	return f.Get(ctx, projectID)
}

func (f fakeRepositorySyncService) List(ctx context.Context, projectID string) ([]repositorysync.State, error) {
	state, err := f.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return []repositorysync.State{state}, nil
}

func (f fakeRepositorySyncService) SyncRepository(ctx context.Context, principalID, projectID, _ string, key string) (repositorysync.Result, error) {
	return f.Sync(ctx, principalID, projectID, key)
}

type fakeGitHubAppService struct {
	status   func(context.Context) (githubapp.Status, error)
	connect  func(context.Context, string) (githubapp.Connection, error)
	begin    func(context.Context, string, int64) (string, error)
	complete func(context.Context, string, string) error
	webhook  func(context.Context, string, string, string, []byte) error
}

func (f fakeGitHubAppService) Status(ctx context.Context) (githubapp.Status, error) {
	return f.status(ctx)
}

func (f fakeGitHubAppService) Connect(ctx context.Context, principalID string) (githubapp.Connection, error) {
	return f.connect(ctx, principalID)
}

func (f fakeGitHubAppService) BeginUserAuthorization(ctx context.Context, state string, installationID int64) (string, error) {
	return f.begin(ctx, state, installationID)
}

func (f fakeGitHubAppService) CompleteConnection(ctx context.Context, state, code string) error {
	return f.complete(ctx, state, code)
}

func (f fakeGitHubAppService) HandleWebhook(ctx context.Context, deliveryID, eventType, signature string, body []byte) error {
	return f.webhook(ctx, deliveryID, eventType, signature, body)
}

type fakeWorkspaceService struct {
	create func(context.Context, string, workspaces.CreateInput) (workspaces.CreateResult, error)
	get    func(context.Context, string) (workspaces.Workspace, error)
	list   func(context.Context) ([]workspaces.Workspace, error)
	update func(context.Context, string, workspaces.UpdateInput) (workspaces.Workspace, error)
	attach func(context.Context, string, string) (workspaces.Workspace, error)
}

type fakeRoomService struct {
	createMessage func(context.Context, string, bool, string, string, string, rooms.CreateMessageInput) (rooms.CreateMessageResult, error)
}

func (fakeRoomService) CreateRoom(context.Context, string, string, string, rooms.CreateRoomInput) (rooms.CreateRoomResult, error) {
	return rooms.CreateRoomResult{}, nil
}

func (fakeRoomService) ListRooms(context.Context, string) ([]rooms.Room, error) {
	return nil, nil
}

func (fakeRoomService) ListParticipants(context.Context, string) ([]rooms.Participant, error) {
	return nil, nil
}

func (f fakeRoomService) CreateMessage(ctx context.Context, principalID string, allowAll bool, workspaceID, roomID, key string, input rooms.CreateMessageInput) (rooms.CreateMessageResult, error) {
	return f.createMessage(ctx, principalID, allowAll, workspaceID, roomID, key, input)
}

func (fakeRoomService) ListMessages(context.Context, string, string, rooms.MessageListOptions) ([]rooms.Message, error) {
	return nil, nil
}

func (fakeRoomService) ListInbox(context.Context, string, bool, string, rooms.InboxOptions) ([]rooms.Mention, error) {
	return nil, nil
}

func (fakeRoomService) UpdateMention(context.Context, string, bool, string, string, rooms.MentionStatusInput) (rooms.Mention, error) {
	return rooms.Mention{}, nil
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

func (f fakeWorkspaceService) Update(ctx context.Context, reference string, input workspaces.UpdateInput) (workspaces.Workspace, error) {
	return f.update(ctx, reference, input)
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
	list       func(context.Context, string, string, int64, int) ([]eventlog.Event, error)
	listRecent func(context.Context, string, string, *int64, int, string) ([]eventlog.Event, error)
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

func (f fakeEventReader) ListRecent(ctx context.Context, organizationID, projectID string, before *int64, limit int, query string) ([]eventlog.Event, error) {
	return f.listRecent(ctx, organizationID, projectID, before, limit, query)
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

func TestRepositorySyncRequiresMaintainerAndForwardsIdempotency(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	var requiredRole string
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"},
		Readiness:      func(context.Context) error { return nil },
		ProjectService: fakeProjectService{},
		RepositorySyncService: fakeRepositorySyncService{
			get: func(context.Context, string) (repositorysync.State, error) {
				return repositorysync.State{}, nil
			},
			sync: func(_ context.Context, principalID, receivedProjectID, key string) (repositorysync.Result, error) {
				if principalID != access.BootstrapPrincipalID || receivedProjectID != projectID || key != "sync-key" {
					t.Fatalf("unexpected sync arguments: %q %q %q", principalID, receivedProjectID, key)
				}
				return repositorysync.Result{State: repositorysync.State{
					ProjectID: projectID, Status: repositorysync.StatusSynced,
					Provider: "github", Visibility: "private",
				}}, nil
			},
		},
		AccessService: fakeAccessService{require: func(_ context.Context, _ access.Principal, receivedProjectID, role string) error {
			if receivedProjectID != projectID {
				t.Fatalf("project role check = %q", receivedProjectID)
			}
			requiredRole = role
			return nil
		}},
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) {
			return backoffice.Overview{}, nil
		}},
		EventReader: fakeEventReader{},
	})
	request := authenticatedRequest(http.MethodPost, "/v1/projects/"+projectID+"/repository-sync", `{}`)
	request.Header.Set("Idempotency-Key", "sync-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if requiredRole != "maintainer" {
		t.Fatalf("required role = %q", requiredRole)
	}
}

func TestConnectGitHubReturnsOfficialInstallationURL(t *testing.T) {
	called := false
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"},
		Readiness:      func(context.Context) error { return nil },
		ProjectService: fakeProjectService{},
		GitHubAppService: fakeGitHubAppService{connect: func(_ context.Context, principalID string) (githubapp.Connection, error) {
			called = true
			if principalID != access.BootstrapPrincipalID {
				t.Fatalf("principal ID = %q", principalID)
			}
			return githubapp.Connection{
				InstallURL: "https://github.com/apps/the-pact/installations/new?state=secret-state",
				ExpiresAt:  time.Date(2026, 8, 14, 12, 10, 0, 0, time.UTC),
			}, nil
		}},
		AccessService:    fakeAccessService{},
		BackofficeReader: fakeBackofficeReader{},
		EventReader:      fakeEventReader{},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/integrations/github/connect", `{}`))
	if response.Code != http.StatusCreated || !called {
		t.Fatalf("status = %d, called = %v, body = %s", response.Code, called, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "https://github.com/apps/the-pact/installations/new") {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestConnectGitHubRejectsNonAdministrator(t *testing.T) {
	principal := access.Principal{
		ID: access.BootstrapPrincipalID, OrganizationID: "00000000-0000-4000-8000-000000000001",
		DisplayName: "Contributor", PrincipalType: "human", OrganizationRole: "member",
	}
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: principal.OrganizationID,
		Build:          buildinfo.Info{Version: "test"},
		Readiness:      func(context.Context) error { return nil },
		ProjectService: fakeProjectService{},
		GitHubAppService: fakeGitHubAppService{connect: func(context.Context, string) (githubapp.Connection, error) {
			t.Fatal("Connect must not be called")
			return githubapp.Connection{}, nil
		}},
		AccessService:    fakeAccessService{principal: &principal},
		BackofficeReader: fakeBackofficeReader{},
		EventReader:      fakeEventReader{},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/integrations/github/connect", `{}`))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGitHubCallbackRunsSetupAndOAuthPhases(t *testing.T) {
	beginCalled := false
	completeCalled := false
	service := fakeGitHubAppService{
		begin: func(_ context.Context, state string, installationID int64) (string, error) {
			beginCalled = true
			if state != "connection-state" || installationID != 42 {
				t.Fatalf("setup callback = %q, %d", state, installationID)
			}
			return "https://github.com/login/oauth/authorize?state=connection-state", nil
		},
		complete: func(_ context.Context, state, code string) error {
			completeCalled = true
			if state != "connection-state" || code != "oauth-code" {
				t.Fatalf("OAuth callback = %q, %q", state, code)
			}
			return nil
		},
	}
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"},
		Readiness:      func(context.Context) error { return nil },
		ProjectService: fakeProjectService{}, GitHubAppService: service,
		AccessService: fakeAccessService{}, BackofficeReader: fakeBackofficeReader{},
		EventReader: fakeEventReader{},
	})
	setupResponse := httptest.NewRecorder()
	setupRequest := httptest.NewRequest(
		http.MethodGet,
		"/v1/integrations/github/callback?state=connection-state&installation_id=42",
		nil,
	)
	handler.ServeHTTP(setupResponse, setupRequest)
	if setupResponse.Code != http.StatusSeeOther ||
		setupResponse.Header().Get("Location") != "https://github.com/login/oauth/authorize?state=connection-state" ||
		!beginCalled {
		t.Fatalf("setup status = %d, location = %q", setupResponse.Code, setupResponse.Header().Get("Location"))
	}

	oauthResponse := httptest.NewRecorder()
	oauthRequest := httptest.NewRequest(
		http.MethodGet,
		"/v1/integrations/github/callback?state=connection-state&code=oauth-code",
		nil,
	)
	handler.ServeHTTP(oauthResponse, oauthRequest)
	if oauthResponse.Code != http.StatusSeeOther ||
		oauthResponse.Header().Get("Location") != "/admin/?github=connected" ||
		!completeCalled {
		t.Fatalf("OAuth status = %d, location = %q", oauthResponse.Code, oauthResponse.Header().Get("Location"))
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

func TestUpdateWorkspaceChangesIdentityAndColor(t *testing.T) {
	const workspaceID = "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
	handler := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build: buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{},
		WorkspaceService: fakeWorkspaceService{update: func(_ context.Context, reference string, input workspaces.UpdateInput) (workspaces.Workspace, error) {
			if reference != workspaceID || input.Name != "Footfall Platform" || input.Description != "Traffic intelligence" || input.Color != "#3877dc" {
				t.Fatalf("reference=%q input=%#v", reference, input)
			}
			return workspaces.Workspace{ID: workspaceID, Name: input.Name, Description: input.Description, Color: input.Color}, nil
		}},
		AccessService: fakeAccessService{}, BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }}, EventReader: fakeEventReader{},
	})
	request := authenticatedRequest(http.MethodPatch, "/v1/workspaces/"+workspaceID, `{"name":"Footfall Platform","description":"Traffic intelligence","color":"#3877dc"}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"color":"#3877dc"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
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

func TestPublicSiteAndAdminUseSeparateCanonicalRoutes(t *testing.T) {
	handler := testHandler(t, fakeProjectService{}, fakeEventReader{})

	rootRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	rootResponse := httptest.NewRecorder()
	handler.ServeHTTP(rootResponse, rootRequest)
	if rootResponse.Code != http.StatusOK ||
		!strings.Contains(rootResponse.Body.String(), `id="site-root"`) {
		t.Fatalf(
			"public site status=%d location=%q body=%s",
			rootResponse.Code,
			rootResponse.Header().Get("Location"),
			rootResponse.Body.String(),
		)
	}

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

	routeRequest := httptest.NewRequest(http.MethodGet, "/admin/workspaces/018f784a/overview", nil)
	routeRequest.Header.Set("Accept", "text/html")
	routeResponse := httptest.NewRecorder()
	handler.ServeHTTP(routeResponse, routeRequest)
	if routeResponse.Code != http.StatusOK || !strings.Contains(routeResponse.Body.String(), `id="root"`) {
		t.Fatalf("SPA route status = %d, body = %s", routeResponse.Code, routeResponse.Body.String())
	}

	missingRequest := httptest.NewRequest(http.MethodGet, "/admin/assets/not-an-asset.js", nil)
	missingRequest.Header.Set("Accept", "text/html")
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

func TestInvitationRegistrationCreatesBrowserSessionWithoutReturningADeviceCredential(t *testing.T) {
	handler := testHandlerWithAccess(t, fakeProjectService{}, fakeAccessService{
		register: func(_ context.Context, input authn.InvitationRegistrationInput, _ authn.SessionMetadata) (authn.CreatedInvitationSession, error) {
			if input.Secret != "pact_inv_test" || input.DisplayName != "Ada" || input.Username != "ada" {
				t.Fatalf("input = %#v", input)
			}
			return authn.CreatedInvitationSession{
				Acceptance:    authn.InvitationAcceptance{ProjectID: "project", ProjectRole: "contributor"},
				Session:       authn.WebSession{ExpiresAt: time.Now().Add(time.Hour)},
				SessionSecret: "pact_web_secret", CSRFSecret: "pact_csrf_secret",
			}, nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/invitations/register", bytes.NewBufferString(`{
		"secret":"pact_inv_test","display_name":"Ada","email":"ada@example.com",
		"username":"ada","password":"a sufficiently long password"
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d, Cache-Control = %q, body = %s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	if strings.Contains(response.Body.String(), "pact_web_secret") || strings.Contains(response.Body.String(), "pact_device_") {
		t.Fatalf("body = %s", response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 2 || cookies[0].Name != authn.WebSessionCookie || !cookies[0].HttpOnly {
		t.Fatalf("cookies = %#v", cookies)
	}
}

func TestBrowserSessionRequiresMatchingCSRFForMutation(t *testing.T) {
	principal := access.Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		DisplayName: "Ada", PrincipalType: "human", OrganizationRole: "owner",
	}
	authentication := fakeAccessService{
		principal: &principal,
		webSession: &authn.WebSession{
			ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e2e", Principal: principal, ExpiresAt: time.Now().Add(time.Hour),
		},
		csrfSecret: "pact_csrf_test_secret",
	}
	handler := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), OrganizationID: principal.OrganizationID,
		Build: buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{}, AuthenticationService: authentication, AccessService: authentication,
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) {
			return backoffice.Overview{}, nil
		}},
		EventReader: fakeEventReader{},
	})

	getRequest := httptest.NewRequest(http.MethodGet, "/v1/auth/session", nil)
	getRequest.AddCookie(&http.Cookie{Name: authn.WebSessionCookie, Value: "pact_web_test_secret"})
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"kind":"web"`) {
		t.Fatalf("GET session status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	missingRequest := httptest.NewRequest(http.MethodPost, "/v1/auth/devices/approve", bytes.NewBufferString(`{"user_code":"ABCD-EFGH"}`))
	missingRequest.Header.Set("Content-Type", "application/json")
	missingRequest.AddCookie(&http.Cookie{Name: authn.WebSessionCookie, Value: "pact_web_test_secret"})
	missingRequest.AddCookie(&http.Cookie{Name: authn.CSRFCookie, Value: "pact_csrf_test_secret"})
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missingRequest)
	if missingResponse.Code != http.StatusForbidden || !strings.Contains(missingResponse.Body.String(), `"code":"csrf_invalid"`) {
		t.Fatalf("missing CSRF status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}

	validRequest := httptest.NewRequest(http.MethodPost, "/v1/auth/devices/approve", bytes.NewBufferString(`{"user_code":"ABCD-EFGH"}`))
	validRequest.Header.Set("Content-Type", "application/json")
	validRequest.Header.Set("X-Pact-CSRF", "pact_csrf_test_secret")
	validRequest.AddCookie(&http.Cookie{Name: authn.WebSessionCookie, Value: "pact_web_test_secret"})
	validRequest.AddCookie(&http.Cookie{Name: authn.CSRFCookie, Value: "pact_csrf_test_secret"})
	validResponse := httptest.NewRecorder()
	handler.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusNoContent {
		t.Fatalf("valid CSRF status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
}

func TestUserAdministrationRequiresInteractiveWebSession(t *testing.T) {
	principal := access.Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		DisplayName: "Ada", PrincipalType: "human", OrganizationRole: "owner",
	}
	called := false
	administration := fakeUserAdminService{
		directory: func(_ context.Context, actor access.Principal) (useradmin.Directory, error) {
			called = true
			if actor.ID != principal.ID {
				t.Fatalf("actor = %#v", actor)
			}
			return useradmin.Directory{Users: []useradmin.User{{PrincipalID: principal.ID, Email: "ada@example.com"}}}, nil
		},
	}
	handler := userAdminTestHandler(principal, administration, "")

	deviceRequest := authenticatedRequest(http.MethodGet, "/v1/admin/users", "")
	deviceResponse := httptest.NewRecorder()
	handler.ServeHTTP(deviceResponse, deviceRequest)
	if deviceResponse.Code != http.StatusForbidden || !strings.Contains(deviceResponse.Body.String(), `"code":"web_session_required"`) {
		t.Fatalf("device status=%d body=%s", deviceResponse.Code, deviceResponse.Body.String())
	}
	if called {
		t.Fatal("directory was called for a device credential")
	}

	webRequest := httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil)
	webRequest.AddCookie(&http.Cookie{Name: authn.WebSessionCookie, Value: "pact_web_test_secret"})
	webResponse := httptest.NewRecorder()
	handler.ServeHTTP(webResponse, webRequest)
	if webResponse.Code != http.StatusOK || !called || !strings.Contains(webResponse.Body.String(), "ada@example.com") {
		t.Fatalf("web status=%d called=%v body=%s", webResponse.Code, called, webResponse.Body.String())
	}
}

func TestUserAdministrationMutationRequiresCSRF(t *testing.T) {
	principal := access.Principal{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", OrganizationID: "00000000-0000-4000-8000-000000000001",
		DisplayName: "Ada", PrincipalType: "human", OrganizationRole: "owner",
	}
	const targetID = "018f784a-68c1-7b0f-8f2a-cfc255f99e2e"
	called := false
	administration := fakeUserAdminService{
		directory: func(context.Context, access.Principal) (useradmin.Directory, error) {
			return useradmin.Directory{}, nil
		},
		update: func(_ context.Context, actor access.Principal, receivedTarget string, input useradmin.UpdateUserInput) (useradmin.User, error) {
			called = true
			if actor.ID != principal.ID || receivedTarget != targetID || input.Status == nil || *input.Status != "disabled" {
				t.Fatalf("actor=%#v target=%q input=%#v", actor, receivedTarget, input)
			}
			return useradmin.User{PrincipalID: targetID, Status: "disabled"}, nil
		},
	}
	handler := userAdminTestHandler(principal, administration, "pact_csrf_test_secret")
	path := "/v1/admin/users/" + targetID

	missingRequest := httptest.NewRequest(http.MethodPatch, path, bytes.NewBufferString(`{"status":"disabled"}`))
	missingRequest.Header.Set("Content-Type", "application/json")
	missingRequest.AddCookie(&http.Cookie{Name: authn.WebSessionCookie, Value: "pact_web_test_secret"})
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missingRequest)
	if missingResponse.Code != http.StatusForbidden || called {
		t.Fatalf("missing CSRF status=%d called=%v body=%s", missingResponse.Code, called, missingResponse.Body.String())
	}

	validRequest := httptest.NewRequest(http.MethodPatch, path, bytes.NewBufferString(`{"status":"disabled"}`))
	validRequest.Header.Set("Content-Type", "application/json")
	validRequest.Header.Set("X-Pact-CSRF", "pact_csrf_test_secret")
	validRequest.AddCookie(&http.Cookie{Name: authn.WebSessionCookie, Value: "pact_web_test_secret"})
	validRequest.AddCookie(&http.Cookie{Name: authn.CSRFCookie, Value: "pact_csrf_test_secret"})
	validResponse := httptest.NewRecorder()
	handler.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusOK || !called || !strings.Contains(validResponse.Body.String(), `"status":"disabled"`) {
		t.Fatalf("valid status=%d called=%v body=%s", validResponse.Code, called, validResponse.Body.String())
	}
}

func userAdminTestHandler(principal access.Principal, administration UserAdminService, csrfSecret string) http.Handler {
	authentication := fakeAccessService{
		principal: &principal,
		webSession: &authn.WebSession{
			ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e3f", Principal: principal, ExpiresAt: time.Now().Add(time.Hour),
		},
		csrfSecret: csrfSecret,
	}
	return New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), OrganizationID: principal.OrganizationID,
		Build: buildinfo.Info{Version: "test"}, Readiness: func(context.Context) error { return nil },
		ProjectService: fakeProjectService{}, AuthenticationService: authentication, AccessService: authentication,
		UserAdminService: administration,
		BackofficeReader: fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) {
			return backoffice.Overview{}, nil
		}},
		EventReader: fakeEventReader{},
	})
}

func TestProjectAccessReturnsMembersAndOwnedAgents(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	called := false
	handler := testHandlerWithAccess(t, fakeProjectService{}, fakeAccessService{
		projectAccess: func(_ context.Context, principal access.Principal, receivedProjectID string) (access.ProjectAccess, error) {
			called = true
			if principal.ID != access.BootstrapPrincipalID || receivedProjectID != projectID {
				t.Fatalf("GetProjectAccess(principal=%q, project=%q)", principal.ID, receivedProjectID)
			}
			return access.ProjectAccess{
				ProjectID: projectID,
				Members: []access.ProjectMember{{
					PrincipalID: access.BootstrapPrincipalID, DisplayName: "Jorge", EffectiveRole: "owner",
				}},
				Agents: []access.ProjectAgent{{
					AgentID: "018f784a-68c1-7b0f-8f2a-cfc255f99e2f", DisplayName: "Codex",
					SponsorPrincipalID: access.BootstrapPrincipalID, SponsorDisplayName: "Jorge", Connected: true,
				}},
			}, nil
		},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/projects/"+projectID+"/access", ""))

	if response.Code != http.StatusOK || !called {
		t.Fatalf("status = %d, called = %v, body = %s", response.Code, called, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(response.Body.String(), `"sponsor_display_name":"Jorge"`) {
		t.Fatalf("headers/body = %#v / %s", response.Header(), response.Body.String())
	}
}

func TestWorkspaceAccessReturnsAdministratorsWithoutProjects(t *testing.T) {
	const workspaceID = "018f784a-68c1-7b0f-8f2a-cfc255f99e4d"
	called := false
	handler := testHandlerWithAccess(t, fakeProjectService{}, fakeAccessService{
		workspaceAccess: func(_ context.Context, principal access.Principal, receivedWorkspaceID string) (access.WorkspaceAccess, error) {
			called = true
			if principal.ID != access.BootstrapPrincipalID || receivedWorkspaceID != workspaceID {
				t.Fatalf("GetWorkspaceAccess(principal=%q, workspace=%q)", principal.ID, receivedWorkspaceID)
			}
			return access.WorkspaceAccess{
				WorkspaceID: workspaceID,
				Members: []access.WorkspaceMember{{
					PrincipalID: access.BootstrapPrincipalID, DisplayName: "Jorge", EffectiveRole: "owner", AccessSource: "organization",
				}},
				Agents: []access.ProjectAgent{},
			}, nil
		},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/workspaces/"+workspaceID+"/access", ""))

	if response.Code != http.StatusOK || !called {
		t.Fatalf("status = %d, called = %v, body = %s", response.Code, called, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(response.Body.String(), `"display_name":"Jorge"`) ||
		!strings.Contains(response.Body.String(), `"agents":[]`) {
		t.Fatalf("headers/body = %#v / %s", response.Header(), response.Body.String())
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

func TestCreateRoomMessageAttributesHumanAndForwardsExplicitMentions(t *testing.T) {
	const (
		workspaceID = "018f784a-68c1-7b0f-8f2a-cfc255f99e10"
		roomID      = "018f784a-68c1-7b0f-8f2a-cfc255f99e11"
		agentID     = "018f784a-68c1-7b0f-8f2a-cfc255f99e12"
	)
	roomService := fakeRoomService{createMessage: func(
		_ context.Context, principalID string, allowAll bool,
		receivedWorkspaceID, receivedRoomID, key string, input rooms.CreateMessageInput,
	) (rooms.CreateMessageResult, error) {
		if principalID != access.BootstrapPrincipalID || !allowAll {
			t.Fatalf("principal=%q allowAll=%t", principalID, allowAll)
		}
		if receivedWorkspaceID != workspaceID || receivedRoomID != roomID || key != "room-message-key" {
			t.Fatalf("workspace=%q room=%q key=%q", receivedWorkspaceID, receivedRoomID, key)
		}
		if input.Body != "@codex revisa esta decisión" || len(input.MentionActorIDs) != 1 || input.MentionActorIDs[0] != agentID || input.AuthorSessionID != "" {
			t.Fatalf("input = %#v", input)
		}
		return rooms.CreateMessageResult{Message: rooms.Message{ID: roomID, WorkspaceID: workspaceID, RoomID: roomID, Body: input.Body}}, nil
	}}
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		Build:          buildinfo.Info{Version: "test"},
		Readiness:      func(context.Context) error { return nil },
		ProjectService: fakeProjectService{},
		WorkspaceService: fakeWorkspaceService{get: func(context.Context, string) (workspaces.Workspace, error) {
			return workspaces.Workspace{ID: workspaceID, Status: "active"}, nil
		}},
		RoomService:      roomService,
		AccessService:    fakeAccessService{},
		BackofficeReader: fakeBackofficeReader{},
		EventReader:      fakeEventReader{},
	})
	request := authenticatedRequest(
		http.MethodPost,
		"/v1/workspaces/"+workspaceID+"/rooms/"+roomID+"/messages",
		`{"body":"@codex revisa esta decisión","mention_actor_ids":["`+agentID+`"]}`,
	)
	request.Header.Set("Idempotency-Key", "room-message-key")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
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

func TestListEventsSupportsReverseHistoryAndSearch(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	service := fakeProjectService{
		get: func(_ context.Context, id string) (projects.Project, error) {
			return projects.Project{ID: id}, nil
		},
	}
	reader := fakeEventReader{
		listRecent: func(_ context.Context, organizationID, id string, before *int64, limit int, query string) ([]eventlog.Event, error) {
			if organizationID != "00000000-0000-4000-8000-000000000001" || id != projectID || before == nil || *before != 80 || limit != 21 || query != "deploy" {
				t.Fatalf("ListRecent(org=%q, project=%q, before=%v, limit=%d, query=%q)", organizationID, id, before, limit, query)
			}
			return []eventlog.Event{{ProjectSequence: 79, Type: "pact.deploy.completed.v1"}}, nil
		},
	}
	handler := testHandler(t, service, reader)
	request := authenticatedRequest(http.MethodGet, "/v1/projects/"+projectID+"/events?order=desc&before=80&limit=20&q=deploy", "")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Events []struct {
				Sequence string `json:"sequence"`
			} `json:"events"`
			NextCursor *string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Events) != 1 || body.Data.Events[0].Sequence != "79" || body.Data.NextCursor == nil || *body.Data.NextCursor != "79" {
		t.Fatalf("page = %#v", body.Data)
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

func TestWorkspaceDirectoryStreamNotifiesChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workspaceID := "018f784a-68c1-7b0f-8f2a-cfc255f99e30"
	call := 0
	current := workspaces.Workspace{
		ID: workspaceID, Name: "Footfall", Slug: "footfall", Status: "active", Version: 1,
		Projects: []workspaces.Project{},
	}
	service := fakeWorkspaceService{
		list: func(context.Context) ([]workspaces.Workspace, error) {
			call++
			switch call {
			case 1:
				return []workspaces.Workspace{current}, nil
			case 2:
				current.Version = 2
				current.Name = "Footfall actualizado"
				return []workspaces.Workspace{current}, nil
			default:
				cancel()
				return []workspaces.Workspace{current}, nil
			}
		},
	}
	handler := New(Config{
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID:       "00000000-0000-4000-8000-000000000001",
		Build:                buildinfo.Info{Version: "test"},
		Readiness:            func(context.Context) error { return nil },
		ProjectService:       fakeProjectService{},
		WorkspaceService:     service,
		AccessService:        fakeAccessService{},
		BackofficeReader:     fakeBackofficeReader{get: func(context.Context, string, string) (backoffice.Overview, error) { return backoffice.Overview{}, nil }},
		EventReader:          fakeEventReader{},
		StreamPollInterval:   time.Millisecond,
		StreamHeartbeatEvery: time.Hour,
	})
	request := authenticatedRequest(http.MethodGet, "/v1/workspaces/events/stream", "").WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, expected := range []string{
		"event: pact.workspace.directory.updated.v1\n",
		`"type":"pact.workspace.directory.updated.v1"`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("workspace stream body does not contain %q: %s", expected, response.Body.String())
		}
	}
}

func TestStreamStopsWhenCredentialIsRevoked(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	principal := access.Principal{
		ID:               "018f784a-68c1-7b0f-8f2a-cfc255f99e20",
		OrganizationID:   "00000000-0000-4000-8000-000000000001",
		DisplayName:      "Stream viewer",
		PrincipalType:    "human",
		OrganizationRole: "member",
	}

	for _, kind := range []string{"web", "device"} {
		t.Run(kind, func(t *testing.T) {
			var revoked atomic.Bool
			revocationChecked := make(chan struct{}, 1)
			authorizationTicks := make(chan time.Time, 1)
			session := authn.WebSession{
				ID:        "018f784a-68c1-7b0f-8f2a-cfc255f99e21",
				Principal: principal,
				ExpiresAt: time.Now().Add(time.Hour),
			}
			device := authn.DevicePrincipal{
				CredentialID: "018f784a-68c1-7b0f-8f2a-cfc255f99e22",
				Principal:    principal,
			}
			authentication := fakeAccessService{
				authenticateWeb: func(_ context.Context, credential string) (authn.WebSession, error) {
					if credential != "pact_web_test_secret" || revoked.Load() {
						select {
						case revocationChecked <- struct{}{}:
						default:
						}
						return authn.WebSession{}, authn.ErrUnauthorized
					}
					return session, nil
				},
				authenticateDevice: func(_ context.Context, credential string) (authn.DevicePrincipal, error) {
					if credential != testToken || revoked.Load() {
						select {
						case revocationChecked <- struct{}{}:
						default:
						}
						return authn.DevicePrincipal{}, authn.ErrUnauthorized
					}
					return device, nil
				},
			}
			streamStarted := make(chan struct{}, 1)
			handler := New(Config{
				Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
				OrganizationID: principal.OrganizationID,
				ProjectService: fakeProjectService{get: func(_ context.Context, id string) (projects.Project, error) {
					return projects.Project{ID: id}, nil
				}},
				AuthenticationService: authentication,
				AccessService:         authentication,
				EventReader: fakeEventReader{list: func(context.Context, string, string, int64, int) ([]eventlog.Event, error) {
					select {
					case streamStarted <- struct{}{}:
					default:
					}
					return nil, nil
				}},
				StreamPollInterval:       time.Hour,
				StreamHeartbeatEvery:     time.Hour,
				streamAuthorizationTicks: authorizationTicks,
			})

			ctx, cancel := context.WithCancel(context.Background())
			request := authenticatedRequest(http.MethodGet, "/v1/projects/"+projectID+"/events/stream", "").WithContext(ctx)
			if kind == "web" {
				request.Header.Del("Authorization")
				request.AddCookie(&http.Cookie{Name: authn.WebSessionCookie, Value: "pact_web_test_secret"})
			}
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				handler.ServeHTTP(response, request)
				close(done)
			}()

			select {
			case <-streamStarted:
			case <-time.After(2 * time.Second):
				cancel()
				<-done
				t.Fatal("stream did not start")
			}
			revoked.Store(true)
			authorizationTicks <- time.Now()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				cancel()
				<-done
				t.Fatal("stream did not stop after credential revocation")
			}
			cancel()
			select {
			case <-revocationChecked:
			default:
				t.Fatal("credential was not revalidated after the stream opened")
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if body := response.Body.String(); body != ": pact event stream\n\n" {
				t.Fatalf("stream wrote data after revocation: %q", body)
			}
		})
	}
}

func TestStreamStopsWhenViewerRoleIsRevoked(t *testing.T) {
	const projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	principal := access.Principal{
		ID:               "018f784a-68c1-7b0f-8f2a-cfc255f99e20",
		OrganizationID:   "00000000-0000-4000-8000-000000000001",
		DisplayName:      "Stream viewer",
		PrincipalType:    "human",
		OrganizationRole: "member",
	}
	device := authn.DevicePrincipal{
		CredentialID: "018f784a-68c1-7b0f-8f2a-cfc255f99e22",
		Principal:    principal,
	}
	var revoked atomic.Bool
	revocationChecked := make(chan struct{}, 1)
	authorizationTicks := make(chan time.Time, 1)
	authentication := fakeAccessService{
		authenticateDevice: func(_ context.Context, credential string) (authn.DevicePrincipal, error) {
			if credential != testToken {
				return authn.DevicePrincipal{}, authn.ErrUnauthorized
			}
			freshDevice := device
			if revoked.Load() {
				freshDevice.Principal.OrganizationRole = "member-after-revalidation"
			}
			return freshDevice, nil
		},
		require: func(_ context.Context, checkedPrincipal access.Principal, id, role string) error {
			if id != projectID || role != "viewer" {
				return fmt.Errorf("unexpected role check: project %q, role %q", id, role)
			}
			if revoked.Load() {
				if checkedPrincipal.OrganizationRole != "member-after-revalidation" {
					return fmt.Errorf("role check used stale principal with organization role %q", checkedPrincipal.OrganizationRole)
				}
				select {
				case revocationChecked <- struct{}{}:
				default:
				}
				return access.ErrForbidden
			}
			return nil
		},
	}
	streamStarted := make(chan struct{}, 1)
	handler := New(Config{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		OrganizationID: principal.OrganizationID,
		ProjectService: fakeProjectService{get: func(_ context.Context, id string) (projects.Project, error) {
			return projects.Project{ID: id}, nil
		}},
		AuthenticationService: authentication,
		AccessService:         authentication,
		EventReader: fakeEventReader{list: func(context.Context, string, string, int64, int) ([]eventlog.Event, error) {
			select {
			case streamStarted <- struct{}{}:
			default:
			}
			return nil, nil
		}},
		StreamPollInterval:       time.Hour,
		StreamHeartbeatEvery:     time.Hour,
		streamAuthorizationTicks: authorizationTicks,
	})

	ctx, cancel := context.WithCancel(context.Background())
	request := authenticatedRequest(http.MethodGet, "/v1/projects/"+projectID+"/events/stream", "").WithContext(ctx)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()

	select {
	case <-streamStarted:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("stream did not start")
	}
	revoked.Store(true)
	authorizationTicks <- time.Now()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("stream did not stop after role revocation")
	}
	cancel()
	select {
	case <-revocationChecked:
	default:
		t.Fatal("viewer role was not revalidated after the stream opened")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); body != ": pact event stream\n\n" {
		t.Fatalf("stream wrote data after role revocation: %q", body)
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
	if body != "" && method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}
