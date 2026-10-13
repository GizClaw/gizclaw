package workspace

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func testHistoryMigrationCheck(t *testing.T, postgres bool) {
	for _, test := range []struct {
		name   string
		stream string
		block  bool
	}{
		{name: "empty sources"},
		{name: "generic log archive", stream: "system"},
		{name: "Workspace", stream: "workspace.history/demo", block: true},
		{name: "Eino", stream: "eino.history", block: true},
		{name: "Flowcraft", stream: "flowcraft.history", block: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := migrationDB(t, postgres)
			if test.stream != "" {
				now := time.Now().UTC()
				_, err := db.Exec(db.Rebind(`INSERT INTO old_workspace
					(stream,id,timestamp_unix_nano,expires_at_unix_nano,kind,severity,message,attributes_json,payload_json)
					VALUES (?,?,?,?,?,?,?,?,?)`), test.stream, "old", now.UnixNano(), now.Add(time.Hour).UnixNano(), "agent", "", "original", "{}", []byte(`{"version":1}`))
				if err != nil {
					t.Fatal(err)
				}
			}
			err := CheckHistoryMigration(t.Context(), db)
			if test.block {
				if !errors.Is(err, ErrHistoryMigrationRequired) || !strings.Contains(err.Error(), `"old_workspace"`) || !strings.Contains(err.Error(), "gizclaw history-migrate") {
					t.Fatalf("legacy History was not identified: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if exists, err := historyTableExists(t.Context(), db, memory.DefaultHistoryTable); err != nil || exists {
				t.Fatalf("read-only check created destination: exists=%t err=%v", exists, err)
			}
			wantCount := 0
			if test.stream != "" {
				wantCount = 1
			}
			var count int
			if err := db.Get(&count, "SELECT count(*) FROM old_workspace"); err != nil || count != wantCount {
				t.Fatalf("read-only check changed source: count=%d err=%v", count, err)
			}
		})
	}
}

func TestHistoryMigrationStartupCheck(t *testing.T)           { testHistoryMigrationCheck(t, false) }
func TestPostgreSQLHistoryMigrationStartupCheck(t *testing.T) { testHistoryMigrationCheck(t, true) }

func TestHistoryMigrationCheckQuotesCatalogNames(t *testing.T) {
	db := migrationDB(t, false)
	name := `arbitrary"legacy`
	quoted := quoteHistoryCatalogName(name)
	if _, err := db.Exec("CREATE TABLE " + quoted + " (stream TEXT,timestamp_unix_nano BIGINT,payload_json BLOB)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO "+quoted+" VALUES ('workspace.history/demo',1,?)", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := CheckHistoryMigration(t.Context(), db); !errors.Is(err, ErrHistoryMigrationRequired) || !strings.Contains(err.Error(), quoted) {
		t.Fatalf("custom catalog name was missed: %v", err)
	}
}

func TestHistoryMigrationCheckDoesNotBypassExistingDestination(t *testing.T) {
	db := migrationDB(t, false)
	store, err := memory.NewSQLHistoryStore(t.Context(), db, memory.DefaultHistoryTable, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.Append(t.Context(), []memory.HistoryEntry{{ConversationID: "new", ID: "new", Role: memory.RoleUser, Text: "new message", CreatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	seedMigrationEntry(t, db, HistoryEntry{ID: historyID(now, now), Type: "agent", Name: "assistant", Text: "legacy", CreatedAt: now}, now.Add(time.Hour))
	if err := CheckHistoryMigration(t.Context(), db); !errors.Is(err, ErrHistoryMigrationRequired) {
		t.Fatalf("existing destination bypassed legacy detection: %v", err)
	}
}

func TestHistoryMigrationCheckAllowsSuccessfulMigration(t *testing.T) {
	db := migrationDB(t, false)
	now := time.Now().UTC()
	seedMigrationEntry(t, db, HistoryEntry{ID: historyID(now, now), Type: "agent", Name: "assistant", Text: "legacy", CreatedAt: now}, now.Add(time.Hour))
	if err := CheckHistoryMigration(t.Context(), db); !errors.Is(err, ErrHistoryMigrationRequired) {
		t.Fatal(err)
	}
	options := migrationOptions()
	options.DryRun = true
	if _, err := MigrateHistory(t.Context(), db, options); err != nil {
		t.Fatal(err)
	}
	if err := CheckHistoryMigration(t.Context(), db); !errors.Is(err, ErrHistoryMigrationRequired) {
		t.Fatal("dry-run bypassed startup check")
	}
	options.DryRun = false
	if _, err := MigrateHistory(t.Context(), db, options); err != nil {
		t.Fatal(err)
	}
	if err := CheckHistoryMigration(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	// A receipt does not suppress later legacy data appearing in the database.
	if _, err := db.Exec(`CREATE TABLE restored_source (stream TEXT,timestamp_unix_nano BIGINT,payload_json BLOB); INSERT INTO restored_source VALUES ('eino.history',1,X'7b7d')`); err != nil {
		t.Fatal(err)
	}
	if err := CheckHistoryMigration(t.Context(), db); !errors.Is(err, ErrHistoryMigrationRequired) {
		t.Fatal("receipt bypassed restored legacy data")
	}
}

func TestHistoryMigrationCheckPropagatesInspectionFailure(t *testing.T) {
	db := migrationDB(t, false)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := CheckHistoryMigration(t.Context(), db); err == nil || errors.Is(err, ErrHistoryMigrationRequired) {
		t.Fatalf("inspection failure was ignored: %v", err)
	}
}

func TestPostgreSQLHistoryMigrationCheckUsesOnlyCurrentSchema(t *testing.T) {
	db := migrationDB(t, true)
	var schema string
	if err := db.Get(&schema, "SELECT current_schema()"); err != nil {
		t.Fatal(err)
	}
	archive := schema + "_archive"
	if _, err := db.Exec("CREATE SCHEMA " + archive); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.Exec("DROP SCHEMA " + archive + " CASCADE") }()
	if _, err := db.Exec("CREATE TABLE " + archive + ".old_workspace (stream TEXT,timestamp_unix_nano BIGINT,payload_json BYTEA); INSERT INTO " + archive + ".old_workspace VALUES ('workspace.history/other',1,decode('7b7d','hex'))"); err != nil {
		t.Fatal(err)
	}
	if err := CheckHistoryMigration(t.Context(), db); err != nil {
		t.Fatalf("unselected schema blocked startup: %v", err)
	}
	now := time.Now().UTC()
	seedMigrationEntry(t, db, HistoryEntry{ID: historyID(now, now), Type: "agent", Name: "assistant", Text: "legacy", CreatedAt: now}, now.Add(time.Hour))
	err := CheckHistoryMigration(t.Context(), db)
	if !errors.Is(err, ErrHistoryMigrationRequired) || strings.Count(err.Error(), `"old_workspace"`) != 1 || !strings.Contains(err.Error(), quoteHistoryCatalogName(schema)) {
		t.Fatalf("partition parent discovery=%v", err)
	}
}
