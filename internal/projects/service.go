package projects

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLength      = 120
	maxSlugLength      = 63
	maxRevisionLength  = 64
	maxIdempotencySize = 200
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var revisionPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

type Repository interface {
	Create(context.Context, string, string, [sha256.Size]byte, CreateInput) (CreateResult, error)
	Get(context.Context, string, string) (Project, error)
}

type Service struct {
	organizationID string
	repository     Repository
}

func NewService(organizationID string, repository Repository) *Service {
	return &Service{
		organizationID: organizationID,
		repository:     repository,
	}
}

func (s *Service) Create(ctx context.Context, idempotencyKey string, input CreateInput) (CreateResult, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.TrimSpace(input.Slug)
	if input.CanonicalRevision != nil {
		trimmed := strings.ToLower(strings.TrimSpace(*input.CanonicalRevision))
		input.CanonicalRevision = &trimmed
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	if err := validateInput(idempotencyKey, input); err != nil {
		return CreateResult{}, err
	}

	canonical, err := json.Marshal(struct {
		Operation      string      `json:"operation"`
		OrganizationID string      `json:"organization_id"`
		Input          CreateInput `json:"input"`
	}{
		Operation:      "project.create",
		OrganizationID: s.organizationID,
		Input:          input,
	})
	if err != nil {
		return CreateResult{}, err
	}

	return s.repository.Create(ctx, s.organizationID, idempotencyKey, sha256.Sum256(canonical), input)
}

func (s *Service) Get(ctx context.Context, projectID string) (Project, error) {
	projectID = strings.TrimSpace(projectID)
	if !validUUID(projectID) {
		return Project{}, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	return s.repository.Get(ctx, s.organizationID, projectID)
}

func validateInput(idempotencyKey string, input CreateInput) error {
	switch {
	case idempotencyKey == "":
		return &ValidationError{Field: "Idempotency-Key", Message: "header is required"}
	case len(idempotencyKey) > maxIdempotencySize:
		return &ValidationError{Field: "Idempotency-Key", Message: "must contain at most 200 characters"}
	case input.Name == "":
		return &ValidationError{Field: "name", Message: "is required"}
	case utf8.RuneCountInString(input.Name) > maxNameLength:
		return &ValidationError{Field: "name", Message: "must contain at most 120 characters"}
	case input.Slug == "":
		return &ValidationError{Field: "slug", Message: "is required"}
	case len(input.Slug) > maxSlugLength:
		return &ValidationError{Field: "slug", Message: "must contain at most 63 characters"}
	case !slugPattern.MatchString(input.Slug):
		return &ValidationError{Field: "slug", Message: "must use lowercase letters, numbers, and single hyphens"}
	case input.CanonicalRevision != nil && *input.CanonicalRevision == "":
		return &ValidationError{Field: "canonical_revision", Message: "must not be empty when provided"}
	case input.CanonicalRevision != nil && len(*input.CanonicalRevision) > maxRevisionLength:
		return &ValidationError{Field: "canonical_revision", Message: "must contain at most 64 characters"}
	case input.CanonicalRevision != nil && !revisionPattern.MatchString(*input.CanonicalRevision):
		return &ValidationError{Field: "canonical_revision", Message: "must be a hexadecimal Git object ID with 7 to 64 characters"}
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, char := range value {
		switch i {
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
