package workspace

import (
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func migrationDB(t *testing.T, postgres bool) *sqlx.DB {
	t.Helper()
	var db *sqlx.DB
	var err error
	if postgres {
		dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("set GIZCLAW_TEST_POSTGRES_DSN for migration integration")
		}
		db, err = sqlx.ConnectContext(t.Context(), "postgres", dsn)
	} else {
		db, err = sqlx.Open("sqlite", filepath.Join(t.TempDir(), "migration.sqlite"))
	}
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if postgres {
		schema := "migration_" + strings.ToLower(rand.Text())
		if _, err := db.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("SET search_path TO " + schema + ",public"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.Exec("DELETE FROM partman.part_config WHERE parent_table=$1", schema+"."+memory.DefaultHistoryTable)
			_, _ = db.Exec("DROP SCHEMA " + schema + " CASCADE")
		})
	}
	payload := "BLOB"
	if postgres {
		payload = "BYTEA"
	}
	for _, name := range []string{"old_workspace", "old_eino"} {
		nullable, primary, partition := "", ",PRIMARY KEY(stream,id)", ""
		if postgres {
			nullable = " NOT NULL"
			primary = ""
			partition = " PARTITION BY RANGE(expires_at_unix_nano)"
		}
		if _, err := db.Exec("CREATE TABLE " + name + " (stream TEXT NOT NULL,id TEXT NOT NULL,timestamp_unix_nano BIGINT NOT NULL,expires_at_unix_nano BIGINT" + nullable + ",kind TEXT NOT NULL,severity TEXT NOT NULL,message TEXT NOT NULL,attributes_json TEXT NOT NULL,payload_json " + payload + " NOT NULL" + primary + ")" + partition); err != nil {
			t.Fatal(err)
		}
		if postgres {
			var schema string
			if err := db.Get(&schema, "SELECT current_schema()"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("SELECT partman.create_parent($1,'expires_at_unix_nano','1 day',p_epoch:='nanoseconds',p_default_table:=false,p_jobmon:=false)", schema+"."+name); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("CREATE TABLE " + name + "_keys (stream TEXT NOT NULL,id TEXT NOT NULL,expires_at_unix_nano BIGINT NOT NULL,PRIMARY KEY(stream,id))"); err != nil {
				t.Fatal(err)
			}
		}
	}
	return db
}

func seedMigrationEntry(t *testing.T, db *sqlx.DB, entry HistoryEntry, expiry time.Time) {
	t.Helper()
	record, err := historyRecord(entry, "workspace.history/demo")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(db.Rebind("INSERT INTO old_workspace VALUES (?,?,?,?,?,?,?,?,?)"), record.Stream, record.ID, record.Time.UnixNano(), expiry.UnixNano(), record.Kind, record.Severity, record.Message, "{}", []byte(record.Payload))
	if err != nil {
		t.Fatal(err)
	}
}

func migrationOptions() HistoryMigration {
	return HistoryMigration{WorkspaceTable: "old_workspace", EinoTable: "old_eino", Retention: 24 * time.Hour}
}

func testHistoryMigration(t *testing.T, postgres bool) {
	db := migrationDB(t, postgres)
	now := time.Now().UTC().Truncate(time.Microsecond)
	expiry := now.Add(3 * time.Hour)
	entry := HistoryEntry{ID: historyID(now, now), Type: "gear", GearID: "gear", Origin: HistoryOriginAgentHost, Name: "user", Text: "你好", CreatedAt: now, Assets: []HistoryAsset{{Name: "workspaces/demo/history/assets/old/audio.ogg", MIMEType: "audio/ogg", Bytes: 42}}}
	objects := newTestObjectStore(t)
	binding := NewHistoryStore(nil, objects, "demo")
	asset, err := binding.writeAsset(entry.ID, AppendHistoryAsset{MIMEType: "audio/ogg", Data: []byte("retained audio bytes")})
	if err != nil {
		t.Fatal(err)
	}
	entry.Assets = []HistoryAsset{asset}
	seedMigrationEntry(t, db, entry, expiry)
	expired := entry
	expired.ID = historyID(now.Add(-48*time.Hour), now)
	expired.CreatedAt = now.Add(-48 * time.Hour)
	seedMigrationEntry(t, db, expired, now.Add(-time.Hour))
	if _, err := db.Exec(db.Rebind("INSERT INTO old_eino VALUES (?,?,?,?,?,?,?,?,?)"), "eino.history", "discarded", now.UnixNano(), expiry.UnixNano(), "message", "", "Eino duplicate", "{}", []byte(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	options := migrationOptions()
	options.DryRun = true
	preview, err := MigrateHistory(t.Context(), db, options)
	if err != nil || preview.Messages != 1 || preview.Expired != 1 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if exists, _ := historyTableExists(t.Context(), db, memory.DefaultHistoryTable); exists {
		t.Fatal("dry run initialized destination")
	}
	options.DryRun = false
	result, err := MigrateHistory(t.Context(), db, options)
	if err != nil || result.Messages != 1 || result.Expired != 1 || len(result.DroppedTables) != 2 {
		t.Fatalf("migration=%+v err=%v", result, err)
	}
	for _, source := range []string{options.WorkspaceTable, options.EinoTable} {
		if exists, _ := historyTableExists(t.Context(), db, source); exists {
			t.Fatalf("source %s remains", source)
		}
		if postgres {
			if exists, _ := historyTableExists(t.Context(), db, source+"_keys"); exists {
				t.Fatalf("source identities remain: %s", source)
			}
			var children int
			if err := db.Get(&children, "SELECT count(*) FROM pg_tables WHERE schemaname=current_schema() AND tablename LIKE $1", source+"_p%"); err != nil || children != 0 {
				t.Fatalf("old children=%d err=%v", children, err)
			}
		}
	}
	store, err := memory.NewSQLHistoryStore(t.Context(), db, memory.DefaultHistoryTable, options.Retention)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), memory.HistoryKey{ConversationID: "demo", ID: entry.ID})
	if err != nil || got.Text != entry.Text || got.ID != entry.ID || len(got.Assets) != 1 || got.Assets[0].Name != entry.Assets[0].Name || !got.ExpiresAt.Equal(expiry) {
		t.Fatalf("imported=%+v err=%v", got, err)
	}
	reader, err := NewHistoryStore(store, objects, "demo").ReadAsset(t.Context(), entry.Assets[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(audio) != "retained audio bytes" {
		t.Fatalf("audio changed: %q err=%v", audio, err)
	}
	page, err := store.Query(t.Context(), memory.HistoryQuery{ConversationID: "demo", Limit: 10})
	if err != nil || len(page.Entries) != 1 {
		t.Fatalf("unexpected Eino/expired rows: %+v err=%v", page, err)
	}
	again, err := MigrateHistory(t.Context(), db, options)
	if err != nil || !again.AlreadyComplete || again.Messages != 1 {
		t.Fatalf("repeat=%+v err=%v", again, err)
	}
}

func TestHistoryOneTimeMigration(t *testing.T)           { testHistoryMigration(t, false) }
func TestPostgreSQLHistoryOneTimeMigration(t *testing.T) { testHistoryMigration(t, true) }

func TestHistoryMigrationInvalidLaterBatchRollsBack(t *testing.T) {
	db := migrationDB(t, false)
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	for index := range 257 {
		at := now.Add(time.Duration(index) * time.Microsecond)
		seedMigrationEntry(t, db, HistoryEntry{ID: historyID(at, at), Type: "agent", Name: "assistant", Text: "valid", CreatedAt: at}, expiry)
	}
	if _, err := db.Exec("UPDATE old_workspace SET payload_json=? WHERE id=(SELECT max(id) FROM old_workspace)", []byte(`{"version":99}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateHistory(t.Context(), db, migrationOptions()); err == nil {
		t.Fatal("accepted unsupported legacy row")
	}
	for _, source := range []string{"old_workspace", "old_eino"} {
		if exists, _ := historyTableExists(t.Context(), db, source); !exists {
			t.Fatalf("source %s was dropped", source)
		}
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM "+memory.DefaultHistoryTable); err != nil || count != 0 {
		t.Fatalf("partial import rows=%d err=%v", count, err)
	}
}

func TestPostgreSQLHistoryDropFailureRollsBack(t *testing.T) {
	db := migrationDB(t, true)
	now := time.Now().UTC()
	seedMigrationEntry(t, db, HistoryEntry{ID: historyID(now, now), Type: "agent", Name: "assistant", Text: "keep", CreatedAt: now}, now.Add(time.Hour))
	if _, err := db.Exec("CREATE VIEW unrelated_dependency AS SELECT id FROM old_eino"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateHistory(context.Background(), db, migrationOptions()); err == nil {
		t.Fatal("DROP unexpectedly cascaded through an unrelated view")
	}
	for _, source := range []string{"old_workspace", "old_eino"} {
		if exists, _ := historyTableExists(t.Context(), db, source); !exists {
			t.Fatalf("source %s lost on failure", source)
		}
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM "+memory.DefaultHistoryTable); err != nil || count != 0 {
		t.Fatalf("partial target=%d err=%v", count, err)
	}
}
