package memory

import (
	"os"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func newTestObjectStore(t testing.TB) *objectstore.Root {
	t.Helper()
	return newTestObjectStoreAt(t, t.TempDir())
}

func newTestObjectStoreAt(t testing.TB, dir string) *objectstore.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	store, err := objectstore.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
