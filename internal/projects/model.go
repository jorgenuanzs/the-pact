package projects

import "time"

type Project struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"-"`
	Name              string    `json:"name"`
	Slug              string    `json:"slug"`
	CanonicalRevision *string   `json:"canonical_revision"`
	Version           int64     `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name              string  `json:"name"`
	Slug              string  `json:"slug"`
	CanonicalRevision *string `json:"canonical_revision,omitempty"`
}

type CreateResult struct {
	Project  Project
	Replayed bool
}
