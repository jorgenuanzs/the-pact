package workspaces

import "time"

// Workspace is the durable collaboration and knowledge boundary around one or
// more Pact projects. Git execution checkouts are modeled separately as
// worktrees in the coordination domain.
type Workspace struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"-"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	Projects       []Project  `json:"projects"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
}

type Project struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	Slug                    string  `json:"slug"`
	Status                  string  `json:"status"`
	RootRepositoryRemoteURL *string `json:"root_repository_remote_url,omitempty"`
}

type CreateInput struct {
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Description string   `json:"description,omitempty"`
	ProjectIDs  []string `json:"project_ids,omitempty"`
}

type CreateResult struct {
	Workspace Workspace
	Replayed  bool
}
