package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
)

type remoteBindingTarget struct {
	WorkspaceID       string
	WorkspaceName     string
	WorkspaceSlug     string
	RepositoryID      string
	RepositoryName    string
	RepositoryRemote  string
	RepositoryPrimary bool
	ProjectID         string
}

type workspaceBindingMatch struct {
	ID   string
	Name string
	Slug string
}

type repositoryBindingMatch struct {
	ID      string
	Name    string
	Remote  string
	Primary bool
}

// resolveRemoteBinding validates the complete folder destination using the
// currently available project, workspace and repository APIs. Hito 4 replaces
// this multi-request lookup with the dedicated repository-binding endpoint.
func resolveRemoteBinding(
	ctx context.Context,
	client *pactclient.Client,
	projectID string,
	localRemote string,
	requestedWorkspaceID string,
	requestedRepositoryID string,
) (remoteBindingTarget, error) {
	localRemote, err := localproject.NormalizeGitRemote(localRemote)
	if err != nil {
		return remoteBindingTarget{}, err
	}
	workspaceList, err := client.ListWorkspaces(ctx)
	if err != nil {
		return remoteBindingTarget{}, fmt.Errorf("resolve workspace for project %s: %w", projectID, err)
	}
	requestedWorkspaceID = strings.TrimSpace(requestedWorkspaceID)
	workspaceMatches := make([]workspaceBindingMatch, 0, 1)
	for _, workspace := range workspaceList {
		if requestedWorkspaceID != "" && workspace.ID != requestedWorkspaceID {
			continue
		}
		for _, project := range workspace.Projects {
			if project.ID == projectID {
				workspaceMatches = append(workspaceMatches, workspaceBindingMatch{
					ID: workspace.ID, Name: workspace.Name, Slug: workspace.Slug,
				})
				break
			}
		}
	}
	if len(workspaceMatches) == 0 {
		if requestedWorkspaceID != "" {
			return remoteBindingTarget{}, fmt.Errorf(
				"project %s does not belong to visible workspace %s", projectID, requestedWorkspaceID,
			)
		}
		return remoteBindingTarget{}, fmt.Errorf("project %s does not belong to a visible workspace", projectID)
	}
	if len(workspaceMatches) > 1 {
		return remoteBindingTarget{}, fmt.Errorf(
			"project %s belongs to multiple visible workspaces; select one with --workspace", projectID,
		)
	}

	repositorySet, err := client.ListProjectRepositories(ctx, projectID)
	if err != nil {
		return remoteBindingTarget{}, fmt.Errorf("resolve repositories for project %s: %w", projectID, err)
	}
	requestedRepositoryID = strings.TrimSpace(requestedRepositoryID)
	repositoryMatches := make([]repositoryBindingMatch, 0, 1)
	requestedFound := false
	for _, repository := range repositorySet.Repositories {
		if repository.ProjectID != "" && repository.ProjectID != projectID {
			return remoteBindingTarget{}, fmt.Errorf(
				"repository %s belongs to project %s, not %s", repository.ID, repository.ProjectID, projectID,
			)
		}
		if requestedRepositoryID != "" && repository.ID != requestedRepositoryID {
			continue
		}
		if requestedRepositoryID != "" {
			requestedFound = true
		}
		if repository.RemoteURL == nil || strings.TrimSpace(*repository.RemoteURL) == "" {
			continue
		}
		remote, normalizeErr := localproject.NormalizeGitRemote(*repository.RemoteURL)
		if normalizeErr != nil {
			continue
		}
		if remote == localRemote {
			repositoryMatches = append(repositoryMatches, repositoryBindingMatch{
				ID: repository.ID, Name: repository.Name, Remote: remote, Primary: repository.Primary,
			})
		}
	}
	if requestedRepositoryID != "" && !requestedFound {
		return remoteBindingTarget{}, fmt.Errorf(
			"repository %s does not belong to project %s", requestedRepositoryID, projectID,
		)
	}
	if len(repositoryMatches) == 0 {
		if requestedRepositoryID != "" {
			return remoteBindingTarget{}, errors.New("selected repository does not match this checkout's Git remote")
		}
		return remoteBindingTarget{}, fmt.Errorf(
			"no repository in project %s matches Git remote %s", projectID, localRemote,
		)
	}
	if len(repositoryMatches) > 1 {
		return remoteBindingTarget{}, errors.New(
			"multiple project repositories match this Git remote; select one with --repository",
		)
	}
	return remoteBindingTarget{
		WorkspaceID: workspaceMatches[0].ID, WorkspaceName: workspaceMatches[0].Name,
		WorkspaceSlug: workspaceMatches[0].Slug, RepositoryID: repositoryMatches[0].ID,
		RepositoryName: repositoryMatches[0].Name, RepositoryRemote: repositoryMatches[0].Remote,
		RepositoryPrimary: repositoryMatches[0].Primary,
		ProjectID:         projectID,
	}, nil
}
