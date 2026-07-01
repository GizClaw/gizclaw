package clicontext

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/cmd/internal/paths"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
)

type ServerConfig = contextstore.ServerConfig
type Config = contextstore.Config
type CLIContext = contextstore.Context
type Store = contextstore.Store
type CreateOptions = contextstore.CreateOptions
type Summary = contextstore.Summary

// DefaultStore returns a Store under the gizclaw config directory.
func DefaultStore() (*Store, error) {
	root, err := paths.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("clicontext: config dir: %w", err)
	}
	return &Store{Root: root}, nil
}

func Load(dir string) (*CLIContext, error) {
	return contextstore.Load(dir)
}
