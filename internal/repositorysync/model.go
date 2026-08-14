package repositorysync

import "time"

const (
	StatusNever       = "never"
	StatusSynced      = "synced"
	StatusFailed      = "failed"
	StatusUnsupported = "unsupported"
	StatusUnavailable = "unavailable"
)

type State struct {
	RepositoryID       string     `json:"repository_id,omitempty"`
	ProjectID          string     `json:"project_id"`
	Provider           string     `json:"provider"`
	RepositoryFullName string     `json:"repository_full_name,omitempty"`
	Status             string     `json:"status"`
	DefaultBranch      string     `json:"default_branch,omitempty"`
	CanonicalRevision  *string    `json:"canonical_revision"`
	Visibility         string     `json:"visibility"`
	ProviderUpdatedAt  *time.Time `json:"provider_updated_at"`
	LastAttemptAt      *time.Time `json:"last_attempt_at"`
	LastSuccessAt      *time.Time `json:"last_success_at"`
	LastErrorCode      *string    `json:"last_error_code"`
	Version            int64      `json:"version"`
}

type Result struct {
	State    State   `json:"state"`
	Changed  bool    `json:"changed"`
	EventID  *string `json:"event_id"`
	Replayed bool    `json:"replayed"`
}

type Reference struct {
	Owner    string
	Name     string
	FullName string
}

type Snapshot struct {
	Provider           string
	RepositoryFullName string
	DefaultBranch      string
	CanonicalRevision  string
	Visibility         string
	ProviderUpdatedAt  *time.Time
}
