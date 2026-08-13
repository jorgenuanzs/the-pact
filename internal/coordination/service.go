package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxIdempotencyLength = 200

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

type Repository interface {
	CheckScopes(context.Context, string, string, []ScopeInput) (ScopeCheckResult, error)
	Start(context.Context, string, string, bool, string, string, [sha256.Size]byte, StartInput) (StartResult, error)
	AttachWorkspace(context.Context, string, string, bool, string, string, [sha256.Size]byte, WorkspaceInput) (WorkspaceResult, error)
	UpdateStatus(context.Context, string, string, bool, string, string, [sha256.Size]byte, StatusInput) (StatusResult, error)
	List(context.Context, string, string) ([]WorkItem, error)
}

type Service struct {
	organizationID string
	repository     Repository
}

func NewService(organizationID string, repository Repository) *Service {
	return &Service{organizationID: organizationID, repository: repository}
}

func (s *Service) CheckScopes(ctx context.Context, projectID string, scopes []ScopeInput) (ScopeCheckResult, error) {
	projectID = strings.TrimSpace(projectID)
	if !validUUID(projectID) {
		return ScopeCheckResult{}, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	normalized, err := normalizeScopes(scopes)
	if err != nil {
		return ScopeCheckResult{}, err
	}
	return s.repository.CheckScopes(ctx, s.organizationID, projectID, normalized)
}

func (s *Service) Start(
	ctx context.Context,
	principalID string,
	allowAll bool,
	projectID string,
	idempotencyKey string,
	input StartInput,
) (StartResult, error) {
	principalID = strings.TrimSpace(principalID)
	projectID = strings.TrimSpace(projectID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.Title = strings.TrimSpace(input.Title)
	input.Goal = strings.TrimSpace(input.Goal)
	input.BaseRevision = strings.ToLower(strings.TrimSpace(input.BaseRevision))
	for index := range input.SuccessCriteria {
		input.SuccessCriteria[index] = strings.TrimSpace(input.SuccessCriteria[index])
	}
	input.SuccessCriteria = compactStrings(input.SuccessCriteria)

	if err := validateCommandIdentity(principalID, projectID, input.SessionID, idempotencyKey); err != nil {
		return StartResult{}, err
	}
	switch {
	case input.Title == "":
		return StartResult{}, &ValidationError{Field: "title", Message: "is required"}
	case utf8.RuneCountInString(input.Title) > 300:
		return StartResult{}, &ValidationError{Field: "title", Message: "must contain at most 300 characters"}
	case input.Goal == "":
		return StartResult{}, &ValidationError{Field: "goal", Message: "is required"}
	case utf8.RuneCountInString(input.Goal) > 10000:
		return StartResult{}, &ValidationError{Field: "goal", Message: "must contain at most 10000 characters"}
	case !revisionPattern.MatchString(input.BaseRevision):
		return StartResult{}, &ValidationError{Field: "base_revision", Message: "must be a hexadecimal Git object ID with 7 to 64 characters"}
	case len(input.SuccessCriteria) > 50:
		return StartResult{}, &ValidationError{Field: "success_criteria", Message: "must contain at most 50 items"}
	}
	for _, criterion := range input.SuccessCriteria {
		if utf8.RuneCountInString(criterion) > 1000 {
			return StartResult{}, &ValidationError{Field: "success_criteria", Message: "items must contain at most 1000 characters"}
		}
	}
	normalized, err := normalizeScopes(input.Scopes)
	if err != nil {
		return StartResult{}, err
	}
	input.Scopes = normalized
	requestHash, err := commandHash("work.start", s.organizationID, projectID, input)
	if err != nil {
		return StartResult{}, err
	}
	return s.repository.Start(ctx, s.organizationID, principalID, allowAll, projectID, idempotencyKey, requestHash, input)
}

func (s *Service) AttachWorkspace(
	ctx context.Context,
	principalID string,
	allowAll bool,
	intentID string,
	idempotencyKey string,
	input WorkspaceInput,
) (WorkspaceResult, error) {
	principalID = strings.TrimSpace(principalID)
	intentID = strings.TrimSpace(intentID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.BaseRevision = strings.ToLower(strings.TrimSpace(input.BaseRevision))
	input.PathRef = strings.TrimSpace(input.PathRef)
	input.GitBranch = strings.TrimSpace(input.GitBranch)
	if !validUUID(principalID) {
		return WorkspaceResult{}, &ValidationError{Field: "principal_id", Message: "must be a UUID"}
	}
	if !validUUID(intentID) {
		return WorkspaceResult{}, &ValidationError{Field: "intent_id", Message: "must be a UUID"}
	}
	if !validUUID(input.SessionID) {
		return WorkspaceResult{}, &ValidationError{Field: "session_id", Message: "must be a UUID"}
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return WorkspaceResult{}, err
	}
	switch {
	case !revisionPattern.MatchString(input.BaseRevision):
		return WorkspaceResult{}, &ValidationError{Field: "base_revision", Message: "must be a hexadecimal Git object ID with 7 to 64 characters"}
	case input.PathRef == "" || len(input.PathRef) > 4096:
		return WorkspaceResult{}, &ValidationError{Field: "path_ref", Message: "must contain between 1 and 4096 characters"}
	case input.GitBranch == "" || len(input.GitBranch) > 255:
		return WorkspaceResult{}, &ValidationError{Field: "git_branch", Message: "must contain between 1 and 255 characters"}
	}
	requestHash, err := commandHash("workspace.attach", s.organizationID, intentID, input)
	if err != nil {
		return WorkspaceResult{}, err
	}
	return s.repository.AttachWorkspace(ctx, s.organizationID, principalID, allowAll, intentID, idempotencyKey, requestHash, input)
}

func (s *Service) UpdateStatus(
	ctx context.Context,
	principalID string,
	allowAll bool,
	intentID string,
	idempotencyKey string,
	input StatusInput,
) (StatusResult, error) {
	principalID = strings.TrimSpace(principalID)
	intentID = strings.TrimSpace(intentID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Summary = strings.TrimSpace(input.Summary)
	input.Reason = strings.TrimSpace(input.Reason)
	if !validUUID(principalID) {
		return StatusResult{}, &ValidationError{Field: "principal_id", Message: "must be a UUID"}
	}
	if !validUUID(intentID) {
		return StatusResult{}, &ValidationError{Field: "intent_id", Message: "must be a UUID"}
	}
	if !validUUID(input.SessionID) {
		return StatusResult{}, &ValidationError{Field: "session_id", Message: "must be a UUID"}
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return StatusResult{}, err
	}
	if _, ok := allowedTargetStatuses[input.Status]; !ok {
		return StatusResult{}, &ValidationError{Field: "status", Message: "must be active, blocked, submitted, completed, cancelled, or abandoned"}
	}
	if input.ExpectedVersion < 1 {
		return StatusResult{}, &ValidationError{Field: "expected_version", Message: "must be greater than zero"}
	}
	if utf8.RuneCountInString(input.Summary) > 10000 || utf8.RuneCountInString(input.Reason) > 2000 {
		return StatusResult{}, &ValidationError{Field: "summary", Message: "summary or reason is too long"}
	}
	requestHash, err := commandHash("intent.status", s.organizationID, intentID, input)
	if err != nil {
		return StatusResult{}, err
	}
	return s.repository.UpdateStatus(ctx, s.organizationID, principalID, allowAll, intentID, idempotencyKey, requestHash, input)
}

func (s *Service) List(ctx context.Context, projectID string) ([]WorkItem, error) {
	projectID = strings.TrimSpace(projectID)
	if !validUUID(projectID) {
		return nil, &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	return s.repository.List(ctx, s.organizationID, projectID)
}

var allowedTargetStatuses = map[string]struct{}{
	"active": {}, "blocked": {}, "submitted": {}, "completed": {}, "cancelled": {}, "abandoned": {},
}

func normalizeScopes(scopes []ScopeInput) ([]ScopeInput, error) {
	if len(scopes) == 0 || len(scopes) > 50 {
		return nil, &ValidationError{Field: "scopes", Message: "must contain between 1 and 50 items"}
	}
	result := make([]ScopeInput, 0, len(scopes))
	seen := make(map[string]struct{})
	for _, scope := range scopes {
		scope.Kind = strings.ToLower(strings.TrimSpace(scope.Kind))
		scope.Mode = strings.ToLower(strings.TrimSpace(scope.Mode))
		if scope.Mode == "" {
			scope.Mode = ClaimModeExclusive
		}
		if scope.Kind != "repository" && scope.Kind != "path" && scope.Kind != "file" {
			return nil, &ValidationError{Field: "scopes", Message: "scope kinds must be repository, path, or file"}
		}
		if scope.Mode != ClaimModeExclusive && scope.Mode != ClaimModeShared {
			return nil, &ValidationError{Field: "scopes", Message: "scope modes must be exclusive or shared"}
		}
		locator := strings.ReplaceAll(strings.TrimSpace(scope.Locator), "\\", "/")
		if scope.Kind == "repository" {
			locator = "."
		} else {
			if locator == "" || strings.HasPrefix(locator, "/") {
				return nil, &ValidationError{Field: "scopes", Message: "scope locators must be relative repository paths"}
			}
			locator = path.Clean(locator)
			if locator == "." || locator == ".." || strings.HasPrefix(locator, "../") || strings.ContainsRune(locator, 0) {
				return nil, &ValidationError{Field: "scopes", Message: "scope locators must stay inside the repository"}
			}
		}
		if len(locator) > 4096 {
			return nil, &ValidationError{Field: "scopes", Message: "scope locators must contain at most 4096 characters"}
		}
		scope.Locator = locator
		key := scope.Kind + "\x00" + scope.Locator + "\x00" + scope.Mode
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, scope)
	}
	return result, nil
}

func validateCommandIdentity(principalID, projectID, sessionID, idempotencyKey string) error {
	if !validUUID(principalID) {
		return &ValidationError{Field: "principal_id", Message: "must be a UUID"}
	}
	if !validUUID(projectID) {
		return &ValidationError{Field: "project_id", Message: "must be a UUID"}
	}
	if !validUUID(sessionID) {
		return &ValidationError{Field: "session_id", Message: "must be a UUID"}
	}
	return validateIdempotencyKey(idempotencyKey)
}

func validateIdempotencyKey(value string) error {
	if value == "" {
		return &ValidationError{Field: "Idempotency-Key", Message: "header is required"}
	}
	if len(value) > maxIdempotencyLength {
		return &ValidationError{Field: "Idempotency-Key", Message: "must contain at most 200 characters"}
	}
	return nil
}

func commandHash(operation, organizationID, aggregateID string, input any) ([sha256.Size]byte, error) {
	canonical, err := json.Marshal(struct {
		Operation      string `json:"operation"`
		OrganizationID string `json:"organization_id"`
		AggregateID    string `json:"aggregate_id"`
		Input          any    `json:"input"`
	}{operation, organizationID, aggregateID, input})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(canonical), nil
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
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
			if !((character >= '0' && character <= '9') ||
				(character >= 'a' && character <= 'f') ||
				(character >= 'A' && character <= 'F')) {
				return false
			}
		}
	}
	return true
}
