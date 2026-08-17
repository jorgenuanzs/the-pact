package access

import "time"

const BootstrapPrincipalID = "00000000-0000-4000-8000-000000000002"

type Principal struct {
	ID               string `json:"id"`
	OrganizationID   string `json:"organization_id"`
	DisplayName      string `json:"display_name"`
	PrincipalType    string `json:"principal_type"`
	OrganizationRole string `json:"organization_role"`
	Bootstrap        bool   `json:"bootstrap"`
}

type CreateInvitationInput struct {
	Email            string        `json:"email"`
	Role             string        `json:"role"`
	OrganizationRole string        `json:"organization_role,omitempty"`
	ExpiresAfter     time.Duration `json:"-"`
}

type Invitation struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	Email            string    `json:"email"`
	Role             string    `json:"role"`
	OrganizationRole string    `json:"organization_role"`
	Status           string    `json:"status"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type CreatedInvitation struct {
	Invitation Invitation `json:"invitation"`
	Secret     string     `json:"secret"`
}

// ProjectAccess is the effective access roster for a project. Organization
// owners and administrators appear even when they do not have a direct project
// membership because their organization role grants access to every project.
type ProjectAccess struct {
	ProjectID   string          `json:"project_id"`
	Members     []ProjectMember `json:"members"`
	Agents      []ProjectAgent  `json:"agents"`
	GeneratedAt time.Time       `json:"generated_at"`
}

type ProjectMember struct {
	PrincipalID      string `json:"principal_id"`
	DisplayName      string `json:"display_name"`
	PrincipalType    string `json:"principal_type"`
	Status           string `json:"status"`
	OrganizationRole string `json:"organization_role"`
	ProjectRole      string `json:"project_role,omitempty"`
	EffectiveRole    string `json:"effective_role"`
	AccessSource     string `json:"access_source"`
	Bootstrap        bool   `json:"bootstrap"`
}

type ProjectAgent struct {
	AgentID              string     `json:"agent_id"`
	DisplayName          string     `json:"display_name"`
	AgentType            string     `json:"agent_type"`
	Status               string     `json:"status"`
	SponsorPrincipalID   string     `json:"sponsor_principal_id"`
	SponsorDisplayName   string     `json:"sponsor_display_name"`
	SponsorEffectiveRole string     `json:"sponsor_effective_role,omitempty"`
	AccessActive         bool       `json:"access_active"`
	Connected            bool       `json:"connected"`
	ActiveSessions       int64      `json:"active_sessions"`
	SessionCount         int64      `json:"session_count"`
	LastClientType       string     `json:"last_client_type,omitempty"`
	LastNodeName         string     `json:"last_node_name,omitempty"`
	LastSeenAt           *time.Time `json:"last_seen_at,omitempty"`
}
