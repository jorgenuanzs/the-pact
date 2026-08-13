package coordination

import "time"

const (
	ClaimModeExclusive = "exclusive"
	ClaimModeShared    = "shared"
)

type ScopeInput struct {
	Kind    string `json:"kind"`
	Locator string `json:"locator"`
	Mode    string `json:"mode,omitempty"`
}

type ResourceRef struct {
	ID           string  `json:"id"`
	RepositoryID *string `json:"repository_id,omitempty"`
	Kind         string  `json:"kind"`
	Locator      string  `json:"locator"`
}

type ScopeClaim struct {
	ID            string      `json:"id"`
	IntentID      string      `json:"intent_id"`
	SessionID     *string     `json:"session_id,omitempty"`
	Resource      ResourceRef `json:"resource"`
	Origin        string      `json:"origin"`
	Mode          string      `json:"mode"`
	Status        string      `json:"status"`
	Version       int64       `json:"version"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	LastRenewedAt time.Time   `json:"last_renewed_at"`
	ExpiresAt     time.Time   `json:"expires_at"`
}

type ScopeOverlap struct {
	Requested        ScopeInput `json:"requested"`
	ExistingClaimID  string     `json:"existing_claim_id"`
	ExistingIntentID string     `json:"existing_intent_id"`
	ExistingTitle    string     `json:"existing_title"`
	ExistingStatus   string     `json:"existing_status"`
	ExistingActorID  string     `json:"existing_actor_id"`
	ExistingActor    string     `json:"existing_actor"`
	ExistingScope    ScopeInput `json:"existing_scope"`
	Blocking         bool       `json:"blocking"`
	Reason           string     `json:"reason"`
	ExpiresAt        time.Time  `json:"expires_at"`
}

type ScopeCheckResult struct {
	Scopes   []ScopeInput   `json:"scopes"`
	Overlaps []ScopeOverlap `json:"overlaps"`
	Blocked  bool           `json:"blocked"`
}

type Intent struct {
	ID                 string         `json:"id"`
	ProjectID          string         `json:"project_id"`
	Title              string         `json:"title"`
	Goal               string         `json:"goal"`
	SuccessCriteria    []string       `json:"success_criteria"`
	Status             string         `json:"status"`
	Summary            *string        `json:"summary,omitempty"`
	StatusDetail       map[string]any `json:"status_detail"`
	BaseRevision       string         `json:"base_revision"`
	ResponsibleAgentID string         `json:"responsible_agent_id"`
	CreatedByActorID   string         `json:"created_by_actor_id"`
	Version            int64          `json:"version"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	StatusChangedAt    time.Time      `json:"status_changed_at"`
	CompletedAt        *time.Time     `json:"completed_at,omitempty"`
}

type Workspace struct {
	ID           string         `json:"id"`
	ProjectID    string         `json:"project_id"`
	RepositoryID string         `json:"repository_id"`
	IntentID     string         `json:"intent_id"`
	SessionID    string         `json:"session_id"`
	BaseRevision string         `json:"base_revision"`
	PathRef      string         `json:"path_ref"`
	GitBranch    string         `json:"git_branch"`
	Status       string         `json:"status"`
	StatusDetail map[string]any `json:"status_detail"`
	Version      int64          `json:"version"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	FrozenAt     *time.Time     `json:"frozen_at,omitempty"`
	ArchivedAt   *time.Time     `json:"archived_at,omitempty"`
}

type WorkItem struct {
	Intent          Intent       `json:"intent"`
	ResponsibleName string       `json:"responsible_name"`
	Scopes          []ScopeClaim `json:"scopes"`
	Workspace       *Workspace   `json:"workspace,omitempty"`
	SessionLive     bool         `json:"session_live"`
	SessionLastSeen *time.Time   `json:"session_last_seen_at,omitempty"`
}

type StartInput struct {
	SessionID       string       `json:"session_id"`
	Title           string       `json:"title"`
	Goal            string       `json:"goal"`
	SuccessCriteria []string     `json:"success_criteria,omitempty"`
	BaseRevision    string       `json:"base_revision"`
	Scopes          []ScopeInput `json:"scopes"`
	AllowOverlap    bool         `json:"allow_overlap,omitempty"`
}

type StartResult struct {
	Intent   Intent         `json:"intent"`
	Claims   []ScopeClaim   `json:"claims"`
	Overlaps []ScopeOverlap `json:"overlaps"`
	EventID  string         `json:"event_id"`
	Replayed bool           `json:"replayed"`
}

type WorkspaceInput struct {
	SessionID    string `json:"session_id"`
	BaseRevision string `json:"base_revision"`
	PathRef      string `json:"path_ref"`
	GitBranch    string `json:"git_branch"`
}

type WorkspaceResult struct {
	Workspace Workspace `json:"workspace"`
	EventID   string    `json:"event_id"`
	Replayed  bool      `json:"replayed"`
}

type StatusInput struct {
	SessionID       string `json:"session_id"`
	Status          string `json:"status"`
	ExpectedVersion int64  `json:"expected_version"`
	Summary         string `json:"summary,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

type StatusResult struct {
	Intent   Intent `json:"intent"`
	EventID  string `json:"event_id"`
	Replayed bool   `json:"replayed"`
}
