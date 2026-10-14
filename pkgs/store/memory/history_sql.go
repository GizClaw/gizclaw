package memory

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/sqlmeta"
	"github.com/jmoiron/sqlx"
)

// DefaultHistoryTable is the Server-owned conversation history table.
const DefaultHistoryTable = "gizclaw_history"

// SQLHistoryStore stores message fields in columns on a borrowed sqlx pool.
// PostgreSQL uses daily pg_partman expiration partitions; SQLite is supported
// for local runtimes. Asset references use a separate relational child table.
type SQLHistoryStore struct {
	db       *sqlx.DB
	tx       *sqlx.Tx
	table    sqlmeta.SQLTable
	keys     sqlmeta.SQLTable
	assets   sqlmeta.SQLTable
	ttl      time.Duration
	prepared *sync.Map
}

// NewSQLHistoryStore initializes and validates a conversation history store.
// PostgreSQL requires a positive TTL and pg_partman installed in schema partman.
// SQLite may use a zero TTL for unretained local histories. No worker is started.
func NewSQLHistoryStore(ctx context.Context, db *sqlx.DB, table string, ttl time.Duration) (*SQLHistoryStore, error) {
	if ttl < 0 {
		return nil, fmt.Errorf("%w: history retention must not be negative", ErrInvalidInput)
	}
	parent, err := sqlmeta.PrepareSQLTable(db, "sql", table)
	if err != nil {
		return nil, err
	}
	if parent.Dialect() == sqlmeta.SQLDialectPostgreSQL && ttl == 0 {
		return nil, fmt.Errorf("%w: PostgreSQL history requires positive retention", ErrInvalidInput)
	}
	keysName, err := sqlmeta.SQLIndexName(parent, "history_keys")
	if err != nil {
		return nil, err
	}
	assetsName, err := sqlmeta.SQLIndexName(parent, "history_assets")
	if err != nil {
		return nil, err
	}
	keys, err := sqlmeta.PrepareSQLTable(db, "sql", keysName)
	if err != nil {
		return nil, err
	}
	assets, err := sqlmeta.PrepareSQLTable(db, "sql", assetsName)
	if err != nil {
		return nil, err
	}
	store := &SQLHistoryStore{db: db, table: parent, keys: keys, assets: assets, ttl: ttl, prepared: &sync.Map{}}
	if err := store.initialize(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

// WithTx borrows an already-open transaction on this store's pool. Its caller
// owns commit/rollback. This allows a one-time import and source-table deletion
// to commit atomically; it does not close or replace the underlying pool.
func (store *SQLHistoryStore) WithTx(tx *sqlx.Tx) (*SQLHistoryStore, error) {
	if tx == nil || tx.DriverName() != store.db.DriverName() {
		return nil, fmt.Errorf("%w: history transaction has an incompatible dialect", ErrInvalidInput)
	}
	result := *store
	result.tx = tx
	result.prepared = &sync.Map{}
	store.prepared.Range(func(key, value any) bool { result.prepared.Store(key, value); return true })
	return &result, nil
}

func (store *SQLHistoryStore) executor() sqlx.ExtContext {
	if store.tx != nil {
		return store.tx
	}
	return store.db
}

func (store *SQLHistoryStore) initialize(ctx context.Context) error {
	partition, primaryKey, nullable := "", "conversation_id,id", ""
	if store.table.Dialect() == sqlmeta.SQLDialectPostgreSQL {
		partition = " PARTITION BY RANGE (expires_at_unix_nano)"
		primaryKey += ",expires_at_unix_nano"
		nullable = " NOT NULL"
	}
	index, err := sqlmeta.SQLIndexName(store.table, "conversation_page_idx")
	if err != nil {
		return err
	}
	expiryIndex, err := sqlmeta.SQLIndexName(store.keys, "expiry_idx")
	if err != nil {
		return err
	}
	statements := []string{
		"CREATE TABLE IF NOT EXISTS " + store.keys.Quoted() + " (conversation_id TEXT NOT NULL,id TEXT NOT NULL,expires_at_unix_nano BIGINT" + nullable + ",PRIMARY KEY(conversation_id,id))",
		"CREATE TABLE IF NOT EXISTS " + store.table.Quoted() + " (conversation_id TEXT NOT NULL,id TEXT NOT NULL,turn_id TEXT NOT NULL,role TEXT NOT NULL,actor_id TEXT NOT NULL,origin TEXT NOT NULL,actor_name TEXT NOT NULL,text TEXT NOT NULL,created_at_unix_nano BIGINT NOT NULL,expires_at_unix_nano BIGINT" + nullable + ",interrupted BIGINT NOT NULL,PRIMARY KEY(" + primaryKey + "))" + partition,
		"CREATE INDEX IF NOT EXISTS \"" + index + "\" ON " + store.table.Quoted() + " (conversation_id,created_at_unix_nano,id)",
		"CREATE TABLE IF NOT EXISTS " + store.assets.Quoted() + " (conversation_id TEXT NOT NULL,id TEXT NOT NULL,position BIGINT NOT NULL,name TEXT NOT NULL,mime_type TEXT NOT NULL,bytes BIGINT NOT NULL,expires_at_unix_nano BIGINT,PRIMARY KEY(conversation_id,id,position))",
		"CREATE INDEX IF NOT EXISTS \"" + expiryIndex + "\" ON " + store.keys.Quoted() + " (expires_at_unix_nano)",
	}
	if err := sqlmeta.EnsureSQLTable(ctx, store.db, store.table, statements...); err != nil {
		return err
	}
	columns, err := sqlmeta.SQLColumns(ctx, store.db, store.table)
	if err != nil {
		return err
	}
	want := map[string]sqlmeta.SQLColumn{}
	for _, column := range []string{"conversation_id", "id", "turn_id", "role", "actor_id", "origin", "actor_name", "text"} {
		want[column] = sqlmeta.SQLColumn{Type: "TEXT", Nullable: false}
	}
	want["conversation_id"] = sqlmeta.SQLColumn{Type: "TEXT", PrimaryKeyPosition: 1}
	want["id"] = sqlmeta.SQLColumn{Type: "TEXT", PrimaryKeyPosition: 2}
	want["created_at_unix_nano"] = sqlmeta.SQLColumn{Type: "BIGINT"}
	want["interrupted"] = sqlmeta.SQLColumn{Type: "BIGINT"}
	want["expires_at_unix_nano"] = sqlmeta.SQLColumn{Type: "BIGINT", Nullable: nullable == ""}
	if partition != "" {
		want["expires_at_unix_nano"] = sqlmeta.SQLColumn{Type: "BIGINT", PrimaryKeyPosition: 3}
	}
	if err := sqlmeta.ValidateSQLColumns(columns, want); err != nil {
		return fmt.Errorf("history: incompatible message table: %w", err)
	}
	if err := sqlmeta.ValidateSQLIndexColumns(ctx, store.db, store.table, index, []string{"conversation_id", "created_at_unix_nano", "id"}); err != nil {
		return err
	}
	keyColumns, err := sqlmeta.SQLColumns(ctx, store.db, store.keys)
	if err != nil {
		return err
	}
	if err := sqlmeta.ValidateSQLColumns(keyColumns, map[string]sqlmeta.SQLColumn{"conversation_id": {Type: "TEXT", PrimaryKeyPosition: 1}, "id": {Type: "TEXT", PrimaryKeyPosition: 2}, "expires_at_unix_nano": {Type: "BIGINT", Nullable: nullable == ""}}); err != nil {
		return fmt.Errorf("history: incompatible identity table: %w", err)
	}
	assetColumns, err := sqlmeta.SQLColumns(ctx, store.db, store.assets)
	if err != nil {
		return err
	}
	if err := sqlmeta.ValidateSQLColumns(assetColumns, map[string]sqlmeta.SQLColumn{"conversation_id": {Type: "TEXT", PrimaryKeyPosition: 1}, "id": {Type: "TEXT", PrimaryKeyPosition: 2}, "position": {Type: "BIGINT", PrimaryKeyPosition: 3}, "name": {Type: "TEXT"}, "mime_type": {Type: "TEXT"}, "bytes": {Type: "BIGINT"}, "expires_at_unix_nano": {Type: "BIGINT", Nullable: true}}); err != nil {
		return fmt.Errorf("history: incompatible asset table: %w", err)
	}
	if partition != "" {
		return store.initializePartman(ctx)
	}
	return nil
}

func historyNano(at time.Time) (int64, error) {
	return sqlmeta.SQLUnixNano(at.UTC())
}

func historyExpiration(at *time.Time) any {
	if at == nil {
		return nil
	}
	return at.UTC().UnixNano()
}

func historyTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	at := time.Unix(0, value.Int64).UTC()
	return &at
}

// Append atomically stores the complete batch. Duplicate keys fail without
// replacing an existing message or extending its original expiration.
func (store *SQLHistoryStore) Append(ctx context.Context, entries []HistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	prepared := make([]HistoryEntry, len(entries))
	for index, entry := range entries {
		if err := validateHistoryEntry(entry); err != nil {
			return err
		}
		if _, err := historyNano(entry.CreatedAt); err != nil {
			return err
		}
		if entry.ExpiresAt == nil && store.ttl > 0 {
			at := entry.CreatedAt.UTC().Add(store.ttl)
			entry.ExpiresAt = &at
		}
		if entry.ExpiresAt != nil {
			if _, err := historyNano(*entry.ExpiresAt); err != nil {
				return err
			}
		}
		for _, asset := range entry.Assets {
			if asset.ExpiresAt != nil {
				if _, err := historyNano(*asset.ExpiresAt); err != nil {
					return err
				}
			}
		}
		prepared[index] = entry
	}
	slices.SortFunc(prepared, func(a, b HistoryEntry) int {
		if result := cmp.Compare(a.ConversationID, b.ConversationID); result != 0 {
			return result
		}
		return cmp.Compare(a.ID, b.ID)
	})
	if err := store.preparePartitions(ctx, prepared); err != nil {
		return err
	}
	tx := store.tx
	if tx == nil {
		var err error
		tx, err = store.db.BeginTxx(ctx, nil)
		if err != nil {
			return sqlmeta.ExternalSQLError("history: begin append", err)
		}
		defer tx.Rollback()
	}
	if store.table.Dialect() == sqlmeta.SQLDialectPostgreSQL {
		if _, err := tx.ExecContext(ctx, "LOCK TABLE ONLY "+store.table.Quoted()+" IN ROW EXCLUSIVE MODE"); err != nil {
			return sqlmeta.ExternalSQLError("history: protect append parent", err)
		}
	}
	for _, entry := range prepared {
		if err := store.appendEntry(ctx, tx, entry); err != nil {
			return err
		}
	}
	if store.tx == nil {
		return sqlmeta.ExternalSQLError("history: commit append", tx.Commit())
	}
	return nil
}

func (store *SQLHistoryStore) appendEntry(ctx context.Context, tx *sqlx.Tx, entry HistoryEntry) error {
	_, err := tx.ExecContext(ctx, tx.Rebind("INSERT INTO "+store.keys.Quoted()+" (conversation_id,id,expires_at_unix_nano) VALUES (?,?,?)"), entry.ConversationID, entry.ID, historyExpiration(entry.ExpiresAt))
	if err != nil {
		var state interface{ SQLState() string }
		if errors.As(err, &state) && state.SQLState() == "23505" || strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("%w: history message already exists", ErrConflict)
		}
		return sqlmeta.ExternalSQLError("history: reserve message identity", err)
	}
	interrupted := 0
	if entry.Interrupted {
		interrupted = 1
	}
	_, err = tx.ExecContext(ctx, tx.Rebind("INSERT INTO "+store.table.Quoted()+" (conversation_id,id,turn_id,role,actor_id,origin,actor_name,text,created_at_unix_nano,expires_at_unix_nano,interrupted) VALUES (?,?,?,?,?,?,?,?,?,?,?)"), entry.ConversationID, entry.ID, entry.TurnID, string(entry.Role), entry.ActorID, entry.Origin, entry.Name, entry.Text, entry.CreatedAt.UTC().UnixNano(), historyExpiration(entry.ExpiresAt), interrupted)
	if err != nil {
		return sqlmeta.ExternalSQLError("history: append message", err)
	}
	for position, asset := range entry.Assets {
		_, err := tx.ExecContext(ctx, tx.Rebind("INSERT INTO "+store.assets.Quoted()+" (conversation_id,id,position,name,mime_type,bytes,expires_at_unix_nano) VALUES (?,?,?,?,?,?,?)"), entry.ConversationID, entry.ID, position, asset.Name, asset.MIMEType, asset.Bytes, historyExpiration(asset.ExpiresAt))
		if err != nil {
			return sqlmeta.ExternalSQLError("history: append asset reference", err)
		}
	}
	return nil
}

const historySelectColumns = "conversation_id,id,turn_id,role,actor_id,origin,actor_name,text,created_at_unix_nano,expires_at_unix_nano,interrupted"

// Query returns one page without exposing expired messages or other conversations.
func (store *SQLHistoryStore) Query(ctx context.Context, query HistoryQuery) (HistoryPage, error) {
	if strings.TrimSpace(query.ConversationID) == "" || query.Limit <= 0 || query.Limit > MaxHistoryQueryLimit {
		return HistoryPage{}, fmt.Errorf("%w: history conversation and bounded positive limit are required", ErrInvalidInput)
	}
	where := []string{"conversation_id = ?", "(expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)"}
	args := []any{query.ConversationID, time.Now().UTC().UnixNano()}
	for _, bound := range []struct {
		at time.Time
		op string
	}{{query.Start, ">="}, {query.End, "<"}} {
		if !bound.at.IsZero() {
			nano, err := historyNano(bound.at)
			if err != nil {
				return HistoryPage{}, err
			}
			where = append(where, "created_at_unix_nano "+bound.op+" ?")
			args = append(args, nano)
		}
	}
	if query.Text != "" {
		predicate := "instr(text, ?) > 0"
		if store.table.Dialect() == sqlmeta.SQLDialectPostgreSQL {
			predicate = "strpos(text, ?) > 0"
		}
		where = append(where, predicate)
		args = append(args, query.Text)
	}
	if query.ExcludeTurnID != "" {
		where = append(where, "turn_id <> ?")
		args = append(args, query.ExcludeTurnID)
	}
	if len(query.IDs) > MaxHistoryQueryLimit {
		return HistoryPage{}, fmt.Errorf("%w: too many history IDs", ErrInvalidInput)
	}
	if len(query.IDs) > 0 {
		where = append(where, "id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(query.IDs)), ",")+")")
		for _, id := range query.IDs {
			args = append(args, id)
		}
	}
	order, op := "ASC", ">"
	if query.Descending {
		order, op = "DESC", "<"
	}
	if query.Cursor != nil {
		nano, err := historyNano(query.Cursor.CreatedAt)
		if err != nil || query.Cursor.ID == "" {
			return HistoryPage{}, fmt.Errorf("%w: invalid history cursor", ErrInvalidInput)
		}
		where = append(where, "(created_at_unix_nano,id) "+op+" (?,?)")
		args = append(args, nano, query.Cursor.ID)
	}
	args = append(args, query.Limit+1)
	statement := "SELECT " + historySelectColumns + " FROM " + store.table.Quoted() + " WHERE " + strings.Join(where, " AND ") + " ORDER BY created_at_unix_nano " + order + ",id " + order + " LIMIT ?"
	entries, err := store.read(ctx, statement, args...)
	if err != nil {
		return HistoryPage{}, err
	}
	page := HistoryPage{Entries: entries}
	if len(entries) > query.Limit {
		page.Entries = entries[:query.Limit]
		last := page.Entries[len(page.Entries)-1]
		page.Next = &HistoryPosition{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func (store *SQLHistoryStore) read(ctx context.Context, statement string, args ...any) ([]HistoryEntry, error) {
	executor := store.executor()
	rows, err := executor.QueryContext(ctx, store.db.Rebind(statement), args...)
	if err != nil {
		return nil, sqlmeta.ExternalSQLError("history: query messages", err)
	}
	var entries []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		var created, interrupted int64
		var expiration sql.NullInt64
		if err := rows.Scan(&entry.ConversationID, &entry.ID, &entry.TurnID, &entry.Role, &entry.ActorID, &entry.Origin, &entry.Name, &entry.Text, &created, &expiration, &interrupted); err != nil {
			rows.Close()
			return nil, sqlmeta.ExternalSQLError("history: decode message", err)
		}
		entry.CreatedAt = time.Unix(0, created).UTC()
		entry.ExpiresAt = historyTime(expiration)
		entry.Interrupted = interrupted != 0
		if err := validateHistoryEntry(entry); err != nil {
			rows.Close()
			return nil, fmt.Errorf("history: invalid stored message: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, sqlmeta.ExternalSQLError("history: read messages", err)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return entries, nil
	}
	ids := make([]any, len(entries))
	positions := make(map[string]int, len(entries))
	for index, entry := range entries {
		ids[index] = entry.ID
		positions[entry.ID] = index
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	assetArgs := append([]any{entries[0].ConversationID}, ids...)
	assetRows, err := executor.QueryContext(ctx, store.db.Rebind("SELECT id,name,mime_type,bytes,expires_at_unix_nano FROM "+store.assets.Quoted()+" WHERE conversation_id=? AND id IN ("+placeholders+") ORDER BY id,position"), assetArgs...)
	if err != nil {
		return nil, sqlmeta.ExternalSQLError("history: query assets", err)
	}
	defer assetRows.Close()
	for assetRows.Next() {
		var id string
		var asset HistoryAsset
		var expiration sql.NullInt64
		if err := assetRows.Scan(&id, &asset.Name, &asset.MIMEType, &asset.Bytes, &expiration); err != nil {
			return nil, sqlmeta.ExternalSQLError("history: decode asset", err)
		}
		asset.ExpiresAt = historyTime(expiration)
		index := positions[id]
		entries[index].Assets = append(entries[index].Assets, asset)
	}
	return entries, sqlmeta.ExternalSQLError("history: read assets", assetRows.Err())
}

// Get reads an exact, unexpired message key.
func (store *SQLHistoryStore) Get(ctx context.Context, key HistoryKey) (HistoryEntry, error) {
	if err := validateHistoryKey(key); err != nil {
		return HistoryEntry{}, err
	}
	entries, err := store.read(ctx, "SELECT "+historySelectColumns+" FROM "+store.table.Quoted()+" WHERE conversation_id=? AND id=? AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)", key.ConversationID, key.ID, time.Now().UTC().UnixNano())
	if err != nil {
		return HistoryEntry{}, err
	}
	if len(entries) == 0 {
		return HistoryEntry{}, ErrNotFound
	}
	return entries[0], nil
}

// Delete removes one message and its asset references. Binary asset cleanup is
// owned by the caller. A missing message returns ErrNotFound.
func (store *SQLHistoryStore) Delete(ctx context.Context, key HistoryKey) error {
	if err := validateHistoryKey(key); err != nil {
		return err
	}
	tx := store.tx
	if tx == nil {
		var err error
		tx, err = store.db.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}
	if store.table.Dialect() == sqlmeta.SQLDialectPostgreSQL {
		if _, err := tx.ExecContext(ctx, "LOCK TABLE ONLY "+store.table.Quoted()+" IN ROW EXCLUSIVE MODE"); err != nil {
			return sqlmeta.ExternalSQLError("history: protect deletion parent", err)
		}
	}
	for _, table := range []sqlmeta.SQLTable{store.assets, store.table} {
		if _, err := tx.ExecContext(ctx, tx.Rebind("DELETE FROM "+table.Quoted()+" WHERE conversation_id=? AND id=?"), key.ConversationID, key.ID); err != nil {
			return sqlmeta.ExternalSQLError("history: delete message data", err)
		}
	}
	result, err := tx.ExecContext(ctx, tx.Rebind("DELETE FROM "+store.keys.Quoted()+" WHERE conversation_id=? AND id=?"), key.ConversationID, key.ID)
	if err != nil {
		return sqlmeta.ExternalSQLError("history: delete message identity", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	if store.tx == nil {
		return sqlmeta.ExternalSQLError("history: commit deletion", tx.Commit())
	}
	return nil
}

// Maintain reclaims expired content and delegates PostgreSQL partitions to
// pg_partman. Identities with assets survive until CleanupHistory deletes the
// binary objects and acknowledges removal, including after a partition is gone.
func (store *SQLHistoryStore) Maintain(ctx context.Context) error {
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if store.table.Dialect() == sqlmeta.SQLDialectPostgreSQL {
		if err := store.maintainPartman(ctx, tx); err != nil {
			return err
		}
	}
	now := time.Now().UTC().UnixNano()
	if store.table.Dialect() == sqlmeta.SQLDialectSQLite {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+store.table.Quoted()+" WHERE expires_at_unix_nano <= ?", now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind("DELETE FROM "+store.keys.Quoted()+" WHERE expires_at_unix_nano <= ? AND NOT EXISTS (SELECT 1 FROM "+store.assets.Quoted()+" a WHERE a.conversation_id="+store.keys.Quoted()+".conversation_id AND a.id="+store.keys.Quoted()+".id)"), now); err != nil {
		return err
	}
	return sqlmeta.ExternalSQLError("history: commit retention", tx.Commit())
}
