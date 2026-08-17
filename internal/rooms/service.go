package rooms

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	defaultMessageLimit = 40
	maximumMessageLimit = 100
	defaultInboxLimit   = 50
	maximumInboxLimit   = 100
)

var nonSlugCharacters = regexp.MustCompile(`[^a-z0-9]+`)

type Repository interface {
	CreateRoom(context.Context, string, string, string, string, [sha256.Size]byte, CreateRoomInput) (CreateRoomResult, error)
	ListRooms(context.Context, string, string) ([]Room, error)
	ListParticipants(context.Context, string, string) ([]Participant, error)
	CreateMessage(context.Context, string, string, bool, string, string, string, [sha256.Size]byte, CreateMessageInput) (CreateMessageResult, error)
	ListMessages(context.Context, string, string, string, MessageListOptions) ([]Message, error)
	ListInbox(context.Context, string, string, bool, string, InboxOptions) ([]Mention, error)
	UpdateMention(context.Context, string, string, bool, string, string, string) (Mention, error)
}

type Service struct {
	organizationID string
	repository     Repository
}

func NewService(organizationID string, repository Repository) *Service {
	return &Service{organizationID: organizationID, repository: repository}
}

func (s *Service) CreateRoom(ctx context.Context, actorID, workspaceID, key string, input CreateRoomInput) (CreateRoomResult, error) {
	actorID, workspaceID, key = strings.TrimSpace(actorID), strings.TrimSpace(workspaceID), strings.TrimSpace(key)
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = normalizeSlug(input.Slug)
	if input.Slug == "" {
		input.Slug = normalizeSlug(input.Name)
	}
	input.Description = strings.TrimSpace(input.Description)
	if !validUUID(actorID) || !validUUID(workspaceID) {
		return CreateRoomResult{}, &ValidationError{Field: "workspace_id", Message: "workspace and actor IDs must be UUIDs"}
	}
	if err := validateKey(key); err != nil {
		return CreateRoomResult{}, err
	}
	switch {
	case input.Name == "" || utf8.RuneCountInString(input.Name) > 120:
		return CreateRoomResult{}, &ValidationError{Field: "name", Message: "must contain 1 to 120 characters"}
	case input.Slug == "" || len(input.Slug) > 63:
		return CreateRoomResult{}, &ValidationError{Field: "slug", Message: "must contain 1 to 63 lowercase letters, numbers, or hyphens"}
	case utf8.RuneCountInString(input.Description) > 2000:
		return CreateRoomResult{}, &ValidationError{Field: "description", Message: "must contain at most 2000 characters"}
	}
	hash, err := commandHash("room.create", s.organizationID, workspaceID, input)
	if err != nil {
		return CreateRoomResult{}, err
	}
	return s.repository.CreateRoom(ctx, s.organizationID, actorID, workspaceID, key, hash, input)
}

func (s *Service) ListRooms(ctx context.Context, workspaceID string) ([]Room, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !validUUID(workspaceID) {
		return nil, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	return s.repository.ListRooms(ctx, s.organizationID, workspaceID)
}

func (s *Service) ListParticipants(ctx context.Context, workspaceID string) ([]Participant, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !validUUID(workspaceID) {
		return nil, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	participants, err := s.repository.ListParticipants(ctx, s.organizationID, workspaceID)
	if err != nil {
		return nil, err
	}
	assignHandles(participants)
	return participants, nil
}

func (s *Service) CreateMessage(ctx context.Context, principalID string, allowAll bool, workspaceID, roomID, key string, input CreateMessageInput) (CreateMessageResult, error) {
	principalID, workspaceID, roomID = strings.TrimSpace(principalID), strings.TrimSpace(workspaceID), strings.TrimSpace(roomID)
	key = strings.TrimSpace(key)
	input.Body = strings.TrimSpace(input.Body)
	input.ReplyToMessageID = strings.TrimSpace(input.ReplyToMessageID)
	input.AuthorSessionID = strings.TrimSpace(input.AuthorSessionID)
	input.MentionActorIDs = normalizeIDs(input.MentionActorIDs)
	if !validUUID(principalID) || !validUUID(workspaceID) || !validUUID(roomID) {
		return CreateMessageResult{}, &ValidationError{Field: "room_id", Message: "principal, workspace, and room IDs must be UUIDs"}
	}
	if err := validateKey(key); err != nil {
		return CreateMessageResult{}, err
	}
	switch {
	case input.Body == "" || utf8.RuneCountInString(input.Body) > 50000:
		return CreateMessageResult{}, &ValidationError{Field: "body", Message: "must contain 1 to 50000 characters"}
	case input.ReplyToMessageID != "" && !validUUID(input.ReplyToMessageID):
		return CreateMessageResult{}, &ValidationError{Field: "reply_to_message_id", Message: "must be a UUID"}
	case input.AuthorSessionID != "" && !validUUID(input.AuthorSessionID):
		return CreateMessageResult{}, &ValidationError{Field: "author_session_id", Message: "must be a UUID"}
	case len(input.MentionActorIDs) > 25:
		return CreateMessageResult{}, &ValidationError{Field: "mention_actor_ids", Message: "must contain at most 25 actors"}
	}
	for _, actorID := range input.MentionActorIDs {
		if !validUUID(actorID) {
			return CreateMessageResult{}, &ValidationError{Field: "mention_actor_ids", Message: "must contain UUIDs"}
		}
	}
	hash, err := commandHash("room.message.create", s.organizationID, workspaceID, struct {
		RoomID string             `json:"room_id"`
		Input  CreateMessageInput `json:"input"`
	}{RoomID: roomID, Input: input})
	if err != nil {
		return CreateMessageResult{}, err
	}
	return s.repository.CreateMessage(ctx, s.organizationID, principalID, allowAll, workspaceID, roomID, key, hash, input)
}

func (s *Service) ListMessages(ctx context.Context, workspaceID, roomID string, options MessageListOptions) ([]Message, error) {
	workspaceID, roomID = strings.TrimSpace(workspaceID), strings.TrimSpace(roomID)
	options.BeforeMessageID = strings.TrimSpace(options.BeforeMessageID)
	options.ThreadRootMessageID = strings.TrimSpace(options.ThreadRootMessageID)
	options.Query = strings.TrimSpace(options.Query)
	if !validUUID(workspaceID) || !validUUID(roomID) {
		return nil, &ValidationError{Field: "room_id", Message: "workspace and room IDs must be UUIDs"}
	}
	if options.BeforeMessageID != "" && !validUUID(options.BeforeMessageID) {
		return nil, &ValidationError{Field: "before", Message: "must be a message UUID"}
	}
	if options.ThreadRootMessageID != "" && !validUUID(options.ThreadRootMessageID) {
		return nil, &ValidationError{Field: "thread_root", Message: "must be a message UUID"}
	}
	if utf8.RuneCountInString(options.Query) > 500 {
		return nil, &ValidationError{Field: "q", Message: "must contain at most 500 characters"}
	}
	if options.Limit == 0 {
		options.Limit = defaultMessageLimit
	}
	if options.Limit < 1 || options.Limit > maximumMessageLimit {
		return nil, &ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}
	return s.repository.ListMessages(ctx, s.organizationID, workspaceID, roomID, options)
}

func (s *Service) ListInbox(ctx context.Context, principalID string, allowAll bool, sessionID string, options InboxOptions) ([]Mention, error) {
	principalID, sessionID = strings.TrimSpace(principalID), strings.TrimSpace(sessionID)
	options.WorkspaceID = strings.TrimSpace(options.WorkspaceID)
	options.Status = strings.ToLower(strings.TrimSpace(options.Status))
	if !validUUID(principalID) {
		return nil, &ValidationError{Field: "principal_id", Message: "must be a UUID"}
	}
	if sessionID != "" && !validUUID(sessionID) {
		return nil, &ValidationError{Field: "session_id", Message: "must be a UUID"}
	}
	if options.WorkspaceID != "" && !validUUID(options.WorkspaceID) {
		return nil, &ValidationError{Field: "workspace_id", Message: "must be a UUID"}
	}
	if options.Status == "" {
		options.Status = "pending"
	}
	if options.Status != "pending" && options.Status != "read" && options.Status != "responded" && options.Status != "dismissed" && options.Status != "all" {
		return nil, &ValidationError{Field: "status", Message: "must be pending, read, responded, dismissed, or all"}
	}
	if options.Limit == 0 {
		options.Limit = defaultInboxLimit
	}
	if options.Limit < 1 || options.Limit > maximumInboxLimit {
		return nil, &ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}
	return s.repository.ListInbox(ctx, s.organizationID, principalID, allowAll, sessionID, options)
}

func (s *Service) UpdateMention(ctx context.Context, principalID string, allowAll bool, sessionID, mentionID string, input MentionStatusInput) (Mention, error) {
	principalID, sessionID, mentionID = strings.TrimSpace(principalID), strings.TrimSpace(sessionID), strings.TrimSpace(mentionID)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if !validUUID(principalID) || !validUUID(mentionID) || (sessionID != "" && !validUUID(sessionID)) {
		return Mention{}, &ValidationError{Field: "mention_id", Message: "principal, session, and mention IDs must be UUIDs"}
	}
	if input.Status != "read" && input.Status != "responded" && input.Status != "dismissed" {
		return Mention{}, &ValidationError{Field: "status", Message: "must be read, responded, or dismissed"}
	}
	return s.repository.UpdateMention(ctx, s.organizationID, principalID, allowAll, sessionID, mentionID, input.Status)
}

func normalizeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonSlugCharacters.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func normalizeIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func assignHandles(participants []Participant) {
	counts := make(map[string]int)
	for _, participant := range participants {
		counts[normalizeSlug(participant.DisplayName)]++
	}
	for index := range participants {
		base := normalizeSlug(participants[index].DisplayName)
		if base == "" {
			base = participants[index].Kind
		}
		if counts[base] > 1 {
			compact := strings.ReplaceAll(participants[index].ActorID, "-", "")
			base += "-" + compact[len(compact)-6:]
		}
		participants[index].Handle = base
	}
}

func validateKey(value string) error {
	if value == "" {
		return &ValidationError{Field: "Idempotency-Key", Message: "header is required"}
	}
	if len(value) > 200 {
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
		return [sha256.Size]byte{}, fmt.Errorf("encode %s command: %w", operation, err)
	}
	return sha256.Sum256(encoded), nil
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
			if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
				return false
			}
		}
	}
	return true
}
