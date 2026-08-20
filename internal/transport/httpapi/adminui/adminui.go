package adminui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

const (
	adminPrefix       = "/admin/"
	indexPath         = "index.html"
	immutableCache    = "public, max-age=31536000, immutable"
	shortPublicCache  = "public, max-age=86400"
	applicationShell  = "text/html; charset=utf-8"
	javascriptContent = "text/javascript; charset=utf-8"
)

// dist is generated from web/ by Vite before the Go server is compiled.
//
//go:embed dist
var assets embed.FS

func Handler() http.Handler {
	subtree, err := fs.Sub(assets, "dist")
	if err != nil {
		panic("create embedded admin filesystem: " + err.Error())
	}
	index, err := fs.ReadFile(subtree, indexPath)
	if err != nil {
		panic("read embedded admin shell: " + err.Error())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)

		requestPath, ok := embeddedPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if requestPath == "" || requestPath == indexPath {
			serveIndex(w, index)
			return
		}

		body, readErr := fs.ReadFile(subtree, requestPath)
		if readErr == nil {
			serveAsset(w, requestPath, body)
			return
		}
		if shouldServeApplicationShell(r, requestPath) {
			serveIndex(w, index)
			return
		}

		http.NotFound(w, r)
	})
}

func embeddedPath(urlPath string) (string, bool) {
	if !strings.HasPrefix(urlPath, adminPrefix) {
		return "", false
	}
	relative := strings.TrimPrefix(urlPath, adminPrefix)
	if relative == "" {
		return "", true
	}
	for _, segment := range strings.Split(relative, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}

	// Anchor the clean operation at the embedded filesystem root. This prevents
	// traversal while still accepting normalized, nested SPA routes.
	cleaned := strings.TrimPrefix(path.Clean("/"+relative), "/")
	if cleaned == "." || cleaned == "" || !fs.ValidPath(cleaned) {
		return "", false
	}
	return cleaned, true
}

func shouldServeApplicationShell(r *http.Request, requestPath string) bool {
	if path.Ext(requestPath) != "" || requestPath == "assets" || strings.HasPrefix(requestPath, "assets/") {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

func serveIndex(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", applicationShell)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func serveAsset(w http.ResponseWriter, requestPath string, body []byte) {
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(requestPath)))
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	// Go may obtain application/javascript from an operating-system MIME
	// database. text/javascript is the interoperable value expected by module
	// scripts and keeps the response deterministic across platforms.
	if path.Ext(requestPath) == ".js" {
		contentType = javascriptContent
	}
	w.Header().Set("Content-Type", contentType)
	if strings.HasPrefix(requestPath, "assets/") {
		w.Header().Set("Cache-Control", immutableCache)
	} else {
		w.Header().Set("Cache-Control", shortPublicCache)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set(
		"Content-Security-Policy",
		"default-src 'self'; connect-src 'self'; img-src 'self' data:; script-src 'self'; script-src-attr 'none'; style-src 'self' https://fonts.googleapis.com; style-src-attr 'none'; font-src 'self' https://fonts.gstatic.com; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'",
	)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}
