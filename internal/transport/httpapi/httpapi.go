package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/authn"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/contextpack"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/githubapp"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/projectrepo"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/repositorysync"
	"github.com/jorgenuanzs/the-pact/internal/rooms"
	"github.com/jorgenuanzs/the-pact/internal/transport/httpapi/adminui"
	"github.com/jorgenuanzs/the-pact/internal/transport/httpapi/publicui"
	"github.com/jorgenuanzs/the-pact/internal/useradmin"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

const maxRequestBody = 1 << 20

type ProjectService interface {
	Create(context.Context, string, projects.CreateInput) (projects.CreateResult, error)
	Get(context.Context, string) (projects.Project, error)
	List(context.Context) ([]projects.Project, error)
}

type RepositorySyncService interface {
	Get(context.Context, string) (repositorysync.State, error)
	GetRepository(context.Context, string, string) (repositorysync.State, error)
	List(context.Context, string) ([]repositorysync.State, error)
	Sync(context.Context, string, string, string) (repositorysync.Result, error)
	SyncRepository(context.Context, string, string, string, string) (repositorysync.Result, error)
}

type ProjectRepositoryService interface {
	List(context.Context, string) ([]projectrepo.Repository, error)
	ListAvailable(context.Context, string) ([]projectrepo.AvailableRepository, error)
	Attach(context.Context, string, string, projectrepo.AttachInput) (projectrepo.Repository, error)
}

type GitHubAppService interface {
	Status(context.Context) (githubapp.Status, error)
	Connect(context.Context, string) (githubapp.Connection, error)
	BeginUserAuthorization(context.Context, string, int64) (string, error)
	CompleteConnection(context.Context, string, string) error
	HandleWebhook(context.Context, string, string, string, []byte) error
}

type WorkspaceService interface {
	Create(context.Context, string, workspaces.CreateInput) (workspaces.CreateResult, error)
	Get(context.Context, string) (workspaces.Workspace, error)
	List(context.Context) ([]workspaces.Workspace, error)
	Update(context.Context, string, workspaces.UpdateInput) (workspaces.Workspace, error)
	AttachProject(context.Context, string, string) (workspaces.Workspace, error)
}

type KnowledgeService interface {
	CreateResource(context.Context, string, string, string, knowledge.CreateResourceInput) (knowledge.CreateResourceResult, error)
	ListResources(context.Context, string, knowledge.ListOptions) ([]knowledge.Resource, error)
	CreateRecord(context.Context, string, string, string, knowledge.CreateRecordInput) (knowledge.CreateRecordResult, error)
	GetRecord(context.Context, string, string) (knowledge.Record, error)
	ListRecords(context.Context, string, knowledge.ListOptions) ([]knowledge.Record, error)
	UpdateRecordStatus(context.Context, string, string, string, string, knowledge.RecordStatusInput) (knowledge.RecordStatusResult, error)
	Context(context.Context, string) (knowledge.WorkspaceContext, error)
}

type RoomService interface {
	CreateRoom(context.Context, string, string, string, rooms.CreateRoomInput) (rooms.CreateRoomResult, error)
	ListRooms(context.Context, string) ([]rooms.Room, error)
	ListParticipants(context.Context, string) ([]rooms.Participant, error)
	CreateMessage(context.Context, string, bool, string, string, string, rooms.CreateMessageInput) (rooms.CreateMessageResult, error)
	ListMessages(context.Context, string, string, rooms.MessageListOptions) ([]rooms.Message, error)
	ListInbox(context.Context, string, bool, string, rooms.InboxOptions) ([]rooms.Mention, error)
	UpdateMention(context.Context, string, bool, string, string, rooms.MentionStatusInput) (rooms.Mention, error)
}

type AgentSessionService interface {
	Start(context.Context, string, string, agentsession.StartInput) (agentsession.Session, error)
	Heartbeat(context.Context, string, bool, string) (agentsession.Session, error)
	Observe(context.Context, string, string, string, agentsession.ObservationInput) (agentsession.ObservationResult, error)
	Close(context.Context, string, bool, string) error
}

type CoordinationService interface {
	CheckScopes(context.Context, string, []coordination.ScopeInput) (coordination.ScopeCheckResult, error)
	Start(context.Context, string, bool, string, string, coordination.StartInput) (coordination.StartResult, error)
	AttachWorkspace(context.Context, string, bool, string, string, coordination.WorkspaceInput) (coordination.WorkspaceResult, error)
	UpdateStatus(context.Context, string, bool, string, string, coordination.StatusInput) (coordination.StatusResult, error)
	List(context.Context, string) ([]coordination.WorkItem, error)
}

type HandoffService interface {
	OfferHandoff(context.Context, string, bool, string, string, string, coordination.OfferHandoffInput) (coordination.HandoffResult, error)
	ListHandoffs(context.Context, string, string) ([]coordination.Handoff, error)
	UpdateHandoffStatus(context.Context, string, bool, string, string, string, string, coordination.HandoffStatusInput) (coordination.HandoffResult, error)
}

type ContextPackService interface {
	Compile(context.Context, string, bool, string, string, string, contextpack.CompileInput) (contextpack.CompileResult, error)
	Get(context.Context, string, string) (contextpack.ContextPack, error)
}

type AccessService interface {
	RequireProjectRole(context.Context, access.Principal, string, string) error
	VisibleProjectIDs(context.Context, access.Principal) (map[string]struct{}, error)
	CanCreateProject(access.Principal) bool
	GetProjectAccess(context.Context, access.Principal, string) (access.ProjectAccess, error)
	CreateInvitation(context.Context, access.Principal, string, access.CreateInvitationInput) (access.CreatedInvitation, error)
	RevokeInvitation(context.Context, access.Principal, string) error
	GrantProjectOwner(context.Context, access.Principal, string) error
}

type AuthenticationService interface {
	SetupStatus(context.Context) (authn.SetupStatus, error)
	Setup(context.Context, authn.SetupInput, authn.SessionMetadata) (authn.CreatedWebSession, error)
	Login(context.Context, authn.LoginInput, authn.SessionMetadata) (authn.CreatedWebSession, error)
	AuthenticateWeb(context.Context, string) (authn.WebSession, error)
	AuthenticateDevice(context.Context, string) (authn.DevicePrincipal, error)
	ValidateCSRF(authn.WebSession, string) bool
	LogoutWeb(context.Context, authn.WebSession) error
	ChangePassword(context.Context, authn.WebSession, authn.ChangePasswordInput) error
	PreviewInvitation(context.Context, string) (authn.InvitationPreview, error)
	RegisterInvitation(context.Context, authn.InvitationRegistrationInput, authn.SessionMetadata) (authn.CreatedInvitationSession, error)
	AcceptInvitation(context.Context, access.Principal, string) (authn.InvitationAcceptance, error)
	BeginDevice(context.Context, authn.BeginDeviceInput) (authn.DeviceAuthorization, error)
	ApproveDevice(context.Context, access.Principal, string) error
	ExchangeDevice(context.Context, string) (authn.DeviceExchange, error)
	RevokeCurrentDevice(context.Context, authn.DevicePrincipal) error
	ListDevices(context.Context, access.Principal) ([]authn.Device, error)
	RevokeDevice(context.Context, access.Principal, string) error
}

type UserAdminService interface {
	Directory(context.Context, access.Principal) (useradmin.Directory, error)
	GetUser(context.Context, access.Principal, string) (useradmin.User, error)
	UpdateUser(context.Context, access.Principal, string, useradmin.UpdateUserInput) (useradmin.User, error)
	SetProjectPermission(context.Context, access.Principal, string, string, string) (useradmin.User, error)
	RemoveProjectPermission(context.Context, access.Principal, string, string) (useradmin.User, error)
	RevokeUserSessions(context.Context, access.Principal, string) (useradmin.User, error)
	CreateInvitation(context.Context, access.Principal, useradmin.CreateInvitationInput) (useradmin.CreatedInvitation, error)
	RevokeInvitation(context.Context, access.Principal, string) error
}

type ReadinessCheck func(context.Context) error

type Config struct {
	Logger                   *slog.Logger
	OrganizationID           string
	Build                    buildinfo.Info
	Readiness                ReadinessCheck
	ProjectService           ProjectService
	RepositorySyncService    RepositorySyncService
	ProjectRepositoryService ProjectRepositoryService
	GitHubAppService         GitHubAppService
	WorkspaceService         WorkspaceService
	KnowledgeService         KnowledgeService
	RoomService              RoomService
	AgentSessionService      AgentSessionService
	CoordinationService      CoordinationService
	HandoffService           HandoffService
	ContextPackService       ContextPackService
	AuthenticationService    AuthenticationService
	AccessService            AccessService
	UserAdminService         UserAdminService
	BackofficeReader         backoffice.Reader
	EventReader              eventlog.Reader
	StreamShutdown           <-chan struct{}
	StreamPollInterval       time.Duration
	StreamHeartbeatEvery     time.Duration
	StreamAuthorizationEvery time.Duration
	streamAuthorizationTicks <-chan time.Time
}

type API struct {
	logger                   *slog.Logger
	organizationID           string
	build                    buildinfo.Info
	readiness                ReadinessCheck
	projects                 ProjectService
	repositorySync           RepositorySyncService
	projectRepositories      ProjectRepositoryService
	githubApp                GitHubAppService
	workspaces               WorkspaceService
	knowledge                KnowledgeService
	rooms                    RoomService
	agentSessions            AgentSessionService
	coordination             CoordinationService
	handoffs                 HandoffService
	contextPacks             ContextPackService
	authentication           AuthenticationService
	access                   AccessService
	userAdmin                UserAdminService
	backoffice               backoffice.Reader
	events                   eventlog.Reader
	streamShutdown           <-chan struct{}
	streamPollInterval       time.Duration
	streamHeartbeatEvery     time.Duration
	streamAuthorizationEvery time.Duration
	streamAuthorizationTicks <-chan time.Time
}

func New(cfg Config) http.Handler {
	if cfg.AuthenticationService == nil {
		cfg.AuthenticationService, _ = cfg.AccessService.(AuthenticationService)
	}
	if cfg.StreamPollInterval <= 0 {
		cfg.StreamPollInterval = time.Second
	}
	if cfg.StreamHeartbeatEvery <= 0 {
		cfg.StreamHeartbeatEvery = 15 * time.Second
	}
	if cfg.StreamAuthorizationEvery <= 0 {
		cfg.StreamAuthorizationEvery = 15 * time.Second
	}

	api := &API{
		logger:                   cfg.Logger,
		organizationID:           cfg.OrganizationID,
		build:                    cfg.Build,
		readiness:                cfg.Readiness,
		projects:                 cfg.ProjectService,
		repositorySync:           cfg.RepositorySyncService,
		projectRepositories:      cfg.ProjectRepositoryService,
		githubApp:                cfg.GitHubAppService,
		workspaces:               cfg.WorkspaceService,
		knowledge:                cfg.KnowledgeService,
		rooms:                    cfg.RoomService,
		agentSessions:            cfg.AgentSessionService,
		coordination:             cfg.CoordinationService,
		handoffs:                 cfg.HandoffService,
		contextPacks:             cfg.ContextPackService,
		authentication:           cfg.AuthenticationService,
		access:                   cfg.AccessService,
		userAdmin:                cfg.UserAdminService,
		backoffice:               cfg.BackofficeReader,
		events:                   cfg.EventReader,
		streamShutdown:           cfg.StreamShutdown,
		streamPollInterval:       cfg.StreamPollInterval,
		streamHeartbeatEvery:     cfg.StreamHeartbeatEvery,
		streamAuthorizationEvery: cfg.StreamAuthorizationEvery,
		streamAuthorizationTicks: cfg.streamAuthorizationTicks,
	}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", publicui.Handler())
	mux.Handle("GET /assets/", publicui.Handler())
	mux.Handle("GET /favicon.svg", publicui.Handler())
	mux.Handle("GET /robots.txt", publicui.Handler())
	mux.Handle("GET /sitemap.xml", publicui.Handler())
	mux.HandleFunc("GET /livez", api.handleLive)
	mux.HandleFunc("GET /readyz", api.handleReady)
	mux.HandleFunc("GET /version", api.handleVersion)
	mux.HandleFunc("GET /admin", api.handleAdminRedirect)
	mux.Handle("GET /admin/", adminui.Handler())
	mux.HandleFunc("GET /v1/auth/setup", api.handleAuthSetupStatus)
	mux.HandleFunc("POST /v1/auth/setup", api.handleAuthSetup)
	mux.HandleFunc("POST /v1/auth/login", api.handleAuthLogin)
	mux.HandleFunc("POST /v1/auth/invitations/preview", api.handleAuthInvitationPreview)
	mux.HandleFunc("POST /v1/auth/invitations/register", api.handleAuthInvitationRegister)
	mux.HandleFunc("POST /v1/auth/devices", api.handleAuthBeginDevice)
	mux.HandleFunc("POST /v1/auth/devices/exchange", api.handleAuthExchangeDevice)
	mux.Handle("GET /v1/auth/session", api.requireAuth(http.HandlerFunc(api.handleAuthSession)))
	mux.Handle("DELETE /v1/auth/session", api.requireAuth(http.HandlerFunc(api.handleAuthLogout)))
	mux.Handle("PUT /v1/auth/password", api.requireAuth(http.HandlerFunc(api.handleAuthChangePassword)))
	mux.Handle("POST /v1/auth/invitations/accept", api.requireAuth(http.HandlerFunc(api.handleAuthInvitationAccept)))
	mux.Handle("POST /v1/auth/devices/approve", api.requireAuth(http.HandlerFunc(api.handleAuthApproveDevice)))
	mux.Handle("GET /v1/auth/devices", api.requireAuth(http.HandlerFunc(api.handleAuthListDevices)))
	mux.Handle("DELETE /v1/auth/devices/{deviceID}", api.requireAuth(http.HandlerFunc(api.handleAuthRevokeDevice)))
	mux.Handle("DELETE /v1/auth/device/current", api.requireAuth(http.HandlerFunc(api.handleAuthRevokeCurrentDevice)))
	mux.Handle("GET /v1/admin/users", api.requireAuth(http.HandlerFunc(api.handleAdminListUsers)))
	mux.Handle("GET /v1/admin/users/{principalID}", api.requireAuth(http.HandlerFunc(api.handleAdminGetUser)))
	mux.Handle("PATCH /v1/admin/users/{principalID}", api.requireAuth(http.HandlerFunc(api.handleAdminUpdateUser)))
	mux.Handle("DELETE /v1/admin/users/{principalID}", api.requireAuth(http.HandlerFunc(api.handleAdminDisableUser)))
	mux.Handle("DELETE /v1/admin/users/{principalID}/sessions", api.requireAuth(http.HandlerFunc(api.handleAdminRevokeUserSessions)))
	mux.Handle("PUT /v1/admin/users/{principalID}/projects/{projectID}", api.requireAuth(http.HandlerFunc(api.handleAdminSetProjectPermission)))
	mux.Handle("DELETE /v1/admin/users/{principalID}/projects/{projectID}", api.requireAuth(http.HandlerFunc(api.handleAdminRemoveProjectPermission)))
	mux.Handle("POST /v1/admin/invitations", api.requireAuth(http.HandlerFunc(api.handleAdminCreateInvitation)))
	mux.Handle("DELETE /v1/admin/invitations/{invitationID}", api.requireAuth(http.HandlerFunc(api.handleAdminRevokeInvitation)))
	mux.Handle("GET /v1/projects", api.requireAuth(http.HandlerFunc(api.handleListProjects)))
	mux.Handle("POST /v1/projects", api.requireAuth(http.HandlerFunc(api.handleCreateProject)))
	mux.Handle("GET /v1/workspaces", api.requireAuth(http.HandlerFunc(api.handleListWorkspaces)))
	mux.Handle("POST /v1/workspaces", api.requireAuth(http.HandlerFunc(api.handleCreateWorkspace)))
	mux.Handle("GET /v1/workspaces/{workspaceID}", api.requireAuth(http.HandlerFunc(api.handleGetWorkspace)))
	mux.Handle("PATCH /v1/workspaces/{workspaceID}", api.requireAuth(http.HandlerFunc(api.handleUpdateWorkspace)))
	mux.Handle("PUT /v1/workspaces/{workspaceID}/projects/{projectID}", api.requireAuth(http.HandlerFunc(api.handleAttachWorkspaceProject)))
	mux.Handle("GET /v1/workspaces/{workspaceID}/resources", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleListResources))))
	mux.Handle("POST /v1/workspaces/{workspaceID}/resources", api.requireAuth(api.requireWorkspaceRole("contributor", http.HandlerFunc(api.handleCreateResource))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/records", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleListRecords))))
	mux.Handle("POST /v1/workspaces/{workspaceID}/records", api.requireAuth(api.requireWorkspaceRole("contributor", http.HandlerFunc(api.handleCreateRecord))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/records/{recordID}", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleGetRecord))))
	mux.Handle("POST /v1/workspaces/{workspaceID}/records/{recordID}/status", api.requireAuth(api.requireWorkspaceRole("maintainer", http.HandlerFunc(api.handleUpdateRecordStatus))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/context", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleWorkspaceContext))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/rooms", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleListRooms))))
	mux.Handle("POST /v1/workspaces/{workspaceID}/rooms", api.requireAuth(api.requireWorkspaceRole("maintainer", http.HandlerFunc(api.handleCreateRoom))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/participants", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleListRoomParticipants))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/rooms/{roomID}/messages", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleListRoomMessages))))
	mux.Handle("POST /v1/workspaces/{workspaceID}/rooms/{roomID}/messages", api.requireAuth(api.requireWorkspaceRole("contributor", http.HandlerFunc(api.handleCreateRoomMessage))))
	mux.Handle("GET /v1/projects/{projectID}", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleGetProject))))
	mux.Handle("GET /v1/projects/{projectID}/repository-sync", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleGetRepositorySync))))
	mux.Handle("POST /v1/projects/{projectID}/repository-sync", api.requireAuth(api.requireProjectRole("maintainer", http.HandlerFunc(api.handleSyncRepository))))
	mux.Handle("GET /v1/projects/{projectID}/repositories", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleListProjectRepositories))))
	mux.Handle("POST /v1/projects/{projectID}/repositories", api.requireAuth(api.requireProjectRole("maintainer", http.HandlerFunc(api.handleAttachProjectRepository))))
	mux.Handle("GET /v1/projects/{projectID}/repositories/{repositoryID}/sync", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleGetProjectRepositorySync))))
	mux.Handle("POST /v1/projects/{projectID}/repositories/{repositoryID}/sync", api.requireAuth(api.requireProjectRole("maintainer", http.HandlerFunc(api.handleSyncProjectRepository))))
	mux.Handle("GET /v1/integrations/github", api.requireAuth(http.HandlerFunc(api.handleGitHubStatus)))
	mux.Handle("POST /v1/integrations/github/connect", api.requireAuth(http.HandlerFunc(api.handleConnectGitHub)))
	mux.Handle("GET /v1/integrations/github/repositories", api.requireAuth(http.HandlerFunc(api.handleListAuthorizedGitHubRepositories)))
	mux.HandleFunc("GET /v1/integrations/github/callback", api.handleGitHubCallback)
	mux.HandleFunc("POST /v1/integrations/github/webhook", api.handleGitHubWebhook)
	mux.Handle("GET /v1/projects/{projectID}/overview", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleProjectOverview))))
	mux.Handle("GET /v1/projects/{projectID}/access", api.requireAuth(http.HandlerFunc(api.handleProjectAccess)))
	mux.Handle("GET /v1/projects/{projectID}/events", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleListEvents))))
	mux.Handle("GET /v1/projects/{projectID}/events/stream", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleStreamEvents))))
	mux.Handle("POST /v1/projects/{projectID}/agent-sessions", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleStartAgentSession))))
	mux.Handle("POST /v1/projects/{projectID}/scope-checks", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleScopeCheck))))
	mux.Handle("GET /v1/projects/{projectID}/work-items", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleListWork))))
	mux.Handle("POST /v1/projects/{projectID}/work-items", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleStartWork))))
	mux.Handle("GET /v1/projects/{projectID}/handoffs", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleListHandoffs))))
	mux.Handle("POST /v1/projects/{projectID}/intents/{intentID}/handoffs", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleOfferHandoff))))
	mux.Handle("POST /v1/projects/{projectID}/intents/{intentID}/handoffs/{handoffID}/status", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleUpdateHandoffStatus))))
	mux.Handle("POST /v1/projects/{projectID}/intents/{intentID}/context-packs", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleCompileContextPack))))
	mux.Handle("GET /v1/projects/{projectID}/context-packs/{contextPackID}", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleGetContextPack))))
	mux.Handle("POST /v1/intents/{intentID}/workspace", api.requireAuth(http.HandlerFunc(api.handleAttachWorkspace)))
	mux.Handle("POST /v1/intents/{intentID}/worktree", api.requireAuth(http.HandlerFunc(api.handleAttachWorktree)))
	mux.Handle("POST /v1/intents/{intentID}/status", api.requireAuth(http.HandlerFunc(api.handleUpdateIntentStatus)))
	mux.Handle("POST /v1/agent-sessions/{sessionID}/heartbeat", api.requireAuth(http.HandlerFunc(api.handleAgentHeartbeat)))
	mux.Handle("POST /v1/agent-sessions/{sessionID}/repository-observations", api.requireAuth(http.HandlerFunc(api.handleRepositoryObservation)))
	mux.Handle("DELETE /v1/agent-sessions/{sessionID}", api.requireAuth(http.HandlerFunc(api.handleCloseAgentSession)))
	mux.Handle("POST /v1/projects/{projectID}/invitations", api.requireAuth(http.HandlerFunc(api.handleCreateInvitation)))
	mux.Handle("DELETE /v1/invitations/{invitationID}", api.requireAuth(http.HandlerFunc(api.handleRevokeInvitation)))
	mux.Handle("GET /v1/me", api.requireAuth(http.HandlerFunc(api.handleMe)))
	mux.Handle("GET /v1/me/room-mentions", api.requireAuth(http.HandlerFunc(api.handleMyRoomMentions)))
	mux.Handle("POST /v1/me/room-mentions/{mentionID}/status", api.requireAuth(http.HandlerFunc(api.handleMyRoomMentionStatus)))
	mux.Handle("GET /v1/agent-sessions/{sessionID}/room-mentions", api.requireAuth(http.HandlerFunc(api.handleAgentRoomMentions)))
	mux.Handle("POST /v1/agent-sessions/{sessionID}/room-mentions/{mentionID}/status", api.requireAuth(http.HandlerFunc(api.handleAgentRoomMentionStatus)))
	mux.Handle("/{$}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/livez", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/readyz", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/version", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/admin", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/admin/", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}", api.methodNotAllowed(http.MethodGet+", "+http.MethodPatch))
	mux.Handle("/v1/workspaces/{workspaceID}/projects/{projectID}", api.methodNotAllowed(http.MethodPut))
	mux.Handle("/v1/admin/users", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/admin/users/{principalID}", api.methodNotAllowed(http.MethodGet+", "+http.MethodPatch+", "+http.MethodDelete))
	mux.Handle("/v1/admin/users/{principalID}/sessions", api.methodNotAllowed(http.MethodDelete))
	mux.Handle("/v1/admin/users/{principalID}/projects/{projectID}", api.methodNotAllowed(http.MethodPut+", "+http.MethodDelete))
	mux.Handle("/v1/admin/invitations", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/admin/invitations/{invitationID}", api.methodNotAllowed(http.MethodDelete))
	mux.Handle("/v1/workspaces/{workspaceID}/resources", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}/records", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}/records/{recordID}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/workspaces/{workspaceID}/records/{recordID}/status", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}/context", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/workspaces/{workspaceID}/rooms", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}/participants", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/workspaces/{workspaceID}/rooms/{roomID}/messages", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/projects/{projectID}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/repository-sync", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/repositories", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/repositories/{repositoryID}/sync", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/integrations/github", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/integrations/github/connect", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/integrations/github/repositories", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/integrations/github/callback", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/integrations/github/webhook", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/overview", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/access", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/events", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/events/stream", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/agent-sessions", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/scope-checks", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/work-items", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/handoffs", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/intents/{intentID}/handoffs", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/intents/{intentID}/handoffs/{handoffID}/status", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/intents/{intentID}/context-packs", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/context-packs/{contextPackID}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/intents/{intentID}/workspace", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/intents/{intentID}/worktree", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/intents/{intentID}/status", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/agent-sessions/{sessionID}/heartbeat", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/agent-sessions/{sessionID}/repository-observations", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/agent-sessions/{sessionID}", api.methodNotAllowed(http.MethodDelete))
	mux.Handle("/v1/projects/{projectID}/invitations", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/invitations/{invitationID}", api.methodNotAllowed(http.MethodDelete))
	mux.Handle("/v1/me", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/me/room-mentions", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/me/room-mentions/{mentionID}/status", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/agent-sessions/{sessionID}/room-mentions", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/agent-sessions/{sessionID}/room-mentions/{mentionID}/status", api.methodNotAllowed(http.MethodPost))
	mux.HandleFunc("/", api.handleNotFound)

	return api.requestContext(api.accessLog(api.recoverPanic(mux)))
}

func (a *API) handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "live"}})
}

func (a *API) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if a.readiness == nil || a.readiness(ctx) != nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "not_ready", "Service unavailable", "Pact is not ready to receive traffic.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ready"}})
}

func (a *API) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": a.build})
}

func (a *API) handleAdminRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/", http.StatusPermanentRedirect)
}

func (a *API) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projectList, err := a.projects.List(r.Context())
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	principal, _ := principalFromContext(r.Context())
	visible, err := a.access.VisibleProjectIDs(r.Context(), principal)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if visible != nil {
		filtered := make([]projects.Project, 0, len(projectList))
		for _, project := range projectList {
			if _, ok := visible[project.ID]; ok {
				filtered = append(filtered, project)
			}
		}
		projectList = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"projects": projectList,
	}})
}

func (a *API) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	if a.access == nil || !a.access.CanCreateProject(principal) {
		a.writeDomainError(w, r, access.ErrForbidden)
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}

	var input projects.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}

	result, err := a.projects.Create(r.Context(), r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if err := a.access.GrantProjectOwner(r.Context(), principal, result.Project.ID); err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Location", "/v1/projects/"+result.Project.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": result.Project})
}

func (a *API) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if a.workspaces == nil {
		a.writeDomainError(w, r, errors.New("workspace service is not configured"))
		return
	}
	workspaceList, err := a.workspaces.List(r.Context())
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	principal, _ := principalFromContext(r.Context())
	visible, err := a.access.VisibleProjectIDs(r.Context(), principal)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	workspaceList = filterVisibleWorkspaces(workspaceList, visible)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"workspaces": workspaceList,
	}})
}

func (a *API) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	if a.workspaces == nil {
		a.writeDomainError(w, r, errors.New("workspace service is not configured"))
		return
	}
	if a.access == nil || !a.access.CanCreateProject(principal) {
		a.writeDomainError(w, r, access.ErrForbidden)
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input workspaces.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	result, err := a.workspaces.Create(r.Context(), r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Location", "/v1/workspaces/"+result.Workspace.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": result.Workspace})
}

func (a *API) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	if a.workspaces == nil {
		a.writeDomainError(w, r, errors.New("workspace service is not configured"))
		return
	}
	workspace, err := a.workspaces.Get(r.Context(), r.PathValue("workspaceID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	principal, _ := principalFromContext(r.Context())
	visible, err := a.access.VisibleProjectIDs(r.Context(), principal)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	filtered := filterVisibleWorkspaces([]workspaces.Workspace{workspace}, visible)
	if len(filtered) == 0 {
		a.writeDomainError(w, r, access.ErrForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": filtered[0]})
}

func (a *API) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	if a.workspaces == nil {
		a.writeDomainError(w, r, errors.New("workspace service is not configured"))
		return
	}
	if a.access == nil || !a.access.CanCreateProject(principal) {
		a.writeDomainError(w, r, access.ErrForbidden)
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input workspaces.UpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	workspace, err := a.workspaces.Update(r.Context(), r.PathValue("workspaceID"), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": workspace})
}

func (a *API) handleAttachWorkspaceProject(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	if a.workspaces == nil {
		a.writeDomainError(w, r, errors.New("workspace service is not configured"))
		return
	}
	if a.access == nil || !a.access.CanCreateProject(principal) {
		a.writeDomainError(w, r, access.ErrForbidden)
		return
	}
	workspace, err := a.workspaces.AttachProject(
		r.Context(), r.PathValue("workspaceID"), r.PathValue("projectID"),
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": workspace})
}

func (a *API) handleListResources(w http.ResponseWriter, r *http.Request) {
	if a.knowledge == nil {
		a.writeDomainError(w, r, errors.New("knowledge service is not configured"))
		return
	}
	options, err := knowledgeListOptions(r, "kind")
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", err.Error())
		return
	}
	resources, err := a.knowledge.ListResources(r.Context(), r.PathValue("workspaceID"), options)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"resources": resources}})
}

func (a *API) handleCreateResource(w http.ResponseWriter, r *http.Request) {
	if a.knowledge == nil {
		a.writeDomainError(w, r, errors.New("knowledge service is not configured"))
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input knowledge.CreateResourceInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.knowledge.CreateResource(
		r.Context(), principal.ID, r.PathValue("workspaceID"), r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": result.Resource})
}

func (a *API) handleListRecords(w http.ResponseWriter, r *http.Request) {
	if a.knowledge == nil {
		a.writeDomainError(w, r, errors.New("knowledge service is not configured"))
		return
	}
	options, err := knowledgeListOptions(r, "type")
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", err.Error())
		return
	}
	records, err := a.knowledge.ListRecords(r.Context(), r.PathValue("workspaceID"), options)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"records": records}})
}

func (a *API) handleCreateRecord(w http.ResponseWriter, r *http.Request) {
	if a.knowledge == nil {
		a.writeDomainError(w, r, errors.New("knowledge service is not configured"))
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input knowledge.CreateRecordInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.knowledge.CreateRecord(
		r.Context(), principal.ID, r.PathValue("workspaceID"), r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Location", "/v1/workspaces/"+r.PathValue("workspaceID")+"/records/"+result.Record.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": result.Record})
}

func (a *API) handleGetRecord(w http.ResponseWriter, r *http.Request) {
	if a.knowledge == nil {
		a.writeDomainError(w, r, errors.New("knowledge service is not configured"))
		return
	}
	record, err := a.knowledge.GetRecord(r.Context(), r.PathValue("workspaceID"), r.PathValue("recordID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": record})
}

func (a *API) handleUpdateRecordStatus(w http.ResponseWriter, r *http.Request) {
	if a.knowledge == nil {
		a.writeDomainError(w, r, errors.New("knowledge service is not configured"))
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input knowledge.RecordStatusInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.knowledge.UpdateRecordStatus(
		r.Context(), principal.ID, r.PathValue("workspaceID"), r.PathValue("recordID"),
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result.Record})
}

func (a *API) handleWorkspaceContext(w http.ResponseWriter, r *http.Request) {
	if a.knowledge == nil {
		a.writeDomainError(w, r, errors.New("knowledge service is not configured"))
		return
	}
	context, err := a.knowledge.Context(r.Context(), r.PathValue("workspaceID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": context})
}

func (a *API) handleListRooms(w http.ResponseWriter, r *http.Request) {
	if a.rooms == nil {
		a.writeDomainError(w, r, errors.New("room service is not configured"))
		return
	}
	roomList, err := a.rooms.ListRooms(r.Context(), r.PathValue("workspaceID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"rooms": roomList}})
}

func (a *API) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if a.rooms == nil {
		a.writeDomainError(w, r, errors.New("room service is not configured"))
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input rooms.CreateRoomInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.rooms.CreateRoom(
		r.Context(), principal.ID, r.PathValue("workspaceID"),
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Location", "/v1/workspaces/"+r.PathValue("workspaceID")+"/rooms/"+result.Room.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": result.Room})
}

func (a *API) handleListRoomParticipants(w http.ResponseWriter, r *http.Request) {
	if a.rooms == nil {
		a.writeDomainError(w, r, errors.New("room service is not configured"))
		return
	}
	participants, err := a.rooms.ListParticipants(r.Context(), r.PathValue("workspaceID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"participants": participants}})
}

func (a *API) handleListRoomMessages(w http.ResponseWriter, r *http.Request) {
	if a.rooms == nil {
		a.writeDomainError(w, r, errors.New("room service is not configured"))
		return
	}
	limit, err := boundedQueryLimit(r, 40, 100)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", err.Error())
		return
	}
	messages, err := a.rooms.ListMessages(
		r.Context(), r.PathValue("workspaceID"), r.PathValue("roomID"),
		rooms.MessageListOptions{
			BeforeMessageID:     r.URL.Query().Get("before"),
			ThreadRootMessageID: r.URL.Query().Get("thread_root"),
			Query:               r.URL.Query().Get("q"), Limit: limit,
		},
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"messages": messages}})
}

func (a *API) handleCreateRoomMessage(w http.ResponseWriter, r *http.Request) {
	if a.rooms == nil {
		a.writeDomainError(w, r, errors.New("room service is not configured"))
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input rooms.CreateMessageInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.rooms.CreateMessage(
		r.Context(), principal.ID, principalCanManageAll(principal),
		r.PathValue("workspaceID"), r.PathValue("roomID"),
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": result.Message})
}

func (a *API) handleMyRoomMentions(w http.ResponseWriter, r *http.Request) {
	a.handleRoomMentions(w, r, "")
}

func (a *API) handleAgentRoomMentions(w http.ResponseWriter, r *http.Request) {
	a.handleRoomMentions(w, r, r.PathValue("sessionID"))
}

func (a *API) handleRoomMentions(w http.ResponseWriter, r *http.Request, sessionID string) {
	if a.rooms == nil {
		a.writeDomainError(w, r, errors.New("room service is not configured"))
		return
	}
	limit, err := boundedQueryLimit(r, 50, 100)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	mentions, err := a.rooms.ListInbox(
		r.Context(), principal.ID, principalCanManageAll(principal), sessionID,
		rooms.InboxOptions{
			WorkspaceID: r.URL.Query().Get("workspace_id"),
			Status:      r.URL.Query().Get("status"), Limit: limit,
		},
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"mentions": mentions}})
}

func (a *API) handleMyRoomMentionStatus(w http.ResponseWriter, r *http.Request) {
	a.handleRoomMentionStatus(w, r, "")
}

func (a *API) handleAgentRoomMentionStatus(w http.ResponseWriter, r *http.Request) {
	a.handleRoomMentionStatus(w, r, r.PathValue("sessionID"))
}

func (a *API) handleRoomMentionStatus(w http.ResponseWriter, r *http.Request, sessionID string) {
	if a.rooms == nil {
		a.writeDomainError(w, r, errors.New("room service is not configured"))
		return
	}
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input rooms.MentionStatusInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	mention, err := a.rooms.UpdateMention(
		r.Context(), principal.ID, principalCanManageAll(principal), sessionID,
		r.PathValue("mentionID"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": mention})
}

func boundedQueryLimit(r *http.Request, defaultValue, maximum int) (int, error) {
	limit := defaultValue
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maximum {
			return 0, fmt.Errorf("limit must be an integer between 1 and %d", maximum)
		}
		limit = value
	}
	return limit, nil
}

func knowledgeListOptions(r *http.Request, kindKey string) (knowledge.ListOptions, error) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 250 {
			return knowledge.ListOptions{}, errors.New("limit must be an integer between 1 and 250")
		}
		limit = value
	}
	return knowledge.ListOptions{
		Query:  r.URL.Query().Get("q"),
		Kind:   r.URL.Query().Get(kindKey),
		Status: r.URL.Query().Get("status"),
		Limit:  limit,
	}, nil
}

func filterVisibleWorkspaces(workspaceList []workspaces.Workspace, visible map[string]struct{}) []workspaces.Workspace {
	if visible == nil {
		return workspaceList
	}
	filtered := make([]workspaces.Workspace, 0, len(workspaceList))
	for _, workspace := range workspaceList {
		projects := make([]workspaces.Project, 0, len(workspace.Projects))
		for _, project := range workspace.Projects {
			if _, ok := visible[project.ID]; ok {
				projects = append(projects, project)
			}
		}
		if len(projects) == 0 {
			continue
		}
		workspace.Projects = projects
		filtered = append(filtered, workspace)
	}
	return filtered
}

func (a *API) handleGetProject(w http.ResponseWriter, r *http.Request) {
	project, err := a.projects.Get(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": project})
}

func (a *API) handleStartAgentSession(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.agentSessions == nil {
		a.writeDomainError(w, r, errors.New("agent session service is not configured"))
		return
	}
	var input agentsession.StartInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	session, err := a.agentSessions.Start(r.Context(), principal.ID, r.PathValue("projectID"), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/agent-sessions/"+session.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": session})
}

func (a *API) handleScopeCheck(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.coordination == nil {
		a.writeDomainError(w, r, errors.New("coordination service is not configured"))
		return
	}
	var input struct {
		Scopes []coordination.ScopeInput `json:"scopes"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	result, err := a.coordination.CheckScopes(r.Context(), r.PathValue("projectID"), input.Scopes)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) handleListWork(w http.ResponseWriter, r *http.Request) {
	if a.coordination == nil {
		a.writeDomainError(w, r, errors.New("coordination service is not configured"))
		return
	}
	items, err := a.coordination.List(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"work_items": items}})
}

func (a *API) handleStartWork(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.coordination == nil {
		a.writeDomainError(w, r, errors.New("coordination service is not configured"))
		return
	}
	var input coordination.StartInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.coordination.Start(
		r.Context(), principal.ID, principalCanManageAll(principal),
		r.PathValue("projectID"), r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Location", "/v1/intents/"+result.Intent.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

func (a *API) handleListHandoffs(w http.ResponseWriter, r *http.Request) {
	if a.handoffs == nil {
		a.writeDomainError(w, r, errors.New("coordination service is not configured"))
		return
	}
	handoffs, err := a.handoffs.ListHandoffs(
		r.Context(), r.PathValue("projectID"), strings.TrimSpace(r.URL.Query().Get("intent_id")),
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"handoffs": handoffs}})
}

func (a *API) handleOfferHandoff(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.handoffs == nil {
		a.writeDomainError(w, r, errors.New("coordination service is not configured"))
		return
	}
	var input coordination.OfferHandoffInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.handoffs.OfferHandoff(
		r.Context(), principal.ID, principalCanManageAll(principal),
		r.PathValue("projectID"), r.PathValue("intentID"), r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Location", "/v1/projects/"+r.PathValue("projectID")+"/handoffs?intent_id="+r.PathValue("intentID"))
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

func (a *API) handleUpdateHandoffStatus(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.handoffs == nil {
		a.writeDomainError(w, r, errors.New("coordination service is not configured"))
		return
	}
	var input coordination.HandoffStatusInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.handoffs.UpdateHandoffStatus(
		r.Context(), principal.ID, principalCanManageAll(principal),
		r.PathValue("projectID"), r.PathValue("intentID"), r.PathValue("handoffID"),
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) handleCompileContextPack(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.contextPacks == nil {
		a.writeDomainError(w, r, errors.New("context pack service is not configured"))
		return
	}
	var input contextpack.CompileInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.contextPacks.Compile(
		r.Context(), principal.ID, principalCanManageAll(principal),
		r.PathValue("projectID"), r.PathValue("intentID"), r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Location", "/v1/projects/"+r.PathValue("projectID")+"/context-packs/"+result.Pack.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

func (a *API) handleGetContextPack(w http.ResponseWriter, r *http.Request) {
	if a.contextPacks == nil {
		a.writeDomainError(w, r, errors.New("context pack service is not configured"))
		return
	}
	pack, err := a.contextPacks.Get(r.Context(), r.PathValue("projectID"), r.PathValue("contextPackID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": pack})
}

func (a *API) handleAttachWorkspace(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", "</v1/intents/"+r.PathValue("intentID")+"/worktree>; rel=\"successor-version\"")
	a.attachExecutionWorktree(w, r, true)
}

func (a *API) handleAttachWorktree(w http.ResponseWriter, r *http.Request) {
	a.attachExecutionWorktree(w, r, false)
}

func (a *API) attachExecutionWorktree(w http.ResponseWriter, r *http.Request, legacy bool) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.coordination == nil {
		a.writeDomainError(w, r, errors.New("coordination service is not configured"))
		return
	}
	var input coordination.WorkspaceInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.coordination.AttachWorkspace(
		r.Context(), principal.ID, principalCanManageAll(principal),
		r.PathValue("intentID"), r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	if legacy {
		w.Header().Set("Location", "/v1/intents/"+r.PathValue("intentID")+"/workspace")
		writeJSON(w, http.StatusCreated, map[string]any{"data": result})
		return
	}
	w.Header().Set("Location", "/v1/intents/"+r.PathValue("intentID")+"/worktree")
	writeJSON(w, http.StatusCreated, map[string]any{"data": coordination.WorktreeResult{
		Worktree: result.Workspace, EventID: result.EventID, Replayed: result.Replayed,
	}})
}

func (a *API) handleUpdateIntentStatus(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.coordination == nil {
		a.writeDomainError(w, r, errors.New("coordination service is not configured"))
		return
	}
	var input coordination.StatusInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.coordination.UpdateStatus(
		r.Context(), principal.ID, principalCanManageAll(principal),
		r.PathValue("intentID"), r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func principalCanManageAll(principal access.Principal) bool {
	return principal.OrganizationRole == "owner" || principal.OrganizationRole == "admin"
}

func (a *API) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if a.agentSessions == nil {
		a.writeDomainError(w, r, errors.New("agent session service is not configured"))
		return
	}
	principal, _ := principalFromContext(r.Context())
	allowAll := principalCanManageAll(principal)
	session, err := a.agentSessions.Heartbeat(r.Context(), principal.ID, allowAll, r.PathValue("sessionID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": session})
}

func (a *API) handleRepositoryObservation(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.agentSessions == nil {
		a.writeDomainError(w, r, errors.New("agent session service is not configured"))
		return
	}
	var input agentsession.ObservationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.agentSessions.Observe(
		r.Context(), principal.ID, r.PathValue("sessionID"),
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) handleCloseAgentSession(w http.ResponseWriter, r *http.Request) {
	if a.agentSessions == nil {
		a.writeDomainError(w, r, errors.New("agent session service is not configured"))
		return
	}
	principal, _ := principalFromContext(r.Context())
	allowAll := principalCanManageAll(principal)
	if err := a.agentSessions.Close(r.Context(), principal.ID, allowAll, r.PathValue("sessionID")); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input struct {
		Email          string `json:"email"`
		Role           string `json:"role"`
		ExpiresInHours int    `json:"expires_in_hours"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	expiresAfter := time.Duration(input.ExpiresInHours) * time.Hour
	if input.ExpiresInHours == 0 {
		expiresAfter = 24 * time.Hour
	}
	principal, _ := principalFromContext(r.Context())
	created, err := a.access.CreateInvitation(r.Context(), principal, r.PathValue("projectID"), access.CreateInvitationInput{
		Email: input.Email, Role: input.Role, ExpiresAfter: expiresAfter,
	})
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", "/v1/invitations/"+created.Invitation.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (a *API) handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	if err := a.access.RevokeInvitation(r.Context(), principal, r.PathValue("invitationID")); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": principal})
}

func (a *API) handleProjectAccess(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	roster, err := a.access.GetProjectAccess(r.Context(), principal, r.PathValue("projectID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"data": roster})
}

func (a *API) handleGetRepositorySync(w http.ResponseWriter, r *http.Request) {
	if a.repositorySync == nil {
		a.writeDomainError(w, r, errors.New("repository sync service is not configured"))
		return
	}
	state, err := a.repositorySync.Get(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": state})
}

func (a *API) handleSyncRepository(w http.ResponseWriter, r *http.Request) {
	if a.repositorySync == nil {
		a.writeDomainError(w, r, errors.New("repository sync service is not configured"))
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.repositorySync.Sync(
		r.Context(), principal.ID, r.PathValue("projectID"), r.Header.Get("Idempotency-Key"),
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) handleListProjectRepositories(w http.ResponseWriter, r *http.Request) {
	if a.projectRepositories == nil || a.repositorySync == nil {
		a.writeDomainError(w, r, errors.New("project repository services are not configured"))
		return
	}
	repositories, err := a.projectRepositories.List(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	states, err := a.repositorySync.List(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"repositories": repositories, "sync_states": states,
	}})
}

func (a *API) handleAttachProjectRepository(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	if a.projectRepositories == nil {
		a.writeDomainError(w, r, errors.New("project repository service is not configured"))
		return
	}
	var input projectrepo.AttachInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	principal, _ := principalFromContext(r.Context())
	repository, err := a.projectRepositories.Attach(r.Context(), principal.ID, r.PathValue("projectID"), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/projects/"+r.PathValue("projectID")+"/repositories/"+repository.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": repository})
}

func (a *API) handleGetProjectRepositorySync(w http.ResponseWriter, r *http.Request) {
	if a.repositorySync == nil {
		a.writeDomainError(w, r, errors.New("repository sync service is not configured"))
		return
	}
	state, err := a.repositorySync.GetRepository(
		r.Context(), r.PathValue("projectID"), r.PathValue("repositoryID"),
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": state})
}

func (a *API) handleSyncProjectRepository(w http.ResponseWriter, r *http.Request) {
	if a.repositorySync == nil {
		a.writeDomainError(w, r, errors.New("repository sync service is not configured"))
		return
	}
	principal, _ := principalFromContext(r.Context())
	result, err := a.repositorySync.SyncRepository(
		r.Context(), principal.ID, r.PathValue("projectID"), r.PathValue("repositoryID"),
		r.Header.Get("Idempotency-Key"),
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) handleGitHubStatus(w http.ResponseWriter, r *http.Request) {
	if a.githubApp == nil {
		a.writeDomainError(w, r, errors.New("GitHub App service is not configured"))
		return
	}
	status, err := a.githubApp.Status(r.Context())
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": status})
}

func (a *API) handleConnectGitHub(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || !principalCanManageAll(principal) {
		a.writeDomainError(w, r, access.ErrForbidden)
		return
	}
	if a.githubApp == nil {
		a.writeDomainError(w, r, errors.New("GitHub App service is not configured"))
		return
	}
	connection, err := a.githubApp.Connect(r.Context(), principal.ID)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": connection})
}

func (a *API) handleListAuthorizedGitHubRepositories(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if projectID == "" {
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", "project_id is required.")
		return
	}
	principal, _ := principalFromContext(r.Context())
	if err := a.access.RequireProjectRole(r.Context(), principal, projectID, "viewer"); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if a.projectRepositories == nil {
		a.writeDomainError(w, r, errors.New("project repository service is not configured"))
		return
	}
	repositories, err := a.projectRepositories.ListAvailable(r.Context(), projectID)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": repositories})
}

func (a *API) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if a.githubApp == nil {
		a.redirectGitHubResult(w, r, "error", "not_configured")
		return
	}
	state := r.URL.Query().Get("state")
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	installationRaw := strings.TrimSpace(r.URL.Query().Get("installation_id"))
	if installationRaw != "" {
		installationID, err := strconv.ParseInt(installationRaw, 10, 64)
		if err != nil || installationID <= 0 {
			a.redirectGitHubResult(w, r, "error", "invalid_callback")
			return
		}
		authorizationURL, err := a.githubApp.BeginUserAuthorization(r.Context(), state, installationID)
		if err != nil {
			a.redirectGitHubResult(w, r, "error", "connection_failed")
			return
		}
		if code == "" {
			http.Redirect(w, r, authorizationURL, http.StatusSeeOther)
			return
		}
	}
	if code == "" {
		a.redirectGitHubResult(w, r, "error", "invalid_callback")
		return
	}
	err := a.githubApp.CompleteConnection(r.Context(), state, code)
	if err != nil {
		a.logger.Warn("GitHub App connection failed", "error", err)
		a.redirectGitHubResult(w, r, "error", "connection_failed")
		return
	}
	a.redirectGitHubResult(w, r, "connected", "")
}

func (a *API) redirectGitHubResult(w http.ResponseWriter, r *http.Request, result, reason string) {
	query := url.Values{"github": []string{result}}
	if reason != "" {
		query.Set("reason", reason)
	}
	http.Redirect(w, r, "/admin/?"+query.Encode(), http.StatusSeeOther)
}

func (a *API) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if a.githubApp == nil {
		a.writeDomainError(w, r, githubapp.ErrNotConfigured)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_webhook", "Invalid webhook", "The webhook body could not be read.")
		return
	}
	if len(body) > maxRequestBody {
		writeProblem(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "Request too large", "The webhook body exceeds the maximum size.")
		return
	}
	err = a.githubApp.HandleWebhook(
		r.Context(), r.Header.Get("X-GitHub-Delivery"), r.Header.Get("X-GitHub-Event"),
		r.Header.Get("X-Hub-Signature-256"), body,
	)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleProjectOverview(w http.ResponseWriter, r *http.Request) {
	project, err := a.projects.Get(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if a.backoffice == nil {
		a.writeDomainError(w, r, errors.New("backoffice reader is not configured"))
		return
	}

	overview, err := a.backoffice.Get(r.Context(), a.organizationID, project.ID)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	if a.coordination != nil {
		overview.WorkItems, err = a.coordination.List(r.Context(), project.ID)
		if err != nil {
			a.writeDomainError(w, r, err)
			return
		}
	}
	if a.handoffs != nil {
		overview.Handoffs, err = a.handoffs.ListHandoffs(r.Context(), project.ID, "")
		if err != nil {
			a.writeDomainError(w, r, err)
			return
		}
	}
	var repositorySync *repositorysync.State
	if a.repositorySync != nil {
		state, syncErr := a.repositorySync.Get(r.Context(), project.ID)
		if syncErr != nil {
			a.writeDomainError(w, r, syncErr)
			return
		}
		repositorySync = &state
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"project":         project,
		"repository_sync": repositorySync,
		"code_activity":   overview.CodeActivity,
		"counts":          overview.Counts,
		"active_work":     overview.ActiveWork,
		"recent_events":   overview.RecentEvents,
		"work_items":      overview.WorkItems,
		"handoffs":        overview.Handoffs,
		"generated_at":    overview.GeneratedAt,
	}})
}

func (a *API) handleListEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, err := a.projects.Get(r.Context(), projectID); err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	order := strings.TrimSpace(r.URL.Query().Get("order"))
	if order != "" && order != "asc" && order != "desc" {
		writeProblem(w, r, http.StatusBadRequest, "invalid_order", "Invalid event order", "order must be asc or desc")
		return
	}
	if order == "desc" {
		before, err := optionalEventCursor(r.URL.Query().Get("before"))
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "invalid_cursor", "Invalid event cursor", err.Error())
			return
		}
		_, limit, err := eventPage(r)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "invalid_cursor", "Invalid event cursor", err.Error())
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(query) > 200 {
			writeProblem(w, r, http.StatusBadRequest, "invalid_query", "Invalid event search", "q must not exceed 200 characters")
			return
		}
		history, ok := a.events.(eventlog.HistoryReader)
		if !ok {
			a.writeDomainError(w, r, errors.New("reverse event history is not configured"))
			return
		}
		events, err := history.ListRecent(r.Context(), a.organizationID, projectID, before, limit+1, query)
		if err != nil {
			a.writeDomainError(w, r, err)
			return
		}
		writeEventPage(w, events, limit)
		return
	}

	after, limit, err := eventPage(r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_cursor", "Invalid event cursor", err.Error())
		return
	}

	events, err := a.events.List(r.Context(), a.organizationID, projectID, after, limit+1)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	writeEventPage(w, events, limit)
}

func writeEventPage(w http.ResponseWriter, events []eventlog.Event, limit int) {
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	var nextCursor *string
	if len(events) > 0 {
		cursor := strconv.FormatInt(events[len(events)-1].ProjectSequence, 10)
		nextCursor = &cursor
	}

	responses := make([]eventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, newEventResponse(event))
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"events":      responses,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	}})
}

type eventResponse struct {
	ID               string          `json:"id"`
	ProjectID        string          `json:"project_id"`
	Sequence         string          `json:"sequence"`
	Type             string          `json:"type"`
	Version          int16           `json:"version"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      string          `json:"aggregate_id"`
	AggregateVersion int64           `json:"aggregate_version"`
	CommandID        string          `json:"command_id"`
	CorrelationID    string          `json:"correlation_id"`
	ActorID          *string         `json:"actor_id,omitempty"`
	SessionID        *string         `json:"session_id,omitempty"`
	IntentID         *string         `json:"intent_id,omitempty"`
	CausationID      *string         `json:"causation_id,omitempty"`
	OccurredAt       time.Time       `json:"occurred_at"`
	RecordedAt       time.Time       `json:"recorded_at"`
	Data             json.RawMessage `json:"data"`
}

func newEventResponse(event eventlog.Event) eventResponse {
	return eventResponse{
		ID:               event.ID,
		ProjectID:        event.ProjectID,
		Sequence:         strconv.FormatInt(event.ProjectSequence, 10),
		Type:             event.Type,
		Version:          event.Version,
		AggregateType:    event.AggregateType,
		AggregateID:      event.AggregateID,
		AggregateVersion: event.AggregateVersion,
		CommandID:        event.CommandID,
		CorrelationID:    event.CorrelationID,
		ActorID:          event.ActorID,
		SessionID:        event.SessionID,
		IntentID:         event.IntentID,
		CausationID:      event.CausationID,
		OccurredAt:       event.OccurredAt,
		RecordedAt:       event.RecordedAt,
		Data:             event.Payload,
	}
}

func (a *API) revalidateStreamAuthorization(
	ctx context.Context,
	authentication requestAuthentication,
	projectID string,
) error {
	if a.authentication == nil || a.access == nil || strings.TrimSpace(authentication.Credential) == "" {
		return authn.ErrUnauthorized
	}

	var principal access.Principal
	switch authentication.Kind {
	case "web":
		if authentication.Web == nil {
			return authn.ErrUnauthorized
		}
		session, err := a.authentication.AuthenticateWeb(ctx, authentication.Credential)
		if err != nil {
			return err
		}
		if session.ID != authentication.Web.ID ||
			session.Principal.ID != authentication.Web.Principal.ID ||
			session.Principal.OrganizationID != authentication.Web.Principal.OrganizationID {
			return authn.ErrUnauthorized
		}
		principal = session.Principal
	case "device":
		if authentication.Device == nil {
			return authn.ErrUnauthorized
		}
		device, err := a.authentication.AuthenticateDevice(ctx, authentication.Credential)
		if err != nil {
			return err
		}
		if device.CredentialID != authentication.Device.CredentialID ||
			device.Principal.ID != authentication.Device.Principal.ID ||
			device.Principal.OrganizationID != authentication.Device.Principal.OrganizationID {
			return authn.ErrUnauthorized
		}
		principal = device.Principal
	default:
		return authn.ErrUnauthorized
	}

	return a.access.RequireProjectRole(ctx, principal, projectID, "viewer")
}

func (a *API) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	authentication, ok := authenticationFromContext(r.Context())
	if !ok {
		a.writeDomainError(w, r, authn.ErrUnauthorized)
		return
	}
	if _, err := a.projects.Get(r.Context(), projectID); err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	after, err := parseCursor(r.Header.Get("Last-Event-ID"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_cursor", "Invalid event cursor", err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, r, http.StatusInternalServerError, "stream_unsupported", "Streaming unavailable", "The HTTP connection does not support streaming.")
		return
	}
	controller := http.NewResponseController(w)
	writeDeadlineSupported := true
	refreshWriteDeadline := func() bool {
		if !writeDeadlineSupported {
			return true
		}
		err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if errors.Is(err, http.ErrNotSupported) {
			writeDeadlineSupported = false
			return true
		}
		if err != nil {
			a.logger.WarnContext(r.Context(), "could not set stream write deadline", "error", err)
			return false
		}
		return true
	}
	if !refreshWriteDeadline() {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, ": pact event stream\n\n")
	flusher.Flush()

	poll := time.NewTicker(a.streamPollInterval)
	defer poll.Stop()
	heartbeat := time.NewTicker(a.streamHeartbeatEvery)
	defer heartbeat.Stop()
	authorizationTicks := a.streamAuthorizationTicks
	var authorization *time.Ticker
	if authorizationTicks == nil {
		authorization = time.NewTicker(a.streamAuthorizationEvery)
		authorizationTicks = authorization.C
		defer authorization.Stop()
	}

	revalidateAuthorization := func() bool {
		if authErr := a.revalidateStreamAuthorization(r.Context(), authentication, projectID); authErr != nil {
			a.logger.InfoContext(
				r.Context(),
				"event stream authorization is no longer valid",
				"authentication_kind", authentication.Kind,
				"project_id", projectID,
				"error", authErr,
			)
			return false
		}
		return true
	}
	checkAuthorization := func() bool {
		select {
		case <-authorizationTicks:
			return revalidateAuthorization()
		default:
			return true
		}
	}

	for {
		select {
		case <-a.streamShutdown:
			return
		default:
		}
		if !checkAuthorization() {
			return
		}

		events, listErr := a.events.List(r.Context(), a.organizationID, projectID, after, 100)
		if listErr != nil {
			a.logger.ErrorContext(r.Context(), "event stream query failed", "error", listErr, "project_id", projectID)
			return
		}

		for _, event := range events {
			if !checkAuthorization() {
				return
			}
			if !refreshWriteDeadline() {
				return
			}
			body, marshalErr := json.Marshal(newEventResponse(event))
			if marshalErr != nil {
				a.logger.ErrorContext(r.Context(), "event stream encoding failed", "error", marshalErr, "event_id", event.ID)
				return
			}
			if _, writeErr := fmt.Fprintf(
				w,
				"id: %d\nevent: %s\ndata: %s\n\n",
				event.ProjectSequence,
				sseEventName(event.Type),
				body,
			); writeErr != nil {
				return
			}
			after = event.ProjectSequence
		}
		if len(events) > 0 {
			flusher.Flush()
			continue
		}

		select {
		case <-r.Context().Done():
			return
		case <-a.streamShutdown:
			return
		case <-authorizationTicks:
			if !revalidateAuthorization() {
				return
			}
		case <-poll.C:
		case <-heartbeat.C:
			if !refreshWriteDeadline() {
				return
			}
			if _, writeErr := io.WriteString(w, ": keepalive\n\n"); writeErr != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (a *API) methodNotAllowed(allowed string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allowed)
		writeProblem(
			w,
			r,
			http.StatusMethodNotAllowed,
			"method_not_allowed",
			"Method not allowed",
			"The requested resource does not support this HTTP method.",
		)
	})
}

func (a *API) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeProblem(
		w,
		r,
		http.StatusNotFound,
		"route_not_found",
		"Route not found",
		"The requested API route does not exist.",
	)
}

func sseEventName(value string) string {
	if value == "" {
		return "pact.event"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' ||
			char == '_' ||
			char == '-' {
			continue
		}
		return "pact.event"
	}
	return value
}

func (a *API) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *projects.ValidationError
	var authenticationValidationErr *authn.ValidationError
	var agentValidationErr *agentsession.ValidationError
	var accessValidationErr *access.ValidationError
	var coordinationValidationErr *coordination.ValidationError
	var workspaceValidationErr *workspaces.ValidationError
	var knowledgeValidationErr *knowledge.ValidationError
	var roomValidationErr *rooms.ValidationError
	var contextValidationErr *contextpack.ValidationError
	var repositorySyncValidationErr *repositorysync.ValidationError
	var providerErr *repositorysync.ProviderError
	var projectRepositoryValidationErr *projectrepo.ValidationError
	var githubProviderErr *githubapp.ProviderError
	var userAdminValidationErr *useradmin.ValidationError
	var scopeConflictErr *coordination.ScopeConflictError
	switch {
	case errors.As(err, &validationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", validationErr.Error())
	case errors.As(err, &authenticationValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", authenticationValidationErr.Error())
	case errors.As(err, &agentValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", agentValidationErr.Error())
	case errors.As(err, &accessValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", accessValidationErr.Error())
	case errors.As(err, &coordinationValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", coordinationValidationErr.Error())
	case errors.As(err, &workspaceValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", workspaceValidationErr.Error())
	case errors.As(err, &knowledgeValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", knowledgeValidationErr.Error())
	case errors.As(err, &roomValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", roomValidationErr.Error())
	case errors.As(err, &contextValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", contextValidationErr.Error())
	case errors.As(err, &repositorySyncValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", repositorySyncValidationErr.Error())
	case errors.As(err, &projectRepositoryValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", projectRepositoryValidationErr.Error())
	case errors.As(err, &userAdminValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", userAdminValidationErr.Error())
	case errors.As(err, &githubProviderErr):
		writeProblem(w, r, http.StatusBadGateway, githubProviderErr.Code, "GitHub integration failed", "Pact could not complete the operation with GitHub.")
	case errors.As(err, &providerErr):
		if providerErr.RetryAfter != "" {
			w.Header().Set("Retry-After", providerErr.RetryAfter)
		}
		writeProblem(w, r, http.StatusFailedDependency, providerErr.Code, "GitHub synchronization failed", "Pact could not read the canonical repository state from GitHub.")
	case errors.As(err, &scopeConflictErr):
		writeScopeConflict(w, r, scopeConflictErr)
	case errors.Is(err, authn.ErrUnauthorized), errors.Is(err, authn.ErrInvalidCredentials):
		writeProblem(w, r, http.StatusUnauthorized, "invalid_credentials", "Authentication failed", "The username, email, password, or session is invalid.")
	case errors.Is(err, authn.ErrSetupUnavailable):
		writeProblem(w, r, http.StatusForbidden, "setup_unavailable", "Initial setup unavailable", err.Error())
	case errors.Is(err, authn.ErrAlreadyConfigured):
		writeProblem(w, r, http.StatusConflict, "setup_complete", "Initial setup complete", err.Error())
	case errors.Is(err, authn.ErrAccountExists):
		writeProblem(w, r, http.StatusConflict, "account_exists", "Account already exists", err.Error())
	case errors.Is(err, authn.ErrInvitationInvalid):
		writeProblem(w, r, http.StatusUnauthorized, "invitation_invalid", "Invalid invitation", err.Error())
	case errors.Is(err, authn.ErrInvitationMismatch):
		writeProblem(w, r, http.StatusForbidden, "invitation_mismatch", "Invitation mismatch", err.Error())
	case errors.Is(err, authn.ErrDeviceCodeInvalid):
		writeProblem(w, r, http.StatusBadRequest, "device_authorization_invalid", "Invalid device authorization", err.Error())
	case errors.Is(err, authn.ErrAuthorizationDenied):
		writeProblem(w, r, http.StatusForbidden, "device_authorization_denied", "Device authorization denied", err.Error())
	case errors.Is(err, authn.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "authentication_resource_not_found", "Authentication resource not found", err.Error())
	case errors.Is(err, access.ErrUnauthorized):
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "Unauthorized", "Authentication is required.")
	case errors.Is(err, access.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "forbidden", "Forbidden", "The current identity does not have permission for this operation.")
	case errors.Is(err, coordination.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "coordination_forbidden", "Forbidden", err.Error())
	case errors.Is(err, coordination.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "coordinated_work_not_found", "Coordinated work not found", err.Error())
	case errors.Is(err, coordination.ErrRepositoryUnavailable):
		writeProblem(w, r, http.StatusConflict, "project_repository_unavailable", "Project repository unavailable", err.Error())
	case errors.Is(err, coordination.ErrVersionConflict):
		writeProblem(w, r, http.StatusConflict, "intent_version_conflict", "Intent changed", err.Error())
	case errors.Is(err, coordination.ErrInvalidTransition):
		writeProblem(w, r, http.StatusConflict, "invalid_intent_transition", "Invalid intent transition", err.Error())
	case errors.Is(err, coordination.ErrWorkspaceExists):
		writeProblem(w, r, http.StatusConflict, "workspace_exists", "Workspace already exists", err.Error())
	case errors.Is(err, coordination.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, coordination.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	case errors.Is(err, coordination.ErrHandoffExists):
		writeProblem(w, r, http.StatusConflict, "handoff_exists", "Handoff already offered", err.Error())
	case errors.Is(err, coordination.ErrInvalidHandoffStatus):
		writeProblem(w, r, http.StatusConflict, "invalid_handoff_transition", "Invalid handoff transition", err.Error())
	case errors.Is(err, coordination.ErrKnowledgeRecordNotFound):
		writeProblem(w, r, http.StatusNotFound, "handoff_record_not_found", "Knowledge record not found", err.Error())
	case errors.Is(err, access.ErrInvitationInvalid):
		writeProblem(w, r, http.StatusUnauthorized, "invitation_invalid", "Invalid invitation", err.Error())
	case errors.Is(err, access.ErrInvitationExists):
		writeProblem(w, r, http.StatusConflict, "invitation_exists", "Invitation already exists", err.Error())
	case errors.Is(err, access.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "access_resource_not_found", "Access resource not found", err.Error())
	case errors.Is(err, useradmin.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "user_administration_forbidden", "Forbidden", "The current account cannot administer this user or role.")
	case errors.Is(err, useradmin.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "user_administration_resource_not_found", "User administration resource not found", err.Error())
	case errors.Is(err, useradmin.ErrAccountExists):
		writeProblem(w, r, http.StatusConflict, "user_account_exists", "User account already exists", err.Error())
	case errors.Is(err, useradmin.ErrInvitationExists):
		writeProblem(w, r, http.StatusConflict, "user_invitation_exists", "Pending invitation already exists", err.Error())
	case errors.Is(err, useradmin.ErrLastOwner):
		writeProblem(w, r, http.StatusConflict, "last_owner_required", "An active owner is required", err.Error())
	case errors.Is(err, useradmin.ErrSelfManagement):
		writeProblem(w, r, http.StatusConflict, "self_administration_restricted", "Operation not allowed on the current account", err.Error())
	case errors.Is(err, useradmin.ErrGlobalProjectRole):
		writeProblem(w, r, http.StatusConflict, "global_project_access", "Global project access already applies", err.Error())
	case errors.Is(err, useradmin.ErrInactiveUser):
		writeProblem(w, r, http.StatusConflict, "inactive_user", "User is disabled", err.Error())
	case errors.Is(err, agentsession.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "agent_session_not_found", "Agent session not found", err.Error())
	case errors.Is(err, agentsession.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, agentsession.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	case errors.Is(err, projects.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "project_not_found", "Project not found", "The requested project does not exist.")
	case errors.Is(err, projects.ErrSlugTaken):
		writeProblem(w, r, http.StatusConflict, "project_slug_taken", "Project already exists", err.Error())
	case errors.Is(err, projects.ErrRepositoryTaken):
		writeProblem(w, r, http.StatusConflict, "repository_already_connected", "Repository already connected", err.Error())
	case errors.Is(err, projects.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, projects.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	case errors.Is(err, repositorysync.ErrRepositoryUnavailable):
		writeProblem(w, r, http.StatusConflict, "project_repository_unavailable", "Project repository unavailable", err.Error())
	case errors.Is(err, repositorysync.ErrUnsupportedRemote):
		writeProblem(w, r, http.StatusUnprocessableEntity, "repository_provider_unsupported", "Repository provider unsupported", err.Error())
	case errors.Is(err, repositorysync.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, repositorysync.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	case errors.Is(err, projectrepo.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "project_repository_not_found", "Project repository not found", err.Error())
	case errors.Is(err, projectrepo.ErrProviderNotFound):
		writeProblem(w, r, http.StatusNotFound, "github_repository_not_found", "GitHub repository not found", err.Error())
	case errors.Is(err, projectrepo.ErrAlreadyAttached):
		writeProblem(w, r, http.StatusConflict, "repository_already_attached", "Repository already attached", err.Error())
	case errors.Is(err, githubapp.ErrNotConfigured):
		writeProblem(w, r, http.StatusServiceUnavailable, "github_app_not_configured", "GitHub App unavailable", err.Error())
	case errors.Is(err, githubapp.ErrInvalidState):
		writeProblem(w, r, http.StatusBadRequest, "github_connection_invalid", "Invalid GitHub connection", err.Error())
	case errors.Is(err, githubapp.ErrInstallationDenied):
		writeProblem(w, r, http.StatusForbidden, "github_installation_denied", "GitHub installation denied", err.Error())
	case errors.Is(err, githubapp.ErrWebhookSignature):
		writeProblem(w, r, http.StatusUnauthorized, "github_webhook_signature_invalid", "Invalid webhook signature", err.Error())
	case errors.Is(err, workspaces.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "workspace_not_found", "Workspace not found", err.Error())
	case errors.Is(err, workspaces.ErrProjectNotFound):
		writeProblem(w, r, http.StatusNotFound, "workspace_project_not_found", "Project not found", err.Error())
	case errors.Is(err, workspaces.ErrSlugTaken):
		writeProblem(w, r, http.StatusConflict, "workspace_slug_taken", "Workspace already exists", err.Error())
	case errors.Is(err, workspaces.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, workspaces.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	case errors.Is(err, knowledge.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "knowledge_record_not_found", "Knowledge record not found", err.Error())
	case errors.Is(err, knowledge.ErrResourceNotFound):
		writeProblem(w, r, http.StatusNotFound, "knowledge_resource_not_found", "Knowledge resource not found", err.Error())
	case errors.Is(err, knowledge.ErrResourceExists):
		writeProblem(w, r, http.StatusConflict, "knowledge_resource_exists", "Knowledge resource already exists", err.Error())
	case errors.Is(err, knowledge.ErrVersionConflict):
		writeProblem(w, r, http.StatusConflict, "knowledge_version_conflict", "Knowledge record changed", err.Error())
	case errors.Is(err, knowledge.ErrInvalidTransition):
		writeProblem(w, r, http.StatusConflict, "knowledge_invalid_transition", "Invalid knowledge transition", err.Error())
	case errors.Is(err, knowledge.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, knowledge.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	case errors.Is(err, rooms.ErrWorkspaceNotFound):
		writeProblem(w, r, http.StatusNotFound, "workspace_not_found", "Workspace not found", err.Error())
	case errors.Is(err, rooms.ErrRoomNotFound):
		writeProblem(w, r, http.StatusNotFound, "room_not_found", "Room not found", err.Error())
	case errors.Is(err, rooms.ErrMessageNotFound):
		writeProblem(w, r, http.StatusNotFound, "room_message_not_found", "Message not found", err.Error())
	case errors.Is(err, rooms.ErrMentionNotFound):
		writeProblem(w, r, http.StatusNotFound, "room_mention_not_found", "Mention not found", err.Error())
	case errors.Is(err, rooms.ErrParticipantNotFound):
		writeProblem(w, r, http.StatusUnprocessableEntity, "room_participant_unavailable", "Mention target unavailable", err.Error())
	case errors.Is(err, rooms.ErrSlugTaken):
		writeProblem(w, r, http.StatusConflict, "room_slug_taken", "Room already exists", err.Error())
	case errors.Is(err, rooms.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "room_author_forbidden", "Forbidden", err.Error())
	case errors.Is(err, rooms.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, rooms.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	case errors.Is(err, contextpack.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "context_pack_forbidden", "Forbidden", err.Error())
	case errors.Is(err, contextpack.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "context_pack_not_found", "Context pack not found", err.Error())
	case errors.Is(err, contextpack.ErrIntegrity):
		writeProblem(w, r, http.StatusConflict, "context_pack_integrity_failed", "Context pack integrity failed", err.Error())
	case errors.Is(err, contextpack.ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", err.Error())
	case errors.Is(err, contextpack.ErrCommandIncomplete):
		writeProblem(w, r, http.StatusConflict, "command_incomplete", "Command result unavailable", err.Error())
	default:
		a.logger.ErrorContext(r.Context(), "request failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal server error", "The request could not be completed.")
	}
}

func writeScopeConflict(w http.ResponseWriter, r *http.Request, conflict *coordination.ScopeConflictError) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":  "https://the-pact.dev/problems/scope_conflict",
		"title": "Scope already reserved", "status": http.StatusConflict,
		"detail": conflict.Error(), "instance": r.URL.Path,
		"code": "scope_conflict", "request_id": requestIDFromContext(r.Context()),
		"overlaps": conflict.Overlaps,
	})
}

func eventPage(r *http.Request) (int64, int, error) {
	after, err := parseCursor(r.URL.Query().Get("after"))
	if err != nil {
		return 0, 0, err
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 || value > 200 {
			return 0, 0, errors.New("limit must be an integer between 1 and 200")
		}
		limit = value
	}
	return after, limit, nil
}

func parseCursor(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cursor < 0 {
		return 0, errors.New("after and Last-Event-ID must be non-negative event cursors")
	}
	return cursor, nil
}

func optionalEventCursor(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	cursor, err := parseCursor(raw)
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}

func hasJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "https://the-pact.dev/problems/" + code,
		"title":      title,
		"status":     status,
		"detail":     detail,
		"instance":   r.URL.Path,
		"code":       code,
		"request_id": requestIDFromContext(r.Context()),
	})
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
