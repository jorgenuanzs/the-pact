package githubapp

import "time"

type Installation struct {
	InstallationID      int64             `json:"installation_id"`
	AccountID           int64             `json:"account_id"`
	AccountLogin        string            `json:"account_login"`
	AccountType         string            `json:"account_type"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
	Status              string            `json:"status"`
	InstalledAt         time.Time         `json:"installed_at"`
	SuspendedAt         *time.Time        `json:"suspended_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	Version             int64             `json:"version"`
}

type Repository struct {
	GitHubRepositoryID int64      `json:"github_repository_id"`
	InstallationID     int64      `json:"installation_id"`
	OwnerLogin         string     `json:"owner_login"`
	Name               string     `json:"name"`
	FullName           string     `json:"full_name"`
	Private            bool       `json:"private"`
	Visibility         string     `json:"visibility"`
	DefaultBranch      string     `json:"default_branch"`
	HTMLURL            string     `json:"html_url"`
	CloneURL           string     `json:"clone_url"`
	Status             string     `json:"status"`
	ProviderUpdatedAt  *time.Time `json:"provider_updated_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Version            int64      `json:"version"`
}

type Status struct {
	Configured      bool           `json:"configured"`
	Installations   []Installation `json:"installations"`
	RepositoryCount int            `json:"repository_count"`
}

type Connection struct {
	InstallURL string    `json:"install_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type ProviderInstallation struct {
	ID                  int64
	AccountID           int64
	AccountLogin        string
	AccountType         string
	RepositorySelection string
	Permissions         map[string]string
	SuspendedAt         *time.Time
}

type ProviderRepository struct {
	ID                int64
	InstallationID    int64
	OwnerLogin        string
	Name              string
	FullName          string
	Private           bool
	Visibility        string
	DefaultBranch     string
	HTMLURL           string
	CloneURL          string
	ProviderUpdatedAt *time.Time
}
