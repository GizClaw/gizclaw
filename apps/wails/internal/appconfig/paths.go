package appconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	EnvConfigHome = "GIZCLAW_DESKTOP_CONFIG_HOME"
	AppDirName    = "GizClaw"
)

type Paths struct {
	ConfigRoot string `json:"config_root"`
	PodsDir    string `json:"pods_dir"`
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
	return Paths{ConfigRoot: root, PodsDir: filepath.Join(root, "pods")}
}

func (p Paths) Ensure() error {
	if p.ConfigRoot == "" || p.PodsDir == "" {
		return fmt.Errorf("appconfig: incomplete paths")
	}
	if err := secureDir(p.PodsDir); err != nil {
		return fmt.Errorf("appconfig: mkdir pods: %w", err)
	}
	return nil
}
