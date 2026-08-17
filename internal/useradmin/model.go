package useradmin

import "time"

type ProjectPermission struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	ProjectSlug string `json:"project_slug"`
	Role        string `json:"role"`
}

type User struct {
	PrincipalID      string              `json:"principal_id"`
	DisplayName      string              `json:"display_name"`
	Email            string              `json:"email"`
	Username         string              `json:"username"`
	Status           string              `json:"status"`
	OrganizationRole string              `json:"organization_role"`
	ProjectRoles     []ProjectPermission `json:"project_roles"`
	ActiveSessions   int64               `json:"active_sessions"`
	ActiveDevices    int64               `json:"active_devices"`
	LastLoginAt      *time.Time          `json:"last_login_at,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

type Invitation struct {
	ID                   string    `json:"id"`
	Email                string    `json:"email"`
	OrganizationRole     string    `json:"organization_role"`
	ProjectID            string    `json:"project_id,omitempty"`
	ProjectName          string    `json:"project_name,omitempty"`
	ProjectRole          string    `json:"project_role,omitempty"`
	Status               string    `json:"status"`
	ExpiresAt            time.Time `json:"expires_at"`
	CreatedAt            time.Time `json:"created_at"`
	CreatedByPrincipalID string    `json:"created_by_principal_id"`
	CreatedByDisplayName string    `json:"created_by_display_name"`
}

type CreatedInvitation struct {
	Invitation Invitation `json:"invitation"`
	Secret     string     `json:"secret"`
}

type AdminEvent struct {
	ID                string         `json:"id"`
	Action            string         `json:"action"`
	ActorPrincipalID  string         `json:"actor_principal_id"`
	ActorDisplayName  string         `json:"actor_display_name"`
	TargetPrincipalID string         `json:"target_principal_id,omitempty"`
	TargetDisplayName string         `json:"target_display_name,omitempty"`
	Details           map[string]any `json:"details"`
	CreatedAt         time.Time      `json:"created_at"`
}

type Directory struct {
	Users       []User       `json:"users"`
	Invitations []Invitation `json:"invitations"`
	Events      []AdminEvent `json:"events"`
	GeneratedAt time.Time    `json:"generated_at"`
}

type UpdateUserInput struct {
	DisplayName      *string `json:"display_name,omitempty"`
	Email            *string `json:"email,omitempty"`
	Username         *string `json:"username,omitempty"`
	Status           *string `json:"status,omitempty"`
	OrganizationRole *string `json:"organization_role,omitempty"`
}

type CreateInvitationInput struct {
	Email            string        `json:"email"`
	OrganizationRole string        `json:"organization_role"`
	ProjectID        string        `json:"project_id,omitempty"`
	ProjectRole      string        `json:"project_role,omitempty"`
	ExpiresAfter     time.Duration `json:"-"`
}
