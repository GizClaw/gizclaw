package kv

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

// AddMembers adds members idempotently to one collection.
func (s *SQL) AddMembers(ctx context.Context, key Key, members ...string) error {
	_, err := s.ApplyMutation(ctx, Mutation{AddMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// RemoveMembers removes members idempotently from one collection.
func (s *SQL) RemoveMembers(ctx context.Context, key Key, members ...string) error {
	_, err := s.ApplyMutation(ctx, Mutation{RemoveMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// HasMember queries the collection/member composite primary key.
func (s *SQL) HasMember(ctx context.Context, key Key, member string) (found bool, err error) {
	defer func() { err = sqlCollectionError(err) }()
	unlock, err := s.lock()
	if err != nil {
		return false, err
	}
	defer unlock()
	var kind int
	query := "SELECT value_kind, EXISTS(SELECT 1 FROM " + s.members + " m WHERE m.encoded_key = k.encoded_key AND m.member = ?) FROM " + s.quoted + " k WHERE encoded_key = ? AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)"
	err = s.db.QueryRowContext(ctx, s.db.Rebind(query), []byte(member), s.opts.encode(key), time.Now().UnixNano()).Scan(&kind, &found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if kind != 1 {
		return false, ErrWrongType
	}
	return found, nil
}

// ListMembers returns members for an exact collection key using its index.
func (s *SQL) ListMembers(ctx context.Context, key Key) (members []string, err error) {
	defer func() { err = sqlCollectionError(err) }()
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	query := "SELECT k.value_kind, m.member IS NOT NULL, m.member FROM " + s.quoted + " k LEFT JOIN " + s.members + " m ON m.encoded_key = k.encoded_key WHERE k.encoded_key = ? AND (k.expires_at_unix_nano IS NULL OR k.expires_at_unix_nano > ?)"
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(query), s.opts.encode(key), time.Now().UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind int
		var member []byte
		var present bool
		if err := rows.Scan(&kind, &present, &member); err != nil {
			return nil, err
		}
		if kind != 1 {
			return nil, ErrWrongType
		}
		if present {
			members = append(members, string(member))
		}
	}
	return members, rows.Err()
}

func (s *SQL) clearMembers(ctx context.Context, tx *sqlx.Tx, keys [][]byte) error {
	for len(keys) > 0 {
		count := min(len(keys), 256)
		args := make([]any, count)
		for i, key := range keys[:count] {
			args[i] = key
		}
		query := "DELETE FROM " + s.members + " WHERE encoded_key IN (" + strings.TrimSuffix(strings.Repeat("?,", count), ",") + ")"
		if _, err := tx.ExecContext(ctx, s.db.Rebind(query), args...); err != nil {
			return err
		}
		keys = keys[count:]
	}
	return nil
}

// ApplyMutation locks the affected keys and atomically updates records and
// collection/member rows. Conditions are checked before any mutation is applied.
func (s *SQL) ApplyMutation(ctx context.Context, mutation Mutation) (applied bool, err error) {
	defer func() { err = sqlCollectionError(err) }()
	if err := mutation.validate(ctx, s.opts); err != nil {
		return false, err
	}
	unlock, err := s.lock()
	if err != nil {
		return false, err
	}
	defer unlock()
	recordKeys := s.mutationKeys(mutation.Entries, mutation.DeleteKeys)
	lockKeys := append([][]byte(nil), recordKeys...)
	for _, c := range mutation.Conditions {
		lockKeys = append(lockKeys, s.opts.encode(c.Key))
	}
	for _, groups := range [][]SetMembers{mutation.AddMembers, mutation.RemoveMembers, mutation.AddOrderedMembers, mutation.RemoveOrderedMembers} {
		for _, g := range groups {
			lockKeys = append(lockKeys, s.opts.encode(g.Key))
		}
	}
	tx, err := s.beginWrite(ctx, lockKeys)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now()
	for _, c := range mutation.Conditions {
		var value []byte
		var kind int
		query := "SELECT value, value_kind FROM " + s.quoted + " WHERE encoded_key = ? AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)"
		err := tx.QueryRowContext(ctx, s.db.Rebind(query), s.opts.encode(c.Key), now.UnixNano()).Scan(&value, &kind)
		if errors.Is(err, sql.ErrNoRows) {
			if c.Expected != nil {
				return false, nil
			}
			continue
		}
		if err != nil {
			return false, err
		}
		if c.Expected == nil {
			return false, nil
		}
		if kind != 0 {
			return false, ErrWrongType
		}
		if !bytes.Equal(value, c.Expected) {
			return false, nil
		}
	}
	prepared, err := s.prepare(mutation.Entries, now)
	if err != nil {
		return false, err
	}
	if err := s.clearMembers(ctx, tx, recordKeys); err != nil {
		return false, err
	}
	for _, entry := range prepared {
		if err := s.upsert(ctx, tx, entry); err != nil {
			return false, err
		}
	}
	for _, key := range mutation.DeleteKeys {
		if _, err := tx.ExecContext(ctx, s.db.Rebind("DELETE FROM "+s.quoted+" WHERE encoded_key = ?"), s.opts.encode(key)); err != nil {
			return false, err
		}
	}
	for phase, groups := range [][]SetMembers{mutation.AddMembers, mutation.RemoveMembers, mutation.AddOrderedMembers, mutation.RemoveOrderedMembers} {
		expectedKind := 1 + phase/2
		for _, g := range groups {
			if len(g.Members) == 0 {
				continue
			}
			key := s.opts.encode(g.Key)
			var kind int
			err := tx.QueryRowContext(ctx, s.db.Rebind("SELECT value_kind FROM "+s.quoted+" WHERE encoded_key = ? AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano > ?)"), key, now.UnixNano()).Scan(&kind)
			if err == nil && kind != expectedKind {
				return false, ErrWrongType
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return false, err
			}
			if errors.Is(err, sql.ErrNoRows) {
				if err := s.clearMembers(ctx, tx, [][]byte{key}); err != nil {
					return false, err
				}
				if phase%2 == 1 {
					continue
				}
				query := "INSERT INTO " + s.quoted + " (encoded_key,value,value_kind) VALUES (?,?,?) ON CONFLICT(encoded_key) DO UPDATE SET value=excluded.value,value_kind=excluded.value_kind,expires_at_unix_nano=NULL"
				if _, err := tx.ExecContext(ctx, s.db.Rebind(query), key, []byte{}, expectedKind); err != nil {
					return false, err
				}
			}
			for _, member := range g.Members {
				query := "INSERT INTO " + s.members + " (encoded_key,member) VALUES (?,?) ON CONFLICT(encoded_key,member) DO NOTHING"
				if phase%2 == 1 {
					query = "DELETE FROM " + s.members + " WHERE encoded_key = ? AND member = ?"
				}
				if _, err := tx.ExecContext(ctx, s.db.Rebind(query), key, []byte(member)); err != nil {
					return false, err
				}
			}
			if phase%2 == 1 {
				query := "DELETE FROM " + s.quoted + " WHERE encoded_key = ? AND NOT EXISTS (SELECT 1 FROM " + s.members + " WHERE encoded_key = ?)"
				if _, err := tx.ExecContext(ctx, s.db.Rebind(query), key, key); err != nil {
					return false, err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, storage.ExternalSQLError("kv: commit collection mutation", err)
	}
	return true, nil
}

func sqlCollectionError(err error) error {
	if err == nil || errors.Is(err, ErrWrongType) || errors.Is(err, ErrInvalidDeadline) || errors.Is(err, ErrStoreClosed) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return storage.ExternalSQLError("kv: sql collection operation", err)
}
