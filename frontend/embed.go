// Package frontend embeds the built frontend assets.
package frontend

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed build/*
var assets embed.FS

// Handler returns an http.Handler that serves the embedded frontend.
// It strips the "build" prefix and serves index.html for SPA routes.
func Handler() http.Handler {
	// Strip the "build" prefix
	sub, err := fs.Sub(assets, "build")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if file exists
		f, err := sub.Open(path[1:]) // Remove leading slash
		if err != nil {
			// File not found, serve index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close() //nolint:errcheck // Close error not actionable

		fileServer.ServeHTTP(w, r)
	})
}
