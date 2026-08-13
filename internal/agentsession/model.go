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
