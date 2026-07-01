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

func TestDefaultPathsUsesOSConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv(EnvConfigHome, "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error = %v", err)
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

func TestNormalizeView(t *testing.T) {
	for _, view := range []string{"admin", "play"} {
		if got := NormalizeView(view); got != view {
			t.Fatalf("NormalizeView(%q) = %q", view, got)
		}
	}
	if got := NormalizeView("unknown"); got != DefaultView {
		t.Fatalf("NormalizeView(unknown) = %q, want %q", got, DefaultView)
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

	want := State{
		LastContext:    "local",
		LastView:       "admin",
		SessionActive:  true,
		SessionContext: "local",
		SessionView:    "admin",
	}
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

func TestStateStoreErrors(t *testing.T) {
	if _, err := (StateStore{}).Load(); err == nil {
		t.Fatal("Load(empty file) error = nil")
	}
	if err := (StateStore{}).Save(State{}); err == nil {
		t.Fatal("Save(empty file) error = nil")
	}

	bad := StateStore{File: filepath.Join(t.TempDir(), "state.json")}
	if err := os.WriteFile(bad.File, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := bad.Load(); err == nil {
		t.Fatal("Load(invalid json) error = nil")
	}
}
