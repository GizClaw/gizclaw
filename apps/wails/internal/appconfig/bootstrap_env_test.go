package appconfig

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestBootstrapEnvironmentStoreIsEditableDotenvAndPrivate(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile}
	content := "# Provider credentials\nTOKEN_A=replacement\nTOKEN_B='second value'\n"
	if err := store.Replace(content); err != nil {
		t.Fatal(err)
	}
	values, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values["TOKEN_A"] != "replacement" || values["TOKEN_B"] != "second value" {
		t.Fatalf("Load() = %#v", values)
	}
	stored, err := store.Content()
	if err != nil || stored != content {
		t.Fatalf("Content() = %q, %v", stored, err)
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
	target := filepath.Join(root, "target.env")
	if err := os.WriteFile(target, []byte("TOKEN=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "bootstrap.env")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := (BootstrapEnvironmentStore{Path: link}).Load(); err == nil {
		t.Fatal("Load() error = nil")
	}
}

func TestParseBootstrapEnvironment(t *testing.T) {
	content := "# comment\nexport PLAIN=value # comment\nSINGLE='literal # value'\nDOUBLE=\"line\\nvalue\"\nEMPTY=\n"
	values, err := ParseBootstrapEnvironment(content)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PLAIN":  "value",
		"SINGLE": "literal # value",
		"DOUBLE": "line\nvalue",
		"EMPTY":  "",
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("ParseBootstrapEnvironment() = %#v, want %#v", values, want)
	}
}

func TestParseBootstrapEnvironmentRejectsInvalidInput(t *testing.T) {
	for _, content := range []string{
		"NOT AN ASSIGNMENT\n",
		"1INVALID=value\n",
		"DUPLICATE=first\nDUPLICATE=second\n",
		"QUOTE='unterminated\n",
		"QUOTE=\"value\" trailing\n",
		"TOKEN=" + strings.Repeat("x", maxBootstrapEnvironmentSize) + "\n",
	} {
		if _, err := ParseBootstrapEnvironment(content); err == nil {
			t.Fatalf("ParseBootstrapEnvironment(%q) error = nil", content)
		}
	}
}

func TestCleanupIncompletePods(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	for _, id := range []string{"complete", "incomplete", "failed", "not a pod!"} {
		if err := os.Mkdir(filepath.Join(paths.PodsDir, id), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.MarkInitializing("incomplete"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkInitializing("failed"); err != nil {
		t.Fatal(err)
	}
	if err := store.FailInitialization("failed", errors.New("apply rejected")); err != nil {
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
	if _, err := os.Stat(filepath.Join(paths.PodsDir, "not a pod!")); err != nil {
		t.Fatalf("unrelated directory blocked cleanup or was removed: %v", err)
	}
	status, err := store.Initialization("failed")
	if err != nil || status == nil || status.State != "failed" || status.Error != "apply rejected" {
		t.Fatalf("failed initialization = %+v, %v", status, err)
	}
}
