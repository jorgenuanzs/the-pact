package projectrepo

import (
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("project repository was not found")
	ErrProviderNotFound = errors.New("authorized GitHub repository was not found")
	ErrAlreadyAttached  = errors.New("GitHub repository is already attached to this project")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

type Repository struct {
	ID                 string     `json:"id"`
	ProjectID          string     `json:"project_id"`
	GitHubRepositoryID *int64     `json:"github_repository_id"`
	Slug               string     `json:"slug"`
	Name               string     `json:"name"`
	VCSType            string     `json:"vcs_type"`
	Status             string     `json:"status"`
	RemoteURL          *string    `json:"remote_url"`
	DefaultBranch      string     `json:"default_branch"`
	ObjectFormat       string     `json:"object_format"`
	Purpose            string     `json:"purpose"`
	Required           bool       `json:"required"`
	Primary            bool       `json:"primary"`
	GitHubFullName     string     `json:"github_full_name,omitempty"`
	Visibility         string     `json:"visibility,omitempty"`
	SyncStatus         string     `json:"sync_status"`
	CanonicalRevision  *string    `json:"canonical_revision"`
	LastSuccessAt      *time.Time `json:"last_success_at"`
	Version            int64      `json:"version"`
}

type AvailableRepository struct {
	GitHubRepositoryID   int64     `json:"github_repository_id"`
	InstallationID       int64     `json:"installation_id"`
	AccountLogin         string    `json:"account_login"`
	Name                 string    `json:"name"`
	FullName             string    `json:"full_name"`
	Private              bool      `json:"private"`
	Visibility           string    `json:"visibility"`
	DefaultBranch        string    `json:"default_branch"`
	HTMLURL              string    `json:"html_url"`
	CloneURL             string    `json:"clone_url"`
	AttachedRepositoryID *string   `json:"attached_repository_id"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type AttachInput struct {
	GitHubRepositoryID int64  `json:"github_repository_id"`
	Purpose            string `json:"purpose"`
	Required           *bool  `json:"required,omitempty"`
	Primary            bool   `json:"primary"`
}
