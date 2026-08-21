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
	otherProjectID := "028f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+cliTestToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/workspaces":
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"workspaces": []map[string]any{{
				"id": cliTestWorkspaceID, "name": "Workspace", "slug": "workspace", "status": "active",
				"projects": []map[string]any{{"id": bindingTestProjectID, "name": "Project", "slug": "project", "status": "active"}},
			}}}})
		case "/v1/projects/" + bindingTestProjectID + "/repositories":
			remote := "https://github.com/example/project"
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
				"repositories": []map[string]any{{
					"id": cliTestRepositoryID, "project_id": otherProjectID, "name": "project",
					"slug": "project", "remote_url": remote, "primary": true,
				}},
				"sync_states": []any{},
			}})
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
	if err == nil || !strings.Contains(err.Error(), "does not belong to visible workspace") {
		t.Fatalf("workspace validation error = %v", err)
	}

	_, err = resolveRemoteBinding(
		context.Background(), client, bindingTestProjectID,
		"git@github.com:example/project.git", cliTestWorkspaceID, cliTestRepositoryID,
	)
	if err == nil || !strings.Contains(err.Error(), "belongs to project "+otherProjectID) {
		t.Fatalf("repository validation error = %v", err)
	}
}
