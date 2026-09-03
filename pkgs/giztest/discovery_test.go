package giztest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRecursiveSortedAndDeduplicated(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "a.giztest.yaml")
	b := filepath.Join(nested, "b.giztest.yaml")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Discover([]string{root, a})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("Discover = %#v", got)
	}
}
func TestDiscoverRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.giztest.yaml")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.giztest.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover([]string{link}); err == nil {
		t.Fatal("Discover accepted symlink")
	}
}
