package appconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	EnvConfigHome = "GIZCLAW_DESKTOP_CONFIG_HOME"
	AppDirName    = "gizclaw-desktop"
)

type Paths struct {
	ConfigRoot string `json:"config_root"`
	ContextDir string `json:"context_dir"`
	StateFile  string `json:"state_file"`
}

func DefaultPaths() (Paths, error) {
	if root := os.Getenv(EnvConfigHome); root != "" {
		return NewPaths(root), nil
	}
	userConfig, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("appconfig: user config dir: %w", err)
	}
	return NewPaths(filepath.Join(userConfig, AppDirName)), nil
}

func NewPaths(root string) Paths {
	return Paths{
		ConfigRoot: root,
		ContextDir: filepath.Join(root, "contexts"),
		StateFile:  filepath.Join(root, "state.json"),
	}
}

func (p Paths) Ensure() error {
	if p.ConfigRoot == "" || p.ContextDir == "" || p.StateFile == "" {
		return fmt.Errorf("appconfig: incomplete paths")
	}
	if err := os.MkdirAll(p.ContextDir, 0o700); err != nil {
		return fmt.Errorf("appconfig: mkdir contexts: %w", err)
	}
	return nil
}
