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

// PurgeExpired removes at most limit entries from the internal expiry index.
// It discovers candidates using configured reads and rechecks deadlines in a
// fresh transaction, preserving concurrent refreshes. If a historical candidate
// is already absent there, it discards the uncommitted batch and rediscovers
// once on the primary. The count reports index entries removed by a successful
// commit, so repeated calls can make progress before historical reads advance.
// It does not advance the cluster's MVCC GC safe point. Registry-backed stores
// run this maintenance automatically; direct borrowers may call it themselves.
func (s *TiKV) PurgeExpired(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 1024 {
		return 0, errors.New("kv: tikv expiry batch limit must be between 1 and 1024")
	}
	rediscoverMissing := len(s.labels) > 0 && !requiresStrongReads(ctx)
	prefix := tiKVExpiryPrefix()
	for {
		candidates, err := s.expiredCandidates(ctx, limit)
		if err != nil || len(candidates) == 0 {
			return 0, err
		}
		var removed int
		var rediscover bool
		_, err = s.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
			removed, rediscover = 0, false
			for _, indexKey := range candidates {
				if len(indexKey) < len(prefix)+8 {
					return false, errors.New("kv: invalid tikv expiry key")
				}
				if _, err := tx.Get(ctx, indexKey); tikverr.IsErrNotFound(err) {
					if rediscoverMissing {
						rediscover = true
						return false, nil
					}
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
				removed++
			}
			return removed > 0, ctx.Err()
		})
		if err != nil {
			return 0, err
		}
		if !rediscover {
			return removed, nil
		}
		// Only a discarded transaction can reach this retry; never replay an
		// uncertain commit. A second discovery cannot trigger another retry.
		rediscoverMissing = false
		ctx = WithPrimaryReads(ctx)
	}
}

func (s *TiKV) expiredCandidates(ctx context.Context, limit int) ([][]byte, error) {
	view, err := s.readView(ctx, nil, nil, false)
	if err != nil {
		return nil, tiKVError("expiry snapshot", err)
	}
	defer view.close()
	snapshot := view.snapshot
	prefix := tiKVExpiryPrefix()
	end := binary.BigEndian.AppendUint64(bytes.Clone(prefix), uint64(time.Now().UnixNano()))
	end = tiKVPrefixEnd(end)
	snapshot.SetScanBatchSize(max(2, min(limit, 256)))
	it, err := snapshot.Iter(prefix, end)
	if err != nil {
		return nil, tiKVError("scan expiry", view.readError(err))
	}
	defer it.Close()
	var candidates [][]byte
	for it.Valid() && len(candidates) < limit {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidates = append(candidates, bytes.Clone(it.Key()))
		if len(candidates) == limit {
			break
		}
		if err := it.Next(); err != nil {
			return nil, tiKVError("scan expiry next", view.readError(err))
		}
	}
	return candidates, ctx.Err()
}
