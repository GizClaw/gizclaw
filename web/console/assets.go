// Package console serves the monitoring UI bundled into the executable.
package console

import (
	"embed"
	"io/fs"
	"net/http"
)

// Build with npm run build:console before compiling Go. Missing assets must
// fail compilation so a release cannot silently omit the monitoring UI.
//
//go:embed dist/index.html dist/assets
var assets embed.FS

// Handler serves the embedded console at paths relative to its mount point.
func Handler() http.Handler {
	root, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return http.FileServerFS(root)
}
