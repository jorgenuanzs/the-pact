package publicui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

const (
	indexPath        = "site.html"
	immutableCache   = "public, max-age=31536000, immutable"
	shortPublicCache = "public, max-age=3600"
	htmlContent      = "text/html; charset=utf-8"
	jsContent        = "text/javascript; charset=utf-8"
)

// dist is generated from web/site.html by Vite before the server is compiled.
//
//go:embed dist
var assets embed.FS

func Handler() http.Handler {
	subtree, err := fs.Sub(assets, "dist")
	if err != nil {
		panic("create embedded public site filesystem: " + err.Error())
	}
	index, err := fs.ReadFile(subtree, indexPath)
	if err != nil {
		panic("read embedded public site shell: " + err.Error())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		requestPath, ok := embeddedPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if requestPath == "" {
			serveIndex(w, index)
			return
		}
		body, readErr := fs.ReadFile(subtree, requestPath)
		if readErr != nil {
			http.NotFound(w, r)
			return
		}
		serveAsset(w, requestPath, body)
	})
}

func embeddedPath(urlPath string) (string, bool) {
	if urlPath == "/" {
		return "", true
	}
	allowed := strings.HasPrefix(urlPath, "/assets/") || urlPath == "/favicon.svg" || urlPath == "/robots.txt" || urlPath == "/sitemap.xml"
	if !allowed {
		return "", false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(urlPath, "/"), "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	cleaned := strings.TrimPrefix(path.Clean(urlPath), "/")
	if cleaned == "." || cleaned == "" || !fs.ValidPath(cleaned) {
		return "", false
	}
	return cleaned, true
}

func serveIndex(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", htmlContent)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func serveAsset(w http.ResponseWriter, requestPath string, body []byte) {
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(requestPath)))
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	if path.Ext(requestPath) == ".js" {
		contentType = jsContent
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
		"default-src 'self'; connect-src 'self' https://api.github.com; img-src 'self' data:; script-src 'self'; script-src-attr 'none'; style-src 'self' https://fonts.googleapis.com; style-src-attr 'none'; font-src 'self' https://fonts.gstatic.com; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'",
	)
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}
