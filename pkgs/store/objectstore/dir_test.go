package objectstore

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	for _, name := range []string{"alpha/a.bin", "alpha/nested/b.bin", "beta/icon.png"} {
		if err := store.Put(name, reader(name)); err != nil {
			t.Fatalf("Put(%q): %v", name, err)
		}
	}

	if err := store.DeletePrefix("alpha"); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	items, err := store.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "beta/icon.png" {
		t.Fatalf("remaining items = %#v, want beta/icon.png", items)
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

func TestDirPutFailureKeepsExistingObjectAndRemovesTemp(t *testing.T) {
	root := t.TempDir()
	store := Dir(root)
	if err := store.Put("asset.txt", strings.NewReader("old")); err != nil {
		t.Fatalf("initial Put: %v", err)
	}

	if err := store.Put("asset.txt", &failingReader{data: []byte("partial")}); err == nil {
		t.Fatal("replacement Put error = nil")
	}

	r, err := store.Get("asset.txt")
	if err != nil {
		t.Fatalf("Get existing after failed Put: %v", err)
	}
	data, err := io.ReadAll(r)
	if closeErr := r.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("data after failed Put = %q, want old", data)
	}
	matches, err := filepath.Glob(filepath.Join(root, putTempPrefix+"*"))
	if err != nil {
		t.Fatalf("Glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files remain: %v", matches)
	}
}

func TestDirPutFailureDoesNotCreateNewObject(t *testing.T) {
	root := t.TempDir()
	store := Dir(root)
	if err := store.Put("new.txt", &failingReader{data: []byte("partial")}); err == nil {
		t.Fatal("Put error = nil")
	}
	if _, err := store.Get("new.txt"); !os.IsNotExist(err) {
		t.Fatalf("Get error = %v, want not exist", err)
	}
	assertNoPutTemps(t, root)
}

func TestDirListDoesNotExposeInProgressPut(t *testing.T) {
	root := t.TempDir()
	store := Dir(root)
	reader, writer := io.Pipe()
	putErr := make(chan error, 1)
	go func() {
		putErr <- store.Put("asset.txt", reader)
	}()
	if _, err := writer.Write([]byte("partial")); err != nil {
		t.Fatalf("write partial object: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		matches, err := filepath.Glob(filepath.Join(store.metadataRoot(), "put", putTempPrefix+"*"))
		if err != nil {
			t.Fatalf("Glob temp files: %v", err)
		}
		if len(matches) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Put did not create staging file")
		}
		time.Sleep(time.Millisecond)
	}
	items, err := store.List("")
	if err != nil {
		t.Fatalf("List during Put: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List during Put = %#v, want no staging objects", items)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	if err := <-putErr; err != nil {
		t.Fatalf("Put: %v", err)
	}
}

func TestDirMetadataFailureRestoresExistingObjectAndDeadline(t *testing.T) {
	root := t.TempDir()
	store := Dir(root)
	oldDeadline := time.Now().Add(time.Hour).UTC()
	if err := store.PutWithDeadline("asset.txt", strings.NewReader("old"), oldDeadline); err != nil {
		t.Fatalf("initial PutWithDeadline: %v", err)
	}
	metadataDir := filepath.Dir(store.metadataPath("asset.txt"))
	if err := os.Chmod(metadataDir, 0o500); err != nil {
		t.Fatalf("Chmod metadata dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(metadataDir, 0o700) })

	if err := store.PutWithDeadline("asset.txt", strings.NewReader("new"), time.Now().Add(2*time.Hour)); err == nil {
		t.Fatal("replacement PutWithDeadline error = nil")
	}
	if err := os.Chmod(metadataDir, 0o700); err != nil {
		t.Fatalf("restore metadata dir mode: %v", err)
	}

	r, err := store.Get("asset.txt")
	if err != nil {
		t.Fatalf("Get restored object: %v", err)
	}
	data, readErr := io.ReadAll(r)
	closeErr := r.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read restored object: read=%v close=%v", readErr, closeErr)
	}
	if string(data) != "old" {
		t.Fatalf("restored data = %q, want old", data)
	}
	items, err := store.List("")
	if err != nil {
		t.Fatalf("List restored object: %v", err)
	}
	if len(items) != 1 || !items[0].Deadline.Equal(oldDeadline) {
		t.Fatalf("restored items = %#v, want old deadline %v", items, oldDeadline)
	}
	assertNoPutTemps(t, root)
}

func TestDirPutWithTTLExpiresObjects(t *testing.T) {
	store := Dir(t.TempDir())
	if err := store.PutWithTTL("history/audio.opus", strings.NewReader("audio"), 20*time.Millisecond); err != nil {
		t.Fatalf("PutWithTTL: %v", err)
	}

	items, err := store.List("history")
	if err != nil {
		t.Fatalf("List before deadline: %v", err)
	}
	if len(items) != 1 || items[0].Name != "history/audio.opus" || items[0].Deadline.IsZero() {
		t.Fatalf("List before deadline = %#v, want history/audio.opus with deadline", items)
	}

	time.Sleep(40 * time.Millisecond)
	if _, err := store.Get("history/audio.opus"); err == nil {
		t.Fatal("Get after deadline error = nil")
	} else if !strings.Contains(err.Error(), fs.ErrNotExist.Error()) {
		t.Fatalf("Get after deadline error = %v, want not exist", err)
	}
	items, err = store.List("history")
	if err != nil {
		t.Fatalf("List after deadline: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List after deadline = %#v, want empty", items)
	}
}

func TestDirPutClearsObjectDeadline(t *testing.T) {
	store := Dir(t.TempDir())
	if err := store.PutWithDeadline("history/audio.opus", strings.NewReader("old"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("PutWithDeadline: %v", err)
	}
	if err := store.Put("history/audio.opus", strings.NewReader("new")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	items, err := store.List("history")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "history/audio.opus" || !items[0].Deadline.IsZero() {
		t.Fatalf("List = %#v, want permanent history/audio.opus", items)
	}
}

func TestDirPutWithTTLRejectsNonPositiveTTL(t *testing.T) {
	store := Dir(t.TempDir())
	if err := store.PutWithTTL("history/audio.opus", strings.NewReader("audio"), 0); err == nil {
		t.Fatal("PutWithTTL ttl=0 error = nil")
	}
}

func TestDirDeletePrefixRemovesObjectDeadlines(t *testing.T) {
	store := Dir(t.TempDir())
	if err := store.PutWithDeadline("history/a.opus", strings.NewReader("a"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("PutWithDeadline(a): %v", err)
	}
	if err := store.PutWithDeadline("other/b.opus", strings.NewReader("b"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("PutWithDeadline(b): %v", err)
	}
	if err := store.DeletePrefix("history"); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if err := store.Put("history/a.opus", strings.NewReader("new")); err != nil {
		t.Fatalf("Put replacement: %v", err)
	}

	items, err := store.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List len = %d, want 2: %#v", len(items), items)
	}
	for _, item := range items {
		if item.Name == "history/a.opus" && !item.Deadline.IsZero() {
			t.Fatalf("replacement deadline = %v, want zero", item.Deadline)
		}
		if strings.HasPrefix(item.Name, ".objectstore-meta/") {
			t.Fatalf("List leaked metadata item %#v", item)
		}
	}
}

func TestDirRejectsInvalidObjectNames(t *testing.T) {
	store := Dir(t.TempDir())
	for _, name := range []string{"", "/", "../outside", "a/../b", "/tmp/object", ".objectstore-meta/expires/x"} {
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

func TestDirAllowsObjectNamesStartingWithPutTempPrefix(t *testing.T) {
	store := Dir(t.TempDir())
	name := putTempPrefix + "public/asset.txt"
	if err := store.Put(name, strings.NewReader("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	items, err := store.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != name {
		t.Fatalf("List = %#v, want %q", items, name)
	}
	r, err := store.Get(name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, readErr := io.ReadAll(r)
	closeErr := r.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read object: read=%v close=%v", readErr, closeErr)
	}
	if string(data) != "data" {
		t.Fatalf("data = %q, want data", data)
	}
}

func reader(s string) io.Reader {
	return &stringReader{s: s}
}

type stringReader struct {
	s string
	i int
}

type failingReader struct {
	data []byte
}

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, os.ErrInvalid
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, os.ErrInvalid
}

func assertNoPutTemps(t *testing.T, root string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, metadataRoot, "put", putTempPrefix+"*"))
	if err != nil {
		t.Fatalf("Glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files remain: %v", matches)
	}
}

func (r *stringReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
