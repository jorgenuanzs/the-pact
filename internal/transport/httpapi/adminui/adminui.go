package adminui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed assets/*
var assets embed.FS

func Handler() http.Handler {
	subtree, err := fs.Sub(assets, "assets")
	if err != nil {
		panic("create embedded admin filesystem: " + err.Error())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)

		requestPath := strings.TrimPrefix(r.URL.Path, "/admin/")
		switch requestPath {
		case "":
			requestPath = "index.html"
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
		case "styles.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
		case "app.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
		default:
			http.NotFound(w, r)
			return
		}

		body, readErr := fs.ReadFile(subtree, path.Clean(requestPath))
		if readErr != nil {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set(
		"Content-Security-Policy",
		"default-src 'self'; connect-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'",
	)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}
