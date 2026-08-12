// Package buildinfo exposes immutable identity embedded in the GizClaw binary.
package buildinfo

var (
	// Version is the software version embedded by the build pipeline.
	Version = "dev"
	// Commit is the source commit embedded by the build pipeline.
	Commit = "dev"
)
