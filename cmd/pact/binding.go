package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jorgenuanzs/the-pact/internal/localproject"
	"github.com/jorgenuanzs/the-pact/internal/pactclient"
	"github.com/jorgenuanzs/the-pact/internal/repositorybinding"
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

// resolveRemoteBinding delegates matching and visibility enforcement to the
// server. The CLI only applies the explicit project/repository selectors and
// rejects ambiguous results.
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
	matches, err := client.ResolveRepositoryBinding(ctx, repositorybinding.ResolveInput{
		RemoteURL: localRemote, WorkspaceID: strings.TrimSpace(requestedWorkspaceID),
	})
	if err != nil {
		return remoteBindingTarget{}, fmt.Errorf("resolve repository binding: %w", err)
	}
	requestedRepositoryID = strings.TrimSpace(requestedRepositoryID)
	filtered := make([]repositorybinding.Match, 0, len(matches))
	for _, match := range matches {
		if match.ProjectID != projectID {
			continue
		}
		if requestedRepositoryID != "" && match.RepositoryID != requestedRepositoryID {
			continue
		}
		filtered = append(filtered, match)
	}
	if len(filtered) == 0 {
		if requestedRepositoryID != "" {
			return remoteBindingTarget{}, errors.New("selected repository does not match this checkout, project, and workspace")
		}
		return remoteBindingTarget{}, fmt.Errorf(
			"no visible repository in project %s matches Git remote %s", projectID, localRemote,
		)
	}
	if len(filtered) > 1 {
		return remoteBindingTarget{}, errors.New(
			"multiple repository bindings match this Git remote; select a workspace and repository explicitly",
		)
	}
	match := filtered[0]
	return remoteBindingTarget{
		WorkspaceID: match.WorkspaceID, WorkspaceName: match.WorkspaceName,
		WorkspaceSlug: match.WorkspaceSlug, RepositoryID: match.RepositoryID,
		RepositoryName: match.RepositoryName, RepositoryRemote: localRemote,
		RepositoryPrimary: match.Primary,
		ProjectID:         projectID,
	}, nil
}
