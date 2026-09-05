// Package monitorweb embeds the monitor UI built by npm run build:monitor.
package monitorweb

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// Handler serves the two application routes and immutable local assets.
func Handler() http.Handler {
	root, _ := fs.Sub(assets, "dist")
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(405)
			return
		}
		switch r.URL.Path {
		case "/monitor", "/monitor/", "/monitor/node", "/monitor/peer":
			data, err := fs.ReadFile(root, "index.html")
			if err != nil {
				http.Error(w, "Monitor assets unavailable: run npm run build:monitor before building Go", 503)
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method == http.MethodGet {
				_, _ = w.Write(data)
			}
		default:
			if !strings.HasPrefix(r.URL.Path, "/monitor/assets/") || strings.HasSuffix(r.URL.Path, "/") {
				http.NotFound(w, r)
				return
			}
			http.StripPrefix("/monitor/", files).ServeHTTP(w, r)
		}
	})
}
