package clicontext

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/cmd/internal/paths"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
)

// DefaultStore returns a Store under the gizclaw config directory.
func DefaultStore() (*contextstore.Store, error) {
	root, err := paths.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("clicontext: config dir: %w", err)
	}
	return &contextstore.Store{Root: root}, nil
}

func Load(dir string) (*contextstore.Context, error) {
	return contextstore.Load(dir)
}
