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

type RepositoryCatalog interface {
	GetSource(context.Context, string, string) (projects.SourceRepository, error)
	ListSources(context.Context, string) ([]projects.SourceRepository, error)
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
	repositories   RepositoryCatalog
	repository     Repository
	provider       Provider
}

func NewService(
	organizationID string, projectCatalog ProjectCatalog, repository Repository, provider Provider,
	repositoryCatalog ...RepositoryCatalog,
) *Service {
	service := &Service{
		organizationID: organizationID, projects: projectCatalog,
		repository: repository, provider: provider,
	}
	if len(repositoryCatalog) > 0 {
		service.repositories = repositoryCatalog[0]
	}
	return service
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
	if project.RootRepository == nil {
		return unavailableState(project.ID, project.CanonicalRevision), nil
	}
	return s.get(ctx, project, *project.RootRepository)
}

func (s *Service) GetRepository(
	ctx context.Context, projectID, repositoryID string,
) (State, error) {
	projectID = strings.TrimSpace(projectID)
	repositoryID = strings.TrimSpace(repositoryID)
	if !validUUID(projectID) || !validUUID(repositoryID) {
		return State{}, &ValidationError{Field: "repository_id", Message: "project and repository IDs must be UUIDs"}
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return State{}, err
	}
	if project.RootRepository != nil && project.RootRepository.ID == repositoryID {
		return s.get(ctx, project, *project.RootRepository)
	}
	if s.repositories == nil {
		return State{}, ErrRepositoryUnavailable
	}
	repository, err := s.repositories.GetSource(ctx, projectID, repositoryID)
	if err != nil {
		return State{}, err
	}
	return s.get(ctx, project, repository)
}

func (s *Service) List(ctx context.Context, projectID string) ([]State, error) {
	projectID = strings.TrimSpace(projectID)
	if !validUUID(projectID) {
		return nil, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var repositories []projects.SourceRepository
	if s.repositories != nil {
		repositories, err = s.repositories.ListSources(ctx, projectID)
		if err != nil {
			return nil, err
		}
	} else if project.RootRepository != nil {
		repositories = []projects.SourceRepository{*project.RootRepository}
	}
	states := make([]State, 0, len(repositories))
	for _, repository := range repositories {
		state, err := s.get(ctx, project, repository)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (s *Service) get(ctx context.Context, project projects.Project, repository projects.SourceRepository) (State, error) {
	reference, state := describeRepository(project, repository)
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
	project, repository, err := s.rootRepository(ctx, projectID)
	if err != nil {
		return Result{}, err
	}
	return s.sync(ctx, principalID, project, repository, idempotencyKey)
}

func (s *Service) SyncRepository(
	ctx context.Context, principalID, projectID, repositoryID, idempotencyKey string,
) (Result, error) {
	projectID = strings.TrimSpace(projectID)
	repositoryID = strings.TrimSpace(repositoryID)
	if !validUUID(projectID) || !validUUID(repositoryID) {
		return Result{}, &ValidationError{Field: "repository_id", Message: "project and repository IDs must be UUIDs"}
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return Result{}, err
	}
	var repository projects.SourceRepository
	if project.RootRepository != nil && project.RootRepository.ID == repositoryID {
		repository = *project.RootRepository
	} else if s.repositories != nil {
		repository, err = s.repositories.GetSource(ctx, projectID, repositoryID)
		if err != nil {
			return Result{}, err
		}
	} else {
		return Result{}, ErrRepositoryUnavailable
	}
	return s.sync(ctx, principalID, project, repository, idempotencyKey)
}

func (s *Service) sync(
	ctx context.Context, principalID string, project projects.Project,
	repository projects.SourceRepository, idempotencyKey string,
) (Result, error) {
	principalID = strings.TrimSpace(principalID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if !validUUID(principalID) {
		return Result{}, &ValidationError{Field: "principal_id", Message: "must be a UUID"}
	}
	if idempotencyKey == "" {
		return Result{}, &ValidationError{Field: "Idempotency-Key", Message: "header is required"}
	}
	if len(idempotencyKey) > maxIdempotencyLength {
		return Result{}, &ValidationError{Field: "Idempotency-Key", Message: "must contain at most 200 characters"}
	}
	reference, err := syncableRepository(repository)
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
	project, repository, err := s.rootRepository(ctx, projectID)
	if err != nil {
		return Result{}, err
	}
	return s.syncScheduled(ctx, project, repository)
}

func (s *Service) SyncScheduledProject(ctx context.Context, projectID string) ([]Result, error) {
	projectID = strings.TrimSpace(projectID)
	if !validUUID(projectID) {
		return nil, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var repositories []projects.SourceRepository
	if s.repositories != nil {
		repositories, err = s.repositories.ListSources(ctx, projectID)
		if err != nil {
			return nil, err
		}
	} else if project.RootRepository != nil {
		repositories = []projects.SourceRepository{*project.RootRepository}
	}
	results := make([]Result, 0, len(repositories))
	var syncErr error
	for _, repository := range repositories {
		result, err := s.syncScheduled(ctx, project, repository)
		if errors.Is(err, ErrUnsupportedRemote) || errors.Is(err, ErrRepositoryUnavailable) {
			continue
		}
		if err != nil {
			syncErr = errors.Join(syncErr, fmt.Errorf("repository %s: %w", repository.ID, err))
			continue
		}
		results = append(results, result)
	}
	return results, syncErr
}

func (s *Service) syncScheduled(
	ctx context.Context, project projects.Project, repository projects.SourceRepository,
) (Result, error) {
	reference, err := syncableRepository(repository)
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

func (s *Service) rootRepository(ctx context.Context, projectID string) (projects.Project, projects.SourceRepository, error) {
	projectID = strings.TrimSpace(projectID)
	if !validUUID(projectID) {
		return projects.Project{}, projects.SourceRepository{}, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return projects.Project{}, projects.SourceRepository{}, err
	}
	if project.RootRepository == nil {
		return projects.Project{}, projects.SourceRepository{}, ErrRepositoryUnavailable
	}
	return project, *project.RootRepository, nil
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

func unavailableState(projectID string, revision *string) State {
	return State{
		ProjectID: projectID, Provider: "unknown", Status: StatusUnavailable,
		Visibility: "unknown", CanonicalRevision: revision,
	}
}

func describeRepository(project projects.Project, repository projects.SourceRepository) (Reference, State) {
	revision := (*string)(nil)
	if project.RootRepository != nil && project.RootRepository.ID == repository.ID {
		revision = project.CanonicalRevision
	}
	state := State{
		RepositoryID: repository.ID, ProjectID: project.ID, Provider: "unknown",
		Status: StatusUnavailable, Visibility: "unknown", CanonicalRevision: revision,
		DefaultBranch: repository.DefaultBranch,
	}
	if repository.Status != "active" || repository.RemoteURL == nil {
		return Reference{}, state
	}
	reference, err := ParseGitHubRemote(*repository.RemoteURL)
	if err != nil {
		state.Status = StatusUnsupported
		return Reference{}, state
	}
	state.Provider = "github"
	state.RepositoryFullName = reference.FullName
	state.Status = StatusNever
	return reference, state
}

func syncableRepository(repository projects.SourceRepository) (Reference, error) {
	if repository.Status != "active" || repository.RemoteURL == nil {
		return Reference{}, ErrRepositoryUnavailable
	}
	return ParseGitHubRemote(*repository.RemoteURL)
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
