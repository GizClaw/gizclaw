package appconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBootstrapEnvironmentStoreIsWriteOnlyAndPrivate(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile}
	if err := store.Update(map[string]string{"TOKEN_A": "first", "TOKEN_B": "second"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(map[string]string{"TOKEN_A": "replacement", "TOKEN_B": ""}); err != nil {
		t.Fatal(err)
	}
	values, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values["TOKEN_A"] != "replacement" {
		t.Fatalf("Load() = %#v", values)
	}
	info, err := os.Stat(paths.BootstrapEnvFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("bootstrap environment mode = %o", info.Mode().Perm())
	}
	if info, err := os.Stat(filepath.Dir(paths.BootstrapEnvFile)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("bootstrap environment directory = %v, %v", info, err)
	}
}

func TestBootstrapEnvironmentStoreRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "bootstrap-env.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := (BootstrapEnvironmentStore{Path: link}).Load(); err == nil {
		t.Fatal("Load() error = nil")
	}
}

func TestCleanupIncompletePods(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	for _, id := range []string{"complete", "incomplete"} {
		if err := os.Mkdir(filepath.Join(paths.PodsDir, id), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.MarkInitializing("incomplete"); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanupIncomplete(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.PodsDir, "incomplete")); !os.IsNotExist(err) {
		t.Fatalf("incomplete pod still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.PodsDir, "complete")); err != nil {
		t.Fatalf("complete pod was removed: %v", err)
	}
}
