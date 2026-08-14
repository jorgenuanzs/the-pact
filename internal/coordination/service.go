package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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

type HandoffRepository interface {
	OfferHandoff(context.Context, string, string, bool, string, string, string, [sha256.Size]byte, OfferHandoffInput) (HandoffResult, error)
	ListHandoffs(context.Context, string, string, string) ([]Handoff, error)
	UpdateHandoffStatus(context.Context, string, string, bool, string, string, string, string, [sha256.Size]byte, HandoffStatusInput) (HandoffResult, error)
}

type Service struct {
	organizationID string
	repository     Repository
	handoffs       HandoffRepository
}

func NewService(organizationID string, repository Repository) *Service {
	handoffs, _ := repository.(HandoffRepository)
	return &Service{organizationID: organizationID, repository: repository, handoffs: handoffs}
}

func (s *Service) OfferHandoff(
	ctx context.Context, principalID string, allowAll bool, projectID, intentID, key string, input OfferHandoffInput,
) (HandoffResult, error) {
	if s.handoffs == nil {
		return HandoffResult{}, errors.New("handoff repository is not configured")
	}
	principalID, projectID, intentID = strings.TrimSpace(principalID), strings.TrimSpace(projectID), strings.TrimSpace(intentID)
	key, input.SessionID, input.Summary = strings.TrimSpace(key), strings.TrimSpace(input.SessionID), strings.TrimSpace(input.Summary)
	if !validUUID(principalID) || !validUUID(projectID) || !validUUID(intentID) || !validUUID(input.SessionID) {
		return HandoffResult{}, &ValidationError{Field: "identity", Message: "principal, project, intent, and session IDs must be UUIDs"}
	}
	if err := validateIdempotencyKey(key); err != nil {
		return HandoffResult{}, err
	}
	if input.Summary == "" || utf8.RuneCountInString(input.Summary) > 10000 {
		return HandoffResult{}, &ValidationError{Field: "summary", Message: "must contain 1 to 10000 characters"}
	}
	input.Completed = compactHandoffStrings(input.Completed)
	input.RemainingWork = compactHandoffStrings(input.RemainingWork)
	input.Blockers = compactHandoffStrings(input.Blockers)
	input.NextSteps = compactHandoffStrings(input.NextSteps)
	if len(input.Completed) > 100 || len(input.RemainingWork) > 100 || len(input.Blockers) > 100 || len(input.NextSteps) > 100 {
		return HandoffResult{}, &ValidationError{Field: "handoff", Message: "each work list must contain at most 100 items"}
	}
	for _, value := range append(append(append(input.Completed, input.RemainingWork...), input.Blockers...), input.NextSteps...) {
		if utf8.RuneCountInString(value) > 2000 {
			return HandoffResult{}, &ValidationError{Field: "handoff", Message: "work list items must contain at most 2000 characters"}
		}
	}
	if len(input.Validations) > 100 {
		return HandoffResult{}, &ValidationError{Field: "validations", Message: "must contain at most 100 items"}
	}
	for index := range input.Validations {
		validation := &input.Validations[index]
		validation.Name = strings.TrimSpace(validation.Name)
		validation.Status = strings.ToLower(strings.TrimSpace(validation.Status))
		validation.Details = strings.TrimSpace(validation.Details)
		if validation.Name == "" || utf8.RuneCountInString(validation.Name) > 300 ||
			(validation.Status != "passed" && validation.Status != "failed" && validation.Status != "pending" && validation.Status != "skipped") ||
			utf8.RuneCountInString(validation.Details) > 4000 {
			return HandoffResult{}, &ValidationError{Field: "validations", Message: "must contain a valid name, status, and optional details"}
		}
	}
	input.LinkedRecordIDs = compactUUIDs(input.LinkedRecordIDs)
	if len(input.LinkedRecordIDs) > 100 {
		return HandoffResult{}, &ValidationError{Field: "linked_record_ids", Message: "must contain at most 100 records"}
	}
	for _, recordID := range input.LinkedRecordIDs {
		if !validUUID(recordID) {
			return HandoffResult{}, &ValidationError{Field: "linked_record_ids", Message: "items must be UUIDs"}
		}
	}
	if input.ExpiresInHours == 0 {
		input.ExpiresInHours = 72
	}
	if input.ExpiresInHours < 1 || input.ExpiresInHours > 168 {
		return HandoffResult{}, &ValidationError{Field: "expires_in_hours", Message: "must be between 1 and 168"}
	}
	hash, err := commandHash("handoff.offer", s.organizationID, intentID, input)
	if err != nil {
		return HandoffResult{}, err
	}
	return s.handoffs.OfferHandoff(ctx, s.organizationID, principalID, allowAll, projectID, intentID, key, hash, input)
}

func (s *Service) ListHandoffs(ctx context.Context, projectID, intentID string) ([]Handoff, error) {
	if s.handoffs == nil {
		return nil, errors.New("handoff repository is not configured")
	}
	projectID, intentID = strings.TrimSpace(projectID), strings.TrimSpace(intentID)
	if !validUUID(projectID) || (intentID != "" && !validUUID(intentID)) {
		return nil, &ValidationError{Field: "intent_id", Message: "project and optional intent IDs must be UUIDs"}
	}
	return s.handoffs.ListHandoffs(ctx, s.organizationID, projectID, intentID)
}

func (s *Service) UpdateHandoffStatus(
	ctx context.Context, principalID string, allowAll bool, projectID, intentID, handoffID, key string, input HandoffStatusInput,
) (HandoffResult, error) {
	if s.handoffs == nil {
		return HandoffResult{}, errors.New("handoff repository is not configured")
	}
	principalID, projectID, intentID = strings.TrimSpace(principalID), strings.TrimSpace(projectID), strings.TrimSpace(intentID)
	handoffID, key, input.SessionID = strings.TrimSpace(handoffID), strings.TrimSpace(key), strings.TrimSpace(input.SessionID)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if !validUUID(principalID) || !validUUID(projectID) || !validUUID(intentID) || !validUUID(handoffID) || !validUUID(input.SessionID) {
		return HandoffResult{}, &ValidationError{Field: "identity", Message: "principal, project, intent, handoff, and session IDs must be UUIDs"}
	}
	if err := validateIdempotencyKey(key); err != nil {
		return HandoffResult{}, err
	}
	if input.Status != "accepted" && input.Status != "withdrawn" {
		return HandoffResult{}, &ValidationError{Field: "status", Message: "must be accepted or withdrawn"}
	}
	if input.ExpectedVersion < 1 {
		return HandoffResult{}, &ValidationError{Field: "expected_version", Message: "must be greater than zero"}
	}
	hash, err := commandHash("handoff.status", s.organizationID, handoffID, input)
	if err != nil {
		return HandoffResult{}, err
	}
	return s.handoffs.UpdateHandoffStatus(ctx, s.organizationID, principalID, allowAll, projectID, intentID, handoffID, key, hash, input)
}

func compactHandoffStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func compactUUIDs(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
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
