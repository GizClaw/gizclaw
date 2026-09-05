package kv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

const sqlInitializationTimeout = 30 * time.Second

// SQL implements one prefix-scoped Store over a SQLite or PostgreSQL table.
// The connection pool is borrowed and remains owned by the caller.
type SQL struct {
	db     *sqlx.DB
	opts   *Options
	table  storage.SQLTable
	quoted string

	mu     sync.RWMutex
	closed bool
}

// NewSQLWithDB builds a prefix-scoped Store over a borrowed SQL pool. Prefix is
// used as the quoted backend table name and is not added to encoded keys.
func NewSQLWithDB(db *sqlx.DB, prefix string, options *Options) (*SQL, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sqlInitializationTimeout)
	defer cancel()
	return newSQLWithDB(ctx, db, prefix, options)
}

func newSQLWithDB(ctx context.Context, db *sqlx.DB, prefix string, options *Options) (*SQL, error) {
	table, err := storage.PrepareSQLTable(db, "kv", prefix)
	if err != nil {
		return nil, fmt.Errorf("kv: sql prefix %q: %w", prefix, err)
	}
	if err := ensureSQLSchema(ctx, db, table); err != nil {
		return nil, fmt.Errorf("kv: initialize sql prefix %q: %w", prefix, err)
	}
	store := &SQL{db: db, opts: options, table: table, quoted: table.Quoted()}
	if err := store.checkSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *SQL) checkSchema(ctx context.Context) error {
	columns, err := storage.SQLColumns(ctx, store.db, store.table)
	if err != nil {
		return fmt.Errorf("kv: inspect sql table %q: %w", store.table.Name(), err)
	}
	binaryType := "BLOB"
	if store.table.Dialect() == storage.SQLDialectPostgreSQL {
		binaryType = "BYTEA"
	}
	want := map[string]storage.SQLColumn{
		"encoded_key":          {Type: binaryType, Nullable: false, PrimaryKeyPosition: 1},
		"value":                {Type: binaryType, Nullable: false},
		"expires_at_unix_nano": {Type: "BIGINT", Nullable: true},
	}
	if err := storage.ValidateSQLColumns(columns, want); err != nil {
		return fmt.Errorf("kv: incompatible sql table %q: %w", store.table.Name(), err)
	}
	index, err := storage.SQLIndexName(store.table, "expires_idx")
	if err != nil {
		return fmt.Errorf("kv: derive sql table %q expiration index: %w", store.table.Name(), err)
	}
	if err := storage.ValidateSQLIndexColumns(ctx, store.db, store.table, index, []string{"expires_at_unix_nano"}); err != nil {
		return fmt.Errorf("kv: incompatible sql table %q expiration index: %w", store.table.Name(), err)
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
		return nil, fmt.Errorf("kv: sql store is closed: %w", ErrStoreClosed)
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
	query := store.db.Rebind("SELECT value, expires_at_unix_nano FROM " + store.quoted + " WHERE encoded_key = ?")
	err := store.db.QueryRowContext(ctx, query, encoded).Scan(&value, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, storage.ExternalSQLError("kv: get sql key", err)
	}
	if expires.Valid && expires.Int64 <= time.Now().UnixNano() {
		deleteQuery := store.db.Rebind("DELETE FROM " + store.quoted + " WHERE encoded_key = ? AND expires_at_unix_nano <= ?")
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
		entries, err := store.listAfter(ctx, prefix, nil, nil)
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
	return store.listAfter(ctx, prefix, after, &limit)
}

func (store *SQL) listAfter(ctx context.Context, prefix, after Key, limit *int) ([]Entry, error) {
	unlock, err := store.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit != nil && *limit == 0 {
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
	rows, err := store.db.QueryContext(ctx, store.db.Rebind(query), args...)
	if err != nil {
		return nil, storage.ExternalSQLError("kv: list sql keys", err)
	}
	defer rows.Close()
	now := time.Now().UnixNano()
	entries := make([]Entry, 0)
	for rows.Next() {
		var key, value []byte
		var expires sql.NullInt64
		if err := rows.Scan(&key, &value, &expires); err != nil {
			return nil, storage.ExternalSQLError("kv: scan sql key", err)
		}
		if len(prefixBytes) > 0 && !bytes.HasPrefix(key, prefixBytes) {
			break
		}
		if expires.Valid && expires.Int64 <= now {
			continue
		}
		entries = append(entries, Entry{Key: store.opts.decode(key), Value: append([]byte(nil), value...)})
		if limit != nil && len(entries) == *limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, storage.ExternalSQLError("kv: list sql rows", err)
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
	mutationKeys := store.mutationKeys(entries, keys)
	tx, err := store.beginWrite(ctx, mutationKeys)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now()
	prepared, err := store.prepare(entries, now)
	if err != nil {
		return err
	}
	// Upsert and delete already replace/remove expired target rows. Avoid a
	// separate expiry DELETE on the PostgreSQL ordinary-write hot path.
	if store.table.Dialect() != storage.SQLDialectPostgreSQL {
		if err := store.cleanupExpired(ctx, tx, now.UnixNano(), mutationKeys); err != nil {
			return err
		}
	}
	for _, entry := range prepared {
		if err := store.upsert(ctx, tx, entry); err != nil {
			return err
		}
	}
	deleteQuery := store.db.Rebind("DELETE FROM " + store.quoted + " WHERE encoded_key = ?")
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, deleteQuery, store.opts.encode(key)); err != nil {
			return storage.ExternalSQLError("kv: delete sql key", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return storage.ExternalSQLError("kv: commit sql mutation", err)
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
	mutationKeys := store.mutationKeys(append(slices.Clone(guards), entries...), nil)
	tx, err := store.beginWrite(ctx, mutationKeys)
	if err != nil {
		return nil, nil, false, err
	}
	defer tx.Rollback()
	now := time.Now()
	if err := store.cleanupExpired(ctx, tx, now.UnixNano(), mutationKeys); err != nil {
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
		insert := store.db.Rebind("INSERT INTO " + store.quoted + " (encoded_key, value, expires_at_unix_nano) VALUES (?, ?, ?) ON CONFLICT(encoded_key) DO NOTHING")
		result, err := tx.ExecContext(ctx, insert, guard.key, guard.value, guard.expires)
		if err != nil {
			return nil, nil, false, storage.ExternalSQLError("kv: claim sql guard", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, nil, false, storage.ExternalSQLError("kv: inspect sql guard claim", err)
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
		return nil, nil, false, storage.ExternalSQLError("kv: commit sql conditional create", err)
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
	mutationKeys := store.mutationKeys(entries, append(slices.Clone(keys), guard))
	tx, err := store.beginWrite(ctx, mutationKeys)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now()
	if err := store.cleanupExpired(ctx, tx, now.UnixNano(), mutationKeys); err != nil {
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
	deleteQuery := store.db.Rebind("DELETE FROM " + store.quoted + " WHERE encoded_key = ?")
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, deleteQuery, store.opts.encode(key)); err != nil {
			return false, storage.ExternalSQLError("kv: delete compared sql key", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, storage.ExternalSQLError("kv: commit sql compare-and-mutate", err)
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
			nanoseconds, err := storage.SQLUnixNano(entry.Deadline)
			if err != nil {
				return nil, fmt.Errorf("kv: invalid SQL deadline: %w", ErrInvalidDeadline)
			}
			expires = nanoseconds
		}
		value := make([]byte, len(entry.Value))
		copy(value, entry.Value)
		prepared = append(prepared, sqlEntry{
			key: store.opts.encode(entry.Key), value: value, expires: expires,
		})
	}
	return prepared, nil
}

func (store *SQL) beginWrite(ctx context.Context, keys [][]byte) (*sqlx.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, storage.ExternalSQLError("kv: begin sql transaction", err)
	}
	if store.table.Dialect() == storage.SQLDialectPostgreSQL {
		// Transaction advisory locks also protect absent guards. Every mutation
		// participates, including ordinary Set/Delete. Sort lock IDs (rather
		// than keys) so even hash collisions cannot introduce lock-order cycles.
		ids := make([]int64, 0, len(keys))
		for _, key := range keys {
			digest := sha256.Sum256(append([]byte(store.table.Name()+"\x00"), key...))
			ids = append(ids, int64(binary.BigEndian.Uint64(digest[:8])))
		}
		slices.Sort(ids)
		for _, id := range slices.Compact(ids) {
			if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", id); err != nil {
				_ = tx.Rollback()
				return nil, storage.ExternalSQLError("kv: lock sql key", err)
			}
		}
	}
	return tx, nil
}

func (store *SQL) mutationKeys(entries []Entry, keys []Key) [][]byte {
	encoded := make([][]byte, 0, len(entries))
	for _, entry := range entries {
		encoded = append(encoded, store.opts.encode(entry.Key))
	}
	for _, key := range keys {
		encoded = append(encoded, store.opts.encode(key))
	}
	slices.SortFunc(encoded, bytes.Compare)
	return slices.CompactFunc(encoded, bytes.Equal)
}

func (store *SQL) cleanupExpired(ctx context.Context, tx *sqlx.Tx, now int64, keys [][]byte) error {
	if store.table.Dialect() == storage.SQLDialectPostgreSQL {
		// Do not touch unrelated rows: that would reintroduce contention and
		// acquire row locks outside the canonical mutation lock order.
		query := store.db.Rebind("DELETE FROM " + store.quoted + " WHERE encoded_key = ? AND expires_at_unix_nano <= ?")
		for _, key := range keys {
			if _, err := tx.ExecContext(ctx, query, key, now); err != nil {
				return storage.ExternalSQLError("kv: clean expired sql key", err)
			}
		}
		return nil
	}
	query := store.db.Rebind("DELETE FROM " + store.quoted + " WHERE expires_at_unix_nano IS NOT NULL AND expires_at_unix_nano <= ?")
	if _, err := tx.ExecContext(ctx, query, now); err != nil {
		return storage.ExternalSQLError("kv: clean expired sql keys", err)
	}
	return nil
}

func (store *SQL) upsert(ctx context.Context, tx *sqlx.Tx, entry sqlEntry) error {
	query := store.db.Rebind("INSERT INTO " + store.quoted + " (encoded_key, value, expires_at_unix_nano) VALUES (?, ?, ?) ON CONFLICT(encoded_key) DO UPDATE SET value = excluded.value, expires_at_unix_nano = excluded.expires_at_unix_nano")
	if _, err := tx.ExecContext(ctx, query, entry.key, entry.value, entry.expires); err != nil {
		return storage.ExternalSQLError("kv: upsert sql key", err)
	}
	return nil
}

func (store *SQL) txGet(ctx context.Context, tx *sqlx.Tx, key Key) ([]byte, bool, error) {
	query := store.db.Rebind("SELECT value FROM " + store.quoted + " WHERE encoded_key = ?")
	var value []byte
	if err := tx.QueryRowContext(ctx, query, store.opts.encode(key)).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, storage.ExternalSQLError("kv: inspect sql guard", err)
	}
	return append([]byte(nil), value...), true, nil
}

var _ Store = (*SQL)(nil)
