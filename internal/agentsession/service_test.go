package agentsession

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct {
	start     func(context.Context, string, string, string, StartInput) (Session, error)
	heartbeat func(context.Context, string, string, bool, string) (Session, error)
	close     func(context.Context, string, string, bool, string) error
}

func (f fakeRepository) Start(ctx context.Context, organizationID, sponsorID, projectID string, input StartInput) (Session, error) {
	return f.start(ctx, organizationID, sponsorID, projectID, input)
}

func (f fakeRepository) Heartbeat(ctx context.Context, organizationID, sponsorID string, allowAll bool, sessionID string) (Session, error) {
	return f.heartbeat(ctx, organizationID, sponsorID, allowAll, sessionID)
}

func (f fakeRepository) Close(ctx context.Context, organizationID, sponsorID string, allowAll bool, sessionID string) error {
	return f.close(ctx, organizationID, sponsorID, allowAll, sessionID)
}

func TestStartNormalizesAgentIdentity(t *testing.T) {
	var received StartInput
	service := NewService("organization", fakeRepository{
		start: func(_ context.Context, organizationID, sponsorID, projectID string, input StartInput) (Session, error) {
			if organizationID != "organization" || sponsorID != "018f784a-68c1-7b0f-8f2a-cfc255f99e2e" || projectID != "018f784a-68c1-7b0f-8f2a-cfc255f99e1d" {
				t.Fatalf("organization=%q project=%q", organizationID, projectID)
			}
			received = input
			return Session{ID: "session"}, nil
		},
	})
	_, err := service.Start(context.Background(), "018f784a-68c1-7b0f-8f2a-cfc255f99e2e", "018f784a-68c1-7b0f-8f2a-cfc255f99e1d", StartInput{
		NodeKey:    " node ",
		NodeName:   " Computer ",
		AgentName:  " Kimi ",
		AgentType:  " KIMI ",
		ClientType: " KIMI-CLI ",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if received.NodeKey != "node" || received.AgentName != "Kimi" ||
		received.AgentType != "kimi" || received.ClientType != "kimi-cli" {
		t.Fatalf("input = %#v", received)
	}
}

func TestStartRejectsInvalidProjectID(t *testing.T) {
	service := NewService("organization", fakeRepository{
		start: func(context.Context, string, string, string, StartInput) (Session, error) {
			t.Fatal("repository should not be called")
			return Session{}, nil
		},
	})
	_, err := service.Start(context.Background(), "018f784a-68c1-7b0f-8f2a-cfc255f99e2e", "not-a-uuid", StartInput{})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "project_id" {
		t.Fatalf("Start() error = %v", err)
	}
}
