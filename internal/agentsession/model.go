package agentsession

import "time"

type StartInput struct {
	NodeKey    string `json:"node_key"`
	NodeName   string `json:"node_name"`
	AgentName  string `json:"agent_name"`
	AgentType  string `json:"agent_type"`
	ClientType string `json:"client_type"`
	ObserveGit bool   `json:"observe_git"`
}

type EnrollmentInput struct {
	ProjectID  string `json:"project_id"`
	AgentType  string `json:"agent_type"`
	ClientType string `json:"client_type"`
}

type Enrollment struct {
	ID                 string     `json:"id"`
	WorkspaceID        string     `json:"workspace_id"`
	ProjectID          string     `json:"project_id"`
	AgentID            string     `json:"agent_id"`
	AgentName          string     `json:"agent_name"`
	SponsorPrincipalID string     `json:"sponsor_principal_id"`
	AgentType          string     `json:"agent_type"`
	ClientType         string     `json:"client_type"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ActivatedAt        *time.Time `json:"activated_at,omitempty"`
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
}

type EnrollmentResult struct {
	Enrollment Enrollment `json:"enrollment"`
	Created    bool       `json:"created"`
}

type Session struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	ActorID    string    `json:"actor_id"`
	ActorName  string    `json:"actor_name"`
	NodeID     string    `json:"node_id"`
	NodeName   string    `json:"node_name"`
	ClientType string    `json:"client_type"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type ObservationInput struct {
	WorkspaceID     *string `json:"workspace_id,omitempty"`
	Dirty           bool    `json:"dirty"`
	DiffFingerprint string  `json:"diff_fingerprint"`
	ChangedPaths    int     `json:"changed_paths"`
	HeadRevision    string  `json:"head_revision,omitempty"`
	Branch          string  `json:"branch,omitempty"`
}

type RepositoryObservation struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	SessionID       string    `json:"session_id"`
	ActorID         string    `json:"actor_id"`
	NodeID          string    `json:"node_id"`
	WorkspaceID     *string   `json:"workspace_id,omitempty"`
	IntentID        *string   `json:"intent_id,omitempty"`
	Dirty           bool      `json:"dirty"`
	DiffFingerprint string    `json:"diff_fingerprint"`
	ChangedPaths    int       `json:"changed_paths"`
	HeadRevision    string    `json:"head_revision,omitempty"`
	Branch          string    `json:"branch,omitempty"`
	Version         int64     `json:"version"`
	ObservedAt      time.Time `json:"observed_at"`
}

type ObservationResult struct {
	Observation RepositoryObservation `json:"observation"`
	EventID     *string               `json:"event_id,omitempty"`
	EventType   *string               `json:"event_type,omitempty"`
	Replayed    bool                  `json:"replayed"`
}
