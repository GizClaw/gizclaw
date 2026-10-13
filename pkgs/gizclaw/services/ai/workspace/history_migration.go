package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

// HistoryMigration selects the retired Workspace and Eino SQL LogStore parents.
// Both selections may identify the same dedicated history table.
// All Server instances must be stopped before applying this one-time migration.
type HistoryMigration struct {
	WorkspaceTable string
	EinoTable      string
	Retention      time.Duration
	DryRun         bool
}

// HistoryMigrationResult reports copied messages and intentionally expired rows.
// A completed migration removes both sources; it never imports Eino records.
type HistoryMigrationResult struct {
	Messages        int64
	Expired         int64
	DiscardedEino   int64
	AlreadyComplete bool
	DroppedTables   []string
}

const historyMigrationReceipt = "gizclaw_history_migration"

// MigrateHistory converts Workspace History to relational memory.HistoryStore
// rows and drops both retired history tables in the same transaction. Source
// IDs, timestamps, expirations and binary object references are preserved.
// A malformed row, conflicting target or DROP failure rolls back the full import.
func MigrateHistory(ctx context.Context, db *sqlx.DB, options HistoryMigration) (HistoryMigrationResult, error) {
	var report HistoryMigrationResult
	if options.Retention <= 0 {
		return report, fmt.Errorf("history migration: positive retention is required")
	}
	for _, name := range []string{options.WorkspaceTable, options.EinoTable} {
		if err := storage.ValidateSQLIdentifier(name); err != nil {
			return report, err
		}
		if name == memory.DefaultHistoryTable || name == historyMigrationReceipt {
			return report, fmt.Errorf("history migration: a source aliases the target")
		}
	}
	workspaceTable, err := storage.PrepareSQLTable(db, "sql", options.WorkspaceTable)
	if err != nil {
		return report, err
	}
	einoTable, err := storage.PrepareSQLTable(db, "sql", options.EinoTable)
	if err != nil {
		return report, err
	}
	workspaceExists, err := historyTableExists(ctx, db, workspaceTable.Name())
	if err != nil {
		return report, err
	}
	einoExists, err := historyTableExists(ctx, db, einoTable.Name())
	if err != nil {
		return report, err
	}
	if !workspaceExists && !einoExists {
		return completedHistoryMigration(ctx, db, options)
	}
	if !workspaceExists || !einoExists {
		return report, fmt.Errorf("history migration: both dedicated source tables must exist")
	}
	for _, table := range []storage.SQLTable{workspaceTable, einoTable} {
		if err := validateLegacyHistorySchema(ctx, db, table); err != nil {
			return report, err
		}
		if table.Dialect() == storage.SQLDialectPostgreSQL {
			keysName := legacyHistoryKeysName(table.Name())
			exists, err := historyTableExists(ctx, db, keysName)
			if err != nil {
				return report, err
			}
			if exists {
				keys, err := storage.PrepareSQLTable(db, "sql", keysName)
				if err != nil {
					return report, err
				}
				columns, err := storage.SQLColumns(ctx, db, keys)
				if err != nil {
					return report, err
				}
				if err := storage.ValidateSQLColumns(columns, map[string]storage.SQLColumn{"stream": {Type: "TEXT", PrimaryKeyPosition: 1}, "id": {Type: "TEXT", PrimaryKeyPosition: 2}, "expires_at_unix_nano": {Type: "BIGINT"}}); err != nil {
					return report, fmt.Errorf("history migration: source identity table is incompatible: %w", err)
				}
			}
		}
	}
	var destination *memory.SQLHistoryStore
	if !options.DryRun {
		destination, err = memory.NewSQLHistoryStore(ctx, db, memory.DefaultHistoryTable, options.Retention)
		if err != nil {
			return report, err
		}
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	if workspaceTable.Dialect() == storage.SQLDialectPostgreSQL {
		for _, operation := range []string{"run_maintenance", "drop_partition_time"} {
			if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "pg_partman "+operation); err != nil {
				return report, err
			}
		}
		// Includes attached children. Old writers cannot change either source
		// between validation, copying and source removal.
		if _, err := tx.ExecContext(ctx, "LOCK TABLE "+workspaceTable.Quoted()+","+einoTable.Quoted()+" IN ACCESS EXCLUSIVE MODE"); err != nil {
			return report, storage.ExternalSQLError("history migration: lock source tables", err)
		}
	}
	var invalid int
	workspaceGuard := "(stream LIKE 'workspace.history/%' AND kind NOT IN ('gear','agent')) OR stream NOT LIKE 'workspace.history/%'"
	if options.WorkspaceTable == options.EinoTable {
		workspaceGuard = "(stream LIKE 'workspace.history/%' AND kind NOT IN ('gear','agent')) OR (stream NOT LIKE 'workspace.history/%' AND stream NOT IN ('eino.history','flowcraft.history'))"
	}
	if err := tx.GetContext(ctx, &invalid, "SELECT count(*) FROM "+workspaceTable.Quoted()+" WHERE "+workspaceGuard); err != nil {
		return report, err
	}
	if invalid != 0 {
		return report, fmt.Errorf("history migration: workspace source contains unrelated records")
	}
	einoGuard := "stream NOT IN ('eino.history','flowcraft.history')"
	if options.WorkspaceTable == options.EinoTable {
		einoGuard += " AND stream NOT LIKE 'workspace.history/%'"
	}
	if err := tx.GetContext(ctx, &invalid, "SELECT count(*) FROM "+einoTable.Quoted()+" WHERE "+einoGuard); err != nil {
		return report, err
	}
	if invalid != 0 {
		return report, fmt.Errorf("history migration: Eino source contains unrelated records")
	}
	if err := tx.GetContext(ctx, &report.DiscardedEino, "SELECT count(*) FROM "+einoTable.Quoted()+" WHERE stream IN ('eino.history','flowcraft.history')"); err != nil {
		return report, err
	}
	var writer *memory.SQLHistoryStore
	if destination != nil {
		writer, err = destination.WithTx(tx)
		if err != nil {
			return report, err
		}
		var count int64
		if err := tx.GetContext(ctx, &count, "SELECT count(*) FROM "+memory.DefaultHistoryTable); err != nil {
			return report, err
		}
		if count != 0 {
			return report, fmt.Errorf("history migration: destination must be empty")
		}
	}
	now := time.Now().UTC()
	lastStream, lastID := "", ""
	for {
		rows, err := tx.QueryContext(ctx, tx.Rebind("SELECT stream,id,timestamp_unix_nano,expires_at_unix_nano,kind,message,attributes_json,payload_json FROM "+workspaceTable.Quoted()+" WHERE stream LIKE 'workspace.history/%' AND (stream,id) > (?,?) ORDER BY stream,id LIMIT 256"), lastStream, lastID)
		if err != nil {
			return report, storage.ExternalSQLError("history migration: read source batch", err)
		}
		batch := make([]memory.HistoryEntry, 0, 256)
		count := 0
		for rows.Next() {
			var record logstore.Record
			var created int64
			var expiration sql.NullInt64
			var attributes string
			if err := rows.Scan(&record.Stream, &record.ID, &created, &expiration, &record.Kind, &record.Message, &attributes, &record.Payload); err != nil {
				rows.Close()
				return report, err
			}
			count++
			lastStream, lastID = record.Stream, record.ID
			if expiration.Valid && expiration.Int64 <= now.UnixNano() {
				report.Expired++
				continue
			}
			record.Time = time.Unix(0, created).UTC()
			entry, err := historyEntryFromRecord(record, record.Stream)
			if err != nil {
				rows.Close()
				return report, fmt.Errorf("history migration: invalid workspace record %q: %w", record.ID, err)
			}
			if expiration.Valid {
				at := time.Unix(0, expiration.Int64).UTC()
				if entry.ExpiresAt == nil || at.Before(*entry.ExpiresAt) {
					entry.ExpiresAt = &at
				}
			}
			if entry.ExpiresAt == nil {
				at := entry.CreatedAt.Add(options.Retention)
				entry.ExpiresAt = &at
			}
			if !entry.ExpiresAt.After(now) {
				report.Expired++
				continue
			}
			conversation := strings.TrimPrefix(record.Stream, historyStreamPrefix)
			if strings.TrimSpace(conversation) == "" {
				rows.Close()
				return report, fmt.Errorf("history migration: empty conversation")
			}
			binding := HistoryStore{Workspace: conversation}
			batch = append(batch, binding.memoryEntry(entry))
			report.Messages++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return report, err
		}
		if err := rows.Close(); err != nil {
			return report, err
		}
		if writer != nil {
			if err := writer.Append(ctx, batch); err != nil {
				return report, err
			}
			groups := make(map[string][]memory.HistoryEntry)
			for _, entry := range batch {
				groups[entry.ConversationID] = append(groups[entry.ConversationID], entry)
			}
			for conversation, expectedEntries := range groups {
				ids := make([]string, 0, len(expectedEntries))
				expected := make(map[string]memory.HistoryEntry, len(expectedEntries))
				for _, entry := range expectedEntries {
					ids = append(ids, entry.ID)
					expected[entry.ID] = entry
				}
				page, err := writer.Query(ctx, memory.HistoryQuery{ConversationID: conversation, IDs: ids, Limit: len(ids)})
				if err != nil || len(page.Entries) != len(ids) {
					return report, fmt.Errorf("history migration: imported batch count differs")
				}
				for _, entry := range page.Entries {
					if !sameMigratedHistory(expected[entry.ID], entry) {
						return report, fmt.Errorf("history migration: imported record verification failed for %q", entry.ID)
					}
				}
			}

		}
		if count < 256 {
			break
		}
	}
	if options.DryRun {
		return report, nil
	}
	if _, err := tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+historyMigrationReceipt+" (id BIGINT NOT NULL PRIMARY KEY,workspace_table TEXT NOT NULL,eino_table TEXT NOT NULL,messages BIGINT NOT NULL,expired BIGINT NOT NULL,eino_discarded BIGINT NOT NULL)"); err != nil {
		return report, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind("INSERT INTO "+historyMigrationReceipt+" (id,workspace_table,eino_table,messages,expired,eino_discarded) VALUES (1,?,?,?,?,?)"), options.WorkspaceTable, options.EinoTable, report.Messages, report.Expired, report.DiscardedEino); err != nil {
		return report, err
	}
	for _, table := range []storage.SQLTable{workspaceTable, einoTable} {
		if len(report.DroppedTables) > 0 && report.DroppedTables[0] == table.Name() {
			continue
		}
		if err := dropLegacyHistoryTable(ctx, tx, table, &report); err != nil {
			return report, err
		}
	}
	if err := tx.Commit(); err != nil {
		return report, storage.ExternalSQLError("history migration: commit import and source deletion", err)
	}
	return report, nil
}

func sameMigratedHistory(expected, actual memory.HistoryEntry) bool {
	if expected.ConversationID != actual.ConversationID || expected.ID != actual.ID || expected.Role != actual.Role || expected.ActorID != actual.ActorID || expected.Origin != actual.Origin || expected.Name != actual.Name || expected.Text != actual.Text || !expected.CreatedAt.Equal(actual.CreatedAt) || expected.ExpiresAt == nil || actual.ExpiresAt == nil || !expected.ExpiresAt.Equal(*actual.ExpiresAt) || len(expected.Assets) != len(actual.Assets) {
		return false
	}
	for index, asset := range expected.Assets {
		other := actual.Assets[index]
		if asset.Name != other.Name || asset.MIMEType != other.MIMEType || asset.Bytes != other.Bytes {
			return false
		}
		if (asset.ExpiresAt == nil) != (other.ExpiresAt == nil) || asset.ExpiresAt != nil && !asset.ExpiresAt.Equal(*other.ExpiresAt) {
			return false
		}
	}
	return true
}

func historyTableExists(ctx context.Context, db *sqlx.DB, table string) (bool, error) {
	var exists bool
	if db.DriverName() == "postgres" {
		err := db.GetContext(ctx, &exists, "SELECT to_regclass(quote_ident(current_schema())||'.'||quote_ident($1)) IS NOT NULL", table)
		return exists, err
	}
	err := db.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)", table)
	return exists, err
}

func validateLegacyHistorySchema(ctx context.Context, db *sqlx.DB, table storage.SQLTable) error {
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		var child bool
		if err := db.GetContext(ctx, &child, "SELECT EXISTS(SELECT 1 FROM pg_inherits WHERE inhrelid=to_regclass(quote_ident(current_schema())||'.'||quote_ident($1)))", table.Name()); err != nil {
			return err
		}
		if child {
			return fmt.Errorf("history migration: source %q is a child partition; select its logical parent", table.Name())
		}
	}
	columns, err := storage.SQLColumns(ctx, db, table)
	if err != nil {
		return err
	}
	payload := "BLOB"
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		payload = "BYTEA"
	}
	want := map[string]string{"stream": "TEXT", "id": "TEXT", "timestamp_unix_nano": "BIGINT", "expires_at_unix_nano": "BIGINT", "kind": "TEXT", "severity": "TEXT", "message": "TEXT", "attributes_json": "TEXT", "payload_json": payload}
	if len(columns) != len(want) {
		return fmt.Errorf("history migration: source %q is not a legacy SQL LogStore", table.Name())
	}
	for name, typ := range want {
		if columns[name].Type != typ {
			return fmt.Errorf("history migration: source %q has incompatible column %q", table.Name(), name)
		}
	}
	return nil
}

func completedHistoryMigration(ctx context.Context, db *sqlx.DB, options HistoryMigration) (HistoryMigrationResult, error) {
	var result HistoryMigrationResult
	exists, err := historyTableExists(ctx, db, historyMigrationReceipt)
	if err != nil || !exists {
		return result, fmt.Errorf("history migration: source tables are absent without a completion receipt")
	}
	if exists, err := historyTableExists(ctx, db, memory.DefaultHistoryTable); err != nil || !exists {
		return result, fmt.Errorf("history migration: completed destination is absent")
	}
	var workspace, eino string
	if err := db.QueryRowContext(ctx, "SELECT workspace_table,eino_table,messages,expired,eino_discarded FROM "+historyMigrationReceipt+" WHERE id=1").Scan(&workspace, &eino, &result.Messages, &result.Expired, &result.DiscardedEino); err != nil {
		return result, err
	}
	if workspace != options.WorkspaceTable || eino != options.EinoTable {
		return result, fmt.Errorf("history migration: completed source selection differs")
	}
	result.AlreadyComplete = true
	return result, nil
}

func dropLegacyHistoryTable(ctx context.Context, tx *sqlx.Tx, table storage.SQLTable, report *HistoryMigrationResult) error {
	var template string
	if table.Dialect() == storage.SQLDialectPostgreSQL {
		keys := legacyHistoryKeysName(table.Name())
		if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS \""+keys+"\""); err != nil {
			return storage.ExternalSQLError("history migration: remove retired identities", err)
		}
		var schema string
		if err := tx.GetContext(ctx, &schema, "SELECT current_schema()"); err != nil {
			return err
		}
		var templates []string
		if err := tx.SelectContext(ctx, &templates, "SELECT coalesce(template_table,'') FROM partman.part_config WHERE parent_table=$1", schema+"."+table.Name()); err != nil {
			return err
		}
		if len(templates) == 1 {
			template = templates[0]
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM partman.part_config WHERE parent_table=$1", schema+"."+table.Name()); err != nil {
			return err
		}
	}
	// No CASCADE: an unrelated dependency prevents deletion and rolls back.
	if _, err := tx.ExecContext(ctx, "DROP TABLE "+table.Quoted()); err != nil {
		return storage.ExternalSQLError("history migration: remove retired history parent", err)
	}
	if strings.HasPrefix(template, "partman.template_") {
		var users int
		if err := tx.GetContext(ctx, &users, "SELECT count(*) FROM partman.part_config WHERE template_table=$1", template); err != nil {
			return err
		}
		if users == 0 {
			name := strings.TrimPrefix(template, "partman.")
			if err := storage.ValidateSQLIdentifier(name); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS partman.\""+name+"\""); err != nil {
				return err
			}
		}
	}
	report.DroppedTables = append(report.DroppedTables, table.Name())
	return nil
}

func legacyHistoryKeysName(parent string) string {
	// Frozen auxiliary-name algorithm of the retired PostgreSQL LogStore.
	if len(parent) > 53 {
		digest := sha256.Sum256([]byte("gizclaw/logstore/postgres/v1\x00" + parent))
		parent = fmt.Sprintf("%s_%x", parent[:44], digest[:4])
	}
	return parent + "_keys"
}
