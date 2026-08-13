package backoffice

import (
	"encoding/json"
	"time"

	"github.com/jorgenuanzs/the-pact/internal/coordination"
)

const (
	CodeActivityUnobserved = "unobserved"
	CodeActivityIdle       = "idle"
	CodeActivityEditing    = "editing"
	CodeActivityRecent     = "recent"

	ReasonNoConnectedObserver   = "no_connected_observer"
	ReasonObserverWithoutChange = "observer_connected_no_recent_change"
	ReasonFreshWorkspaceDiff    = "fresh_workspace_diff"
	ReasonFreshExternalChange   = "fresh_external_git_change"
	ReasonRecentWorkspaceDiff   = "recent_workspace_diff"
	ReasonRecentExternalChange  = "recent_external_git_change"
	ReasonRecentChangeset       = "recent_changeset"
)

type Overview struct {
	CodeActivity CodeActivity            `json:"code_activity"`
	Counts       Counts                  `json:"counts"`
	ActiveWork   []ActiveWork            `json:"active_work"`
	RecentEvents []RecentEvent           `json:"recent_events"`
	WorkItems    []coordination.WorkItem `json:"work_items"`
	GeneratedAt  time.Time               `json:"generated_at"`
}

type CodeActivity struct {
	State             string     `json:"state"`
	Reason            string     `json:"reason"`
	Source            *string    `json:"source"`
	ObservedAt        *time.Time `json:"observed_at"`
	ObserverConnected bool       `json:"observer_connected"`
	ObserverCount     int64      `json:"observer_count"`
	ObserverFreshSecs int64      `json:"observer_freshness_seconds"`
	ActiveWindowSecs  int64      `json:"active_window_seconds"`
	RecentWindowSecs  int64      `json:"recent_window_seconds"`
}

type Counts struct {
	Repositories       int64 `json:"repositories"`
	LiveSessions       int64 `json:"live_sessions"`
	ConnectedNodes     int64 `json:"connected_nodes"`
	ConnectedObservers int64 `json:"connected_observers"`
	ActiveIntents      int64 `json:"active_intents"`
	BlockedIntents     int64 `json:"blocked_intents"`
	LiveWorkspaces     int64 `json:"live_workspaces"`
	ActiveScopeClaims  int64 `json:"active_scope_claims"`
	PendingChangesets  int64 `json:"pending_changesets"`
	Events             int64 `json:"events"`
}

type ActiveWork struct {
	SessionID        string    `json:"session_id"`
	ActorID          string    `json:"actor_id"`
	ActorName        string    `json:"actor_name"`
	ActorKind        string    `json:"actor_kind"`
	ClientType       string    `json:"client_type"`
	SessionStatus    string    `json:"session_status"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	NodeID           *string   `json:"node_id"`
	NodeName         *string   `json:"node_name"`
	NodeStatus       *string   `json:"node_status"`
	IntentID         *string   `json:"intent_id"`
	IntentTitle      *string   `json:"intent_title"`
	IntentStatus     *string   `json:"intent_status"`
	WorkspaceID      *string   `json:"workspace_id"`
	WorkspaceStatus  *string   `json:"workspace_status"`
	WorkspaceBranch  *string   `json:"workspace_branch"`
	WorkspacePathRef *string   `json:"workspace_path_ref"`
}

type RecentEvent struct {
	ID         string          `json:"id"`
	Sequence   string          `json:"sequence"`
	Type       string          `json:"type"`
	ActorID    *string         `json:"actor_id"`
	ActorName  *string         `json:"actor_name"`
	SessionID  *string         `json:"session_id"`
	IntentID   *string         `json:"intent_id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Data       json.RawMessage `json:"data"`
}
