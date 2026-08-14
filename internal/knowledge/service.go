package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxIdempotencySize = 200
	maxTitleLength     = 300
	maxLocatorLength   = 4096
	maxDescription     = 10000
	maxRecordBody      = 50000
	maxEvidenceNote    = 4000
	maxSearchLength    = 500
	defaultListLimit   = 100
	maximumListLimit   = 250
)

var resourceKinds = allowed("url", "repository", "document", "pull_request", "ticket", "meeting", "dashboard", "infrastructure", "other")
var resourceStatuses = allowed("active", "archived")
var classifications = allowed("public", "internal", "confidential", "restricted")
var recordTypes = allowed("decision", "requirement", "constraint", "assumption", "risk", "open_question", "finding", "procedure", "incident", "validation_result", "note")
var recordStatuses = allowed("proposed", "accepted", "disputed", "superseded", "revoked", "expired", "rejected")
var authorities = allowed("informational", "team", "organization", "external")
var evidenceRelations = allowed("supports", "contradicts", "origin", "validates")

var transitions = map[string]map[string]bool{
	"proposed": {"accepted": true, "disputed": true, "rejected": true},
	"disputed": {"accepted": true, "rejected": true, "revoked": true},
	"accepted": {"disputed": true, "superseded": true, "revoked": true, "expired": true},
}

type Repository interface {
	CreateResource(context.Context, string, string, string, string, [sha256.Size]byte, CreateResourceInput) (CreateResourceResult, error)
	ListResources(context.Context, string, string, ListOptions) ([]Resource, error)
	CreateRecord(context.Context, string, string, string, string, [sha256.Size]byte, CreateRecordInput) (CreateRecordResult, error)
	GetRecord(context.Context, string, string, string) (Record, error)
	ListRecords(context.Context, string, string, ListOptions) ([]Record, error)
	UpdateRecordStatus(context.Context, string, string, string, string, string, [sha256.Size]byte, RecordStatusInput) (RecordStatusResult, error)
}

type Service struct {
	organizationID string
	repository     Repository
	now            func() time.Time
}

func NewService(organizationID string, repository Repository) *Service {
	return &Service{organizationID: organizationID, repository: repository, now: time.Now}
}

func (s *Service) CreateResource(ctx context.Context, actorID, workspaceID, key string, input CreateResourceInput) (CreateResourceResult, error) {
	actorID, workspaceID, key = strings.TrimSpace(actorID), strings.TrimSpace(workspaceID), strings.TrimSpace(key)
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	input.Title, input.Locator = strings.TrimSpace(input.Title), strings.TrimSpace(input.Locator)
	input.Description = strings.TrimSpace(input.Description)
	input.Classification = strings.ToLower(strings.TrimSpace(input.Classification))
	if input.Classification == "" {
		input.Classification = "internal"
	}
	if input.Metadata == nil {
		input.Metadata = make(map[string]any)
	}
	if err := validateResource(actorID, workspaceID, key, input); err != nil {
		return CreateResourceResult{}, err
	}
	hash, err := commandHash("knowledge.resource.create", s.organizationID, workspaceID, input)
	if err != nil {
		return CreateResourceResult{}, err
	}
	return s.repository.CreateResource(ctx, s.organizationID, actorID, workspaceID, key, hash, input)
}

func (s *Service) ListResources(ctx context.Context, workspaceID string, options ListOptions) ([]Resource, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !validUUID(workspaceID) {
		return nil, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	options = normalizeListOptions(options)
	if err := validateListOptions(options, resourceKinds); err != nil {
		return nil, err
	}
	if options.Status != "" && !resourceStatuses[options.Status] {
		return nil, &ValidationError{Field: "status", Message: "must be active or archived"}
	}
	return s.repository.ListResources(ctx, s.organizationID, workspaceID, options)
}

func (s *Service) CreateRecord(ctx context.Context, actorID, workspaceID, key string, input CreateRecordInput) (CreateRecordResult, error) {
	actorID, workspaceID, key = strings.TrimSpace(actorID), strings.TrimSpace(workspaceID), strings.TrimSpace(key)
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	input.Title, input.Body = strings.TrimSpace(input.Title), strings.TrimSpace(input.Body)
	input.Authority = strings.ToLower(strings.TrimSpace(input.Authority))
	if input.Authority == "" {
		input.Authority = "team"
	}
	if input.ValidFrom != nil {
		value := input.ValidFrom.UTC()
		input.ValidFrom = &value
	}
	if input.ValidTo != nil {
		value := input.ValidTo.UTC()
		input.ValidTo = &value
	}
	if input.Metadata == nil {
		input.Metadata = make(map[string]any)
	}
	input.Evidence = normalizeEvidence(input.Evidence)
	if err := validateRecord(actorID, workspaceID, key, input); err != nil {
		return CreateRecordResult{}, err
	}
	hash, err := commandHash("knowledge.record.create", s.organizationID, workspaceID, input)
	if err != nil {
		return CreateRecordResult{}, err
	}
	return s.repository.CreateRecord(ctx, s.organizationID, actorID, workspaceID, key, hash, input)
}

func (s *Service) GetRecord(ctx context.Context, workspaceID, recordID string) (Record, error) {
	if !validUUID(strings.TrimSpace(workspaceID)) {
		return Record{}, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	if !validUUID(strings.TrimSpace(recordID)) {
		return Record{}, &ValidationError{Field: "record_id", Message: "must be a UUID"}
	}
	return s.repository.GetRecord(ctx, s.organizationID, strings.TrimSpace(workspaceID), strings.TrimSpace(recordID))
}

func (s *Service) ListRecords(ctx context.Context, workspaceID string, options ListOptions) ([]Record, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !validUUID(workspaceID) {
		return nil, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	options = normalizeListOptions(options)
	if err := validateListOptions(options, recordTypes); err != nil {
		return nil, err
	}
	if options.Status != "" && !recordStatuses[options.Status] {
		return nil, &ValidationError{Field: "status", Message: "is not a supported record status"}
	}
	return s.repository.ListRecords(ctx, s.organizationID, workspaceID, options)
}

func (s *Service) UpdateRecordStatus(ctx context.Context, actorID, workspaceID, recordID, key string, input RecordStatusInput) (RecordStatusResult, error) {
	actorID, workspaceID = strings.TrimSpace(actorID), strings.TrimSpace(workspaceID)
	recordID, key = strings.TrimSpace(recordID), strings.TrimSpace(key)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Reason = strings.TrimSpace(input.Reason)
	input.SupersedingRecordID = strings.TrimSpace(input.SupersedingRecordID)
	if !validUUID(actorID) {
		return RecordStatusResult{}, &ValidationError{Field: "actor_id", Message: "must be a UUID"}
	}
	if !validUUID(workspaceID) || !validUUID(recordID) {
		return RecordStatusResult{}, &ValidationError{Field: "record_id", Message: "workspace and record IDs must be UUIDs"}
	}
	if err := validateKey(key); err != nil {
		return RecordStatusResult{}, err
	}
	if !recordStatuses[input.Status] || input.Status == RecordStatusProposed {
		return RecordStatusResult{}, &ValidationError{Field: "status", Message: "is not a supported target status"}
	}
	if input.ExpectedVersion < 1 {
		return RecordStatusResult{}, &ValidationError{Field: "expected_version", Message: "must be greater than zero"}
	}
	if utf8.RuneCountInString(input.Reason) > 4000 {
		return RecordStatusResult{}, &ValidationError{Field: "reason", Message: "must contain at most 4000 characters"}
	}
	if input.Status == RecordStatusSuperseded {
		if !validUUID(input.SupersedingRecordID) || input.SupersedingRecordID == recordID {
			return RecordStatusResult{}, &ValidationError{Field: "superseding_record_id", Message: "must identify another record"}
		}
	} else if input.SupersedingRecordID != "" {
		return RecordStatusResult{}, &ValidationError{Field: "superseding_record_id", Message: "is only valid when status is superseded"}
	}
	hash, err := commandHash("knowledge.record.status", s.organizationID, workspaceID, struct {
		RecordID string            `json:"record_id"`
		Input    RecordStatusInput `json:"input"`
	}{recordID, input})
	if err != nil {
		return RecordStatusResult{}, err
	}
	return s.repository.UpdateRecordStatus(ctx, s.organizationID, actorID, workspaceID, recordID, key, hash, input)
}

func (s *Service) Context(ctx context.Context, workspaceID string) (WorkspaceContext, error) {
	records, err := s.ListRecords(ctx, workspaceID, ListOptions{Limit: maximumListLimit})
	if err != nil {
		return WorkspaceContext{}, err
	}
	resources, err := s.ListResources(ctx, workspaceID, ListOptions{Status: "active", Limit: maximumListLimit})
	if err != nil {
		return WorkspaceContext{}, err
	}
	return compileContext(workspaceID, records, resources, s.now().UTC()), nil
}

func compileContext(workspaceID string, records []Record, resources []Resource, generatedAt time.Time) WorkspaceContext {
	result := WorkspaceContext{
		WorkspaceID: workspaceID, Decisions: []Record{}, Requirements: []Record{},
		Constraints: []Record{}, OpenQuestions: []Record{}, Risks: []Record{},
		OtherRecords: []Record{}, Resources: resources, Warnings: []string{}, GeneratedAt: generatedAt,
	}
	if len(records) == maximumListLimit {
		result.Warnings = append(result.Warnings, "Record context reached the 250-item retrieval limit; refine or archive knowledge before relying on completeness.")
	}
	if len(resources) == maximumListLimit {
		result.Warnings = append(result.Warnings, "Resource context reached the 250-item retrieval limit; refine or archive sources before relying on completeness.")
	}
	for _, record := range records {
		if record.Status == "rejected" || record.Status == "revoked" || record.Status == "expired" || record.Status == "superseded" {
			continue
		}
		if record.Status == RecordStatusDisputed {
			result.Warnings = append(result.Warnings, "Disputed "+record.Type+": "+record.Title)
		}
		if record.Status == RecordStatusProposed && record.Type != RecordOpenQuestion && record.Type != RecordRisk {
			continue
		}
		switch record.Type {
		case RecordDecision:
			result.Decisions = append(result.Decisions, record)
		case RecordRequirement:
			result.Requirements = append(result.Requirements, record)
		case RecordConstraint:
			result.Constraints = append(result.Constraints, record)
		case RecordOpenQuestion:
			result.OpenQuestions = append(result.OpenQuestions, record)
		case RecordRisk:
			result.Risks = append(result.Risks, record)
		default:
			result.OtherRecords = append(result.OtherRecords, record)
		}
	}
	return result
}

func validateResource(actorID, workspaceID, key string, input CreateResourceInput) error {
	if !validUUID(actorID) || !validUUID(workspaceID) {
		return &ValidationError{Field: "workspace_id", Message: "workspace and actor IDs must be UUIDs"}
	}
	if err := validateKey(key); err != nil {
		return err
	}
	switch {
	case !resourceKinds[input.Kind]:
		return &ValidationError{Field: "kind", Message: "is not a supported resource kind"}
	case input.Title == "" || utf8.RuneCountInString(input.Title) > maxTitleLength:
		return &ValidationError{Field: "title", Message: "must contain 1 to 300 characters"}
	case input.Locator == "" || len(input.Locator) > maxLocatorLength || containsControl(input.Locator):
		return &ValidationError{Field: "locator", Message: "must contain 1 to 4096 safe characters"}
	case utf8.RuneCountInString(input.Description) > maxDescription:
		return &ValidationError{Field: "description", Message: "must contain at most 10000 characters"}
	case !classifications[input.Classification]:
		return &ValidationError{Field: "classification", Message: "must be public, internal, confidential, or restricted"}
	}
	if parsed, err := url.Parse(input.Locator); err == nil && parsed.IsAbs() {
		if parsed.User != nil {
			return &ValidationError{Field: "locator", Message: "must not contain embedded credentials"}
		}
		for key := range parsed.Query() {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			if strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") || strings.Contains(normalized, "signature") || strings.Contains(normalized, "apikey") {
				return &ValidationError{Field: "locator", Message: "must not contain secret-bearing query parameters"}
			}
		}
	}
	return nil
}

func validateRecord(actorID, workspaceID, key string, input CreateRecordInput) error {
	if !validUUID(actorID) || !validUUID(workspaceID) {
		return &ValidationError{Field: "workspace_id", Message: "workspace and actor IDs must be UUIDs"}
	}
	if err := validateKey(key); err != nil {
		return err
	}
	switch {
	case !recordTypes[input.Type]:
		return &ValidationError{Field: "type", Message: "is not a supported record type"}
	case input.Title == "" || utf8.RuneCountInString(input.Title) > maxTitleLength:
		return &ValidationError{Field: "title", Message: "must contain 1 to 300 characters"}
	case input.Body == "" || utf8.RuneCountInString(input.Body) > maxRecordBody:
		return &ValidationError{Field: "body", Message: "must contain 1 to 50000 characters"}
	case !authorities[input.Authority]:
		return &ValidationError{Field: "authority", Message: "is not supported"}
	case input.ValidFrom != nil && input.ValidTo != nil && input.ValidTo.Before(*input.ValidFrom):
		return &ValidationError{Field: "valid_to", Message: "must not be before valid_from"}
	}
	for _, evidence := range input.Evidence {
		if !validUUID(evidence.ResourceID) {
			return &ValidationError{Field: "evidence.resource_id", Message: "must be a UUID"}
		}
		if !evidenceRelations[evidence.Relation] {
			return &ValidationError{Field: "evidence.relation", Message: "is not supported"}
		}
		if utf8.RuneCountInString(evidence.Note) > maxEvidenceNote {
			return &ValidationError{Field: "evidence.note", Message: "must contain at most 4000 characters"}
		}
	}
	return nil
}

func normalizeEvidence(values []EvidenceInput) []EvidenceInput {
	seen := make(map[string]bool)
	result := make([]EvidenceInput, 0, len(values))
	for _, evidence := range values {
		evidence.ResourceID = strings.TrimSpace(evidence.ResourceID)
		evidence.Relation = strings.ToLower(strings.TrimSpace(evidence.Relation))
		if evidence.Relation == "" {
			evidence.Relation = "supports"
		}
		evidence.Note = strings.TrimSpace(evidence.Note)
		key := evidence.ResourceID + "\x00" + evidence.Relation
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, evidence)
	}
	return result
}

func normalizeListOptions(options ListOptions) ListOptions {
	options.Query = strings.TrimSpace(options.Query)
	options.Kind = strings.ToLower(strings.TrimSpace(options.Kind))
	options.Status = strings.ToLower(strings.TrimSpace(options.Status))
	if options.Limit <= 0 {
		options.Limit = defaultListLimit
	}
	return options
}

func validateListOptions(options ListOptions, kinds map[string]bool) error {
	switch {
	case utf8.RuneCountInString(options.Query) > maxSearchLength:
		return &ValidationError{Field: "q", Message: "must contain at most 500 characters"}
	case options.Kind != "" && !kinds[options.Kind]:
		return &ValidationError{Field: "kind", Message: "is not supported"}
	case options.Limit > maximumListLimit:
		return &ValidationError{Field: "limit", Message: "must not exceed 250"}
	}
	return nil
}

func validateKey(key string) error {
	if key == "" {
		return &ValidationError{Field: "Idempotency-Key", Message: "header is required"}
	}
	if len(key) > maxIdempotencySize {
		return &ValidationError{Field: "Idempotency-Key", Message: "must contain at most 200 characters"}
	}
	return nil
}

func commandHash(operation, organizationID, workspaceID string, input any) ([sha256.Size]byte, error) {
	encoded, err := json.Marshal(struct {
		Operation      string `json:"operation"`
		OrganizationID string `json:"organization_id"`
		WorkspaceID    string `json:"workspace_id"`
		Input          any    `json:"input"`
	}{operation, organizationID, workspaceID, input})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func allowed(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
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

func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
}
