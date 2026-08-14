package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/backoffice"
	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPServerExposesSafeProjectContext(t *testing.T) {
	const (
		projectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
		sessionID = "018f784a-68c1-7b0f-8f2a-cfc255f99e3f"
		secret    = "mcp-test-super-secret-token"
	)
	now := time.Now().UTC().Truncate(time.Second)
	remote := "https://username:embedded-secret@example.com/acme/project.git?token=hidden"
	project := projects.Project{
		ID: projectID, Name: "Footfall", Slug: "footfall", Status: "active", Version: 3,
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
		RootRepository: &projects.SourceRepository{
			ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e2e", Slug: "primary", Name: "Primary",
			VCSType: "git", Status: "active", RemoteURL: &remote, DefaultBranch: "main",
			ObjectFormat: "sha1", Version: 1,
		},
	}
	sharedWorkspace := workspaces.Workspace{
		ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e4a", Name: "Footfall Product",
		Slug: "footfall-product", Description: "Shared product context", Status: "active", Version: 1,
		Projects: []workspaces.Project{{
			ID: projectID, Name: project.Name, Slug: project.Slug, Status: project.Status,
			RootRepositoryRemoteURL: &remote,
		}},
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	sharedKnowledge := knowledge.WorkspaceContext{
		WorkspaceID: sharedWorkspace.ID,
		Decisions: []knowledge.Record{{
			ID: "018f784a-68c1-7b0f-8f2a-cfc255f99e5b", WorkspaceID: sharedWorkspace.ID,
			Type: "decision", Title: "Use PostgreSQL", Body: "One durable shared store",
			Status: "accepted", Authority: "team", Evidence: []knowledge.Evidence{},
			Metadata: map[string]any{}, Version: 2, CreatedAt: now, UpdatedAt: now,
		}},
		Requirements: []knowledge.Record{}, Constraints: []knowledge.Record{},
		OpenQuestions: []knowledge.Record{}, Risks: []knowledge.Record{},
		OtherRecords: []knowledge.Record{}, Resources: []knowledge.Resource{},
		Warnings: []string{}, GeneratedAt: now,
	}
	privatePath := "/Users/private/workspaces/footfall"
	overview := backoffice.Overview{
		CodeActivity: backoffice.CodeActivity{State: backoffice.CodeActivityEditing},
		Counts:       backoffice.Counts{LiveSessions: 1, ConnectedObservers: 1},
		ActiveWork: []backoffice.ActiveWork{{
			SessionID: sessionID, ActorID: "actor-1", ActorName: "Codex", ActorKind: "agent",
			ClientType: "codex-mcp", SessionStatus: "active", LastSeenAt: now, ExpiresAt: now.Add(time.Minute),
			WorkspacePathRef: &privatePath,
		}},
		RecentEvents: []backoffice.RecentEvent{{
			ID: "event-1", Sequence: "7", Type: "pact.project.created.v1", OccurredAt: now,
			Data: json.RawMessage(`{"root_repository":{"remote_url":"https://token@example.com/repo.git"},"api_token":"must-not-leak","name":"Footfall"}`),
		}},
		WorkItems: []coordination.WorkItem{{
			Intent: coordination.Intent{
				ID: "intent-1", ProjectID: projectID, Title: "Improve API", Goal: "Safer endpoint",
				Status: "active", StatusDetail: map[string]any{}, BaseRevision: strings.Repeat("a", 40), Version: 1,
			},
			ResponsibleName: "Codex", SessionLive: true,
			Scopes: []coordination.ScopeClaim{{
				Resource: coordination.ResourceRef{Kind: "path", Locator: "internal/api"},
				Mode:     "exclusive", Status: "active",
			}},
			Workspace: &coordination.Workspace{
				ID: "workspace-1", IntentID: "intent-1", SessionID: sessionID,
				GitBranch: "pact/intent-improve-api", PathRef: privatePath, Status: "ready",
			},
		}},
		GeneratedAt: now,
	}
	principal := access.Principal{
		ID: "principal-1", OrganizationID: "organization-1", DisplayName: "Jorge",
		PrincipalType: "human", OrganizationRole: "owner", TokenID: "private-token-id",
	}
	var observationReceived atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": principal})
		case "/v1/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"projects": []projects.Project{project}}})
		case "/v1/workspaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"workspaces": []workspaces.Workspace{sharedWorkspace}}})
		case "/v1/workspaces/" + sharedWorkspace.ID + "/context":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": sharedKnowledge})
		case "/v1/projects/" + projectID:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": project})
		case "/v1/projects/" + projectID + "/overview":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"project": project, "code_activity": overview.CodeActivity, "counts": overview.Counts,
				"active_work": overview.ActiveWork, "recent_events": overview.RecentEvents,
				"work_items":   overview.WorkItems,
				"generated_at": overview.GeneratedAt,
			}})
		case "/v1/projects/" + projectID + "/repositories":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"repositories": []any{}, "sync_states": []any{},
			}})
		case "/v1/agent-sessions/" + sessionID + "/repository-observations":
			var input agentsession.ObservationInput
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode observation: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if request.Header.Get("Idempotency-Key") == "" || input.ChangedPaths != 1 {
				t.Errorf("observation input = %#v; idempotency = %q", input, request.Header.Get("Idempotency-Key"))
			}
			observationReceived.Store(true)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": agentsession.ObservationResult{
				Observation: agentsession.RepositoryObservation{
					ID: "observation-1", ProjectID: projectID, SessionID: sessionID,
					Dirty: input.Dirty, ChangedPaths: input.ChangedPaths,
					DiffFingerprint: input.DiffFingerprint, HeadRevision: input.HeadRevision,
					Branch: input.Branch, Version: 1, ObservedAt: now,
				},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := newRealGitRepository(t, remote)
	privateFilename := "customer-private-roadmap.md"
	if err := os.WriteFile(filepath.Join(root, privateFilename), []byte("private content"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pactclient.New(server.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &mcpRuntime{
		binding: localproject.Binding{Root: root, ServerURL: server.URL, ProjectID: projectID},
		client:  client,
		session: agentsession.Session{
			ID: sessionID, ProjectID: projectID, ActorID: "actor-1", ActorName: "Codex",
			NodeID: "node-1", NodeName: "Test node", ClientType: "codex-mcp",
			StartedAt: now, ExpiresAt: now.Add(time.Minute),
		},
	}
	mcpServer := newMCPServer(runtime, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "pact-test", Version: "v0.0.0"}, nil)
	clientSession, err := mcpClient.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	})

	toolNames := make(map[string]bool)
	for tool, toolErr := range clientSession.Tools(context.Background(), nil) {
		if toolErr != nil {
			t.Fatal(toolErr)
		}
		toolNames[tool.Name] = true
	}
	for _, expected := range []string{
		"pact.project_context", "pact.list_projects", "pact.list_workspaces", "pact.refresh_git_observation",
		"pact.workspace_context", "pact.list_resources", "pact.add_resource", "pact.list_records",
		"pact.propose_record", "pact.review_record", "pact.check_scopes", "pact.start_work",
		"pact.list_work", "pact.update_work", "pact.list_handoffs", "pact.offer_handoff",
		"pact.update_handoff", "pact.compile_context_pack", "pact.get_context_pack",
		"pact.list_repositories", "pact.get_repository_sync", "pact.sync_repository",
	} {
		if !toolNames[expected] {
			t.Errorf("MCP tool %q was not registered", expected)
		}
	}

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pact.project_context"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("project context returned a tool error: %#v", result.Content)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	contextJSON := string(encoded)
	for _, expected := range []string{
		"Footfall", "Footfall Product", "Shared product context", "Use PostgreSQL", "One durable shared store", "codex-mcp", "Improve API", "internal/api", "pact/intent-improve-api",
		`"changed_paths":1`, `"remote_url":"[REDACTED]"`, `"api_token":"[REDACTED]"`,
	} {
		if !strings.Contains(contextJSON, expected) {
			t.Errorf("project context does not contain %q: %s", expected, contextJSON)
		}
	}
	for _, forbidden := range []string{secret, "embedded-secret", "must-not-leak", privatePath, root, privateFilename, "private content", "private-token-id"} {
		if strings.Contains(contextJSON, forbidden) {
			t.Errorf("project context leaked %q: %s", forbidden, contextJSON)
		}
	}

	listResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pact.list_projects"})
	if err != nil || listResult.IsError {
		t.Fatalf("list projects error = %v; result = %#v", err, listResult)
	}
	listJSON, err := json.Marshal(listResult.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(listJSON), "embedded-secret") || strings.Contains(string(listJSON), "remote_url") {
		t.Fatalf("project list leaked the remote URL: %s", listJSON)
	}
	workspaceResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pact.list_workspaces"})
	if err != nil || workspaceResult.IsError {
		t.Fatalf("list workspaces error = %v; result = %#v", err, workspaceResult)
	}
	workspaceJSON, err := json.Marshal(workspaceResult.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(workspaceJSON), "embedded-secret") || strings.Contains(string(workspaceJSON), "remote_url") {
		t.Fatalf("workspace list leaked the remote URL: %s", workspaceJSON)
	}

	refreshResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "pact.refresh_git_observation"})
	if err != nil || refreshResult.IsError {
		t.Fatalf("refresh observation error = %v; result = %#v", err, refreshResult)
	}
	refreshJSON, err := json.Marshal(refreshResult.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !observationReceived.Load() || !strings.Contains(string(refreshJSON), `"changed_paths":1`) {
		t.Fatalf("observation received = %v; result = %s", observationReceived.Load(), refreshJSON)
	}
	for _, forbidden := range []string{root, privateFilename, "private content"} {
		if strings.Contains(string(refreshJSON), forbidden) {
			t.Errorf("observation result leaked %q: %s", forbidden, refreshJSON)
		}
	}
}

func TestSanitizeEventDataRecurses(t *testing.T) {
	clean := sanitizeEventData(map[string]any{
		"items":         []any{map[string]any{"password": "hidden", "safe": "visible"}},
		"client-secret": "hidden", "count": float64(2),
	})
	encoded, err := json.Marshal(clean)
	if err != nil {
		t.Fatal(err)
	}
	got := string(encoded)
	if strings.Contains(got, "hidden") || !strings.Contains(got, "visible") || strings.Count(got, "[REDACTED]") != 2 {
		t.Fatalf("sanitized data = %s", got)
	}
}
