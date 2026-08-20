package adminui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

func TestHandlerServesReactApplicationShell(t *testing.T) {
	response := serve(t, "/admin/", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != applicationShell {
		t.Fatalf("Content-Type = %q, want %q", contentType, applicationShell)
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
	if body := response.Body.String(); !strings.Contains(body, `id="root"`) || !strings.Contains(body, `type="module"`) {
		t.Fatalf("response is not the Vite application shell: %s", body)
	}
}

func TestHandlerServesHashedFrontendAssetsWithImmutableCaching(t *testing.T) {
	assetPaths := embeddedAssetsByExtension(t, ".js", ".css")
	for extension, assetPath := range assetPaths {
		t.Run(extension, func(t *testing.T) {
			response := serve(t, adminPrefix+assetPath, "")
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if cacheControl := response.Header().Get("Cache-Control"); cacheControl != immutableCache {
				t.Fatalf("Cache-Control = %q, want %q", cacheControl, immutableCache)
			}
			contentType := response.Header().Get("Content-Type")
			if extension == ".js" && contentType != javascriptContent {
				t.Fatalf("Content-Type = %q, want %q", contentType, javascriptContent)
			}
			if extension == ".css" && !strings.HasPrefix(contentType, "text/css") {
				t.Fatalf("Content-Type = %q, want CSS", contentType)
			}
			if response.Body.Len() == 0 {
				t.Fatal("asset body is empty")
			}
		})
	}
}

func TestHandlerServesPublicAssetsWithShortCaching(t *testing.T) {
	response := serve(t, "/admin/favicon.svg", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "image/svg+xml" {
		t.Fatalf("Content-Type = %q, want SVG", contentType)
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != shortPublicCache {
		t.Fatalf("Cache-Control = %q, want %q", cacheControl, shortPublicCache)
	}
}

func TestHandlerFallsBackToApplicationShellForHTMLRoutes(t *testing.T) {
	root := serve(t, "/admin/", "")
	route := serve(t, "/admin/workspaces/018f784a/overview", "text/html,application/xhtml+xml")

	if route.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", route.Code, http.StatusOK)
	}
	if route.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", route.Header().Get("Cache-Control"))
	}
	if route.Body.String() != root.Body.String() {
		t.Fatal("SPA route did not return the application shell")
	}
}

func TestHandlerDoesNotFallBackForUnknownAssetsOrNonHTMLRequests(t *testing.T) {
	tests := []struct {
		name   string
		target string
		accept string
	}{
		{name: "missing javascript", target: "/admin/assets/missing.js", accept: "text/html"},
		{name: "missing image", target: "/admin/missing.svg", accept: "text/html"},
		{name: "missing extensionless asset", target: "/admin/assets/missing", accept: "text/html"},
		{name: "JSON navigation", target: "/admin/workspaces/missing", accept: "application/json"},
		{name: "path traversal", target: "/admin/../favicon.svg", accept: "text/html"},
		{name: "outside mount", target: "/not-admin", accept: "text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := serve(t, tt.target, tt.accept)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
			if strings.Contains(strings.ToLower(response.Body.String()), "<!doctype html>") {
				t.Fatal("unknown asset unexpectedly returned the application shell")
			}
		})
	}
}

func TestHandlerKeepsContentSecurityPolicyStrict(t *testing.T) {
	response := serve(t, "/admin/", "")
	policy := response.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"script-src 'self'",
		"script-src-attr 'none'",
		"style-src-attr 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
	} {
		if !strings.Contains(policy, directive) {
			t.Errorf("Content-Security-Policy does not contain %q: %q", directive, policy)
		}
	}
	for _, unsafe := range []string{"'unsafe-inline'", "'unsafe-eval'"} {
		if strings.Contains(policy, unsafe) {
			t.Errorf("Content-Security-Policy contains %q: %q", unsafe, policy)
		}
	}
}

func embeddedAssetsByExtension(t *testing.T, extensions ...string) map[string]string {
	t.Helper()
	subtree, err := fs.Sub(assets, "dist")
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]string, len(extensions))
	err = fs.WalkDir(subtree, ".", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		extension := path.Ext(assetPath)
		for _, wanted := range extensions {
			if extension == wanted {
				found[wanted] = assetPath
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, extension := range extensions {
		if found[extension] == "" {
			t.Fatalf("embedded Vite output does not contain a %s asset", extension)
		}
	}
	return found
}

func serve(t *testing.T, target, accept string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, request)
	return response
}
