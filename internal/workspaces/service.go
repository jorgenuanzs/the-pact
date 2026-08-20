package workspaces

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLength        = 120
	maxSlugLength        = 63
	maxDescriptionLength = 4000
	maxIdempotencySize   = 200
	DefaultColor         = "#c9ee4d"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

type Repository interface {
	Create(context.Context, string, string, [sha256.Size]byte, CreateInput) (CreateResult, error)
	Get(context.Context, string, string) (Workspace, error)
	List(context.Context, string) ([]Workspace, error)
	Update(context.Context, string, string, UpdateInput) (Workspace, error)
	AttachProject(context.Context, string, string, string) (Workspace, error)
}

type Service struct {
	organizationID string
	repository     Repository
}

func NewService(organizationID string, repository Repository) *Service {
	return &Service{organizationID: organizationID, repository: repository}
}

func (s *Service) Create(ctx context.Context, idempotencyKey string, input CreateInput) (CreateResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Description = strings.TrimSpace(input.Description)
	input.Color = normalizeColor(input.Color)
	input.ProjectIDs = normalizeProjectIDs(input.ProjectIDs)
	if err := validateCreate(idempotencyKey, input); err != nil {
		return CreateResult{}, err
	}
	canonical, err := json.Marshal(struct {
		Operation      string      `json:"operation"`
		OrganizationID string      `json:"organization_id"`
		Input          CreateInput `json:"input"`
	}{Operation: "workspace.create", OrganizationID: s.organizationID, Input: input})
	if err != nil {
		return CreateResult{}, err
	}
	return s.repository.Create(ctx, s.organizationID, idempotencyKey, sha256.Sum256(canonical), input)
}

func (s *Service) Get(ctx context.Context, reference string) (Workspace, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" || (!validUUID(reference) && !slugPattern.MatchString(reference)) {
		return Workspace{}, &ValidationError{Field: "workspace_id", Message: "must be a UUID or workspace slug"}
	}
	return s.repository.Get(ctx, s.organizationID, reference)
}

func (s *Service) List(ctx context.Context) ([]Workspace, error) {
	return s.repository.List(ctx, s.organizationID)
}

func (s *Service) Update(ctx context.Context, reference string, input UpdateInput) (Workspace, error) {
	reference = strings.TrimSpace(reference)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Color = normalizeColor(input.Color)
	if reference == "" || (!validUUID(reference) && !slugPattern.MatchString(reference)) {
		return Workspace{}, &ValidationError{Field: "workspace_id", Message: "must be a UUID or workspace slug"}
	}
	if err := validateWorkspaceDetails(input.Name, input.Description, input.Color); err != nil {
		return Workspace{}, err
	}
	return s.repository.Update(ctx, s.organizationID, reference, input)
}

func (s *Service) AttachProject(ctx context.Context, workspaceID, projectID string) (Workspace, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	projectID = strings.TrimSpace(projectID)
	if !validUUID(workspaceID) {
		return Workspace{}, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	if !validUUID(projectID) {
		return Workspace{}, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	return s.repository.AttachProject(ctx, s.organizationID, workspaceID, projectID)
}

func validateCreate(idempotencyKey string, input CreateInput) error {
	switch {
	case idempotencyKey == "":
		return &ValidationError{Field: "Idempotency-Key", Message: "header is required"}
	case len(idempotencyKey) > maxIdempotencySize:
		return &ValidationError{Field: "Idempotency-Key", Message: "must contain at most 200 characters"}
	case input.Slug == "":
		return &ValidationError{Field: "slug", Message: "is required"}
	case len(input.Slug) > maxSlugLength || !slugPattern.MatchString(input.Slug):
		return &ValidationError{Field: "slug", Message: "must use at most 63 lowercase letters, numbers, and single hyphens"}
	}
	if err := validateWorkspaceDetails(input.Name, input.Description, input.Color); err != nil {
		return err
	}
	for _, projectID := range input.ProjectIDs {
		if !validUUID(projectID) {
			return &ValidationError{Field: "project_ids", Message: "must contain only UUIDs"}
		}
	}
	return nil
}

func validateWorkspaceDetails(name, description, color string) error {
	switch {
	case name == "":
		return &ValidationError{Field: "name", Message: "is required"}
	case utf8.RuneCountInString(name) > maxNameLength:
		return &ValidationError{Field: "name", Message: "must contain at most 120 characters"}
	case utf8.RuneCountInString(description) > maxDescriptionLength:
		return &ValidationError{Field: "description", Message: "must contain at most 4000 characters"}
	case !colorPattern.MatchString(color):
		return &ValidationError{Field: "color", Message: "must be a six-digit hexadecimal color"}
	default:
		return nil
	}
}

func normalizeColor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DefaultColor
	}
	return value
}

func normalizeProjectIDs(values []string) []string {
	if values == nil {
		return make([]string, 0)
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
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
