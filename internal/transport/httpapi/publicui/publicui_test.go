package publicui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

func TestHandlerServesPublicLanding(t *testing.T) {
	response := serve(t, "/")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("Content-Type") != htmlContent {
		t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	body := response.Body.String()
	if !strings.Contains(body, `id="site-root"`) || !strings.Contains(body, `type="module"`) {
		t.Fatalf("response is not the public site shell: %s", body)
	}
}

func TestHandlerServesHashedAssetsAndPublicMetadata(t *testing.T) {
	subtree, err := fs.Sub(assets, "dist")
	if err != nil {
		t.Fatal(err)
	}
	var jsAsset string
	err = fs.WalkDir(subtree, "assets", func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr == nil && !entry.IsDir() && path.Ext(assetPath) == ".js" && jsAsset == "" {
			jsAsset = assetPath
		}
		return walkErr
	})
	if err != nil || jsAsset == "" {
		t.Fatalf("find JavaScript asset: path=%q err=%v", jsAsset, err)
	}
	assetResponse := serve(t, "/"+jsAsset)
	if assetResponse.Code != http.StatusOK || assetResponse.Header().Get("Cache-Control") != immutableCache {
		t.Fatalf("asset status=%d cache=%q", assetResponse.Code, assetResponse.Header().Get("Cache-Control"))
	}
	faviconResponse := serve(t, "/favicon.svg")
	if faviconResponse.Code != http.StatusOK || faviconResponse.Header().Get("Cache-Control") != shortPublicCache {
		t.Fatalf("favicon status=%d cache=%q", faviconResponse.Code, faviconResponse.Header().Get("Cache-Control"))
	}
}

func TestHandlerRejectsUnknownAndTraversalPaths(t *testing.T) {
	for _, target := range []string{"/admin/", "/missing", "/assets/missing.js", "/assets/../favicon.svg"} {
		response := serve(t, target)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want %d", target, response.Code, http.StatusNotFound)
		}
	}
}

func TestHandlerKeepsSecurityPolicyStrict(t *testing.T) {
	policy := serve(t, "/").Header().Get("Content-Security-Policy")
	for _, directive := range []string{"script-src 'self'", "style-src-attr 'none'", "frame-ancestors 'none'", "connect-src 'self' https://api.github.com"} {
		if !strings.Contains(policy, directive) {
			t.Errorf("policy does not contain %q: %q", directive, policy)
		}
	}
}

func serve(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, request)
	return response
}
