package httpserver

import (
	"io/fs"
	"net/http"
)

// newSPAHandler returns an HTTP handler that serves a single-page app from the given fs.FS.
// It falls back to serving index.html for any path that doesn't exist in the filesystem,
// allowing the client-side router to take over. This is the standard SPA pattern for
// serving React, Vue, Svelte, etc. apps that handle all routing client-side.
func newSPAHandler(uiFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the requested path exists in the filesystem
		path := r.URL.Path
		if path == "" || path == "/" {
			path = "index.html"
		} else if path[0] == '/' {
			path = path[1:]
		}

		// Try to open the file; if it doesn't exist, fall back to index.html
		if _, err := fs.Stat(uiFS, path); err != nil {
			path = "index.html"
		}

		// Serve the file (or index.html as fallback)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + path
		http.FileServerFS(uiFS).ServeHTTP(w, r2)
	})
}
