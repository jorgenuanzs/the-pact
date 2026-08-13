package projects

import "time"

type Project struct {
	ID                string            `json:"id"`
	OrganizationID    string            `json:"-"`
	Name              string            `json:"name"`
	Slug              string            `json:"slug"`
	Status            string            `json:"status"`
	CanonicalRevision *string           `json:"canonical_revision"`
	RootRepository    *SourceRepository `json:"root_repository"`
	Version           int64             `json:"version"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type CreateInput struct {
	Name              string                 `json:"name"`
	Slug              string                 `json:"slug"`
	CanonicalRevision *string                `json:"canonical_revision,omitempty"`
	RootRepository    *SourceRepositoryInput `json:"root_repository,omitempty"`
}

type CreateResult struct {
	Project  Project
	Replayed bool
}

type SourceRepository struct {
	ID            string  `json:"id"`
	Slug          string  `json:"slug"`
	Name          string  `json:"name"`
	VCSType       string  `json:"vcs_type"`
	Status        string  `json:"status"`
	RemoteURL     *string `json:"remote_url"`
	DefaultBranch string  `json:"default_branch"`
	ObjectFormat  string  `json:"object_format"`
	Version       int64   `json:"version"`
}

type SourceRepositoryInput struct {
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	RemoteURL     string `json:"remote_url"`
	DefaultBranch string `json:"default_branch"`
	ObjectFormat  string `json:"object_format"`
}
