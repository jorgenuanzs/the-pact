package knowledge

import "time"

const (
	RecordDecision         = "decision"
	RecordRequirement      = "requirement"
	RecordConstraint       = "constraint"
	RecordOpenQuestion     = "open_question"
	RecordRisk             = "risk"
	RecordStatusProposed   = "proposed"
	RecordStatusAccepted   = "accepted"
	RecordStatusDisputed   = "disputed"
	RecordStatusSuperseded = "superseded"
)

type Resource struct {
	ID               string         `json:"id"`
	WorkspaceID      string         `json:"workspace_id"`
	Kind             string         `json:"kind"`
	Title            string         `json:"title"`
	Locator          string         `json:"locator"`
	Description      string         `json:"description"`
	Classification   string         `json:"classification"`
	Status           string         `json:"status"`
	Metadata         map[string]any `json:"metadata"`
	CreatedByActorID string         `json:"created_by_actor_id"`
	Version          int64          `json:"version"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	ArchivedAt       *time.Time     `json:"archived_at,omitempty"`
}

type Evidence struct {
	Relation  string    `json:"relation"`
	Note      string    `json:"note"`
	Resource  Resource  `json:"resource"`
	CreatedAt time.Time `json:"created_at"`
}

type Record struct {
	ID                   string         `json:"id"`
	WorkspaceID          string         `json:"workspace_id"`
	Type                 string         `json:"type"`
	Title                string         `json:"title"`
	Body                 string         `json:"body"`
	Status               string         `json:"status"`
	Authority            string         `json:"authority"`
	ValidFrom            time.Time      `json:"valid_from"`
	ValidTo              *time.Time     `json:"valid_to,omitempty"`
	SupersededByRecordID *string        `json:"superseded_by_record_id,omitempty"`
	Metadata             map[string]any `json:"metadata"`
	CreatedByActorID     string         `json:"created_by_actor_id"`
	LastChangedByActorID string         `json:"last_changed_by_actor_id"`
	Evidence             []Evidence     `json:"evidence"`
	Version              int64          `json:"version"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

type CreateResourceInput struct {
	Kind           string         `json:"kind"`
	Title          string         `json:"title"`
	Locator        string         `json:"locator"`
	Description    string         `json:"description,omitempty"`
	Classification string         `json:"classification,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type CreateResourceResult struct {
	Resource Resource
	Replayed bool
}

type EvidenceInput struct {
	ResourceID string `json:"resource_id"`
	Relation   string `json:"relation,omitempty"`
	Note       string `json:"note,omitempty"`
}

type CreateRecordInput struct {
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Authority string          `json:"authority,omitempty"`
	ValidFrom *time.Time      `json:"valid_from,omitempty"`
	ValidTo   *time.Time      `json:"valid_to,omitempty"`
	Evidence  []EvidenceInput `json:"evidence,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
}

type CreateRecordResult struct {
	Record   Record
	Replayed bool
}

type RecordStatusInput struct {
	Status              string `json:"status"`
	ExpectedVersion     int64  `json:"expected_version"`
	Reason              string `json:"reason,omitempty"`
	SupersedingRecordID string `json:"superseding_record_id,omitempty"`
}

type RecordStatusResult struct {
	Record   Record
	Replayed bool
}

type ListOptions struct {
	Query  string
	Kind   string
	Status string
	Limit  int
}

type WorkspaceContext struct {
	WorkspaceID   string     `json:"workspace_id"`
	Decisions     []Record   `json:"decisions"`
	Requirements  []Record   `json:"requirements"`
	Constraints   []Record   `json:"constraints"`
	OpenQuestions []Record   `json:"open_questions"`
	Risks         []Record   `json:"risks"`
	OtherRecords  []Record   `json:"other_records"`
	Resources     []Resource `json:"resources"`
	Warnings      []string   `json:"warnings"`
	GeneratedAt   time.Time  `json:"generated_at"`
}
