package agentsession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var ErrNotFound = errors.New("agent session not found")

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
