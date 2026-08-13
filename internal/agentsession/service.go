package agentsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrNotFound            = errors.New("agent session not found")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with a different observation")
	ErrCommandIncomplete   = errors.New("the previous observation command has not completed")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type Repository interface {
	Start(context.Context, string, string, string, StartInput) (Session, error)
	Heartbeat(context.Context, string, string, bool, string) (Session, error)
	Observe(context.Context, string, string, string, string, [sha256.Size]byte, ObservationInput) (ObservationResult, error)
	Close(context.Context, string, string, bool, string) error
}

type Service struct {
	organizationID string
	repository     Repository
}

func NewService(organizationID string, repository Repository) *Service {
	return &Service{organizationID: organizationID, repository: repository}
}

func (s *Service) Start(ctx context.Context, sponsorPrincipalID, projectID string, input StartInput) (Session, error) {
	sponsorPrincipalID = strings.TrimSpace(sponsorPrincipalID)
	projectID = strings.TrimSpace(projectID)
	input.NodeKey = strings.TrimSpace(input.NodeKey)
	input.NodeName = strings.TrimSpace(input.NodeName)
	input.AgentName = strings.TrimSpace(input.AgentName)
	input.AgentType = strings.ToLower(strings.TrimSpace(input.AgentType))
	input.ClientType = strings.ToLower(strings.TrimSpace(input.ClientType))
	if err := validateUUID("project_id", projectID); err != nil {
		return Session{}, err
	}
	if err := validateUUID("sponsor_principal_id", sponsorPrincipalID); err != nil {
		return Session{}, err
	}
	for field, value := range map[string]string{
		"node_key": input.NodeKey, "node_name": input.NodeName,
		"agent_name": input.AgentName, "agent_type": input.AgentType,
		"client_type": input.ClientType,
	} {
		if value == "" {
			return Session{}, &ValidationError{Field: field, Message: "is required"}
		}
	}
	if len(input.NodeKey) > 255 {
		return Session{}, &ValidationError{Field: "node_key", Message: "must contain at most 255 characters"}
	}
	if utf8.RuneCountInString(input.NodeName) > 200 || utf8.RuneCountInString(input.AgentName) > 200 {
		return Session{}, &ValidationError{Field: "name", Message: "must contain at most 200 characters"}
	}
	if len(input.AgentType) > 100 || len(input.ClientType) > 100 {
		return Session{}, &ValidationError{Field: "client_type", Message: "must contain at most 100 characters"}
	}
	return s.repository.Start(ctx, s.organizationID, sponsorPrincipalID, projectID, input)
}

func (s *Service) Heartbeat(ctx context.Context, sponsorPrincipalID string, allowAll bool, sessionID string) (Session, error) {
	sponsorPrincipalID = strings.TrimSpace(sponsorPrincipalID)
	sessionID = strings.TrimSpace(sessionID)
	if err := validateUUID("sponsor_principal_id", sponsorPrincipalID); err != nil {
		return Session{}, err
	}
	if err := validateUUID("session_id", sessionID); err != nil {
		return Session{}, err
	}
	return s.repository.Heartbeat(ctx, s.organizationID, sponsorPrincipalID, allowAll, sessionID)
}

func (s *Service) Observe(
	ctx context.Context,
	sponsorPrincipalID string,
	sessionID string,
	idempotencyKey string,
	input ObservationInput,
) (ObservationResult, error) {
	sponsorPrincipalID = strings.TrimSpace(sponsorPrincipalID)
	sessionID = strings.TrimSpace(sessionID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	input.DiffFingerprint = strings.ToLower(strings.TrimSpace(input.DiffFingerprint))
	input.HeadRevision = strings.ToLower(strings.TrimSpace(input.HeadRevision))
	input.Branch = strings.TrimSpace(input.Branch)
	if input.WorkspaceID != nil {
		trimmed := strings.TrimSpace(*input.WorkspaceID)
		input.WorkspaceID = &trimmed
	}
	if err := validateUUID("sponsor_principal_id", sponsorPrincipalID); err != nil {
		return ObservationResult{}, err
	}
	if err := validateUUID("session_id", sessionID); err != nil {
		return ObservationResult{}, err
	}
	switch {
	case idempotencyKey == "":
		return ObservationResult{}, &ValidationError{Field: "Idempotency-Key", Message: "header is required"}
	case len(idempotencyKey) > 200:
		return ObservationResult{}, &ValidationError{Field: "Idempotency-Key", Message: "must contain at most 200 characters"}
	case len(input.DiffFingerprint) != sha256.Size*2:
		return ObservationResult{}, &ValidationError{Field: "diff_fingerprint", Message: "must be a SHA-256 hexadecimal digest"}
	case input.ChangedPaths < 0 || input.ChangedPaths > 1_000_000:
		return ObservationResult{}, &ValidationError{Field: "changed_paths", Message: "must be between 0 and 1000000"}
	case !input.Dirty && input.ChangedPaths != 0:
		return ObservationResult{}, &ValidationError{Field: "changed_paths", Message: "must be zero when dirty is false"}
	case input.Dirty && input.ChangedPaths == 0:
		return ObservationResult{}, &ValidationError{Field: "changed_paths", Message: "must be greater than zero when dirty is true"}
	case len(input.HeadRevision) > 64:
		return ObservationResult{}, &ValidationError{Field: "head_revision", Message: "must contain at most 64 characters"}
	case input.HeadRevision != "" && len(input.HeadRevision) < 7:
		return ObservationResult{}, &ValidationError{Field: "head_revision", Message: "must contain at least 7 characters"}
	case input.HeadRevision != "" && !isHex(input.HeadRevision):
		return ObservationResult{}, &ValidationError{Field: "head_revision", Message: "must be a hexadecimal Git object ID"}
	case len(input.Branch) > 255:
		return ObservationResult{}, &ValidationError{Field: "branch", Message: "must contain at most 255 characters"}
	case input.WorkspaceID != nil && validateUUID("workspace_id", *input.WorkspaceID) != nil:
		return ObservationResult{}, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	if _, err := hex.DecodeString(input.DiffFingerprint); err != nil {
		return ObservationResult{}, &ValidationError{Field: "diff_fingerprint", Message: "must be a SHA-256 hexadecimal digest"}
	}
	canonical, err := json.Marshal(struct {
		Operation      string           `json:"operation"`
		OrganizationID string           `json:"organization_id"`
		SessionID      string           `json:"session_id"`
		Input          ObservationInput `json:"input"`
	}{
		Operation: "repository.observe", OrganizationID: s.organizationID,
		SessionID: sessionID, Input: input,
	})
	if err != nil {
		return ObservationResult{}, err
	}
	return s.repository.Observe(
		ctx, s.organizationID, sponsorPrincipalID, sessionID,
		idempotencyKey, sha256.Sum256(canonical), input,
	)
}

func (s *Service) Close(ctx context.Context, sponsorPrincipalID string, allowAll bool, sessionID string) error {
	sponsorPrincipalID = strings.TrimSpace(sponsorPrincipalID)
	sessionID = strings.TrimSpace(sessionID)
	if err := validateUUID("sponsor_principal_id", sponsorPrincipalID); err != nil {
		return err
	}
	if err := validateUUID("session_id", sessionID); err != nil {
		return err
	}
	return s.repository.Close(ctx, s.organizationID, sponsorPrincipalID, allowAll, sessionID)
}

func validateUUID(field, value string) error {
	if len(value) != 36 {
		return &ValidationError{Field: field, Message: "must be a UUID"}
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return &ValidationError{Field: field, Message: "must be a UUID"}
			}
		default:
			if !((character >= '0' && character <= '9') ||
				(character >= 'a' && character <= 'f') ||
				(character >= 'A' && character <= 'F')) {
				return &ValidationError{Field: field, Message: "must be a UUID"}
			}
		}
	}
	return nil
}

func isHex(value string) bool {
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return value != ""
}
