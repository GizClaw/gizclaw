package kv

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/internal/sqlmigration"
	"github.com/jmoiron/sqlx"
)

const sqlInitializationTimeout = 30 * time.Second

// SQL implements Store over one migrated SQLite or PostgreSQL table. The
// connection pool is borrowed and remains owned by the caller.
type SQL struct {
	db        *sqlx.DB
	opts      *Options
	namespace sqlmigration.Namespace
	quoted    string

	mu     sync.RWMutex
	closed bool
}

// NewSQLWithDB builds a table-scoped Store over a borrowed SQL pool.
func NewSQLWithDB(db *sqlx.DB, table string, options *Options) (*SQL, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sqlInitializationTimeout)
	defer cancel()
	return newSQLWithDB(ctx, db, table, options)
}

func newSQLWithDB(ctx context.Context, db *sqlx.DB, table string, options *Options) (*SQL, error) {
	namespace, err := sqlmigration.Prepare(db, "kv", table)
	if err != nil {
		return nil, fmt.Errorf("kv: sql table %q: %w", table, err)
	}
	if _, err := sqlmigration.Run(ctx, db, "kv", table, sqlMigrations(namespace)...); err != nil {
		return nil, fmt.Errorf("kv: initialize sql table %q: %w", table, err)
	}
	quoted, err := sqlmigration.Quote(namespace.Dialect, table)
	if err != nil {
		return nil, fmt.Errorf("kv: quote sql table %q: %w", table, err)
	}
	store := &SQL{db: db, opts: options, namespace: namespace, quoted: quoted}
	if err := store.checkSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *SQL) checkSchema(ctx context.Context) error {
	columns, err := sqlmigration.Columns(ctx, store.db, store.namespace)
	if err != nil {
		return fmt.Errorf("kv: inspect sql table %q: %w", store.namespace.Table, err)
	}
	binaryType := "BLOB"
	if store.namespace.Dialect == sqlmigration.PostgreSQL {
		binaryType = "BYTEA"
	}
	want := map[string]sqlmigration.Column{
		"encoded_key":          {Type: binaryType, Nullable: false, PrimaryKeyPosition: 1},
		"value":                {Type: binaryType, Nullable: false},
		"expires_at_unix_nano": {Type: "BIGINT", Nullable: true},
	}
	if err := sqlmigration.ValidateColumns(columns, want); err != nil {
		return fmt.Errorf("kv: incompatible sql table %q: %w", store.namespace.Table, err)
	}
	index := strings.TrimSuffix(store.namespace.VersionTable, "_schema_versions") + "_expires_idx"
	if err := sqlmigration.ValidateIndexColumns(ctx, store.db, store.namespace, index, []string{"expires_at_unix_nano"}); err != nil {
		return fmt.Errorf("kv: incompatible sql table %q expiration index: %w", store.namespace.Table, err)
	}
	return nil
}

func (store *SQL) lock() (func(), error) {
	if store == nil {
		return nil, errors.New("kv: sql store is nil")
	}
	store.mu.RLock()
	if store.closed || store.db == nil {
		store.mu.RUnlock()
		return nil, errors.New("kv: sql store is closed")
	}
	return store.mu.RUnlock, nil
}

// Get returns a cloned value and treats expired rows as absent.
func (store *SQL) Get(ctx context.Context, key Key) ([]byte, error) {
	unlock, err := store.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return store.getUnlocked(ctx, key)
}

func (store *SQL) getUnlocked(ctx context.Context, key Key) ([]byte, error) {
	encoded := store.opts.encode(key)
	var value []byte
	var expires sql.NullInt64
	query := sqlmigration.Rebind(store.db, "SELECT value, expires_at_unix_nano FROM "+store.quoted+" WHERE encoded_key = ?")
	err := store.db.QueryRowContext(ctx, query, encoded).Scan(&value, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, sqlmigration.ExternalError("kv: get sql key", err)
	}
	if expires.Valid && expires.Int64 <= time.Now().UnixNano() {
		deleteQuery := sqlmigration.Rebind(store.db, "DELETE FROM "+store.quoted+" WHERE encoded_key = ? AND expires_at_unix_nano <= ?")
		_, _ = store.db.ExecContext(ctx, deleteQuery, encoded, time.Now().UnixNano())
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

// Set stores a non-expiring value.
func (store *SQL) Set(ctx context.Context, key Key, value []byte) error {
	return store.BatchSet(ctx, []Entry{{Key: key, Value: value}})
}

// Delete removes a key when present.
func (store *SQL) Delete(ctx context.Context, key Key) error {
	return store.BatchDelete(ctx, []Key{key})
}

// List iterates over a stable snapshot in encoded-key order.
func (store *SQL) List(ctx context.Context, prefix Key) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		entries, err := store.listAfter(ctx, prefix, nil, -1)
		if err != nil {
			yield(Entry{}, err)
			return
		}
		for _, entry := range entries {
			if !yield(entry, nil) {
				return
			}
		}
	}
}

// ListAfter returns at most limit entries strictly after the supplied key.
func (store *SQL) ListAfter(ctx context.Context, prefix, after Key, limit int) ([]Entry, error) {
	if limit < 0 {
		limit = 0
	}
	return store.listAfter(ctx, prefix, after, limit)
}

func (store *SQL) listAfter(ctx context.Context, prefix, after Key, limit int) ([]Entry, error) {
	unlock, err := store.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit == 0 {
		return nil, nil
	}
	prefixBytes := store.opts.encode(prefix)
	if len(prefixBytes) > 0 {
		prefixBytes = append(prefixBytes, store.opts.sep())
	}
	start := prefixBytes
	strict := false
	if len(after) > 0 {
		afterBytes := store.opts.encode(after)
		if bytes.Compare(afterBytes, start) >= 0 {
			start = afterBytes
			strict = true
		}
	}
	operator := ">="
	if strict {
		operator = ">"
	}
	query := "SELECT encoded_key, value, expires_at_unix_nano FROM " + store.quoted
	var args []any
	if len(start) > 0 {
		query += " WHERE encoded_key " + operator + " ?"
		args = append(args, start)
	}
	query += " ORDER BY encoded_key ASC"
	rows, err := store.db.QueryContext(ctx, sqlmigration.Rebind(store.db, query), args...)
	if err != nil {
		return nil, sqlmigration.ExternalError("kv: list sql keys", err)
	}
	defer rows.Close()
	now := time.Now().UnixNano()
	entries := make([]Entry, 0)
	for rows.Next() {
		var key, value []byte
		var expires sql.NullInt64
		if err := rows.Scan(&key, &value, &expires); err != nil {
			return nil, sqlmigration.ExternalError("kv: scan sql key", err)
		}
		if len(prefixBytes) > 0 && !bytes.HasPrefix(key, prefixBytes) {
			break
		}
		if expires.Valid && expires.Int64 <= now {
			continue
		}
		entries = append(entries, Entry{Key: store.opts.decode(key), Value: append([]byte(nil), value...)})
		if limit > 0 && len(entries) == limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, sqlmigration.ExternalError("kv: list sql rows", err)
	}
	return entries, nil
}

// BatchSet atomically stores all entries.
func (store *SQL) BatchSet(ctx context.Context, entries []Entry) error {
	return store.BatchMutate(ctx, entries, nil)
}

// BatchDelete atomically removes all keys.
func (store *SQL) BatchDelete(ctx context.Context, keys []Key) error {
	return store.BatchMutate(ctx, nil, keys)
}

// BatchMutate applies sets followed by deletes in one transaction.
func (store *SQL) BatchMutate(ctx context.Context, entries []Entry, keys []Key) error {
	unlock, err := store.lock()
	if err != nil {
		return err
	}
	defer unlock()
	tx, err := store.beginWrite(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now()
	prepared, err := store.prepare(entries, now)
	if err != nil {
		return err
	}
	if err := store.cleanupExpired(ctx, tx, now.UnixNano()); err != nil {
		return err
	}
	for _, entry := range prepared {
		if err := store.upsert(ctx, tx, entry); err != nil {
			return err
		}
	}
	deleteQuery := sqlmigration.Rebind(store.db, "DELETE FROM "+store.quoted+" WHERE encoded_key = ?")
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, deleteQuery, store.opts.encode(key)); err != nil {
			return sqlmigration.ExternalError("kv: delete sql key", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return sqlmigration.ExternalError("kv: commit sql mutation", err)
	}
	return nil
}

// CreateIfAbsent conditionally creates one guard and related entries.
func (store *SQL) CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) ([]byte, bool, error) {
	_, existing, created, err := store.CreateIfAllAbsent(ctx, []Entry{guard}, entries)
	return existing, created, err
}

// CreateIfAllAbsent atomically claims every guard in input order.
func (store *SQL) CreateIfAllAbsent(ctx context.Context, guards []Entry, entries []Entry) (Key, []byte, bool, error) {
	if len(guards) == 0 {
		return nil, nil, false, errors.New("kv: create-if-all-absent requires at least one guard")
	}
	unlock, err := store.lock()
	if err != nil {
		return nil, nil, false, err
	}
	defer unlock()
	tx, err := store.beginWrite(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	defer tx.Rollback()
	now := time.Now()
	if err := store.cleanupExpired(ctx, tx, now.UnixNano()); err != nil {
		return nil, nil, false, err
	}
	for _, guard := range guards {
		value, exists, err := store.txGet(ctx, tx, guard.Key)
		if err != nil {
			return nil, nil, false, err
		}
		if exists {
			return cloneKey(guard.Key), value, false, nil
		}
	}
	preparedEntries, err := store.prepare(entries, now)
	if err != nil {
		return nil, nil, false, err
	}
	preparedGuards, err := store.prepare(guards, now)
	if err != nil {
		return nil, nil, false, err
	}
	claimed := make(map[string]struct{}, len(preparedGuards))
	for _, guard := range preparedGuards {
		encodedKey := string(guard.key)
		if _, exists := claimed[encodedKey]; exists {
			continue
		}
		claimed[encodedKey] = struct{}{}
		insert := sqlmigration.Rebind(store.db, "INSERT INTO "+store.quoted+" (encoded_key, value, expires_at_unix_nano) VALUES (?, ?, ?) ON CONFLICT(encoded_key) DO NOTHING")
		result, err := tx.ExecContext(ctx, insert, guard.key, guard.value, guard.expires)
		if err != nil {
			return nil, nil, false, sqlmigration.ExternalError("kv: claim sql guard", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, nil, false, sqlmigration.ExternalError("kv: inspect sql guard claim", err)
		}
		if rows == 0 {
			_ = tx.Rollback()
			value, getErr := store.getUnlocked(ctx, store.opts.decode(guard.key))
			return store.opts.decode(guard.key), value, false, getErr
		}
	}
	for _, entry := range preparedEntries {
		if err := store.upsert(ctx, tx, entry); err != nil {
			return nil, nil, false, err
		}
	}
	for _, guard := range preparedGuards {
		if err := store.upsert(ctx, tx, guard); err != nil {
			return nil, nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, false, sqlmigration.ExternalError("kv: commit sql conditional create", err)
	}
	return nil, nil, true, nil
}

// CompareAndMutate applies mutations only when the guard value matches.
func (store *SQL) CompareAndMutate(ctx context.Context, guard Key, expected []byte, entries []Entry, keys []Key) (bool, error) {
	unlock, err := store.lock()
	if err != nil {
		return false, err
	}
	defer unlock()
	tx, err := store.beginWrite(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now()
	if err := store.cleanupExpired(ctx, tx, now.UnixNano()); err != nil {
		return false, err
	}
	value, exists, err := store.txGet(ctx, tx, guard)
	if err != nil {
		return false, err
	}
	if !exists || !bytes.Equal(value, expected) {
		return false, nil
	}
	prepared, err := store.prepare(entries, now)
	if err != nil {
		return false, err
	}
	for _, entry := range prepared {
		if err := store.upsert(ctx, tx, entry); err != nil {
			return false, err
		}
	}
	deleteQuery := sqlmigration.Rebind(store.db, "DELETE FROM "+store.quoted+" WHERE encoded_key = ?")
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, deleteQuery, store.opts.encode(key)); err != nil {
			return false, sqlmigration.ExternalError("kv: delete compared sql key", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, sqlmigration.ExternalError("kv: commit sql compare-and-mutate", err)
	}
	return true, nil
}

// Close marks the logical adapter closed without closing its pool.
func (store *SQL) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	store.closed = true
	store.mu.Unlock()
	return nil
}

type sqlEntry struct {
	key     []byte
	value   []byte
	expires any
}

func (store *SQL) prepare(entries []Entry, now time.Time) ([]sqlEntry, error) {
	prepared := make([]sqlEntry, 0, len(entries))
	for _, entry := range entries {
		var expires any
		if !entry.Deadline.IsZero() {
			if !entry.Deadline.After(now) {
				return nil, ErrInvalidDeadline
			}
			nanoseconds, err := sqlmigration.UnixNano(entry.Deadline)
			if err != nil {
				return nil, fmt.Errorf("kv: invalid SQL deadline: %w", ErrInvalidDeadline)
			}
			expires = nanoseconds
		}
		prepared = append(prepared, sqlEntry{
			key: store.opts.encode(entry.Key), value: append([]byte(nil), entry.Value...), expires: expires,
		})
	}
	return prepared, nil
}

func (store *SQL) beginWrite(ctx context.Context) (*sqlx.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, sqlmigration.ExternalError("kv: begin sql transaction", err)
	}
	if store.namespace.Dialect == sqlmigration.PostgreSQL {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", store.namespace.LockID); err != nil {
			_ = tx.Rollback()
			return nil, sqlmigration.ExternalError("kv: lock sql transaction", err)
		}
	}
	return tx, nil
}

func (store *SQL) cleanupExpired(ctx context.Context, tx *sqlx.Tx, now int64) error {
	query := sqlmigration.Rebind(store.db, "DELETE FROM "+store.quoted+" WHERE expires_at_unix_nano IS NOT NULL AND expires_at_unix_nano <= ?")
	if _, err := tx.ExecContext(ctx, query, now); err != nil {
		return sqlmigration.ExternalError("kv: clean expired sql keys", err)
	}
	return nil
}

func (store *SQL) upsert(ctx context.Context, tx *sqlx.Tx, entry sqlEntry) error {
	query := sqlmigration.Rebind(store.db, "INSERT INTO "+store.quoted+" (encoded_key, value, expires_at_unix_nano) VALUES (?, ?, ?) ON CONFLICT(encoded_key) DO UPDATE SET value = excluded.value, expires_at_unix_nano = excluded.expires_at_unix_nano")
	if _, err := tx.ExecContext(ctx, query, entry.key, entry.value, entry.expires); err != nil {
		return sqlmigration.ExternalError("kv: upsert sql key", err)
	}
	return nil
}

func (store *SQL) txGet(ctx context.Context, tx *sqlx.Tx, key Key) ([]byte, bool, error) {
	query := sqlmigration.Rebind(store.db, "SELECT value FROM "+store.quoted+" WHERE encoded_key = ?")
	var value []byte
	if err := tx.QueryRowContext(ctx, query, store.opts.encode(key)).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, sqlmigration.ExternalError("kv: inspect sql guard", err)
	}
	return append([]byte(nil), value...), true, nil
}

var _ Store = (*SQL)(nil)
