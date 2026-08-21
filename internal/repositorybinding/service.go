package repositorybinding

import (
	"context"
	"strings"

	"github.com/jorgenuanzs/the-pact/internal/gitremote"
)

type Store interface {
	ListCandidates(context.Context, string) ([]Candidate, error)
}

type Service struct {
	organizationID string
	store          Store
}

func NewService(organizationID string, store Store) *Service {
	return &Service{organizationID: organizationID, store: store}
}

func (s *Service) Resolve(ctx context.Context, input ResolveInput) ([]Match, error) {
	normalized, err := gitremote.Normalize(input.RemoteURL)
	if err != nil {
		return nil, &ValidationError{Field: "remote_url", Message: err.Error()}
	}
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID != "" && !validUUID(workspaceID) {
		return nil, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	candidates, err := s.store.ListCandidates(ctx, s.organizationID)
	if err != nil {
		return nil, err
	}
	matches := make([]Match, 0)
	for _, candidate := range candidates {
		if workspaceID != "" && candidate.WorkspaceID != workspaceID {
			continue
		}
		remote, normalizeErr := gitremote.Normalize(candidate.RemoteURL)
		if normalizeErr != nil || remote != normalized {
			continue
		}
		match := candidate.Match
		match.Match = "exact"
		matches = append(matches, match)
	}
	return matches, nil
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
