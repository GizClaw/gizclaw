// Package resources owns the versioned data bundled with GizClaw Desktop.
package resources

import (
	"embed"
	"io/fs"
)

//go:embed local-server
var bundled embed.FS

// LocalServer returns the read-only bootstrap catalog for newly created local
// Servers. Callers receive a filesystem rooted at the catalog itself.
func LocalServer() (fs.FS, error) {
	return fs.Sub(bundled, "local-server")
}
