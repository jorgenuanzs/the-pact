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
	"strconv"
	"strings"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/contextpack"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/repositorysync"
	"github.com/jorgenuanzs/the-pact/internal/transport/httpapi/adminui"
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
	Sync(context.Context, string, string, string) (repositorysync.Result, error)
}

type WorkspaceService interface {
	Create(context.Context, string, workspaces.CreateInput) (workspaces.CreateResult, error)
	Get(context.Context, string) (workspaces.Workspace, error)
	List(context.Context) ([]workspaces.Workspace, error)
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
	Authenticate(context.Context, string) (access.Principal, error)
	RequireProjectRole(context.Context, access.Principal, string, string) error
	VisibleProjectIDs(context.Context, access.Principal) (map[string]struct{}, error)
	CanCreateProject(access.Principal) bool
	CreateInvitation(context.Context, access.Principal, string, access.CreateInvitationInput) (access.CreatedInvitation, error)
	AcceptInvitation(context.Context, access.AcceptInvitationInput) (access.AcceptedInvitation, error)
	RevokeInvitation(context.Context, access.Principal, string) error
	RevokeCurrentToken(context.Context, access.Principal) error
	GrantProjectOwner(context.Context, access.Principal, string) error
}

type ReadinessCheck func(context.Context) error

type Config struct {
	Logger                *slog.Logger
	OrganizationID        string
	Build                 buildinfo.Info
	Readiness             ReadinessCheck
	ProjectService        ProjectService
	RepositorySyncService RepositorySyncService
	WorkspaceService      WorkspaceService
	KnowledgeService      KnowledgeService
	AgentSessionService   AgentSessionService
	CoordinationService   CoordinationService
	HandoffService        HandoffService
	ContextPackService    ContextPackService
	AccessService         AccessService
	BackofficeReader      backoffice.Reader
	EventReader           eventlog.Reader
	StreamShutdown        <-chan struct{}
	StreamPollInterval    time.Duration
	StreamHeartbeatEvery  time.Duration
}

type API struct {
	logger               *slog.Logger
	organizationID       string
	build                buildinfo.Info
	readiness            ReadinessCheck
	projects             ProjectService
	repositorySync       RepositorySyncService
	workspaces           WorkspaceService
	knowledge            KnowledgeService
	agentSessions        AgentSessionService
	coordination         CoordinationService
	handoffs             HandoffService
	contextPacks         ContextPackService
	access               AccessService
	backoffice           backoffice.Reader
	events               eventlog.Reader
	streamShutdown       <-chan struct{}
	streamPollInterval   time.Duration
	streamHeartbeatEvery time.Duration
}

func New(cfg Config) http.Handler {
	if cfg.StreamPollInterval <= 0 {
		cfg.StreamPollInterval = time.Second
	}
	if cfg.StreamHeartbeatEvery <= 0 {
		cfg.StreamHeartbeatEvery = 15 * time.Second
	}

	api := &API{
		logger:               cfg.Logger,
		organizationID:       cfg.OrganizationID,
		build:                cfg.Build,
		readiness:            cfg.Readiness,
		projects:             cfg.ProjectService,
		repositorySync:       cfg.RepositorySyncService,
		workspaces:           cfg.WorkspaceService,
		knowledge:            cfg.KnowledgeService,
		agentSessions:        cfg.AgentSessionService,
		coordination:         cfg.CoordinationService,
		handoffs:             cfg.HandoffService,
		contextPacks:         cfg.ContextPackService,
		access:               cfg.AccessService,
		backoffice:           cfg.BackofficeReader,
		events:               cfg.EventReader,
		streamShutdown:       cfg.StreamShutdown,
		streamPollInterval:   cfg.StreamPollInterval,
		streamHeartbeatEvery: cfg.StreamHeartbeatEvery,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", api.handleAdminRedirect)
	mux.HandleFunc("GET /livez", api.handleLive)
	mux.HandleFunc("GET /readyz", api.handleReady)
	mux.HandleFunc("GET /version", api.handleVersion)
	mux.HandleFunc("GET /admin", api.handleAdminRedirect)
	mux.Handle("GET /admin/", adminui.Handler())
	mux.Handle("GET /v1/projects", api.requireAuth(http.HandlerFunc(api.handleListProjects)))
	mux.Handle("POST /v1/projects", api.requireAuth(http.HandlerFunc(api.handleCreateProject)))
	mux.Handle("GET /v1/workspaces", api.requireAuth(http.HandlerFunc(api.handleListWorkspaces)))
	mux.Handle("POST /v1/workspaces", api.requireAuth(http.HandlerFunc(api.handleCreateWorkspace)))
	mux.Handle("GET /v1/workspaces/{workspaceID}", api.requireAuth(http.HandlerFunc(api.handleGetWorkspace)))
	mux.Handle("PUT /v1/workspaces/{workspaceID}/projects/{projectID}", api.requireAuth(http.HandlerFunc(api.handleAttachWorkspaceProject)))
	mux.Handle("GET /v1/workspaces/{workspaceID}/resources", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleListResources))))
	mux.Handle("POST /v1/workspaces/{workspaceID}/resources", api.requireAuth(api.requireWorkspaceRole("contributor", http.HandlerFunc(api.handleCreateResource))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/records", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleListRecords))))
	mux.Handle("POST /v1/workspaces/{workspaceID}/records", api.requireAuth(api.requireWorkspaceRole("contributor", http.HandlerFunc(api.handleCreateRecord))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/records/{recordID}", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleGetRecord))))
	mux.Handle("POST /v1/workspaces/{workspaceID}/records/{recordID}/status", api.requireAuth(api.requireWorkspaceRole("maintainer", http.HandlerFunc(api.handleUpdateRecordStatus))))
	mux.Handle("GET /v1/workspaces/{workspaceID}/context", api.requireAuth(api.requireWorkspaceRole("viewer", http.HandlerFunc(api.handleWorkspaceContext))))
	mux.Handle("GET /v1/projects/{projectID}", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleGetProject))))
	mux.Handle("GET /v1/projects/{projectID}/repository-sync", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleGetRepositorySync))))
	mux.Handle("POST /v1/projects/{projectID}/repository-sync", api.requireAuth(api.requireProjectRole("maintainer", http.HandlerFunc(api.handleSyncRepository))))
	mux.Handle("GET /v1/projects/{projectID}/overview", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleProjectOverview))))
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
	mux.HandleFunc("POST /v1/invitation-acceptances", api.handleAcceptInvitation)
	mux.Handle("GET /v1/me", api.requireAuth(http.HandlerFunc(api.handleMe)))
	mux.Handle("DELETE /v1/me/tokens/current", api.requireAuth(http.HandlerFunc(api.handleRevokeCurrentToken)))
	mux.Handle("/{$}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/livez", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/readyz", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/version", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/admin", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/admin/", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/workspaces/{workspaceID}/projects/{projectID}", api.methodNotAllowed(http.MethodPut))
	mux.Handle("/v1/workspaces/{workspaceID}/resources", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}/records", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}/records/{recordID}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/workspaces/{workspaceID}/records/{recordID}/status", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/workspaces/{workspaceID}/context", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/repository-sync", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/overview", api.methodNotAllowed(http.MethodGet))
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
	mux.Handle("/v1/invitation-acceptances", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/me", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/me/tokens/current", api.methodNotAllowed(http.MethodDelete))
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

func (a *API) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	if !hasJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Unsupported media type", "Content-Type must be application/json.")
		return
	}
	var input access.AcceptInvitationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid request body", err.Error())
		return
	}
	accepted, err := a.access.AcceptInvitation(r.Context(), input)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"data": accepted})
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

func (a *API) handleRevokeCurrentToken(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFromContext(r.Context())
	if err := a.access.RevokeCurrentToken(r.Context(), principal); err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func (a *API) handleStreamEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
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

	for {
		select {
		case <-a.streamShutdown:
			return
		default:
		}

		events, listErr := a.events.List(r.Context(), a.organizationID, projectID, after, 100)
		if listErr != nil {
			a.logger.ErrorContext(r.Context(), "event stream query failed", "error", listErr, "project_id", projectID)
			return
		}

		for _, event := range events {
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
	var agentValidationErr *agentsession.ValidationError
	var accessValidationErr *access.ValidationError
	var coordinationValidationErr *coordination.ValidationError
	var workspaceValidationErr *workspaces.ValidationError
	var knowledgeValidationErr *knowledge.ValidationError
	var contextValidationErr *contextpack.ValidationError
	var repositorySyncValidationErr *repositorysync.ValidationError
	var providerErr *repositorysync.ProviderError
	var scopeConflictErr *coordination.ScopeConflictError
	switch {
	case errors.As(err, &validationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", validationErr.Error())
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
	case errors.As(err, &contextValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", contextValidationErr.Error())
	case errors.As(err, &repositorySyncValidationErr):
		writeProblem(w, r, http.StatusBadRequest, "validation_error", "Invalid request", repositorySyncValidationErr.Error())
	case errors.As(err, &providerErr):
		if providerErr.RetryAfter != "" {
			w.Header().Set("Retry-After", providerErr.RetryAfter)
		}
		writeProblem(w, r, http.StatusFailedDependency, providerErr.Code, "GitHub synchronization failed", "Pact could not read the canonical repository state from GitHub.")
	case errors.As(err, &scopeConflictErr):
		writeScopeConflict(w, r, scopeConflictErr)
	case errors.Is(err, access.ErrUnauthorized):
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "Unauthorized", "A valid Pact access token is required.")
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
