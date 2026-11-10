// Package webui serves the embedded Next.js static dashboard.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var files embed.FS

// Handler returns an http.Handler that serves the embedded dashboard, falling
// back to index.html so client-side routes work without a server.
func Handler() http.Handler {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "dashboard not built", http.StatusNotFound)
		})
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(r.URL.Path)
		if strings.HasPrefix(clean, "/api/") {
			http.NotFound(w, r)
			return
		}

		rel := strings.TrimPrefix(clean, "/")
		if rel == "" {
			rel = "index.html"
		}

		if st, err := fs.Stat(sub, rel); err == nil {
			if st.IsDir() {
				r.URL.Path = "/" + strings.TrimSuffix(rel, "/") + "/"
			} else if rel == "index.html" {
				r.URL.Path = "/"
			} else {
				r.URL.Path = "/" + rel
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// Next static export writes nested routes as <route>/index.html.
		candidate := path.Join(rel, "index.html")
		if _, err := fs.Stat(sub, candidate); err == nil {
			r.URL.Path = "/" + strings.TrimSuffix(candidate, "index.html")
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
