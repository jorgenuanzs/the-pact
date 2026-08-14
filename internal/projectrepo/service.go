package projectrepo

import (
	"context"
	"regexp"
	"strings"

	"github.com/jorgenuanzs/the-pact/internal/projects"
)

var purposePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type Store interface {
	List(context.Context, string, string) ([]Repository, error)
	ListAvailable(context.Context, string, string) ([]AvailableRepository, error)
	Attach(context.Context, string, string, string, AttachInput) (Repository, error)
	GetSource(context.Context, string, string, string) (projects.SourceRepository, error)
	ListSources(context.Context, string, string) ([]projects.SourceRepository, error)
}

type Service struct {
	organizationID string
	store          Store
}

func NewService(organizationID string, store Store) *Service {
	return &Service{organizationID: organizationID, store: store}
}

func (s *Service) List(ctx context.Context, projectID string) ([]Repository, error) {
	if !validUUID(projectID) {
		return nil, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	return s.store.List(ctx, s.organizationID, projectID)
}

func (s *Service) ListAvailable(ctx context.Context, projectID string) ([]AvailableRepository, error) {
	if !validUUID(projectID) {
		return nil, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	return s.store.ListAvailable(ctx, s.organizationID, projectID)
}

func (s *Service) Attach(ctx context.Context, principalID, projectID string, input AttachInput) (Repository, error) {
	if !validUUID(principalID) {
		return Repository{}, &ValidationError{Field: "principal_id", Message: "must be a UUID"}
	}
	if !validUUID(projectID) {
		return Repository{}, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	if input.GitHubRepositoryID <= 0 {
		return Repository{}, &ValidationError{Field: "github_repository_id", Message: "must be a positive integer"}
	}
	input.Purpose = strings.ToLower(strings.TrimSpace(input.Purpose))
	if input.Purpose == "" {
		input.Purpose = "application"
	}
	if !purposePattern.MatchString(input.Purpose) {
		return Repository{}, &ValidationError{Field: "purpose", Message: "must start with a letter and contain only lowercase letters, numbers, underscores, or hyphens"}
	}
	return s.store.Attach(ctx, s.organizationID, principalID, projectID, input)
}

func (s *Service) GetSource(
	ctx context.Context, projectID, repositoryID string,
) (projects.SourceRepository, error) {
	if !validUUID(projectID) || !validUUID(repositoryID) {
		return projects.SourceRepository{}, &ValidationError{Field: "repository_id", Message: "project and repository IDs must be UUIDs"}
	}
	return s.store.GetSource(ctx, s.organizationID, projectID, repositoryID)
}

func (s *Service) ListSources(ctx context.Context, projectID string) ([]projects.SourceRepository, error) {
	if !validUUID(projectID) {
		return nil, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	return s.store.ListSources(ctx, s.organizationID, projectID)
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
