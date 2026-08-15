// Package api exposes the repository-owned API source definitions to binaries
// that need to inspect the contracts at runtime.
package api

import "embed"

// Files contains the complete HTTP and protobuf API source trees.
//
//go:embed all:http all:proto
var Files embed.FS
