package objectstore

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRootUsesBorrowedRootedFilesystem(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	store, err := NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("nested/file.txt", strings.NewReader("value")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "nested", "file.txt")); err != nil || string(data) != "value" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
	items, err := store.List("nested")
	if err != nil || len(items) != 1 || items[0].Name != "nested/file.txt" {
		t.Fatalf("List() = %+v, %v", items, err)
	}
	if err := store.Delete("nested/file.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("nested/file.txt"); !os.IsNotExist(err) {
		t.Fatalf("Get after Delete error = %v", err)
	}
	if _, err := store.Get("../outside"); err == nil {
		t.Fatal("parent traversal was accepted")
	}
}

func TestRootReplaceTTLAndFailedWrite(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	store, err := NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Hour).UTC()
	if err := store.PutWithDeadline("asset.txt", strings.NewReader("old"), deadline); err != nil {
		t.Fatal(err)
	}
	if err := store.PutWithDeadline("asset.txt", strings.NewReader("new"), deadline); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("asset.txt", &failingReader{data: []byte("partial")}); err == nil {
		t.Fatal("failed replacement was accepted")
	}
	reader, err := store.Get("asset.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil || string(data) != "new" {
		t.Fatalf("Get() = %q, %v", data, err)
	}
	items, err := store.List("")
	if err != nil || len(items) != 1 || !items[0].Deadline.Equal(deadline) {
		t.Fatalf("List() = %+v, %v", items, err)
	}
}

func TestRootRejectsEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	store, err := NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("escape/secret"); err == nil {
		t.Fatal("escaping symlink was accepted")
	}
}
