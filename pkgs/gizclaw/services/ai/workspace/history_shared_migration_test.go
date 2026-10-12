package workspace

import (
	"testing"
	"time"
)

func TestHistoryMigrationSharedLegacyParent(t *testing.T) {
	db := migrationDB(t, false)
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	seedMigrationEntry(t, db, HistoryEntry{ID: historyID(now, now), Type: "agent", Name: "assistant", Text: "workspace", CreatedAt: now}, expiry)
	if _, err := db.Exec("INSERT INTO old_workspace VALUES (?,?,?,?,?,?,?,?,?)", "eino.history", "unused-id", now.UnixNano(), expiry.UnixNano(), "message", "", "discard", "{}", []byte(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE old_eino"); err != nil {
		t.Fatal(err)
	}
	options := migrationOptions()
	options.EinoTable = options.WorkspaceTable
	result, err := MigrateHistory(t.Context(), db, options)
	if err != nil || result.Messages != 1 || result.DiscardedEino != 1 || len(result.DroppedTables) != 1 {
		t.Fatalf("shared migration=%+v err=%v", result, err)
	}
	if exists, _ := historyTableExists(t.Context(), db, options.WorkspaceTable); exists {
		t.Fatal("shared source remains")
	}
}
