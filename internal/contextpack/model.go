package contextpack

import (
	"time"

	"github.com/jorgenuanzs/the-pact/internal/coordination"
	"github.com/jorgenuanzs/the-pact/internal/knowledge"
)

type ProjectSnapshot struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Slug              string  `json:"slug"`
	Status            string  `json:"status"`
	CanonicalRevision *string `json:"canonical_revision,omitempty"`
	Version           int64   `json:"version"`
}

type WorkspaceSnapshot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Status  string `json:"status"`
	Version int64  `json:"version"`
}

type WorktreeSnapshot struct {
	ID           string `json:"id"`
	IntentID     string `json:"intent_id"`
	BaseRevision string `json:"base_revision"`
	GitBranch    string `json:"git_branch"`
	Status       string `json:"status"`
	Version      int64  `json:"version"`
}

type WorkSnapshot struct {
	Intent          coordination.Intent       `json:"intent"`
	ResponsibleName string                    `json:"responsible_name"`
	Scopes          []coordination.ScopeClaim `json:"scopes"`
	Worktree        *WorktreeSnapshot         `json:"worktree,omitempty"`
	SessionLive     bool                      `json:"session_live"`
	SessionLastSeen *time.Time                `json:"session_last_seen_at,omitempty"`
}

type Snapshot struct {
	EventCursor       string    `json:"event_cursor"`
	GitRevision       *string   `json:"git_revision,omitempty"`
	Consistency       string    `json:"consistency"`
	SourceFingerprint string    `json:"source_fingerprint"`
	GeneratedAt       time.Time `json:"generated_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type ContextPack struct {
	ID                  string                     `json:"id"`
	Type                string                     `json:"type"`
	WorkspaceID         string                     `json:"workspace_id"`
	ProjectID           string                     `json:"project_id"`
	IntentID            string                     `json:"intent_id"`
	RequestingSessionID *string                    `json:"requesting_session_id,omitempty"`
	RequestedByActorID  string                     `json:"requested_by_actor_id"`
	Project             ProjectSnapshot            `json:"project"`
	Workspace           WorkspaceSnapshot          `json:"workspace"`
	Intent              coordination.Intent        `json:"intent"`
	ActiveWork          []WorkSnapshot             `json:"active_work"`
	Knowledge           knowledge.WorkspaceContext `json:"knowledge"`
	Handoffs            []coordination.Handoff     `json:"handoffs"`
	Warnings            []string                   `json:"warnings"`
	Snapshot            Snapshot                   `json:"snapshot"`
}

type CompileInput struct {
	SessionID  string `json:"session_id,omitempty"`
	Type       string `json:"type,omitempty"`
	TTLMinutes int    `json:"ttl_minutes,omitempty"`
}

type Draft struct {
	Type        string                     `json:"type"`
	WorkspaceID string                     `json:"workspace_id"`
	ProjectID   string                     `json:"project_id"`
	IntentID    string                     `json:"intent_id"`
	Project     ProjectSnapshot            `json:"project"`
	Workspace   WorkspaceSnapshot          `json:"workspace"`
	Intent      coordination.Intent        `json:"intent"`
	ActiveWork  []WorkSnapshot             `json:"active_work"`
	Knowledge   knowledge.WorkspaceContext `json:"knowledge"`
	Handoffs    []coordination.Handoff     `json:"handoffs"`
	Warnings    []string                   `json:"warnings"`
}

type CompileResult struct {
	Pack     ContextPack `json:"context_pack"`
	EventID  string      `json:"event_id"`
	Replayed bool        `json:"replayed"`
}
