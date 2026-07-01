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
		t.Fatalf("DefaultPaths() error = %v", err)
	}
	if paths.ConfigRoot != root {
		t.Fatalf("ConfigRoot = %q, want %q", paths.ConfigRoot, root)
	}
	if paths.ContextDir != filepath.Join(root, "contexts") {
		t.Fatalf("ContextDir = %q", paths.ContextDir)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if info, err := os.Stat(paths.ContextDir); err != nil || !info.IsDir() {
		t.Fatalf("ContextDir stat = %v/%v", info, err)
	}
}

func TestStateStoreLoadSave(t *testing.T) {
	store := StateStore{File: filepath.Join(t.TempDir(), "state.json")}

	empty, err := store.Load()
	if err != nil {
		t.Fatalf("Load(empty) error = %v", err)
	}
	if empty != (State{}) {
		t.Fatalf("Load(empty) = %+v", empty)
	}

	want := State{SelectedContext: "local", SelectedView: "admin"}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}
