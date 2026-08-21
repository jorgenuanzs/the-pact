package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jorgenuanzs/the-pact/internal/pactclient"
)

const bindingTestProjectID = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"

func TestResolveRemoteBindingRejectsInvalidWorkspaceAndRepositoryMembership(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+cliTestToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/repository-bindings/resolve":
			var input struct {
				WorkspaceID string `json:"workspace_id"`
			}
			_ = json.NewDecoder(request.Body).Decode(&input)
			if input.WorkspaceID != "" && input.WorkspaceID != cliTestWorkspaceID {
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"matches": []any{}}})
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"matches": []map[string]any{{
				"workspace_id": cliTestWorkspaceID, "workspace_name": "Workspace", "workspace_slug": "workspace",
				"project_id": bindingTestProjectID, "project_name": "Project",
				"repository_id": cliTestRepositoryID, "repository_name": "project", "repository_slug": "project",
				"primary": true, "match": "exact",
			}}}})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := pactclient.New(server.URL, cliTestToken)
	if err != nil {
		t.Fatal(err)
	}

	_, err = resolveRemoteBinding(
		context.Background(), client, bindingTestProjectID,
		"git@github.com:example/project.git", "038f784a-68c1-7b0f-8f2a-cfc255f99e3f", "",
	)
	if err == nil || !strings.Contains(err.Error(), "no visible repository") {
		t.Fatalf("workspace validation error = %v", err)
	}

	_, err = resolveRemoteBinding(
		context.Background(), client, bindingTestProjectID,
		"git@github.com:example/project.git", cliTestWorkspaceID, cliTestRepositoryID,
	)
	if err != nil {
		t.Fatalf("repository validation error = %v", err)
	}
}
