package historymigratecmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestCommandDryRunMigrationAndIdempotentRepeat(t *testing.T) {
	root := t.TempDir()
	db, err := sqlx.Open("sqlite", filepath.Join(root, "data.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	legacy, err := logstore.NewSQLStoreWithDB(db, "old_workspace")
	if err != nil {
		t.Fatal(err)
	}
	_, err = logstore.NewSQLStoreWithDB(db, "old_eino")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := legacy.Append(t.Context(), []logstore.Record{{Stream: "workspace.history/demo", ID: now.Format("20060102T150405.000000000Z") + "-1", Kind: "agent", Time: now, Message: "retained", Payload: []byte(`{"version":1,"name":"assistant","replay_available":true}`)}}); err != nil {
		t.Fatal(err)
	}
	config := []byte("storage:\n  database:\n    kind: sqlite\n    dir: data.sqlite\n  files:\n    kind: filesystem.dir\n    dir: objects\nstores:\n  history:\n    kind: sql\n    storage: database\n  assets:\n    kind: objectstore\n    storage: files\n    prefix: history\n    ttl: 24h\nservices:\n  workspace:\n    history_store: history\n    history_assets_store: assets\n")
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, config, 0600); err != nil {
		t.Fatal(err)
	}
	for _, dry := range []bool{true, false, false} {
		command := NewCmd()
		args := []string{root, "--workspace-table", "old_workspace", "--eino-table", "old_eino"}
		if dry {
			args = append(args, "--dry-run")
		}
		command.SetArgs(args)
		var output bytes.Buffer
		command.SetOut(&output)
		if err := command.ExecuteContext(t.Context()); err != nil {
			t.Fatalf("command: %v\n%s", err, output.String())
		}
		var exists bool
		if err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name='old_workspace')"); err != nil || exists != dry {
			t.Fatalf("source exists=%t dry=%t err=%v", exists, dry, err)
		}
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, config) {
		t.Fatal("migration edited the final config or generated identity")
	}
	store, err := memory.NewSQLHistoryStore(t.Context(), db, memory.DefaultHistoryTable, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	page, err := store.Query(t.Context(), memory.HistoryQuery{ConversationID: "demo", Limit: 10})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Text != "retained" {
		t.Fatalf("import=%+v err=%v", page, err)
	}
}
