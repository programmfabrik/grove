package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// uiFS holds the built dashboard (Vite + React + TS). The build is NOT
// committed — ui/dist is gitignored. `make grove-ui` produces it whenever a
// content hash over ui/src says the last one is stale, and a checkout without
// npm gets a one-line placeholder written there instead, so that `go build
// ./...` still has something for this embed to compile against.
//
//go:embed all:ui/dist
var uiFS embed.FS

// uiHandler serves the embedded SPA, falling back to index.html so client-side
// routes and reloads work.
func uiHandler() http.Handler {
	dist, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "grove UI not built — run `npm run build` in ui", http.StatusNotImplemented)
		})
	}
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			r.URL.Path = "/"
			p = "index.html"
		}
		if p == "index.html" {
			// index.html references content-hashed assets; never cache it, so
			// a rebuilt UI always propagates. The hashed assets are immutable.
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		files.ServeHTTP(w, r)
	})
}
