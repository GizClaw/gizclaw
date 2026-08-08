package gameplay

import (
	"os"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func newTestObjectStore(t testing.TB) *objectstore.Root {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
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
