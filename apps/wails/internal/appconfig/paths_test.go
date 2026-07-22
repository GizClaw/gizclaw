package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPathsUsesEnvConfigHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvConfigHome, root)
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.ConfigRoot != root || paths.PodsDir != filepath.Join(root, "pods") || paths.RaidsCacheDir != filepath.Join(root, "raids") {
		t.Fatalf("paths = %+v", paths)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(paths.PodsDir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("PodsDir stat = %v/%v", info, err)
	}
	info, err = os.Stat(paths.RaidsCacheDir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("RaidsCacheDir stat = %v/%v", info, err)
	}
}

func TestDefaultPathsUsesOSConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv(EnvConfigHome, "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.ConfigRoot == "" || filepath.Base(paths.ConfigRoot) != AppDirName {
		t.Fatalf("ConfigRoot = %q", paths.ConfigRoot)
	}
}

func TestPathsEnsureRejectsIncompletePaths(t *testing.T) {
	if err := (Paths{}).Ensure(); err == nil {
		t.Fatal("Ensure(incomplete) error = nil")
	}
}
