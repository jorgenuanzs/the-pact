package rooms

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
)

const (
	testOrganizationID = "00000000-0000-4000-8000-000000000001"
	testPrincipalID    = "00000000-0000-4000-8000-000000000002"
	testWorkspaceID    = "00000000-0000-4000-8000-000000000003"
	testRoomID         = "00000000-0000-4000-8000-000000000004"
	testAgentID        = "00000000-0000-4000-8000-000000000005"
)

type stubRepository struct {
	createRoomInput    CreateRoomInput
	createMessageInput CreateMessageInput
	participants       []Participant
}

func (s *stubRepository) CreateRoom(_ context.Context, _, _, _, _ string, _ [sha256.Size]byte, input CreateRoomInput) (CreateRoomResult, error) {
	s.createRoomInput = input
	return CreateRoomResult{Room: Room{ID: testRoomID, Name: input.Name, Slug: input.Slug}}, nil
}

func (s *stubRepository) ListRooms(context.Context, string, string) ([]Room, error) {
	return nil, nil
}

func (s *stubRepository) ListParticipants(context.Context, string, string) ([]Participant, error) {
	return s.participants, nil
}

func (s *stubRepository) CreateMessage(_ context.Context, _, _ string, _ bool, _, _, _ string, _ [sha256.Size]byte, input CreateMessageInput) (CreateMessageResult, error) {
	s.createMessageInput = input
	return CreateMessageResult{Message: Message{ID: testRoomID, Body: input.Body}}, nil
}

func (s *stubRepository) ListMessages(context.Context, string, string, string, MessageListOptions) ([]Message, error) {
	return nil, nil
}

func (s *stubRepository) ListInbox(context.Context, string, string, bool, string, InboxOptions) ([]Mention, error) {
	return nil, nil
}

func (s *stubRepository) UpdateMention(context.Context, string, string, bool, string, string, string) (Mention, error) {
	return Mention{}, nil
}

func TestCreateRoomDerivesStableSlug(t *testing.T) {
	repository := &stubRepository{}
	service := NewService(testOrganizationID, repository)
	result, err := service.CreateRoom(context.Background(), testPrincipalID, testWorkspaceID, "room-create", CreateRoomInput{
		Name: "  Diseño & Producto  ", Description: "  Conversación durable  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Room.Slug != "dise-o-producto" || repository.createRoomInput.Description != "Conversación durable" {
		t.Fatalf("normalized room = %#v", repository.createRoomInput)
	}
}

func TestParticipantsReceiveUnambiguousHandles(t *testing.T) {
	repository := &stubRepository{participants: []Participant{
		{ActorID: testPrincipalID, DisplayName: "Alex Smith", Kind: "principal"},
		{ActorID: testAgentID, DisplayName: "Alex Smith", Kind: "agent"},
	}}
	service := NewService(testOrganizationID, repository)
	participants, err := service.ListParticipants(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if participants[0].Handle == participants[1].Handle || participants[0].Handle != "alex-smith-000002" || participants[1].Handle != "alex-smith-000005" {
		t.Fatalf("handles = %#v", participants)
	}
}

func TestCreateMessageNormalizesMentionTargetsWithoutCreatingWork(t *testing.T) {
	repository := &stubRepository{}
	service := NewService(testOrganizationID, repository)
	_, err := service.CreateMessage(context.Background(), testPrincipalID, false, testWorkspaceID, testRoomID, "message-create", CreateMessageInput{
		Body:            "  @agent revisa esta decisión  ",
		MentionActorIDs: []string{testAgentID, testAgentID, " "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.createMessageInput.Body != "@agent revisa esta decisión" || len(repository.createMessageInput.MentionActorIDs) != 1 {
		t.Fatalf("normalized message = %#v", repository.createMessageInput)
	}
}

func TestCreateMessageRejectsUnboundedBody(t *testing.T) {
	repository := &stubRepository{}
	service := NewService(testOrganizationID, repository)
	_, err := service.CreateMessage(context.Background(), testPrincipalID, false, testWorkspaceID, testRoomID, "message-create", CreateMessageInput{})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "body" {
		t.Fatalf("error = %v, want body validation", err)
	}
}
