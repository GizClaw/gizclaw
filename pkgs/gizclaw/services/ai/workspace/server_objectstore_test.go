package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
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

func newTestLogStore(t testing.TB) logstore.MutableRecordStore {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := logstore.NewSQLStoreWithDB(db, "workspace_history")
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func newTestHistoryStore(t testing.TB, objects objectstore.ObjectStore, workspace string) *HistoryStore {
	t.Helper()
	return NewHistoryStore(newTestLogStore(t), objects, workspace)
}

func newTestRuntimeStore(t testing.TB, objects objectstore.ObjectStore) ObjectRuntimeStore {
	t.Helper()
	return NewObjectRuntimeStore(objects, newTestLogStore(t), objects)
}
