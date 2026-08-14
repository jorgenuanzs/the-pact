package repositorysync

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jorgenuanzs/the-pact/internal/projects"
)

const maxIdempotencyLength = 200

type ProjectCatalog interface {
	Get(context.Context, string) (projects.Project, error)
	List(context.Context) ([]projects.Project, error)
}

type Repository interface {
	Get(context.Context, string, string, string) (State, bool, error)
	Replay(context.Context, string, string, string, [sha256.Size]byte) (Result, bool, error)
	Apply(context.Context, string, string, string, string, string, [sha256.Size]byte, Snapshot) (Result, error)
	ApplyScheduled(context.Context, string, string, string, Snapshot) (Result, error)
	RecordFailure(context.Context, string, string, string, string, string) (State, error)
}

type Service struct {
	organizationID string
	projects       ProjectCatalog
	repository     Repository
	provider       Provider
}

func NewService(organizationID string, projectCatalog ProjectCatalog, repository Repository, provider Provider) *Service {
	return &Service{
		organizationID: organizationID,
		projects:       projectCatalog,
		repository:     repository,
		provider:       provider,
	}
}

func (s *Service) Get(ctx context.Context, projectID string) (State, error) {
	projectID = strings.TrimSpace(projectID)
	if !validUUID(projectID) {
		return State{}, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return State{}, err
	}
	repository, reference, state := describeProjectRepository(project)
	if repository == nil {
		return state, nil
	}
	if reference.FullName == "" {
		return state, nil
	}
	stored, found, err := s.repository.Get(ctx, s.organizationID, project.ID, repository.ID)
	if err != nil {
		return State{}, err
	}
	if found {
		return stored, nil
	}
	return state, nil
}

func (s *Service) Sync(
	ctx context.Context, principalID, projectID, idempotencyKey string,
) (Result, error) {
	principalID = strings.TrimSpace(principalID)
	projectID = strings.TrimSpace(projectID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if !validUUID(principalID) {
		return Result{}, &ValidationError{Field: "principal_id", Message: "must be a UUID"}
	}
	if !validUUID(projectID) {
		return Result{}, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	if idempotencyKey == "" {
		return Result{}, &ValidationError{Field: "Idempotency-Key", Message: "header is required"}
	}
	if len(idempotencyKey) > maxIdempotencyLength {
		return Result{}, &ValidationError{Field: "Idempotency-Key", Message: "must contain at most 200 characters"}
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return Result{}, err
	}
	repository, reference, err := syncableRepository(project)
	if err != nil {
		return Result{}, err
	}
	hash, err := syncCommandHash(s.organizationID, project.ID, repository.ID)
	if err != nil {
		return Result{}, err
	}
	if replay, found, err := s.repository.Replay(
		ctx, s.organizationID, project.ID, idempotencyKey, hash,
	); err != nil {
		return Result{}, err
	} else if found {
		replay.Replayed = true
		return replay, nil
	}
	snapshot, err := s.provider.Fetch(ctx, reference)
	if err != nil {
		if recordErr := s.recordFailure(ctx, project.ID, repository.ID, reference.FullName, err); recordErr != nil {
			return Result{}, errors.Join(err, fmt.Errorf("record repository sync failure: %w", recordErr))
		}
		return Result{}, err
	}
	return s.repository.Apply(
		ctx, s.organizationID, principalID, project.ID, repository.ID,
		idempotencyKey, hash, snapshot,
	)
}

func (s *Service) SyncScheduled(ctx context.Context, projectID string) (Result, error) {
	projectID = strings.TrimSpace(projectID)
	if !validUUID(projectID) {
		return Result{}, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return Result{}, err
	}
	repository, reference, err := syncableRepository(project)
	if err != nil {
		return Result{}, err
	}
	snapshot, err := s.provider.Fetch(ctx, reference)
	if err != nil {
		if recordErr := s.recordFailure(ctx, project.ID, repository.ID, reference.FullName, err); recordErr != nil {
			return Result{}, errors.Join(err, fmt.Errorf("record repository sync failure: %w", recordErr))
		}
		return Result{}, err
	}
	return s.repository.ApplyScheduled(ctx, s.organizationID, project.ID, repository.ID, snapshot)
}

func (s *Service) ListProjects(ctx context.Context) ([]projects.Project, error) {
	return s.projects.List(ctx)
}

func (s *Service) recordFailure(ctx context.Context, projectID, repositoryID, fullName string, providerErr error) error {
	var failure *ProviderError
	if !errors.As(providerErr, &failure) {
		return nil
	}
	_, err := s.repository.RecordFailure(
		ctx, s.organizationID, projectID, repositoryID, fullName, failure.Code,
	)
	return err
}

func describeProjectRepository(project projects.Project) (*projects.SourceRepository, Reference, State) {
	state := State{
		ProjectID: project.ID, Provider: "unknown", Status: StatusUnavailable,
		Visibility: "unknown", CanonicalRevision: project.CanonicalRevision,
	}
	if project.RootRepository == nil || project.RootRepository.Status != "active" || project.RootRepository.RemoteURL == nil {
		return nil, Reference{}, state
	}
	repository := project.RootRepository
	state.RepositoryID = repository.ID
	state.DefaultBranch = repository.DefaultBranch
	reference, err := ParseGitHubRemote(*repository.RemoteURL)
	if err != nil {
		state.Status = StatusUnsupported
		return repository, Reference{}, state
	}
	state.Provider = "github"
	state.RepositoryFullName = reference.FullName
	state.Status = StatusNever
	return repository, reference, state
}

func syncableRepository(project projects.Project) (*projects.SourceRepository, Reference, error) {
	if project.RootRepository == nil || project.RootRepository.Status != "active" || project.RootRepository.RemoteURL == nil {
		return nil, Reference{}, ErrRepositoryUnavailable
	}
	reference, err := ParseGitHubRemote(*project.RootRepository.RemoteURL)
	if err != nil {
		return nil, Reference{}, err
	}
	return project.RootRepository, reference, nil
}

func syncCommandHash(organizationID, projectID, repositoryID string) ([sha256.Size]byte, error) {
	body, err := json.Marshal(struct {
		Operation      string `json:"operation"`
		OrganizationID string `json:"organization_id"`
		ProjectID      string `json:"project_id"`
		RepositoryID   string `json:"repository_id"`
	}{
		Operation: "repository.sync", OrganizationID: organizationID,
		ProjectID: projectID, RepositoryID: repositoryID,
	})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(body), nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		switch index {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return false
			}
		}
	}
	return true
}
