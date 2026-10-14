package kv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"time"

	redis "github.com/redis/go-redis/v9"
	tikverr "github.com/tikv/client-go/v2/error"
	"github.com/tikv/client-go/v2/txnkv/transaction"
)

func (m *redisMigration) rawTarget(ctx context.Context, key Key) ([]byte, bool, error) {
	snapshot, err := m.target.snapshot(strongReadContext(ctx))
	if err != nil {
		return nil, false, tiKVError("migration snapshot", err)
	}
	raw, err := snapshot.Get(ctx, m.target.recordKey(key))
	if tikverr.IsErrNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, tiKVError("migration read", err)
	}
	return raw.Value, true, nil
}

func (m *redisMigration) importRecord(ctx context.Context, key Key, record redisMigrationRecord) (bool, error) {
	raw, exists, err := m.rawTarget(ctx, key)
	if err != nil {
		return false, err
	}
	if exists {
		if _, err := decodeTiKVValue(raw, time.Now()); !errors.Is(err, ErrMigrationInProgress) {
			if err == nil {
				return false, m.verifyRecord(ctx, key, record, raw)
			}
			if !errors.Is(err, ErrNotFound) {
				return false, err
			}
		} else if record.kind == 0 || m.options.DryRun || m.options.VerifyOnly {
			return false, ErrMigrationInProgress
		}
	}
	if record.deadline != 0 && record.deadline <= time.Now().UnixNano() {
		return false, ErrInvalidDeadline
	}
	if m.options.VerifyOnly {
		return false, ErrMigrationConflict
	}
	if m.options.DryRun {
		return true, nil
	}
	if record.kind == 0 {
		imported := false
		_, err := m.target.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
			imported = false
			value, _, err := m.target.getTxn(ctx, tx, key)
			if err == nil {
				if value.kind != 0 || value.deadline != record.deadline || !bytes.Equal(value.payload, record.payload) {
					return false, ErrMigrationConflict
				}
				return false, nil
			}
			if !errors.Is(err, ErrNotFound) {
				return false, err
			}
			entry := Entry{Key: key, Value: record.payload}
			if record.deadline != 0 {
				entry.Deadline = time.Unix(0, record.deadline)
			}
			if err := m.target.entriesTxn(ctx, tx, []Entry{entry}, nil); err != nil {
				return false, err
			}
			imported = true
			return true, nil
		})
		return imported, err
	}
	// A claim keeps incomplete member rows invisible and rejects ordinary Store
	// mutations. Every member batch writes that guard, preventing SI write skew.
	_, err = m.target.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
		existing, err := tx.Get(ctx, m.target.recordKey(key))
		if tikverr.IsErrNotFound(err) {
			return true, tx.Set(m.target.recordKey(key), m.claim(key, record, 0))
		}
		if err != nil {
			return false, err
		}
		if _, err := m.claimCount(key, record, existing.Value); err != nil {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return false, err
	}
	err = m.eachSourceMember(ctx, key, record, func(members []string) error {
		_, err := m.target.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
			raw, err := tx.Get(ctx, m.target.recordKey(key))
			if err != nil {
				return false, err
			}
			count, err := m.claimCount(key, record, raw.Value)
			if err != nil {
				return false, err
			}
			for _, member := range members {
				memberKey := append(m.target.memberPrefix(key), member...)
				existing, err := tx.Get(ctx, memberKey)
				if tikverr.IsErrNotFound(err) {
					count++
				} else if err != nil {
					return false, err
				} else if !bytes.Equal(existing.Value, []byte{1}) {
					return false, ErrMigrationConflict
				}
				if err := tx.Set(memberKey, []byte{1}); err != nil {
					return false, err
				}
			}
			if count > record.count {
				return false, ErrMigrationConflict
			}
			return true, tx.Set(m.target.recordKey(key), m.claim(key, record, count))
		})
		return err
	})
	if err != nil {
		return false, err
	}
	if err := m.verifyCollection(ctx, key, record); err != nil {
		return false, err
	}
	_, err = m.target.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
		raw, err := tx.Get(ctx, m.target.recordKey(key))
		if err != nil {
			return false, err
		}
		count, err := m.claimCount(key, record, raw.Value)
		if err != nil {
			return false, err
		}
		if count != record.count {
			return false, ErrMigrationConflict
		}
		return true, tx.Set(m.target.recordKey(key), (tiKVValue{kind: record.kind, payload: binary.BigEndian.AppendUint64(nil, count)}).encode())
	})
	return err == nil, err
}

func (m *redisMigration) verifyRecord(ctx context.Context, key Key, record redisMigrationRecord, raw []byte) error {
	value, err := decodeTiKVValue(raw, time.Unix(0, 0))
	if err != nil {
		return err
	}
	if value.kind != record.kind || value.deadline != record.deadline {
		return ErrMigrationConflict
	}
	if record.kind == 0 {
		if !bytes.Equal(value.payload, record.payload) {
			return ErrMigrationConflict
		}
		return nil
	}
	if binary.BigEndian.Uint64(value.payload) != record.count {
		return ErrMigrationConflict
	}
	return m.verifyCollection(ctx, key, record)
}

func (m *redisMigration) verifyCollection(ctx context.Context, key Key, record redisMigrationRecord) error {
	prefix := m.target.memberPrefix(key)
	var count uint64
	err := m.scanTarget(ctx, prefix, tiKVPrefixEnd(prefix), 128, func(rows []redisMigrationRow) error {
		pipe := m.source.client.Pipeline()
		defer pipe.Discard()
		encoded := string(m.source.opts.encode(key))
		for _, row := range rows {
			if !bytes.Equal(row.value, []byte{1}) {
				return ErrMigrationConflict
			}
			member := string(row.key[len(prefix):])
			if record.kind == 1 {
				pipe.SIsMember(ctx, encoded, member)
			} else {
				pipe.ZScore(ctx, encoded, member)
			}
		}
		commands, err := pipe.Exec(ctx)
		if errors.Is(err, redis.Nil) {
			return ErrMigrationConflict
		}
		if err != nil {
			return redisOperationError("migration member verification", err)
		}
		for _, command := range commands {
			if record.kind == 1 {
				value, ok := command.(*redis.BoolCmd)
				if !ok || !value.Val() {
					return ErrMigrationConflict
				}
			} else {
				value, ok := command.(*redis.FloatCmd)
				if !ok || value.Val() != 0 {
					return ErrMigrationConflict
				}
			}
			count++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if count != record.count {
		return ErrMigrationConflict
	}
	current, exists, err := m.readSource(ctx, string(m.source.opts.encode(key)))
	if err != nil {
		return err
	}
	if !exists || current.kind != record.kind || current.count != record.count {
		return ErrMigrationConflict
	}
	return nil
}

type redisMigrationRow struct{ key, value []byte }

// Rotate snapshots between bounded pages so a multi-hour import does not keep
// one old snapshot alive beyond the cluster's MVCC GC retention window.
func (m *redisMigration) scanTarget(ctx context.Context, start, end []byte, limit int, fn func([]redisMigrationRow) error) error {
	for {
		snapshot, err := m.target.snapshot(strongReadContext(ctx))
		if err != nil {
			return err
		}
		snapshot.SetScanBatchSize(limit)
		it, err := snapshot.Iter(start, end)
		if err != nil {
			return err
		}
		var rows []redisMigrationRow
		for it.Valid() && len(rows) < limit {
			if err := ctx.Err(); err != nil {
				it.Close()
				return err
			}
			rows = append(rows, redisMigrationRow{bytes.Clone(it.Key()), bytes.Clone(it.Value())})
			if len(rows) == limit {
				break
			}
			if err := it.Next(); err != nil {
				it.Close()
				return err
			}
		}
		it.Close()
		if len(rows) == 0 {
			return nil
		}
		if err := fn(rows); err != nil {
			return err
		}
		if len(rows) < limit {
			return nil
		}
		start = append(bytes.Clone(rows[len(rows)-1].key), 0)
	}
}

func (m *redisMigration) eachTargetRecord(ctx context.Context, prefix Key, fn func(Key, []byte) error) error {
	raw, exists, err := m.rawTarget(ctx, prefix)
	if err != nil {
		return err
	}
	if exists {
		if err := fn(prefix, raw); err != nil {
			return err
		}
	}
	start := append(m.target.recordKey(prefix), m.target.opts.sep())
	return m.scanTarget(ctx, start, tiKVPrefixEnd(start), 32, func(rows []redisMigrationRow) error {
		for _, row := range rows {
			key := m.target.opts.decode(row.key[len(tiKVNamespace)+1:])
			if err := fn(key, row.value); err != nil {
				return err
			}
		}
		return nil
	})
}
