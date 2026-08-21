package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jorgenuanzs/the-pact/internal/access"
	"github.com/jorgenuanzs/the-pact/internal/repositorybinding"
)

type fakeRepositoryBindingService struct {
	resolve func(context.Context, repositorybinding.ResolveInput) ([]repositorybinding.Match, error)
}

func (f fakeRepositoryBindingService) Resolve(ctx context.Context, input repositorybinding.ResolveInput) ([]repositorybinding.Match, error) {
	return f.resolve(ctx, input)
}

func TestResolveRepositoryBindingReturnsOnlyVisibleMatches(t *testing.T) {
	const visibleProject = "018f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	const hiddenProject = "028f784a-68c1-7b0f-8f2a-cfc255f99e1d"
	handler := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		RepositoryBindingService: fakeRepositoryBindingService{resolve: func(_ context.Context, input repositorybinding.ResolveInput) ([]repositorybinding.Match, error) {
			if input.RemoteURL != "git@github.com:example/repository.git" {
				t.Fatalf("remote = %q", input.RemoteURL)
			}
			return []repositorybinding.Match{
				{ProjectID: visibleProject, RepositoryID: "visible", WorkspaceID: "workspace"},
				{ProjectID: hiddenProject, RepositoryID: "hidden", WorkspaceID: "workspace"},
			}, nil
		}},
		AccessService: fakeAccessService{
			visible: func(context.Context, access.Principal) (map[string]struct{}, error) {
				return map[string]struct{}{visibleProject: {}}, nil
			},
			projectAccess: func(_ context.Context, principal access.Principal, projectID string) (access.ProjectAccess, error) {
				return access.ProjectAccess{ProjectID: projectID, Members: []access.ProjectMember{{
					PrincipalID: principal.ID, EffectiveRole: "maintainer",
				}}}, nil
			},
		},
	})
	request := authenticatedRequest(http.MethodPost, "/v1/repository-bindings/resolve", `{"remote_url":"git@github.com:example/repository.git"}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !containsAll(body, `"repository_id":"visible"`, `"permission":"maintainer"`) || containsAll(body, `"repository_id":"hidden"`) {
		t.Fatalf("body = %s", body)
	}
}

func containsAll(value string, expected ...string) bool {
	for _, item := range expected {
		if len(item) > 0 && !strings.Contains(value, item) {
			return false
		}
	}
	return true
}
