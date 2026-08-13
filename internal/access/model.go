package access

import "time"

const BootstrapPrincipalID = "00000000-0000-4000-8000-000000000002"

type Principal struct {
	ID               string `json:"id"`
	OrganizationID   string `json:"organization_id"`
	DisplayName      string `json:"display_name"`
	PrincipalType    string `json:"principal_type"`
	OrganizationRole string `json:"organization_role"`
	TokenID          string `json:"-"`
	Bootstrap        bool   `json:"bootstrap"`
}

type CreateInvitationInput struct {
	Email        string        `json:"email"`
	Role         string        `json:"role"`
	ExpiresAfter time.Duration `json:"-"`
}

type Invitation struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CreatedInvitation struct {
	Invitation Invitation `json:"invitation"`
	Secret     string     `json:"secret"`
}

type AcceptInvitationInput struct {
	Secret      string `json:"secret"`
	DisplayName string `json:"display_name"`
	TokenName   string `json:"token_name"`
}

type AcceptedInvitation struct {
	Principal   Principal `json:"principal"`
	ProjectID   string    `json:"project_id"`
	ProjectRole string    `json:"project_role"`
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}
