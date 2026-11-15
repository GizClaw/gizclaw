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

// AddMembers idempotently adds opaque members to one exact collection.
func (s *TiKV) AddMembers(ctx context.Context, key Key, members ...string) error {
	_, err := s.ApplyMutation(ctx, Mutation{AddMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// RemoveMembers idempotently removes members from one exact collection.
func (s *TiKV) RemoveMembers(ctx context.Context, key Key, members ...string) error {
	_, err := s.ApplyMutation(ctx, Mutation{RemoveMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// HasMember reads the collection type and member from one snapshot.
func (s *TiKV) HasMember(ctx context.Context, key Key, member string) (bool, error) {
	prefix := s.memberPrefix(key)
	record, memberKey := s.recordKey(key), append(bytes.Clone(prefix), member...)
	view, err := s.readView(ctx, [][]byte{record, memberKey}, prefix, false)
	if err != nil {
		return false, tiKVError("member snapshot", err)
	}
	defer view.close()
	raw, err := view.get(ctx, record)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, tiKVError("collection", err)
	}
	v, err := decodeTiKVValue(raw, time.Now())
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if v.kind != 1 {
		return false, ErrWrongType
	}
	_, err = view.get(ctx, memberKey)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, tiKVError("has member", err)
}

// ListMembers reads all members of one exact ordinary collection.
func (s *TiKV) ListMembers(ctx context.Context, key Key) ([]string, error) {
	return s.members(ctx, key, 1, OrderedRange{})
}

// RangeOrderedMembers seeks a bounded bytewise range in one ordered collection.
func (s *TiKV) RangeOrderedMembers(ctx context.Context, key Key, query OrderedRange) ([]string, error) {
	if err := query.validate(ctx); err != nil {
		return nil, err
	}
	return s.members(ctx, key, 2, query)
}
func (s *TiKV) members(ctx context.Context, key Key, kind byte, query OrderedRange) ([]string, error) {
	prefix := s.memberPrefix(key)
	record := s.recordKey(key)
	start, end := bytes.Clone(prefix), tiKVPrefixEnd(prefix)
	if query.After != nil {
		start = append(start, *query.After...)
		start = append(start, 0)
	}
	if query.Before != nil {
		end = append(bytes.Clone(prefix), *query.Before...)
	}
	view, err := s.readView(ctx, [][]byte{record}, prefix, true, tiKVLocalRange{start: start, end: end})
	if err != nil {
		return nil, tiKVError("member snapshot", err)
	}
	defer view.close()
	raw, err := view.get(ctx, record)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, tiKVError("collection", err)
	}
	v, err := decodeTiKVValue(raw, time.Now())
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if v.kind != kind {
		return nil, ErrWrongType
	}
	if bytes.Compare(start, end) >= 0 {
		return nil, nil
	}
	if query.Limit > 0 {
		view.snapshot.SetScanBatchSize(max(2, min(query.Limit, 256)))
	}
	members, err := view.rangeMembers(ctx, start, end, query.Limit)
	return members, tiKVError("scan members", err)
}

// ApplyMutation checks conditions and updates records and indexes atomically.
func (s *TiKV) ApplyMutation(ctx context.Context, mutation Mutation) (bool, error) {
	if err := mutation.validate(ctx, s.opts); err != nil {
		return false, err
	}
	return s.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
		// Prefill this transaction's snapshot cache in one native batch read.
		// A presence batch can contain hundreds of guards and records; point
		// reads would otherwise require one network round trip per key. Keep
		// every guard write below: caching does not replace conflict detection.
		keys := make([][]byte, 0, len(mutation.Conditions)+len(mutation.Entries)+len(mutation.DeleteKeys))
		for _, condition := range mutation.Conditions {
			keys = append(keys, s.recordKey(condition.Key))
		}
		for _, entry := range mutation.Entries {
			keys = append(keys, s.recordKey(entry.Key))
		}
		for _, key := range mutation.DeleteKeys {
			keys = append(keys, s.recordKey(key))
		}
		keys = uniqueTiKVKeys(keys)
		if len(keys) > 1 {
			if _, err := tx.BatchGet(ctx, keys); err != nil {
				return false, err
			}
		}
		for _, condition := range mutation.Conditions {
			v, raw, err := s.getTxn(ctx, tx, condition.Key)
			if errors.Is(err, ErrNotFound) {
				if condition.Expected != nil {
					return false, nil
				}
			} else {
				if err != nil {
					return false, err
				}
				if condition.Expected == nil {
					return false, nil
				}
				if v.kind != 0 {
					return false, ErrWrongType
				}
				if !bytes.Equal(v.payload, condition.Expected) {
					return false, nil
				}
			}
			// Touch both present and absent guards to prevent write skew. An
			// expired value is logically absent and is removed in this commit.
			if errors.Is(err, ErrNotFound) {
				if err := s.deleteTxn(ctx, tx, condition.Key); err != nil {
					return false, err
				}
			} else if err := tx.Set(s.recordKey(condition.Key), raw); err != nil {
				return false, err
			}
		}
		if err := s.entriesTxn(ctx, tx, mutation.Entries, mutation.DeleteKeys); err != nil {
			return false, err
		}
		for phase, groups := range [][]SetMembers{mutation.AddMembers, mutation.RemoveMembers, mutation.AddOrderedMembers, mutation.RemoveOrderedMembers} {
			for _, group := range groups {
				if err := s.membersTxn(ctx, tx, group, byte(1+phase/2), phase%2 == 0); err != nil {
					return false, err
				}
			}
		}
		return true, ctx.Err()
	})
}

func (s *TiKV) membersTxn(ctx context.Context, tx *transaction.KVTxn, group SetMembers, kind byte, add bool) error {
	if len(group.Members) == 0 {
		return nil
	}
	v, _, err := s.getTxn(ctx, tx, group.Key)
	count := uint64(0)
	if err == nil {
		if v.kind != kind {
			return ErrWrongType
		}
		count = binary.BigEndian.Uint64(v.payload)
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	previous := count
	for _, member := range group.Members {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := append(s.memberPrefix(group.Key), member...)
		_, err := tx.Get(ctx, key)
		if err != nil && !tikverr.IsErrNotFound(err) {
			return err
		}
		exists := err == nil
		if add {
			// Also publish idempotent additions/removals: a historical reader may
			// disagree with the member state this successful mutation confirmed.
			if !exists || s.local != nil {
				if err := tx.Set(key, []byte{1}); err != nil {
					return err
				}
			}
			if !exists {
				count++
			}
		} else {
			if exists {
				if count == 0 {
					return errors.New("kv: invalid tikv collection count")
				}
				count--
			}
			if exists || s.local != nil {
				if err := tx.Delete(key); err != nil {
					return err
				}
			}
		}
	}
	if group.MaxMembers > 0 && count > uint64(group.MaxMembers) && count > previous {
		return ErrMemberLimit
	}
	if count == 0 {
		return tx.Delete(s.recordKey(group.Key))
	}
	return tx.Set(s.recordKey(group.Key), (tiKVValue{kind: kind, payload: binary.BigEndian.AppendUint64(nil, count)}).encode())
}
