package historymigratecmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	// Freeze the retired schema here so migration never depends on a runtime
	// SQL LogStore implementation.
	for _, table := range []string{"old_workspace", "old_eino"} {
		_, err := db.Exec(`CREATE TABLE ` + table + ` (
			stream TEXT NOT NULL, id TEXT NOT NULL, timestamp_unix_nano BIGINT NOT NULL,
			kind TEXT NOT NULL, severity TEXT NOT NULL, message TEXT NOT NULL,
			attributes_json TEXT NOT NULL, payload_json BLOB NOT NULL, expires_at_unix_nano BIGINT NOT NULL,
			PRIMARY KEY (stream, id))`)
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	_, err = db.Exec(`INSERT INTO old_workspace VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"workspace.history/demo", now.Format("20060102T150405.000000000Z")+"-1", now.UnixNano(),
		"agent", "", "retained", "{}", []byte(`{"version":1,"name":"assistant","replay_available":true}`), now.Add(24*time.Hour).UnixNano())
	if err != nil {
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
