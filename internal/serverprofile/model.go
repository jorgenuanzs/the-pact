package serverprofile

import "time"

const SchemaVersion = 3

type Kind string

const (
	KindRemote       Kind = "remote"
	KindManagedLocal Kind = "managed_local"
)

type Profile struct {
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	ServerURL      string    `json:"server_url"`
	Kind           Kind      `json:"kind"`
	PrincipalID    string    `json:"principal_id,omitempty"`
	PrincipalLabel string    `json:"principal_label,omitempty"`
	CredentialRef  string    `json:"credential_ref"`
	CreatedAt      time.Time `json:"created_at"`
	LastUsedAt     time.Time `json:"last_used_at"`
}

type State struct {
	SchemaVersion   int       `json:"schema_version"`
	ActiveProfileID string    `json:"active_profile_id,omitempty"`
	Profiles        []Profile `json:"profiles"`
}

type AuthorizedProfile struct {
	Profile
	DeviceCredential string `json:"-"`
}

type AuthorizedInput struct {
	ServerURL        string
	Label            string
	Kind             Kind
	PrincipalID      string
	PrincipalLabel   string
	DeviceCredential string
}
