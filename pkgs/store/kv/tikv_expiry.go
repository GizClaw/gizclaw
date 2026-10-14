package kv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"time"

	tikverr "github.com/tikv/client-go/v2/error"
	"github.com/tikv/client-go/v2/txnkv/transaction"
)

func tiKVExpiryPrefix() []byte { return append(bytes.Clone(tiKVNamespace), 3) }
func (s *TiKV) expiryKey(key Key, deadline int64) []byte {
	prefix := binary.BigEndian.AppendUint64(tiKVExpiryPrefix(), uint64(deadline))
	return append(prefix, s.opts.encode(key)...)
}

// PurgeExpired processes at most limit entries from the internal expiry index.
// It always uses strong reads and rechecks record deadlines transactionally,
// preserving concurrent refreshes. The count reports index entries processed.
// It does not advance the cluster's MVCC GC safe point. Registry-backed stores
// run this maintenance automatically; direct borrowers may call it themselves.
func (s *TiKV) PurgeExpired(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 1024 {
		return 0, errors.New("kv: tikv expiry batch limit must be between 1 and 1024")
	}
	snapshot, err := s.snapshot(strongReadContext(ctx))
	if err != nil {
		return 0, tiKVError("expiry snapshot", err)
	}
	prefix := tiKVExpiryPrefix()
	end := binary.BigEndian.AppendUint64(bytes.Clone(prefix), uint64(time.Now().UnixNano()))
	end = tiKVPrefixEnd(end)
	snapshot.SetScanBatchSize(max(2, min(limit, 256)))
	it, err := snapshot.Iter(prefix, end)
	if err != nil {
		return 0, tiKVError("scan expiry", err)
	}
	var candidates [][]byte
	for it.Valid() && len(candidates) < limit {
		if err := ctx.Err(); err != nil {
			it.Close()
			return 0, err
		}
		candidates = append(candidates, bytes.Clone(it.Key()))
		if len(candidates) == limit {
			break
		}
		if err := it.Next(); err != nil {
			it.Close()
			return 0, tiKVError("scan expiry next", err)
		}
	}
	it.Close()
	if len(candidates) == 0 {
		return 0, ctx.Err()
	}
	_, err = s.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
		for _, indexKey := range candidates {
			if len(indexKey) < len(prefix)+8 {
				return false, errors.New("kv: invalid tikv expiry key")
			}
			if _, err := tx.Get(ctx, indexKey); tikverr.IsErrNotFound(err) {
				continue
			} else if err != nil {
				return false, err
			}
			deadline := int64(binary.BigEndian.Uint64(indexKey[len(prefix):]))
			key := s.opts.decode(indexKey[len(prefix)+8:])
			v, _, err := s.getTxn(ctx, tx, key)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return false, err
			}
			if v.deadline == deadline && deadline <= time.Now().UnixNano() {
				if err := tx.Delete(s.recordKey(key)); err != nil {
					return false, err
				}
			}
			if err := tx.Delete(indexKey); err != nil {
				return false, err
			}
		}
		return true, ctx.Err()
	})
	if err != nil {
		return 0, err
	}
	return len(candidates), nil
}
