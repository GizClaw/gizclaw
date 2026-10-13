package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestNewRequiresOfflineHistoryMigrationBeforeCreatingDestination(t *testing.T) {
	cfg := validLayeredConfig(t.TempDir())
	registry, physical, err := newStoreRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer physical.Close()
	defer registry.Close()
	db, err := registry.SQL(cfg.Services.Workspace.HistoryStore)
	if err != nil {
		t.Fatal(err)
	}
	const source = "custom_workspace_messages"
	const eino = "custom_agent_messages"
	for _, name := range []string{source, eino} {
		if _, err := db.Exec(`CREATE TABLE ` + name + ` (
			stream TEXT NOT NULL,id TEXT NOT NULL,timestamp_unix_nano BIGINT NOT NULL,
			expires_at_unix_nano BIGINT,kind TEXT NOT NULL,severity TEXT NOT NULL,message TEXT NOT NULL,
			attributes_json TEXT NOT NULL,payload_json BLOB NOT NULL,PRIMARY KEY(stream,id))`); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	id := now.Format("20060102T150405.000000000Z") + "-1"
	if _, err := db.Exec(`INSERT INTO `+source+` VALUES (?,?,?,?,?,?,?,?,?)`, "workspace.history/demo", id, now.UnixNano(), now.Add(time.Hour).UnixNano(), "agent", "", "retained", "{}", []byte(`{"version":1,"name":"assistant","replay_available":true}`)); err != nil {
		t.Fatal(err)
	}
	server, err := newWithOptions(cfg, newServerOptions{Stores: registry})
	if server != nil {
		_ = server.Close()
		t.Fatal("Server started with unmigrated History")
	}
	if !errors.Is(err, workspace.ErrHistoryMigrationRequired) || !strings.Contains(err.Error(), source) || !strings.Contains(err.Error(), "history-migrate") {
		t.Fatalf("startup did not identify actual source: %v", err)
	}
	var exists bool
	if err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name=?)", memory.DefaultHistoryTable); err != nil || exists {
		t.Fatalf("failed startup created empty destination: exists=%t err=%v", exists, err)
	}
	if _, err := workspace.MigrateHistory(t.Context(), db, workspace.HistoryMigration{WorkspaceTable: source, EinoTable: eino, Retention: time.Hour}); err != nil {
		t.Fatal(err)
	}
	server, err = newWithOptions(cfg, newServerOptions{Stores: registry})
	if err != nil {
		t.Fatalf("migrated Server did not start: %v", err)
	}
	defer server.Close()
	entry, err := server.WorkspaceHistory.Get(t.Context(), memory.HistoryKey{ConversationID: "demo", ID: id})
	if err != nil || entry.Text != "retained" {
		t.Fatalf("startup lost migrated data: %+v, %v", entry, err)
	}
}
