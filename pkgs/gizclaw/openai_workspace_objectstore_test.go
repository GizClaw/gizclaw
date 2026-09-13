package gizclaw

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
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

func newTestWorkspaceHistoryLog(t testing.TB) logstore.MutableRecordStore {
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

func newTestWorkspaceRuntimeStore(t testing.TB, objects objectstore.ObjectStore) workspace.ObjectRuntimeStore {
	t.Helper()
	return workspace.NewObjectRuntimeStore(objects, newTestWorkspaceHistoryLog(t), objects)
}
