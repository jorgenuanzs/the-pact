package repositorybinding

type ResolveInput struct {
	RemoteURL   string `json:"remote_url"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

type Match struct {
	WorkspaceID    string `json:"workspace_id"`
	WorkspaceName  string `json:"workspace_name"`
	WorkspaceSlug  string `json:"workspace_slug"`
	ProjectID      string `json:"project_id"`
	ProjectName    string `json:"project_name"`
	RepositoryID   string `json:"repository_id"`
	RepositoryName string `json:"repository_name"`
	RepositorySlug string `json:"repository_slug"`
	Primary        bool   `json:"primary"`
	Match          string `json:"match"`
	Permission     string `json:"permission,omitempty"`
}

type Candidate struct {
	Match
	RemoteURL string
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }
