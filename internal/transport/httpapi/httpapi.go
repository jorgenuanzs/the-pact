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
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/platform/eventlog"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/transport/httpapi/adminui"
)

const maxRequestBody = 1 << 20

type ProjectService interface {
	Create(context.Context, string, projects.CreateInput) (projects.CreateResult, error)
	Get(context.Context, string) (projects.Project, error)
	List(context.Context) ([]projects.Project, error)
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
	Logger               *slog.Logger
	OrganizationID       string
	Build                buildinfo.Info
	Readiness            ReadinessCheck
	ProjectService       ProjectService
	AgentSessionService  AgentSessionService
	CoordinationService  CoordinationService
	AccessService        AccessService
	BackofficeReader     backoffice.Reader
	EventReader          eventlog.Reader
	StreamShutdown       <-chan struct{}
	StreamPollInterval   time.Duration
	StreamHeartbeatEvery time.Duration
}

type API struct {
	logger               *slog.Logger
	organizationID       string
	build                buildinfo.Info
	readiness            ReadinessCheck
	projects             ProjectService
	agentSessions        AgentSessionService
	coordination         CoordinationService
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
		agentSessions:        cfg.AgentSessionService,
		coordination:         cfg.CoordinationService,
		access:               cfg.AccessService,
		backoffice:           cfg.BackofficeReader,
		events:               cfg.EventReader,
		streamShutdown:       cfg.StreamShutdown,
		streamPollInterval:   cfg.StreamPollInterval,
		streamHeartbeatEvery: cfg.StreamHeartbeatEvery,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", api.handleLive)
	mux.HandleFunc("GET /readyz", api.handleReady)
	mux.HandleFunc("GET /version", api.handleVersion)
	mux.HandleFunc("GET /admin", api.handleAdminRedirect)
	mux.Handle("GET /admin/", adminui.Handler())
	mux.Handle("GET /v1/projects", api.requireAuth(http.HandlerFunc(api.handleListProjects)))
	mux.Handle("POST /v1/projects", api.requireAuth(http.HandlerFunc(api.handleCreateProject)))
	mux.Handle("GET /v1/projects/{projectID}", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleGetProject))))
	mux.Handle("GET /v1/projects/{projectID}/overview", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleProjectOverview))))
	mux.Handle("GET /v1/projects/{projectID}/events", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleListEvents))))
	mux.Handle("GET /v1/projects/{projectID}/events/stream", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleStreamEvents))))
	mux.Handle("POST /v1/projects/{projectID}/agent-sessions", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleStartAgentSession))))
	mux.Handle("POST /v1/projects/{projectID}/scope-checks", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleScopeCheck))))
	mux.Handle("GET /v1/projects/{projectID}/work-items", api.requireAuth(api.requireProjectRole("viewer", http.HandlerFunc(api.handleListWork))))
	mux.Handle("POST /v1/projects/{projectID}/work-items", api.requireAuth(api.requireProjectRole("contributor", http.HandlerFunc(api.handleStartWork))))
	mux.Handle("POST /v1/intents/{intentID}/workspace", api.requireAuth(http.HandlerFunc(api.handleAttachWorkspace)))
	mux.Handle("POST /v1/intents/{intentID}/status", api.requireAuth(http.HandlerFunc(api.handleUpdateIntentStatus)))
	mux.Handle("POST /v1/agent-sessions/{sessionID}/heartbeat", api.requireAuth(http.HandlerFunc(api.handleAgentHeartbeat)))
	mux.Handle("POST /v1/agent-sessions/{sessionID}/repository-observations", api.requireAuth(http.HandlerFunc(api.handleRepositoryObservation)))
	mux.Handle("DELETE /v1/agent-sessions/{sessionID}", api.requireAuth(http.HandlerFunc(api.handleCloseAgentSession)))
	mux.Handle("POST /v1/projects/{projectID}/invitations", api.requireAuth(http.HandlerFunc(api.handleCreateInvitation)))
	mux.Handle("DELETE /v1/invitations/{invitationID}", api.requireAuth(http.HandlerFunc(api.handleRevokeInvitation)))
	mux.HandleFunc("POST /v1/invitation-acceptances", api.handleAcceptInvitation)
	mux.Handle("GET /v1/me", api.requireAuth(http.HandlerFunc(api.handleMe)))
	mux.Handle("DELETE /v1/me/tokens/current", api.requireAuth(http.HandlerFunc(api.handleRevokeCurrentToken)))
	mux.Handle("/livez", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/readyz", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/version", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/admin", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/admin/", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/projects/{projectID}", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/overview", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/events", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/events/stream", api.methodNotAllowed(http.MethodGet))
	mux.Handle("/v1/projects/{projectID}/agent-sessions", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/scope-checks", api.methodNotAllowed(http.MethodPost))
	mux.Handle("/v1/projects/{projectID}/work-items", api.methodNotAllowed(http.MethodGet+", "+http.MethodPost))
	mux.Handle("/v1/intents/{intentID}/workspace", api.methodNotAllowed(http.MethodPost))
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

func (a *API) handleAttachWorkspace(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Location", "/v1/intents/"+r.PathValue("intentID")+"/workspace")
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"project":       project,
		"code_activity": overview.CodeActivity,
		"counts":        overview.Counts,
		"active_work":   overview.ActiveWork,
		"recent_events": overview.RecentEvents,
		"work_items":    overview.WorkItems,
		"generated_at":  overview.GeneratedAt,
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
