//go:build integration

package rooms_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/agentsession"
	"github.com/jorgenuanzs/the-pact/internal/config"
	"github.com/jorgenuanzs/the-pact/internal/platform/migrations"
	"github.com/jorgenuanzs/the-pact/internal/platform/postgres"
	"github.com/jorgenuanzs/the-pact/internal/projects"
	"github.com/jorgenuanzs/the-pact/internal/rooms"
	"github.com/jorgenuanzs/the-pact/internal/workspaces"
)

func TestWorkspaceRoomConversationAndExplicitAgentMention(t *testing.T) {
	databaseURL := os.Getenv("PACT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PACT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL, postgres.Config{
		ApplicationName: "pact-rooms-integration-test",
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := migrations.Verify(ctx, pool); err != nil {
		t.Fatalf("verify migrations: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	projectResult, err := projects.NewService(
		config.DefaultLocalOrganizationID,
		projects.NewPostgresRepository(pool),
	).Create(ctx, "rooms-project-"+suffix, projects.CreateInput{
		Name: "Rooms project " + suffix,
		Slug: "rooms-project-" + suffix,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workspace, err := workspaces.NewService(
		config.DefaultLocalOrganizationID,
		workspaces.NewPostgresRepository(pool),
	).Get(ctx, projectResult.Project.Slug)
	if err != nil {
		t.Fatalf("get project workspace: %v", err)
	}

	service := rooms.NewService(config.DefaultLocalOrganizationID, rooms.NewPostgresRepository(pool))
	roomList, err := service.ListRooms(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("list default rooms: %v", err)
	}
	if len(roomList) != 1 || roomList[0].Slug != "general" || !roomList[0].ManagedDefault {
		t.Fatalf("default rooms = %#v", roomList)
	}

	createdRoom, err := service.CreateRoom(ctx, access.BootstrapPrincipalID, workspace.ID, "room-create-"+suffix, rooms.CreateRoomInput{
		Name: "Product decisions", Description: "Soft product context shared by people and agents.",
	})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	replayedRoom, err := service.CreateRoom(ctx, access.BootstrapPrincipalID, workspace.ID, "room-create-"+suffix, rooms.CreateRoomInput{
		Name: "Product decisions", Description: "Soft product context shared by people and agents.",
	})
	if err != nil || !replayedRoom.Replayed || replayedRoom.Room.ID != createdRoom.Room.ID {
		t.Fatalf("replayed room = %#v, error = %v", replayedRoom, err)
	}

	session, err := agentsession.NewService(
		config.DefaultLocalOrganizationID,
		agentsession.NewPostgresRepository(pool),
	).Start(ctx, access.BootstrapPrincipalID, projectResult.Project.ID, agentsession.StartInput{
		NodeKey: "rooms-node-" + suffix, NodeName: "Rooms integration node",
		AgentName: "Codex Rooms " + suffix, AgentType: "codex", ClientType: "codex-mcp",
	})
	if err != nil {
		t.Fatalf("start agent session: %v", err)
	}
	participants, err := service.ListParticipants(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("list participants: %v", err)
	}
	if !containsParticipant(participants, access.BootstrapPrincipalID) || !containsParticipant(participants, session.ActorID) {
		t.Fatalf("participants = %#v", participants)
	}

	messageInput := rooms.CreateMessageInput{
		Body:            "@" + participantHandle(participants, session.ActorID) + " please review the client meeting decision.",
		MentionActorIDs: []string{session.ActorID},
	}
	createdMessage, err := service.CreateMessage(
		ctx, access.BootstrapPrincipalID, true, workspace.ID, createdRoom.Room.ID,
		"room-message-"+suffix, messageInput,
	)
	if err != nil {
		t.Fatalf("create room message: %v", err)
	}
	replayedMessage, err := service.CreateMessage(
		ctx, access.BootstrapPrincipalID, true, workspace.ID, createdRoom.Room.ID,
		"room-message-"+suffix, messageInput,
	)
	if err != nil || !replayedMessage.Replayed || replayedMessage.Message.ID != createdMessage.Message.ID {
		t.Fatalf("replayed message = %#v, error = %v", replayedMessage, err)
	}

	inbox, err := service.ListInbox(ctx, access.BootstrapPrincipalID, true, session.ID, rooms.InboxOptions{
		WorkspaceID: workspace.ID, Status: "pending",
	})
	if err != nil {
		t.Fatalf("list agent room inbox: %v", err)
	}
	if len(inbox) != 1 || inbox[0].Message.ID != createdMessage.Message.ID {
		t.Fatalf("agent inbox = %#v", inbox)
	}
	agentReply, err := service.CreateMessage(
		ctx, access.BootstrapPrincipalID, true, workspace.ID, createdRoom.Room.ID,
		"room-reply-"+suffix, rooms.CreateMessageInput{
			Body:             "Reviewed. The decision is consistent with the current product direction.",
			ReplyToMessageID: createdMessage.Message.ID, AuthorSessionID: session.ID,
		},
	)
	if err != nil {
		t.Fatalf("create agent reply: %v", err)
	}
	if agentReply.Message.AuthorActorID != session.ActorID || agentReply.Message.ThreadRootMessageID == nil || *agentReply.Message.ThreadRootMessageID != createdMessage.Message.ID {
		t.Fatalf("agent reply = %#v", agentReply.Message)
	}
	acknowledged, err := service.UpdateMention(ctx, access.BootstrapPrincipalID, true, session.ID, inbox[0].ID, rooms.MentionStatusInput{Status: "responded"})
	if err != nil || acknowledged.Status != "responded" || acknowledged.ReadAt == nil || acknowledged.RespondedAt == nil {
		t.Fatalf("acknowledged mention = %#v, error = %v", acknowledged, err)
	}

	messages, err := service.ListMessages(ctx, workspace.ID, createdRoom.Room.ID, rooms.MessageListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 || messages[0].ID != createdMessage.Message.ID || messages[1].ID != agentReply.Message.ID {
		t.Fatalf("messages = %#v", messages)
	}
	var intentCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM coordination.intents
		WHERE organization_id = $1 AND project_id = $2
	`, config.DefaultLocalOrganizationID, projectResult.Project.ID).Scan(&intentCount); err != nil {
		t.Fatalf("count intents: %v", err)
	}
	if intentCount != 0 {
		t.Fatalf("room conversation created %d intents, want 0", intentCount)
	}
}

func containsParticipant(participants []rooms.Participant, actorID string) bool {
	for _, participant := range participants {
		if participant.ActorID == actorID {
			return true
		}
	}
	return false
}

func participantHandle(participants []rooms.Participant, actorID string) string {
	for _, participant := range participants {
		if participant.ActorID == actorID {
			return participant.Handle
		}
	}
	return "agent"
}
