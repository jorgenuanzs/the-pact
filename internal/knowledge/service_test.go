package knowledge

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

const (
	testOrganizationID = "00000000-0000-4000-8000-000000000001"
	testActorID        = "00000000-0000-4000-8000-000000000002"
	testWorkspaceID    = "019c3fd8-2b34-7a10-8b75-3fd40dcfb68e"
)

type stubRepository struct {
	resources []Resource
	records   []Record
	created   bool
}

func (s *stubRepository) CreateResource(context.Context, string, string, string, string, [sha256.Size]byte, CreateResourceInput) (CreateResourceResult, error) {
	s.created = true
	return CreateResourceResult{}, nil
}

func (s *stubRepository) ListResources(context.Context, string, string, ListOptions) ([]Resource, error) {
	return s.resources, nil
}

func (s *stubRepository) CreateRecord(context.Context, string, string, string, string, [sha256.Size]byte, CreateRecordInput) (CreateRecordResult, error) {
	return CreateRecordResult{}, errors.New("not implemented")
}

func (s *stubRepository) GetRecord(context.Context, string, string, string) (Record, error) {
	return Record{}, errors.New("not implemented")
}

func (s *stubRepository) ListRecords(context.Context, string, string, ListOptions) ([]Record, error) {
	return s.records, nil
}

func (s *stubRepository) UpdateRecordStatus(context.Context, string, string, string, string, string, [sha256.Size]byte, RecordStatusInput) (RecordStatusResult, error) {
	return RecordStatusResult{}, errors.New("not implemented")
}

func TestCreateResourceRejectsSecretBearingLocator(t *testing.T) {
	repository := &stubRepository{}
	service := NewService(testOrganizationID, repository)
	_, err := service.CreateResource(context.Background(), testActorID, testWorkspaceID, "resource-key", CreateResourceInput{
		Kind: "url", Title: "Private dashboard", Locator: "https://example.com/report?api_key=secret",
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "locator" {
		t.Fatalf("error = %v, want locator validation", err)
	}
	if repository.created {
		t.Fatal("repository was called for an unsafe locator")
	}
}

func TestContextIncludesOnlyCurrentAuthoritativeKnowledge(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	repository := &stubRepository{
		resources: []Resource{{ID: "resource-1", Status: "active"}},
		records: []Record{
			{ID: "accepted", Type: RecordDecision, Title: "Accepted", Status: RecordStatusAccepted},
			{ID: "proposal", Type: RecordDecision, Title: "Proposal", Status: RecordStatusProposed},
			{ID: "question", Type: RecordOpenQuestion, Title: "Question", Status: RecordStatusProposed},
			{ID: "risk", Type: RecordRisk, Title: "Risk", Status: RecordStatusDisputed},
			{ID: "old", Type: RecordConstraint, Title: "Old", Status: RecordStatusSuperseded},
		},
	}
	service := NewService(testOrganizationID, repository)
	service.now = func() time.Time { return now }
	result, err := service.Context(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Decisions) != 1 || result.Decisions[0].ID != "accepted" {
		t.Fatalf("decisions = %#v", result.Decisions)
	}
	if len(result.OpenQuestions) != 1 || len(result.Risks) != 1 || len(result.Constraints) != 0 {
		t.Fatalf("context = %#v", result)
	}
	if len(result.Warnings) != 1 || len(result.Resources) != 1 || !result.GeneratedAt.Equal(now) {
		t.Fatalf("context metadata = %#v", result)
	}
}
