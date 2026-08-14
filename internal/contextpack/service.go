package contextpack

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"

	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
	"github.com/jorgenuanzs/the-pact/internal/projectrepo"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

const maxIdempotencyLength = 200

type Repository interface {
	Create(context.Context, string, string, bool, string, [sha256.Size]byte, CompileInput, Draft) (CompileResult, error)
	Get(context.Context, string, string, string) (ContextPack, error)
}

type ProjectReader interface {
	Get(context.Context, string) (projects.Project, error)
}

type WorkspaceReader interface {
	List(context.Context) ([]workspaces.Workspace, error)
}

type CoordinationReader interface {
	List(context.Context, string) ([]coordination.WorkItem, error)
	ListHandoffs(context.Context, string, string) ([]coordination.Handoff, error)
}

type KnowledgeReader interface {
	Context(context.Context, string) (knowledge.WorkspaceContext, error)
}

type ProjectRepositoryReader interface {
	List(context.Context, string) ([]projectrepo.Repository, error)
}

type Service struct {
	organizationID string
	repository     Repository
	projects       ProjectReader
	workspaces     WorkspaceReader
	coordination   CoordinationReader
	knowledge      KnowledgeReader
	repositories   ProjectRepositoryReader
}

func NewService(
	organizationID string,
	repository Repository,
	projects ProjectReader,
	workspaces WorkspaceReader,
	coordination CoordinationReader,
	knowledge KnowledgeReader,
	repositoryReaders ...ProjectRepositoryReader,
) *Service {
	service := &Service{
		organizationID: organizationID, repository: repository, projects: projects,
		workspaces: workspaces, coordination: coordination, knowledge: knowledge,
	}
	if len(repositoryReaders) > 0 {
		service.repositories = repositoryReaders[0]
	}
	return service
}

func (s *Service) Compile(
	ctx context.Context,
	principalID string,
	allowAll bool,
	projectID, intentID, key string,
	input CompileInput,
) (CompileResult, error) {
	principalID, projectID, intentID = strings.TrimSpace(principalID), strings.TrimSpace(projectID), strings.TrimSpace(intentID)
	key, input.SessionID = strings.TrimSpace(key), strings.TrimSpace(input.SessionID)
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	if input.Type == "" {
		input.Type = "implementation"
	}
	if !validUUID(principalID) || !validUUID(projectID) || !validUUID(intentID) || (input.SessionID != "" && !validUUID(input.SessionID)) {
		return CompileResult{}, &ValidationError{Field: "identity", Message: "principal, project, intent, and optional session IDs must be UUIDs"}
	}
	if key == "" || len(key) > maxIdempotencyLength {
		return CompileResult{}, &ValidationError{Field: "Idempotency-Key", Message: "must contain 1 to 200 characters"}
	}
	if !allowedPackType(input.Type) {
		return CompileResult{}, &ValidationError{Field: "type", Message: "is not a supported context pack type"}
	}
	if input.TTLMinutes == 0 {
		input.TTLMinutes = 5
	}
	if input.TTLMinutes < 1 || input.TTLMinutes > 60 {
		return CompileResult{}, &ValidationError{Field: "ttl_minutes", Message: "must be between 1 and 60"}
	}

	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return CompileResult{}, err
	}
	repositorySnapshots := make([]RepositorySnapshot, 0)
	if s.repositories != nil {
		repositories, err := s.repositories.List(ctx, projectID)
		if err != nil {
			return CompileResult{}, err
		}
		for _, repository := range repositories {
			repositorySnapshots = append(repositorySnapshots, RepositorySnapshot{
				ID: repository.ID, Name: repository.Name, FullName: repository.GitHubFullName,
				Purpose: repository.Purpose, Primary: repository.Primary, Required: repository.Required,
				DefaultBranch: repository.DefaultBranch, CanonicalRevision: repository.CanonicalRevision,
				SyncStatus: repository.SyncStatus, LastSuccessAt: repository.LastSuccessAt,
			})
		}
	}
	workspaceList, err := s.workspaces.List(ctx)
	if err != nil {
		return CompileResult{}, err
	}
	workspace, found := findWorkspace(workspaceList, projectID)
	if !found {
		return CompileResult{}, ErrNotFound
	}
	items, err := s.coordination.List(ctx, projectID)
	if err != nil {
		return CompileResult{}, err
	}
	var intent coordination.Intent
	found = false
	activeWork := make([]WorkSnapshot, 0, len(items))
	for _, item := range items {
		if item.Intent.ID == intentID {
			intent, found = item.Intent, true
		}
		if item.Intent.Status == "active" || item.Intent.Status == "blocked" || item.Intent.Status == "submitted" || item.Intent.ID == intentID {
			activeWork = append(activeWork, workSnapshot(item))
		}
	}
	if !found {
		return CompileResult{}, ErrNotFound
	}
	knowledgeContext, err := s.knowledge.Context(ctx, workspace.ID)
	if err != nil {
		return CompileResult{}, err
	}
	handoffs, err := s.coordination.ListHandoffs(ctx, projectID, intentID)
	if err != nil {
		return CompileResult{}, err
	}
	warnings := append([]string{}, knowledgeContext.Warnings...)
	if project.CanonicalRevision != nil && *project.CanonicalRevision != "" && *project.CanonicalRevision != intent.BaseRevision {
		warnings = append(warnings, "BASE_REVISION_STALE: the intent base differs from the current canonical project revision")
	}
	for _, handoff := range handoffs {
		if handoff.Status == "offered" {
			warnings = append(warnings, "HANDOFF_OFFERED: an unaccepted handoff exists for this intent")
			break
		}
	}
	draft := Draft{
		Type: input.Type, WorkspaceID: workspace.ID, ProjectID: projectID, IntentID: intentID,
		Project: ProjectSnapshot{ID: project.ID, Name: project.Name, Slug: project.Slug,
			Status: project.Status, CanonicalRevision: project.CanonicalRevision,
			Version: project.Version, Repositories: repositorySnapshots},
		Workspace: WorkspaceSnapshot{ID: workspace.ID, Name: workspace.Name, Slug: workspace.Slug,
			Status: workspace.Status, Version: workspace.Version},
		Intent: intent, ActiveWork: activeWork, Knowledge: knowledgeContext,
		Handoffs: handoffs, Warnings: warnings,
	}
	hash, err := compileHash(s.organizationID, projectID, intentID, input)
	if err != nil {
		return CompileResult{}, err
	}
	return s.repository.Create(ctx, s.organizationID, principalID, allowAll, key, hash, input, draft)
}

func (s *Service) Get(ctx context.Context, projectID, packID string) (ContextPack, error) {
	projectID, packID = strings.TrimSpace(projectID), strings.TrimSpace(packID)
	if !validUUID(projectID) || !validUUID(packID) {
		return ContextPack{}, &ValidationError{Field: "context_pack_id", Message: "project and context pack IDs must be UUIDs"}
	}
	return s.repository.Get(ctx, s.organizationID, projectID, packID)
}

func findWorkspace(values []workspaces.Workspace, projectID string) (workspaces.Workspace, bool) {
	for _, workspace := range values {
		for _, project := range workspace.Projects {
			if project.ID == projectID {
				return workspace, true
			}
		}
	}
	return workspaces.Workspace{}, false
}

func workSnapshot(item coordination.WorkItem) WorkSnapshot {
	result := WorkSnapshot{
		Intent: item.Intent, ResponsibleName: item.ResponsibleName, Scopes: item.Scopes,
		SessionLive: item.SessionLive, SessionLastSeen: item.SessionLastSeen,
	}
	if item.Workspace != nil {
		result.Worktree = &WorktreeSnapshot{
			ID: item.Workspace.ID, IntentID: item.Workspace.IntentID,
			BaseRevision: item.Workspace.BaseRevision, GitBranch: item.Workspace.GitBranch,
			Status: item.Workspace.Status, Version: item.Workspace.Version,
		}
	}
	return result
}

func compileHash(organizationID, projectID, intentID string, input CompileInput) ([sha256.Size]byte, error) {
	body, err := json.Marshal(struct {
		Operation      string       `json:"operation"`
		OrganizationID string       `json:"organization_id"`
		ProjectID      string       `json:"project_id"`
		IntentID       string       `json:"intent_id"`
		Input          CompileInput `json:"input"`
	}{"context.compile", organizationID, projectID, intentID, input})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(body), nil
}

func allowedPackType(value string) bool {
	switch value {
	case "implementation", "handoff", "review", "onboarding", "meeting", "incident", "deployment":
		return true
	default:
		return false
	}
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
				return false
			}
		}
	}
	return true
}
