package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/buildinfo"
	"github.com/jorgenuanzs/the-pact/internal/contextpack"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/gitobserve"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/lifecycle"
	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
	"github.com/jorgenuanzs/the-pact/internal/worktree"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
)

type mcpRuntime struct {
	binding                    localproject.Binding
	client                     *pactclient.Client
	session                    agentsession.Session
	ctx                        context.Context
	workspaceObservationErrors chan error
	workspaceMu                sync.Mutex
	workspaceCancel            map[string]context.CancelFunc
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
	WorktreeID      *string   `json:"worktree_id,omitempty"`
	WorktreeStatus  *string   `json:"worktree_status,omitempty"`
	WorktreeBranch  *string   `json:"worktree_branch,omitempty"`
	WorkspaceID     *string   `json:"workspace_id,omitempty"`
	WorkspaceStatus *string   `json:"workspace_status,omitempty"`
	WorkspaceBranch *string   `json:"workspace_branch,omitempty"`
}

type mcpOverview struct {
	CodeActivity backoffice.CodeActivity `json:"code_activity"`
	Counts       backoffice.Counts       `json:"counts"`
	ActiveWork   []mcpActiveWork         `json:"active_work"`
	RecentEvents []mcpRecentEvent        `json:"recent_events"`
	WorkItems    []mcpWorkItem           `json:"work_items"`
	Handoffs     []coordination.Handoff  `json:"handoffs"`
	GeneratedAt  time.Time               `json:"generated_at"`
}

type mcpProjectContextOutput struct {
	Project   mcpProject                  `json:"project"`
	Workspace *mcpSharedWorkspace         `json:"workspace,omitempty"`
	Knowledge *knowledge.WorkspaceContext `json:"knowledge,omitempty"`
	Principal access.Principal            `json:"principal"`
	Session   mcpSessionSummary           `json:"session"`
	Git       mcpGitSnapshot              `json:"git"`
	Overview  mcpOverview                 `json:"overview"`
}

type mcpProjectListOutput struct {
	Projects []mcpProject `json:"projects"`
}

type mcpWorkspaceListOutput struct {
	Workspaces []mcpSharedWorkspace `json:"workspaces"`
}

type mcpWorkspaceProject struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

type mcpSharedWorkspace struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Slug        string                `json:"slug"`
	Description string                `json:"description"`
	Status      string                `json:"status"`
	Projects    []mcpWorkspaceProject `json:"projects"`
	Version     int64                 `json:"version"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

type mcpObservationOutput struct {
	Git         mcpGitSnapshot                     `json:"git"`
	Observation agentsession.RepositoryObservation `json:"observation"`
	EventID     *string                            `json:"event_id,omitempty"`
	EventType   *string                            `json:"event_type,omitempty"`
}

type mcpWorkspaceSummary struct {
	ID           string     `json:"id"`
	IntentID     string     `json:"intent_id"`
	SessionID    string     `json:"session_id"`
	BaseRevision string     `json:"base_revision"`
	GitBranch    string     `json:"git_branch"`
	Status       string     `json:"status"`
	Version      int64      `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	FrozenAt     *time.Time `json:"frozen_at,omitempty"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
}

type mcpWorkItem struct {
	Intent          coordination.Intent       `json:"intent"`
	ResponsibleName string                    `json:"responsible_name"`
	Scopes          []coordination.ScopeClaim `json:"scopes"`
	Worktree        *mcpWorkspaceSummary      `json:"worktree,omitempty"`
	Workspace       *mcpWorkspaceSummary      `json:"workspace,omitempty"`
	SessionLive     bool                      `json:"session_live"`
	SessionLastSeen *time.Time                `json:"session_last_seen_at,omitempty"`
}

type mcpScopeCheckInput struct {
	Scopes []coordination.ScopeInput `json:"scopes" jsonschema:"minimum safe repository scopes to inspect for overlap"`
}

type mcpStartWorkInput struct {
	Title           string                    `json:"title" jsonschema:"short description of the work"`
	Goal            string                    `json:"goal" jsonschema:"concrete outcome this work must achieve"`
	SuccessCriteria []string                  `json:"success_criteria,omitempty" jsonschema:"observable completion criteria"`
	Scopes          []coordination.ScopeInput `json:"scopes" jsonschema:"repository path or file reservations"`
	AllowOverlap    bool                      `json:"allow_overlap,omitempty" jsonschema:"explicitly override blocking scope reservations"`
}

type mcpStartWorkOutput struct {
	Intent        coordination.Intent         `json:"intent"`
	Claims        []coordination.ScopeClaim   `json:"claims"`
	Overlaps      []coordination.ScopeOverlap `json:"overlaps"`
	Worktree      mcpWorkspaceSummary         `json:"worktree"`
	WorktreePath  string                      `json:"worktree_path"`
	Workspace     mcpWorkspaceSummary         `json:"workspace"`
	WorkspacePath string                      `json:"workspace_path"`
}

type mcpListWorkOutput struct {
	WorkItems []mcpWorkItem `json:"work_items"`
}

type mcpUpdateWorkInput struct {
	IntentID        string `json:"intent_id" jsonschema:"PACT intent identifier"`
	Status          string `json:"status" jsonschema:"active blocked submitted completed cancelled or abandoned"`
	ExpectedVersion int64  `json:"expected_version" jsonschema:"current intent version for optimistic concurrency"`
	Summary         string `json:"summary,omitempty" jsonschema:"durable summary of work performed"`
	Reason          string `json:"reason,omitempty" jsonschema:"reason for the status transition"`
}

type mcpUpdateWorkOutput struct {
	Intent  coordination.Intent `json:"intent"`
	EventID string              `json:"event_id"`
}

type mcpListKnowledgeInput struct {
	Query  string `json:"query,omitempty" jsonschema:"optional full-text search query"`
	Kind   string `json:"kind,omitempty" jsonschema:"optional resource kind or record type"`
	Status string `json:"status,omitempty" jsonschema:"optional lifecycle status"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum results from 1 to 250"`
}

type mcpResourceListOutput struct {
	WorkspaceID string               `json:"workspace_id"`
	Resources   []knowledge.Resource `json:"resources"`
}

type mcpAddResourceInput struct {
	Kind           string `json:"kind" jsonschema:"url repository document pull_request ticket meeting dashboard infrastructure or other"`
	Title          string `json:"title" jsonschema:"human-readable source title"`
	Locator        string `json:"locator" jsonschema:"durable URL path identifier or external reference without embedded secrets"`
	Description    string `json:"description,omitempty" jsonschema:"what this source contains and why it matters"`
	Classification string `json:"classification,omitempty" jsonschema:"public internal confidential or restricted; defaults to internal"`
}

type mcpRecordListOutput struct {
	WorkspaceID string             `json:"workspace_id"`
	Records     []knowledge.Record `json:"records"`
}

type mcpEvidenceInput struct {
	ResourceID string `json:"resource_id" jsonschema:"PACT resource identifier"`
	Relation   string `json:"relation,omitempty" jsonschema:"supports contradicts origin or validates"`
	Note       string `json:"note,omitempty" jsonschema:"how the resource relates to the record"`
}

type mcpProposeRecordInput struct {
	Type      string             `json:"type" jsonschema:"decision requirement constraint assumption risk open_question finding procedure incident validation_result or note"`
	Title     string             `json:"title" jsonschema:"concise knowledge title"`
	Body      string             `json:"body" jsonschema:"self-contained durable fact rationale or requirement"`
	Authority string             `json:"authority,omitempty" jsonschema:"informational team organization or external; defaults to team"`
	Evidence  []mcpEvidenceInput `json:"evidence,omitempty" jsonschema:"registered sources supporting or challenging this record"`
}

type mcpReviewRecordInput struct {
	RecordID            string `json:"record_id" jsonschema:"PACT record identifier"`
	Status              string `json:"status" jsonschema:"accepted disputed superseded revoked expired or rejected"`
	ExpectedVersion     int64  `json:"expected_version" jsonschema:"current record version for optimistic concurrency"`
	Reason              string `json:"reason,omitempty" jsonschema:"durable reason for the lifecycle transition"`
	SupersedingRecordID string `json:"superseding_record_id,omitempty" jsonschema:"accepted replacement record required when superseding"`
}

type mcpWorkspaceContextOutput struct {
	Context knowledge.WorkspaceContext `json:"context"`
}

type mcpOfferHandoffInput struct {
	IntentID        string                           `json:"intent_id" jsonschema:"PACT intent identifier"`
	Summary         string                           `json:"summary" jsonschema:"self-contained durable summary for the receiving collaborator"`
	Completed       []string                         `json:"completed,omitempty" jsonschema:"work already completed"`
	RemainingWork   []string                         `json:"remaining_work,omitempty" jsonschema:"work still required"`
	Blockers        []string                         `json:"blockers,omitempty" jsonschema:"known blockers or unresolved risks"`
	NextSteps       []string                         `json:"next_steps,omitempty" jsonschema:"recommended ordered next actions"`
	Validations     []coordination.HandoffValidation `json:"validations,omitempty" jsonschema:"checks already run and their status"`
	LinkedRecordIDs []string                         `json:"linked_record_ids,omitempty" jsonschema:"Workspace knowledge record identifiers relevant to the handoff"`
	ExpiresInHours  int                              `json:"expires_in_hours,omitempty" jsonschema:"offer lifetime from 1 to 168 hours; defaults to 72"`
}

type mcpListHandoffsInput struct {
	IntentID string `json:"intent_id,omitempty" jsonschema:"optional PACT intent identifier"`
}

type mcpHandoffListOutput struct {
	Handoffs []coordination.Handoff `json:"handoffs"`
}

type mcpUpdateHandoffInput struct {
	IntentID        string `json:"intent_id" jsonschema:"PACT intent identifier"`
	HandoffID       string `json:"handoff_id" jsonschema:"PACT handoff identifier"`
	Status          string `json:"status" jsonschema:"accepted or withdrawn"`
	ExpectedVersion int64  `json:"expected_version" jsonschema:"current handoff version for optimistic concurrency"`
}

type mcpCompileContextPackInput struct {
	IntentID   string `json:"intent_id" jsonschema:"PACT intent identifier to compile around"`
	Type       string `json:"type,omitempty" jsonschema:"implementation handoff review onboarding meeting incident or deployment"`
	TTLMinutes int    `json:"ttl_minutes,omitempty" jsonschema:"pack lifetime from 1 to 60 minutes; defaults to 5"`
}

type mcpGetContextPackInput struct {
	ContextPackID string `json:"context_pack_id" jsonschema:"PACT context pack identifier"`
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
	ctx, stop := lifecycle.NotifyContext(context.Background())
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

	runtime := &mcpRuntime{
		binding: binding, client: client, session: session, ctx: ctx,
		workspaceObservationErrors: make(chan error, 1),
		workspaceCancel:            make(map[string]context.CancelFunc),
	}
	defer runtime.stopAllWorkspaceObservers()
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
	case workspaceErr := <-runtime.workspaceObservationErrors:
		stop()
		<-serverErrors
		return workspaceErr
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
				"Its workspace field identifies the durable shared context boundary for this project. " +
				"Use pact.workspace_context when you need the current accepted decisions, requirements, constraints, questions, risks, and sources. " +
				"Register durable sources with pact.add_resource and propose reusable facts with pact.propose_record instead of copying private conversations. " +
				"Before modifying files, call pact.check_scopes and then pact.start_work; " +
				"perform edits only inside the worktree_path returned by pact.start_work. " +
				"Use pact.update_work to report blocked, submitted, completed, cancelled, or abandoned work with a durable summary. " +
				"Use pact.compile_context_pack for a bounded, verifiable snapshot instead of inheriting a conversation. " +
				"Before another collaborator takes over, offer a structured handoff; accepting it confirms receipt but never transfers a local worktree or scope reservation automatically. " +
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
		Description: "Return the connected project, shared Workspace knowledge, authenticated identity, current MCP agent session, private Git observation summary, live work, code activity, and recent durable events.",
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
		Name:        "pact.list_workspaces",
		Title:       "List visible PACT workspaces",
		Description: "List durable collaboration workspaces and the projects grouped within each one.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.listWorkspaces)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.workspace_context",
		Title:       "Get shared Workspace knowledge",
		Description: "Return accepted decisions, requirements, constraints, live questions, risks, evidence-backed records, and registered resources shared by the connected project's Workspace.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.workspaceContext)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.list_resources",
		Title:       "List shared knowledge resources",
		Description: "Search and list source references registered in the connected project's Workspace.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.listResources)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.add_resource",
		Title:       "Register a shared knowledge resource",
		Description: "Register a durable source reference in the connected project's Workspace. Never put credentials or secret query parameters in a locator.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.addResource)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.list_records",
		Title:       "List shared knowledge records",
		Description: "Search and list typed, evidence-backed knowledge records in the connected project's Workspace.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.listRecords)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.propose_record",
		Title:       "Propose a shared knowledge record",
		Description: "Propose a durable decision, requirement, constraint, question, risk, finding, procedure, incident, validation result, or note, optionally linked to registered evidence.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.proposeRecord)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.review_record",
		Title:       "Review a shared knowledge record",
		Description: "Accept, dispute, supersede, revoke, expire, or reject a record using its current version. Requires Workspace maintainer access.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.reviewRecord)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.refresh_git_observation",
		Title:       "Refresh PACT Git observation",
		Description: "Capture Git state locally and submit an authenticated observation to PACT. No file names or contents are transmitted.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.refreshObservation)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.check_scopes",
		Title:       "Check PACT scope reservations",
		Description: "Check repository, path, or file scopes against live reservations before beginning work.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.checkScopes)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.start_work",
		Title:       "Start isolated coordinated work",
		Description: "Create a durable intent, reserve scopes, provision an isolated Git worktree, and register it in PACT. Exclusive overlap is rejected unless allow_overlap is explicitly true.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.startWork)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.list_work",
		Title:       "List coordinated work",
		Description: "List active, blocked, submitted, and recently completed work with actors, scopes, worktrees, and live-session state.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.listWork)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.update_work",
		Title:       "Update coordinated work status",
		Description: "Block, resume, submit, complete, cancel, or abandon an intent using optimistic version control and a durable summary.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.updateWork)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.list_handoffs",
		Title:       "List PACT handoffs",
		Description: "List structured handoff offers and responses for the connected project, optionally filtered to one intent.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.listHandoffs)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.offer_handoff",
		Title:       "Offer a structured handoff",
		Description: "Offer a durable handoff with completed work, remaining work, blockers, next steps, validations, and linked Workspace records. This does not transfer a local worktree.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.offerHandoff)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.update_handoff",
		Title:       "Accept or withdraw a handoff",
		Description: "Accept an offered handoff as a different live collaborator, or withdraw your own offer. Acceptance confirms receipt only and does not transfer worktrees or scope reservations.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.updateHandoff)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.compile_context_pack",
		Title:       "Compile a PACT Context Pack",
		Description: "Compile and persist a bounded context snapshot for an intent with Workspace knowledge, live work, handoffs, Git revision, event cursor, expiration, and a verifiable source fingerprint.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld,
		},
	}, runtime.compileContextPack)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pact.get_context_pack",
		Title:       "Get a PACT Context Pack",
		Description: "Retrieve a persisted Context Pack after its stored payload passes an integrity check.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld,
		},
	}, runtime.getContextPack)
	return server
}

func (r *mcpRuntime) projectContext(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ mcpEmptyInput,
) (*mcp.CallToolResult, mcpProjectContextOutput, error) {
	var (
		principal     access.Principal
		project       projects.Project
		workspaceList []workspaces.Workspace
		overview      backoffice.Overview
		snapshot      gitobserve.Snapshot
	)
	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		principal, err = r.client.Me(groupContext)
		return err
	})
	group.Go(func() error {
		var err error
		workspaceList, err = r.client.ListWorkspaces(groupContext)
		var problem *pactclient.Problem
		if errors.As(err, &problem) && problem.Status == http.StatusNotFound {
			workspaceList = make([]workspaces.Workspace, 0)
			return nil
		}
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
	var sharedWorkspace *mcpSharedWorkspace
	var sharedKnowledge *knowledge.WorkspaceContext
	for index := range workspaceList {
		for _, candidate := range workspaceList[index].Projects {
			if candidate.ID == r.binding.ProjectID {
				output := sharedWorkspaceOutput(workspaceList[index])
				sharedWorkspace = &output
				break
			}
		}
		if sharedWorkspace != nil {
			break
		}
	}
	if sharedWorkspace != nil {
		contextValue, contextErr := r.client.WorkspaceContext(ctx, sharedWorkspace.ID)
		if contextErr != nil {
			return nil, mcpProjectContextOutput{}, contextErr
		}
		sharedKnowledge = &contextValue
	}
	return nil, mcpProjectContextOutput{
		Project: projectOutput(project), Principal: principal,
		Workspace: sharedWorkspace, Knowledge: sharedKnowledge,
		Session: sessionSummary(r.session), Git: snapshotOutput(snapshot),
		Overview: overviewOutput(overview),
	}, nil
}

func (r *mcpRuntime) connectedWorkspace(ctx context.Context) (workspaces.Workspace, error) {
	workspaceList, err := r.client.ListWorkspaces(ctx)
	if err != nil {
		return workspaces.Workspace{}, err
	}
	for _, workspace := range workspaceList {
		for _, project := range workspace.Projects {
			if project.ID == r.binding.ProjectID {
				return workspace, nil
			}
		}
	}
	return workspaces.Workspace{}, errors.New("the connected project is not attached to a visible PACT Workspace")
}

func (r *mcpRuntime) workspaceContext(
	ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput,
) (*mcp.CallToolResult, mcpWorkspaceContextOutput, error) {
	workspace, err := r.connectedWorkspace(ctx)
	if err != nil {
		return nil, mcpWorkspaceContextOutput{}, err
	}
	value, err := r.client.WorkspaceContext(ctx, workspace.ID)
	if err != nil {
		return nil, mcpWorkspaceContextOutput{}, err
	}
	return nil, mcpWorkspaceContextOutput{Context: value}, nil
}

func (r *mcpRuntime) listResources(
	ctx context.Context, _ *mcp.CallToolRequest, input mcpListKnowledgeInput,
) (*mcp.CallToolResult, mcpResourceListOutput, error) {
	workspace, err := r.connectedWorkspace(ctx)
	if err != nil {
		return nil, mcpResourceListOutput{}, err
	}
	resources, err := r.client.ListResources(ctx, workspace.ID, knowledge.ListOptions{
		Query: input.Query, Kind: input.Kind, Status: input.Status, Limit: input.Limit,
	})
	if err != nil {
		return nil, mcpResourceListOutput{}, err
	}
	return nil, mcpResourceListOutput{WorkspaceID: workspace.ID, Resources: resources}, nil
}

func (r *mcpRuntime) addResource(
	ctx context.Context, _ *mcp.CallToolRequest, input mcpAddResourceInput,
) (*mcp.CallToolResult, knowledge.Resource, error) {
	workspace, err := r.connectedWorkspace(ctx)
	if err != nil {
		return nil, knowledge.Resource{}, err
	}
	key, err := newIdempotencyKey("pact-resource-add")
	if err != nil {
		return nil, knowledge.Resource{}, err
	}
	resource, err := r.client.CreateResource(ctx, workspace.ID, key, knowledge.CreateResourceInput{
		Kind: input.Kind, Title: input.Title, Locator: input.Locator,
		Description: input.Description, Classification: input.Classification,
	})
	return nil, resource, err
}

func (r *mcpRuntime) listRecords(
	ctx context.Context, _ *mcp.CallToolRequest, input mcpListKnowledgeInput,
) (*mcp.CallToolResult, mcpRecordListOutput, error) {
	workspace, err := r.connectedWorkspace(ctx)
	if err != nil {
		return nil, mcpRecordListOutput{}, err
	}
	records, err := r.client.ListRecords(ctx, workspace.ID, knowledge.ListOptions{
		Query: input.Query, Kind: input.Kind, Status: input.Status, Limit: input.Limit,
	})
	if err != nil {
		return nil, mcpRecordListOutput{}, err
	}
	return nil, mcpRecordListOutput{WorkspaceID: workspace.ID, Records: records}, nil
}

func (r *mcpRuntime) proposeRecord(
	ctx context.Context, _ *mcp.CallToolRequest, input mcpProposeRecordInput,
) (*mcp.CallToolResult, knowledge.Record, error) {
	workspace, err := r.connectedWorkspace(ctx)
	if err != nil {
		return nil, knowledge.Record{}, err
	}
	evidence := make([]knowledge.EvidenceInput, 0, len(input.Evidence))
	for _, item := range input.Evidence {
		evidence = append(evidence, knowledge.EvidenceInput{
			ResourceID: item.ResourceID, Relation: item.Relation, Note: item.Note,
		})
	}
	key, err := newIdempotencyKey("pact-record-propose")
	if err != nil {
		return nil, knowledge.Record{}, err
	}
	record, err := r.client.CreateRecord(ctx, workspace.ID, key, knowledge.CreateRecordInput{
		Type: input.Type, Title: input.Title, Body: input.Body,
		Authority: input.Authority, Evidence: evidence,
	})
	return nil, record, err
}

func (r *mcpRuntime) reviewRecord(
	ctx context.Context, _ *mcp.CallToolRequest, input mcpReviewRecordInput,
) (*mcp.CallToolResult, knowledge.Record, error) {
	workspace, err := r.connectedWorkspace(ctx)
	if err != nil {
		return nil, knowledge.Record{}, err
	}
	key, err := newIdempotencyKey("pact-record-review")
	if err != nil {
		return nil, knowledge.Record{}, err
	}
	record, err := r.client.UpdateRecordStatus(ctx, workspace.ID, input.RecordID, key, knowledge.RecordStatusInput{
		Status: input.Status, ExpectedVersion: input.ExpectedVersion, Reason: input.Reason,
		SupersedingRecordID: input.SupersedingRecordID,
	})
	return nil, record, err
}

func (r *mcpRuntime) listWorkspaces(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ mcpEmptyInput,
) (*mcp.CallToolResult, mcpWorkspaceListOutput, error) {
	workspaceList, err := r.client.ListWorkspaces(ctx)
	if err != nil {
		return nil, mcpWorkspaceListOutput{}, err
	}
	output := make([]mcpSharedWorkspace, 0, len(workspaceList))
	for _, workspace := range workspaceList {
		output = append(output, sharedWorkspaceOutput(workspace))
	}
	return nil, mcpWorkspaceListOutput{Workspaces: output}, nil
}

func sharedWorkspaceOutput(workspace workspaces.Workspace) mcpSharedWorkspace {
	projects := make([]mcpWorkspaceProject, 0, len(workspace.Projects))
	for _, project := range workspace.Projects {
		projects = append(projects, mcpWorkspaceProject{
			ID: project.ID, Name: project.Name, Slug: project.Slug, Status: project.Status,
		})
	}
	return mcpSharedWorkspace{
		ID: workspace.ID, Name: workspace.Name, Slug: workspace.Slug,
		Description: workspace.Description, Status: workspace.Status,
		Projects: projects, Version: workspace.Version, UpdatedAt: workspace.UpdatedAt,
	}
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

func (r *mcpRuntime) checkScopes(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input mcpScopeCheckInput,
) (*mcp.CallToolResult, coordination.ScopeCheckResult, error) {
	result, err := r.client.CheckScopes(ctx, r.binding.ProjectID, input.Scopes)
	if err != nil {
		return nil, coordination.ScopeCheckResult{}, err
	}
	return nil, result, nil
}

func (r *mcpRuntime) startWork(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input mcpStartWorkInput,
) (*mcp.CallToolResult, mcpStartWorkOutput, error) {
	snapshot, err := gitobserve.Capture(ctx, r.binding.Root)
	if err != nil {
		return nil, mcpStartWorkOutput{}, err
	}
	if snapshot.HeadRevision == "" {
		return nil, mcpStartWorkOutput{}, errors.New("PACT cannot start isolated work without a Git HEAD revision")
	}
	if snapshot.Dirty {
		return nil, mcpStartWorkOutput{}, errors.New("PACT cannot start isolated work from a dirty checkout; commit, stash, or discard the existing changes first")
	}
	startKey, err := newIdempotencyKey("pact-work-start")
	if err != nil {
		return nil, mcpStartWorkOutput{}, err
	}
	started, err := r.client.StartWork(ctx, r.binding.ProjectID, startKey, coordination.StartInput{
		SessionID: r.session.ID, Title: input.Title, Goal: input.Goal,
		SuccessCriteria: input.SuccessCriteria, BaseRevision: snapshot.HeadRevision,
		Scopes: input.Scopes, AllowOverlap: input.AllowOverlap,
	})
	if err != nil {
		return nil, mcpStartWorkOutput{}, err
	}

	localWorkspace, err := worktree.Create(
		ctx, r.binding.Root, started.Intent.ID, started.Intent.Title, started.Intent.BaseRevision,
	)
	if err != nil {
		r.markWorkBlocked(started.Intent, "local_worktree_failed")
		return nil, mcpStartWorkOutput{}, err
	}
	workspaceKey, err := newIdempotencyKey("pact-workspace-attach")
	if err != nil {
		return nil, mcpStartWorkOutput{}, err
	}
	attached, err := r.client.AttachWorktree(ctx, started.Intent.ID, workspaceKey, coordination.WorktreeInput{
		SessionID: r.session.ID, BaseRevision: localWorkspace.BaseRevision,
		PathRef: localWorkspace.PathRef, GitBranch: localWorkspace.Branch,
	})
	if err != nil {
		r.markWorkBlocked(started.Intent, "workspace_registration_failed")
		return nil, mcpStartWorkOutput{}, err
	}
	if err := r.startWorkspaceObserver(localWorkspace.Path, attached.Worktree.ID); err != nil {
		r.markWorkBlocked(started.Intent, "workspace_observation_failed")
		return nil, mcpStartWorkOutput{}, err
	}
	worktreeSummary := workspaceSummary(attached.Worktree)
	return nil, mcpStartWorkOutput{
		Intent: started.Intent, Claims: started.Claims, Overlaps: started.Overlaps,
		Worktree: worktreeSummary, WorktreePath: localWorkspace.Path,
		Workspace: worktreeSummary, WorkspacePath: localWorkspace.Path,
	}, nil
}

func (r *mcpRuntime) listWork(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ mcpEmptyInput,
) (*mcp.CallToolResult, mcpListWorkOutput, error) {
	items, err := r.client.ListWork(ctx, r.binding.ProjectID)
	if err != nil {
		return nil, mcpListWorkOutput{}, err
	}
	return nil, mcpListWorkOutput{WorkItems: workItemsOutput(items)}, nil
}

func (r *mcpRuntime) updateWork(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input mcpUpdateWorkInput,
) (*mcp.CallToolResult, mcpUpdateWorkOutput, error) {
	key, err := newIdempotencyKey("pact-work-status")
	if err != nil {
		return nil, mcpUpdateWorkOutput{}, err
	}
	result, err := r.client.UpdateWorkStatus(ctx, input.IntentID, key, coordination.StatusInput{
		SessionID: r.session.ID, Status: input.Status, ExpectedVersion: input.ExpectedVersion,
		Summary: input.Summary, Reason: input.Reason,
	})
	if err != nil {
		return nil, mcpUpdateWorkOutput{}, err
	}
	if input.Status == "completed" || input.Status == "cancelled" || input.Status == "abandoned" {
		r.stopWorkspaceObserverForIntent(input.IntentID)
	}
	return nil, mcpUpdateWorkOutput{Intent: result.Intent, EventID: result.EventID}, nil
}

func (r *mcpRuntime) listHandoffs(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input mcpListHandoffsInput,
) (*mcp.CallToolResult, mcpHandoffListOutput, error) {
	handoffs, err := r.client.ListHandoffs(ctx, r.binding.ProjectID, input.IntentID)
	if err != nil {
		return nil, mcpHandoffListOutput{}, err
	}
	return nil, mcpHandoffListOutput{Handoffs: handoffs}, nil
}

func (r *mcpRuntime) offerHandoff(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input mcpOfferHandoffInput,
) (*mcp.CallToolResult, coordination.HandoffResult, error) {
	key, err := newIdempotencyKey("pact-handoff-offer")
	if err != nil {
		return nil, coordination.HandoffResult{}, err
	}
	result, err := r.client.OfferHandoff(ctx, r.binding.ProjectID, input.IntentID, key, coordination.OfferHandoffInput{
		SessionID: r.session.ID, Summary: input.Summary, Completed: input.Completed,
		RemainingWork: input.RemainingWork, Blockers: input.Blockers, NextSteps: input.NextSteps,
		Validations: input.Validations, LinkedRecordIDs: input.LinkedRecordIDs,
		ExpiresInHours: input.ExpiresInHours,
	})
	return nil, result, err
}

func (r *mcpRuntime) updateHandoff(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input mcpUpdateHandoffInput,
) (*mcp.CallToolResult, coordination.HandoffResult, error) {
	key, err := newIdempotencyKey("pact-handoff-status")
	if err != nil {
		return nil, coordination.HandoffResult{}, err
	}
	result, err := r.client.UpdateHandoffStatus(
		ctx, r.binding.ProjectID, input.IntentID, input.HandoffID, key,
		coordination.HandoffStatusInput{
			SessionID: r.session.ID, Status: input.Status, ExpectedVersion: input.ExpectedVersion,
		},
	)
	return nil, result, err
}

func (r *mcpRuntime) compileContextPack(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input mcpCompileContextPackInput,
) (*mcp.CallToolResult, contextpack.CompileResult, error) {
	key, err := newIdempotencyKey("pact-context-compile")
	if err != nil {
		return nil, contextpack.CompileResult{}, err
	}
	result, err := r.client.CompileContextPack(
		ctx, r.binding.ProjectID, input.IntentID, key,
		contextpack.CompileInput{SessionID: r.session.ID, Type: input.Type, TTLMinutes: input.TTLMinutes},
	)
	return nil, result, err
}

func (r *mcpRuntime) getContextPack(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input mcpGetContextPackInput,
) (*mcp.CallToolResult, contextpack.ContextPack, error) {
	pack, err := r.client.GetContextPack(ctx, r.binding.ProjectID, input.ContextPackID)
	return nil, pack, err
}

func (r *mcpRuntime) markWorkBlocked(intent coordination.Intent, reason string) {
	key, err := newIdempotencyKey("pact-work-status")
	if err != nil {
		return
	}
	blockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = r.client.UpdateWorkStatus(blockContext, intent.ID, key, coordination.StatusInput{
		SessionID: r.session.ID, Status: "blocked", ExpectedVersion: intent.Version,
		Reason: reason,
	})
}

func (r *mcpRuntime) startWorkspaceObserver(path, workspaceID string) error {
	r.workspaceMu.Lock()
	if _, exists := r.workspaceCancel[workspaceID]; exists {
		r.workspaceMu.Unlock()
		return nil
	}
	observerContext, cancel := context.WithCancel(r.ctx)
	r.workspaceCancel[workspaceID] = cancel
	r.workspaceMu.Unlock()

	snapshot, err := gitobserve.Capture(observerContext, path)
	if err != nil {
		cancel()
		r.workspaceMu.Lock()
		delete(r.workspaceCancel, workspaceID)
		r.workspaceMu.Unlock()
		return err
	}
	if _, err := submitObservationForWorkspace(observerContext, r.client, r.session.ID, &workspaceID, snapshot); err != nil {
		cancel()
		r.workspaceMu.Lock()
		delete(r.workspaceCancel, workspaceID)
		r.workspaceMu.Unlock()
		return err
	}
	go maintainGitObservationsForWorkspace(
		observerContext, path, r.client, r.session.ID, &workspaceID,
		snapshot, 2*time.Second, r.workspaceObservationErrors,
	)
	return nil
}

func (r *mcpRuntime) stopWorkspaceObserverForIntent(intentID string) {
	ctx, cancelRequest := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelRequest()
	items, err := r.client.ListWork(ctx, r.binding.ProjectID)
	if err != nil {
		return
	}
	for _, item := range items {
		if item.Intent.ID == intentID && item.Workspace != nil {
			r.workspaceMu.Lock()
			cancel := r.workspaceCancel[item.Workspace.ID]
			delete(r.workspaceCancel, item.Workspace.ID)
			r.workspaceMu.Unlock()
			if cancel != nil {
				cancel()
			}
			return
		}
	}
}

func (r *mcpRuntime) stopAllWorkspaceObservers() {
	r.workspaceMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(r.workspaceCancel))
	for _, cancel := range r.workspaceCancel {
		cancels = append(cancels, cancel)
	}
	r.workspaceCancel = make(map[string]context.CancelFunc)
	r.workspaceMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
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
			WorktreeID: work.WorkspaceID, WorktreeStatus: work.WorkspaceStatus,
			WorktreeBranch: work.WorkspaceBranch,
			WorkspaceID:    work.WorkspaceID, WorkspaceStatus: work.WorkspaceStatus,
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
		WorkItems:   workItemsOutput(overview.WorkItems),
		Handoffs:    overview.Handoffs,
		GeneratedAt: overview.GeneratedAt,
	}
}

func workItemsOutput(items []coordination.WorkItem) []mcpWorkItem {
	output := make([]mcpWorkItem, 0, len(items))
	for _, item := range items {
		work := mcpWorkItem{
			Intent: item.Intent, ResponsibleName: item.ResponsibleName,
			Scopes: item.Scopes, SessionLive: item.SessionLive,
			SessionLastSeen: item.SessionLastSeen,
		}
		if item.Workspace != nil {
			workspace := workspaceSummary(*item.Workspace)
			work.Worktree = &workspace
			work.Workspace = &workspace
		}
		output = append(output, work)
	}
	return output
}

func workspaceSummary(workspace coordination.Workspace) mcpWorkspaceSummary {
	return mcpWorkspaceSummary{
		ID: workspace.ID, IntentID: workspace.IntentID, SessionID: workspace.SessionID,
		BaseRevision: workspace.BaseRevision, GitBranch: workspace.GitBranch,
		Status: workspace.Status, Version: workspace.Version,
		CreatedAt: workspace.CreatedAt, UpdatedAt: workspace.UpdatedAt,
		FrozenAt: workspace.FrozenAt, ArchivedAt: workspace.ArchivedAt,
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
