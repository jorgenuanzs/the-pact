package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/gitobserve"
	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
)

type mcpRuntime struct {
	binding localproject.Binding
	client  *pactclient.Client
	session agentsession.Session
}

type mcpEmptyInput struct{}

type mcpGitSnapshot struct {
	Dirty        bool   `json:"dirty"`
	Fingerprint  string `json:"diff_fingerprint"`
	ChangedPaths int    `json:"changed_paths"`
	HeadRevision string `json:"head_revision,omitempty"`
	Branch       string `json:"branch,omitempty"`
}

type mcpSessionSummary struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actor_id"`
	ActorName  string    `json:"actor_name"`
	NodeID     string    `json:"node_id"`
	NodeName   string    `json:"node_name"`
	ClientType string    `json:"client_type"`
	StartedAt  time.Time `json:"started_at"`
}

type mcpRecentEvent struct {
	ID         string         `json:"id"`
	Sequence   string         `json:"sequence"`
	Type       string         `json:"type"`
	ActorID    *string        `json:"actor_id,omitempty"`
	SessionID  *string        `json:"session_id,omitempty"`
	IntentID   *string        `json:"intent_id,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
	Data       map[string]any `json:"data"`
}

type mcpProject struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Slug              string               `json:"slug"`
	Status            string               `json:"status"`
	CanonicalRevision *string              `json:"canonical_revision,omitempty"`
	RootRepository    *mcpSourceRepository `json:"root_repository,omitempty"`
	Version           int64                `json:"version"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}

type mcpSourceRepository struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	VCSType       string `json:"vcs_type"`
	Status        string `json:"status"`
	DefaultBranch string `json:"default_branch"`
	ObjectFormat  string `json:"object_format"`
	Version       int64  `json:"version"`
}

type mcpActiveWork struct {
	SessionID       string    `json:"session_id"`
	ActorID         string    `json:"actor_id"`
	ActorName       string    `json:"actor_name"`
	ActorKind       string    `json:"actor_kind"`
	ClientType      string    `json:"client_type"`
	SessionStatus   string    `json:"session_status"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	NodeID          *string   `json:"node_id,omitempty"`
	NodeName        *string   `json:"node_name,omitempty"`
	NodeStatus      *string   `json:"node_status,omitempty"`
	IntentID        *string   `json:"intent_id,omitempty"`
	IntentTitle     *string   `json:"intent_title,omitempty"`
	IntentStatus    *string   `json:"intent_status,omitempty"`
	WorkspaceID     *string   `json:"workspace_id,omitempty"`
	WorkspaceStatus *string   `json:"workspace_status,omitempty"`
	WorkspaceBranch *string   `json:"workspace_branch,omitempty"`
}

type mcpOverview struct {
	CodeActivity backoffice.CodeActivity `json:"code_activity"`
	Counts       backoffice.Counts       `json:"counts"`
	ActiveWork   []mcpActiveWork         `json:"active_work"`
	RecentEvents []mcpRecentEvent        `json:"recent_events"`
	GeneratedAt  time.Time               `json:"generated_at"`
}

type mcpProjectContextOutput struct {
	Project   mcpProject        `json:"project"`
	Principal access.Principal  `json:"principal"`
	Session   mcpSessionSummary `json:"session"`
	Git       mcpGitSnapshot    `json:"git"`
	Overview  mcpOverview       `json:"overview"`
}

type mcpProjectListOutput struct {
	Projects []mcpProject `json:"projects"`
}

type mcpObservationOutput struct {
	Git         mcpGitSnapshot                     `json:"git"`
	Observation agentsession.RepositoryObservation `json:"observation"`
	EventID     *string                            `json:"event_id,omitempty"`
	EventType   *string                            `json:"event_type,omitempty"`
}

func runMCP(args []string, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("expected pact mcp serve")
	}
	flags := flag.NewFlagSet("pact mcp serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	clientType := flags.String("client", "", "MCP client type, such as codex, claude, or kimi")
	agentName := flags.String("name", "", "agent display name (defaults to the client type)")
	projectPath := flags.String("path", ".", "path inside the connected Pact project")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("pact mcp serve accepts no positional arguments")
	}
	*clientType = strings.ToLower(strings.TrimSpace(*clientType))
	if *clientType == "" {
		return errors.New("pact mcp serve requires --client")
	}
	if strings.TrimSpace(*agentName) == "" {
		*agentName = strings.ToUpper((*clientType)[:1]) + (*clientType)[1:]
	}

	binding, err := localproject.LoadBinding(*projectPath)
	if err != nil {
		return err
	}
	login, err := loginForServer(binding.ServerURL)
	if err != nil {
		return err
	}
	node, err := localproject.EnsureNodeIdentity(binding.Root)
	if err != nil {
		return err
	}
	initialSnapshot, err := gitobserve.Capture(context.Background(), binding.Root)
	if err != nil {
		return err
	}
	client, err := pactclient.New(login.ServerURL, login.APIToken)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startContext, cancelStart := context.WithTimeout(ctx, 15*time.Second)
	session, err := client.StartAgentSession(startContext, binding.ProjectID, agentsession.StartInput{
		NodeKey: node.Key, NodeName: node.Name, AgentName: strings.TrimSpace(*agentName),
		AgentType: *clientType, ClientType: *clientType + "-mcp", ObserveGit: true,
	})
	cancelStart()
	if err != nil {
		return fmt.Errorf("start MCP agent session: %w", err)
	}
	defer func() {
		closeContext, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelClose()
		_ = client.CloseAgentSession(closeContext, session.ID)
	}()
	if err := reportObservation(ctx, client, session.ID, initialSnapshot); err != nil {
		return err
	}

	runtime := &mcpRuntime{binding: binding, client: client, session: session}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	server := newMCPServer(runtime, logger)
	heartbeatErrors := make(chan error, 1)
	go maintainHeartbeat(ctx, client, session.ID, heartbeatErrors)
	observationErrors := make(chan error, 1)
	go maintainGitObservations(ctx, binding.Root, client, session.ID, initialSnapshot, 2*time.Second, observationErrors)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Run(ctx, &mcp.StdioTransport{}) }()

	select {
	case serverErr := <-serverErrors:
		if errors.Is(serverErr, context.Canceled) {
			return nil
		}
		return serverErr
	case heartbeatErr := <-heartbeatErrors:
		stop()
		<-serverErrors
		return heartbeatErr
	case observationErr := <-observationErrors:
		stop()
		<-serverErrors
		return observationErr
	case <-ctx.Done():
		<-serverErrors
		return nil
	}
}

func newMCPServer(runtime *mcpRuntime, logger *slog.Logger) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "the-pact", Version: buildinfo.Current().Version},
		&mcp.ServerOptions{
			Logger: logger,
			Instructions: "Use pact.project_context before beginning project work. " +
				"PACT exposes shared operational facts, not private conversations. " +
				"Do not treat agent presence as proof of code changes.",
			Capabilities: &mcp.ServerCapabilities{},
		},
	)
	closedWorld := false
	nonDestructive := false
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.project_context",
		Title:       "Get PACT project context",
		Description: "Return the connected project, authenticated identity, current MCP agent session, private Git observation summary, live work, code activity, and recent durable events.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.projectContext)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.list_projects",
		Title:       "List visible PACT projects",
		Description: "List the projects visible to the authenticated PACT identity.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.listProjects)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.refresh_git_observation",
		Title:       "Refresh PACT Git observation",
		Description: "Capture Git state locally and submit an authenticated observation to PACT. No file names or contents are transmitted.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.refreshObservation)
	return server
}

func (r *mcpRuntime) projectContext(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ mcpEmptyInput,
) (*mcp.CallToolResult, mcpProjectContextOutput, error) {
	var (
		principal access.Principal
		project   projects.Project
		overview  backoffice.Overview
		snapshot  gitobserve.Snapshot
	)
	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		principal, err = r.client.Me(groupContext)
		return err
	})
	group.Go(func() error {
		var err error
		project, overview, err = r.client.GetProjectOverview(groupContext, r.binding.ProjectID)
		return err
	})
	group.Go(func() error {
		var err error
		snapshot, err = gitobserve.Capture(groupContext, r.binding.Root)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, mcpProjectContextOutput{}, err
	}
	return nil, mcpProjectContextOutput{
		Project: projectOutput(project), Principal: principal,
		Session: sessionSummary(r.session), Git: snapshotOutput(snapshot),
		Overview: overviewOutput(overview),
	}, nil
}

func (r *mcpRuntime) listProjects(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ mcpEmptyInput,
) (*mcp.CallToolResult, mcpProjectListOutput, error) {
	projectList, err := r.client.ListProjects(ctx)
	if err != nil {
		return nil, mcpProjectListOutput{}, err
	}
	projectsOutput := make([]mcpProject, 0, len(projectList))
	for _, project := range projectList {
		projectsOutput = append(projectsOutput, projectOutput(project))
	}
	return nil, mcpProjectListOutput{Projects: projectsOutput}, nil
}

func (r *mcpRuntime) refreshObservation(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ mcpEmptyInput,
) (*mcp.CallToolResult, mcpObservationOutput, error) {
	snapshot, err := gitobserve.Capture(ctx, r.binding.Root)
	if err != nil {
		return nil, mcpObservationOutput{}, err
	}
	result, err := submitObservation(ctx, r.client, r.session.ID, snapshot)
	if err != nil {
		return nil, mcpObservationOutput{}, err
	}
	return nil, mcpObservationOutput{
		Git: snapshotOutput(snapshot), Observation: result.Observation,
		EventID: result.EventID, EventType: result.EventType,
	}, nil
}

func snapshotOutput(snapshot gitobserve.Snapshot) mcpGitSnapshot {
	return mcpGitSnapshot{
		Dirty: snapshot.Dirty, Fingerprint: snapshot.Fingerprint,
		ChangedPaths: snapshot.ChangedPaths, HeadRevision: snapshot.HeadRevision,
		Branch: snapshot.Branch,
	}
}

func sessionSummary(session agentsession.Session) mcpSessionSummary {
	return mcpSessionSummary{
		ID: session.ID, ActorID: session.ActorID, ActorName: session.ActorName,
		NodeID: session.NodeID, NodeName: session.NodeName, ClientType: session.ClientType,
		StartedAt: session.StartedAt,
	}
}

func projectOutput(project projects.Project) mcpProject {
	output := mcpProject{
		ID: project.ID, Name: project.Name, Slug: project.Slug, Status: project.Status,
		CanonicalRevision: project.CanonicalRevision, Version: project.Version,
		CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt,
	}
	if project.RootRepository != nil {
		repository := project.RootRepository
		output.RootRepository = &mcpSourceRepository{
			ID: repository.ID, Slug: repository.Slug, Name: repository.Name,
			VCSType: repository.VCSType, Status: repository.Status,
			DefaultBranch: repository.DefaultBranch, ObjectFormat: repository.ObjectFormat,
			Version: repository.Version,
		}
	}
	return output
}

func overviewOutput(overview backoffice.Overview) mcpOverview {
	activeWork := make([]mcpActiveWork, 0, len(overview.ActiveWork))
	for _, work := range overview.ActiveWork {
		activeWork = append(activeWork, mcpActiveWork{
			SessionID: work.SessionID, ActorID: work.ActorID, ActorName: work.ActorName,
			ActorKind: work.ActorKind, ClientType: work.ClientType,
			SessionStatus: work.SessionStatus, LastSeenAt: work.LastSeenAt, ExpiresAt: work.ExpiresAt,
			NodeID: work.NodeID, NodeName: work.NodeName, NodeStatus: work.NodeStatus,
			IntentID: work.IntentID, IntentTitle: work.IntentTitle, IntentStatus: work.IntentStatus,
			WorkspaceID: work.WorkspaceID, WorkspaceStatus: work.WorkspaceStatus,
			WorkspaceBranch: work.WorkspaceBranch,
		})
	}
	events := make([]mcpRecentEvent, 0, len(overview.RecentEvents))
	for _, event := range overview.RecentEvents {
		data := make(map[string]any)
		if len(event.Data) > 0 {
			_ = json.Unmarshal(event.Data, &data)
		}
		events = append(events, mcpRecentEvent{
			ID: event.ID, Sequence: event.Sequence, Type: event.Type,
			ActorID: event.ActorID, SessionID: event.SessionID, IntentID: event.IntentID,
			OccurredAt: event.OccurredAt, Data: sanitizeEventData(data),
		})
	}
	return mcpOverview{
		CodeActivity: overview.CodeActivity, Counts: overview.Counts,
		ActiveWork: activeWork, RecentEvents: events,
		GeneratedAt: overview.GeneratedAt,
	}
}

func sanitizeEventData(data map[string]any) map[string]any {
	clean := make(map[string]any, len(data))
	for key, value := range data {
		normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
		if normalized == "remoteurl" || strings.Contains(normalized, "token") ||
			strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") ||
			strings.Contains(normalized, "passphrase") || strings.Contains(normalized, "credential") ||
			strings.Contains(normalized, "privatekey") || strings.Contains(normalized, "authorization") ||
			strings.Contains(normalized, "apikey") {
			clean[key] = "[REDACTED]"
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			clean[key] = sanitizeEventData(nested)
		case []any:
			items := make([]any, 0, len(nested))
			for _, item := range nested {
				if object, ok := item.(map[string]any); ok {
					items = append(items, sanitizeEventData(object))
				} else {
					items = append(items, item)
				}
			}
			clean[key] = items
		default:
			clean[key] = value
		}
	}
	return clean
}
