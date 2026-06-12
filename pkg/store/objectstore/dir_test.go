package objectstore

import (
	"io"
	"strings"
	"testing"
)

func TestDirPutGetListDelete(t *testing.T) {
	store := Dir(t.TempDir())

	if err := store.Put("a/b.txt", reader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	r, err := store.Get("a/b.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, err := io.ReadAll(r)
	if closeErr := r.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("Get data = %q, want hello", data)
	}

	items, err := store.List("a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "a/b.txt" || items[0].Size != int64(len("hello")) {
		t.Fatalf("List = %#v, want a/b.txt", items)
	}

	if err := store.Delete("a/b.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("a/b.txt"); err == nil {
		t.Fatal("Get after Delete error = nil")
	}
}

func TestDirDeletePrefix(t *testing.T) {
	store := Dir(t.TempDir())
	for _, name := range []string{"pets/a.pixa", "pets/nested/b.pixa", "badges/icon.png"} {
		if err := store.Put(name, reader(name)); err != nil {
			t.Fatalf("Put(%q): %v", name, err)
		}
	}

	if err := store.DeletePrefix("pets"); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	items, err := store.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "badges/icon.png" {
		t.Fatalf("remaining items = %#v, want badges/icon.png", items)
	}
}

func TestDirReplaceMissingAndEmptyPrefix(t *testing.T) {
	store := Dir(t.TempDir())
	if err := store.Put("asset.txt", strings.NewReader("old")); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if err := store.Put("asset.txt", strings.NewReader("new")); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	r, err := store.Get("asset.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, err := io.ReadAll(r)
	if closeErr := r.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("data = %q, want new", data)
	}
	if err := store.Delete("missing.txt"); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
	items, err := store.List("missing")
	if err != nil {
		t.Fatalf("List missing: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List missing = %#v, want empty", items)
	}
	if err := store.DeletePrefix(""); err != nil {
		t.Fatalf("DeletePrefix empty: %v", err)
	}
	if _, err := store.Get("asset.txt"); err != nil {
		t.Fatalf("Get after DeletePrefix empty: %v", err)
	}
}

func TestDirRejectsInvalidObjectNames(t *testing.T) {
	store := Dir(t.TempDir())
	for _, name := range []string{"", "/", "../outside", "a/../b", "/tmp/object"} {
		t.Run(name, func(t *testing.T) {
			if err := store.Put(name, reader("data")); err == nil {
				t.Fatal("Put error = nil")
			}
			if _, err := store.Get(name); err == nil {
				t.Fatal("Get error = nil")
			}
			if err := store.Delete(name); err == nil {
				t.Fatal("Delete error = nil")
			}
			if err := store.DeletePrefix(name); err == nil && name != "" {
				t.Fatal("DeletePrefix error = nil")
			}
			if _, err := store.List(name); err == nil && name != "" {
				t.Fatal("List error = nil")
			}
		})
	}
}

func TestDirNormalizesObjectNames(t *testing.T) {
	store := Dir(t.TempDir())
	if err := store.Put("./a//b.txt", strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	r, err := store.Get("a/b.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
}

func reader(s string) io.Reader {
	return &stringReader{s: s}
}

type stringReader struct {
	s string
	i int
}

func (r *stringReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
