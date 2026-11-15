package kv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/config/retry"
	tikverr "github.com/tikv/client-go/v2/error"
	tikvkv "github.com/tikv/client-go/v2/kv"
	"github.com/tikv/client-go/v2/oracle"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/tikvrpc/interceptor"
	"github.com/tikv/client-go/v2/txnkv/transaction"
	"github.com/tikv/client-go/v2/txnkv/txnsnapshot"
)

// TiKVReadOptions selects native Learner reads using store labels. No fixed
// snapshot delay or application safe-ts probing is used. Local committed writes
// preserve read-your-writes. Ordinary reads cannot switch to a Leader or a store
// with different labels; native replica synchronization errors are propagated.
// Explicit primary reads and mutations bypass regional selection.
type TiKVReadOptions struct {
	// Deprecated: ignored. Labels alone enable regional reads.
	StaleReadDelay time.Duration
	Labels         map[string]string
}

// TiKV is a transactional Store. Records, collection counters and exact member
// keys share one cross-region transaction boundary. The caller owns the client;
// Close is a no-op. TiKV's snapshot isolation is strengthened for mutation
// conditions by writing every guard, including absent guards.
type TiKV struct {
	client *tikv.KVStore
	opts   *Options
	labels []*metapb.StoreLabel
	local  *tiKVLocalWrites
}

// NewTiKVWithClient borrows a transactional client without performing I/O.
// Regional mode adds a context-scoped async routing decorator to the borrowed
// client; other traffic is unchanged. Caller-installed transport decorators must
// retain that client in their delegation chain. Nil opts selects the default
// separator. Zero read options use strong reads.
func NewTiKVWithClient(client *tikv.KVStore, read TiKVReadOptions, opts *Options) (*TiKV, error) {
	if client == nil {
		return nil, errors.New("kv: tikv client is nil")
	}
	store := &TiKV{client: client, opts: opts}
	if len(read.Labels) > 0 {
		store.local = new(tiKVLocalWrites)
	}
	for key, value := range read.Labels {
		if key == "" || value == "" || strings.TrimSpace(key) != key || strings.TrimSpace(value) != value {
			return nil, errors.New("kv: tikv read labels must not be empty")
		}
		store.labels = append(store.labels, &metapb.StoreLabel{Key: key, Value: value})
	}
	slices.SortFunc(store.labels, func(a, b *metapb.StoreLabel) int { return strings.Compare(a.Key, b.Key) })
	if store.local != nil {
		store.installAsyncReadPolicy()
	}
	return store, nil
}

// The versioned application namespace is disjoint from member keys. Length
// encoding keeps binary members and key prefixes from escaping a collection.
var tiKVNamespace = []byte("gizclaw-kv\x00\x01")

func (s *TiKV) recordKey(key Key) []byte {
	return append(append(bytes.Clone(tiKVNamespace), 1), s.opts.encode(key)...)
}

// Duplicate snapshot keys can become duplicate secondary-lock rollback keys in
// client-go. Keep the original input intact for BatchGet's ordered results.
func uniqueTiKVKeys(keys [][]byte) [][]byte {
	if len(keys) < 2 {
		return keys
	}
	seen := make(map[string]struct{}, len(keys))
	unique := make([][]byte, 0, len(keys))
	for _, key := range keys {
		if _, ok := seen[string(key)]; !ok {
			seen[string(key)] = struct{}{}
			unique = append(unique, key)
		}
	}
	return unique
}

func (s *TiKV) memberPrefix(key Key) []byte {
	encoded := s.opts.encode(key)
	prefix := append(bytes.Clone(tiKVNamespace), 2)
	prefix = binary.BigEndian.AppendUint64(prefix, uint64(len(encoded)))
	return append(prefix, encoded...)
}

func tiKVPrefixEnd(prefix []byte) []byte {
	end := bytes.Clone(prefix)
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			return end[:i+1]
		}
	}
	return nil
}

// Kind 0 is a record; 1 and 2 are ordinary and ordered collection counters.
// Kind 3 is a migration claim, rejected by ordinary reads and mutations.
// The header ensures even a nil payload is a nonempty TiKV value.
type tiKVValue struct {
	kind     byte
	deadline int64
	payload  []byte
}

func (v tiKVValue) encode() []byte {
	raw := []byte{1, v.kind}
	raw = binary.BigEndian.AppendUint64(raw, uint64(v.deadline))
	return append(raw, v.payload...)
}

func decodeTiKVValue(raw []byte, now time.Time) (tiKVValue, error) {
	if len(raw) >= 10 && raw[0] == 1 && raw[1] == 3 {
		return tiKVValue{}, ErrMigrationInProgress
	}
	if len(raw) < 10 || raw[0] != 1 || raw[1] > 2 {
		return tiKVValue{}, errors.New("kv: invalid tikv value header")
	}
	v := tiKVValue{kind: raw[1], deadline: int64(binary.BigEndian.Uint64(raw[2:10])), payload: raw[10:]}
	if v.kind != 0 && (v.deadline != 0 || len(v.payload) != 8) {
		return tiKVValue{}, errors.New("kv: invalid tikv collection counter")
	}
	if v.deadline != 0 && v.deadline <= now.UnixNano() {
		return v, ErrNotFound
	}
	return v, nil
}

func (s *TiKV) timestamp(ctx context.Context) (uint64, error) {
	return s.client.GetTimestampWithRetry(retry.NewBackofferWithVars(ctx, 5000, nil), oracle.GlobalTxnScope)
}

func (s *TiKV) snapshot(ctx context.Context) (*txnsnapshot.KVSnapshot, error) {
	snapshot, _, err := s.snapshotWithTimestamp(ctx)
	return snapshot, err
}

func (s *TiKV) snapshotWithTimestamp(ctx context.Context) (*txnsnapshot.KVSnapshot, uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	regional := len(s.labels) > 0 && !requiresStrongReads(ctx)
	var ts uint64
	var err error
	if regional {
		floor := s.local.coveredTimestamp()
		// Use the normal cached PD version without subtracting any delay.
		// The Learner's native read protocol owns replication synchronization.
		ts, err = s.client.GetOracle().GetLowResolutionTimestamp(ctx, &oracle.Option{TxnScope: oracle.GlobalTxnScope})
		if err != nil || ts == 0 {
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			ts, err = s.timestamp(ctx)
		}
		if err == nil {
			ts = max(ts, floor)
		}
	} else {
		ts, err = s.timestamp(ctx)
	}
	if err != nil {
		return nil, 0, err
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	snapshot := s.client.GetSnapshot(ts)
	if regional {
		snapshot.SetMatchStoreLabels(s.labels)
		snapshot.SetReplicaRead(tikvkv.ReplicaReadLearner)
	}
	// client-go scanners do not accept a context. Bound each scan RPC by the
	// caller's remaining deadline and check cancellation at every iterator step.
	timeout := 5 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	if timeout <= 0 {
		return nil, 0, context.DeadlineExceeded
	}
	s.configureTiKVReadContext(ctx, snapshot, timeout, regional)
	return snapshot, ts, nil
}

func (s *TiKV) configureTiKVReadContext(ctx context.Context, snapshot *txnsnapshot.KVSnapshot, timeout time.Duration, regional bool) {
	snapshot.SetKVReadTimeout(timeout)
	snapshot.SetRPCInterceptor(interceptor.NewRPCInterceptor("gizclaw-context", func(next interceptor.RPCInterceptorFunc) interceptor.RPCInterceptorFunc {
		return func(target string, req *tikvrpc.Request) (*tikvrpc.Response, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if regional {
				if err := s.guardRegionalRead(ctx, req); err != nil {
					return nil, err
				}
			}
			return next(target, req)
		}
	}))
}

// Get reads one record using the configured consistency policy.
func (s *TiKV) Get(ctx context.Context, key Key) ([]byte, error) {
	encoded := s.recordKey(key)
	view, err := s.readView(ctx, [][]byte{encoded}, nil, false)
	if err != nil {
		return nil, tiKVError("get snapshot", err)
	}
	defer view.close()
	raw, err := view.get(ctx, encoded)
	if err != nil {
		return nil, tiKVError("get", err)
	}
	value, err := decodeTiKVValue(raw, time.Now())
	if err != nil {
		return nil, err
	}
	if value.kind != 0 {
		return nil, ErrWrongType
	}
	return bytes.Clone(value.payload), nil
}

// BatchGet merges committed local writes with one native snapshot. The SDK
// handles Region routing, request splitting and concurrency under ctx.
func (s *TiKV) BatchGet(ctx context.Context, keys []Key) []GetResult {
	results := make([]GetResult, len(keys))
	if len(keys) == 0 {
		return results
	}
	encoded := make([][]byte, len(keys))
	for i, key := range keys {
		encoded[i] = s.recordKey(key)
	}
	view, err := s.readView(ctx, encoded, nil, false)
	if err != nil {
		for i := range results {
			results[i].Err = tiKVError("batch get snapshot", err)
		}
		return results
	}
	defer view.close()
	values, readErr := view.snapshot.BatchGet(view.ctx, uniqueTiKVKeys(encoded))
	readErr = view.readError(readErr)
	if readErr == nil {
		view.observed()
	}
	now := time.Now()
	for i, key := range encoded {
		result := &results[i]
		if err := ctx.Err(); err != nil {
			result.Err = err
			continue
		}
		raw, ok := values[string(key)]
		err := readErr
		if err == nil && !ok {
			err = ErrNotFound
		}
		data, err := view.resolve(key, raw.Value, err)
		if err != nil {
			result.Err = tiKVError("batch get", err)
			continue
		}
		value, err := decodeTiKVValue(data, now)
		if err != nil {
			result.Err = err
		} else if value.kind != 0 {
			result.Err = ErrNotFound
		} else {
			result.Value = bytes.Clone(value.payload)
		}
	}
	return results
}

// update retries only definitive write conflicts, never uncertain commits.
// A false callback result discards its entire buffer without committing.
func (s *TiKV) update(ctx context.Context, fn func(*transaction.KVTxn) (bool, error)) (bool, error) {
	for attempt := range 32 {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		ts, err := s.timestamp(ctx)
		if err != nil {
			return false, tiKVError("get transaction timestamp", err)
		}
		tx, err := s.client.Begin(tikv.WithStartTS(ts))
		if err != nil {
			return false, tiKVError("begin", err)
		}
		timeout := 5 * time.Second
		if deadline, ok := ctx.Deadline(); ok {
			timeout = min(timeout, time.Until(deadline))
		}
		if timeout <= 0 {
			_ = tx.Rollback()
			return false, context.DeadlineExceeded
		}
		s.configureTiKVReadContext(ctx, tx.GetSnapshot(), timeout, false)
		matched, err := fn(tx)
		var local map[tiKVLocalKey][]byte
		if err == nil && matched && s.local != nil {
			local, err = s.bufferedLocalWrites(ctx, tx)
		}
		if err == nil && matched {
			err = tx.Commit(ctx)
			if err == nil && s.local != nil {
				s.local.publish(tx.CommitTS(), local)
			}
		} else {
			_ = tx.Rollback()
		}
		if !tikverr.IsErrWriteConflict(err) {
			return matched && err == nil, tiKVError("mutate", err)
		}
		if attempt == 31 {
			return false, tiKVError("conflict retry limit", err)
		}
		timer := time.NewTimer(min(time.Duration(attempt+1)*time.Millisecond, 20*time.Millisecond))
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
	}
	panic("unreachable")
}

func tiKVError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidDeadline) || errors.Is(err, ErrWrongType) || errors.Is(err, ErrMemberLimit) || errors.Is(err, ErrMigrationInProgress) || errors.Is(err, ErrMigrationConflict) || errors.Is(err, ErrMigrationUnsupported) || errors.Is(err, errTiKVRegionalReadTarget) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &tiKVOperationError{operation: operation, err: err}
}

type tiKVOperationError struct {
	operation string
	err       error
}

func (e *tiKVOperationError) Error() string { return "kv: tikv " + e.operation + " failed" }
func (e *tiKVOperationError) Unwrap() error { return e.err }

func (s *TiKV) getTxn(ctx context.Context, tx *transaction.KVTxn, key Key) (tiKVValue, []byte, error) {
	raw, err := tx.Get(ctx, s.recordKey(key))
	if tikverr.IsErrNotFound(err) {
		return tiKVValue{}, nil, ErrNotFound
	}
	if err != nil {
		return tiKVValue{}, nil, err
	}
	v, err := decodeTiKVValue(raw.Value, time.Now())
	return v, raw.Value, err
}

func (s *TiKV) deleteTxn(ctx context.Context, tx *transaction.KVTxn, key Key) error {
	v, raw, err := s.getTxn(ctx, tx, key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if raw != nil && v.deadline != 0 {
		if err := tx.Delete(s.expiryKey(key, v.deadline)); err != nil {
			return err
		}
	}
	if err == nil && v.kind != 0 {
		prefix := s.memberPrefix(key)
		it, err := tx.Iter(prefix, tiKVPrefixEnd(prefix))
		if err != nil {
			return err
		}
		defer it.Close()
		for it.Valid() {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := tx.Delete(bytes.Clone(it.Key())); err != nil {
				return err
			}
			if err := it.Next(); err != nil {
				return err
			}
		}
	}
	return tx.Delete(s.recordKey(key))
}

func (s *TiKV) entriesTxn(ctx context.Context, tx *transaction.KVTxn, entries []Entry, deletes []Key) error {
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		deadline := int64(0)
		if !entry.Deadline.IsZero() {
			if !entry.Deadline.After(time.Now()) {
				return ErrInvalidDeadline
			}
			deadline = entry.Deadline.UnixNano()
			if deadline <= 0 || !time.Unix(0, deadline).Equal(entry.Deadline) {
				return ErrInvalidDeadline
			}
		}
		if err := s.deleteTxn(ctx, tx, entry.Key); err != nil {
			return err
		}
		if err := tx.Set(s.recordKey(entry.Key), (tiKVValue{deadline: deadline, payload: entry.Value}).encode()); err != nil {
			return err
		}
		if deadline != 0 {
			if err := tx.Set(s.expiryKey(entry.Key, deadline), []byte{1}); err != nil {
				return err
			}
		}
	}
	for _, key := range deletes {
		if err := s.deleteTxn(ctx, tx, key); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// Set atomically replaces a value or collection and removes any deadline.
func (s *TiKV) Set(ctx context.Context, key Key, value []byte) error {
	return s.BatchSet(ctx, []Entry{{Key: key, Value: value}})
}

// Delete atomically removes a record or a collection and all its members.
func (s *TiKV) Delete(ctx context.Context, key Key) error { return s.BatchDelete(ctx, []Key{key}) }

// BatchSet atomically writes entries, including their absolute deadlines.
func (s *TiKV) BatchSet(ctx context.Context, entries []Entry) error {
	return s.BatchMutate(ctx, entries, nil)
}

// BatchDelete atomically removes records and collections.
func (s *TiKV) BatchDelete(ctx context.Context, keys []Key) error {
	return s.BatchMutate(ctx, nil, keys)
}

// BatchMutate atomically writes entries, then applies deletions.
func (s *TiKV) BatchMutate(ctx context.Context, entries []Entry, keys []Key) error {
	_, err := s.update(ctx, func(tx *transaction.KVTxn) (bool, error) { return true, s.entriesTxn(ctx, tx, entries, keys) })
	return err
}

// CreateIfAbsent atomically creates a record and its guard.
func (s *TiKV) CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) ([]byte, bool, error) {
	_, existing, created, err := s.CreateIfAllAbsent(ctx, []Entry{guard}, entries)
	return existing, created, err
}

// CreateIfAllAbsent requires all guards to be absent in one transaction.
func (s *TiKV) CreateIfAllAbsent(ctx context.Context, guards []Entry, entries []Entry) (Key, []byte, bool, error) {
	if len(guards) == 0 {
		return nil, nil, false, errors.New("kv: create-if-all-absent requires at least one guard")
	}
	var conflict Key
	var existing []byte
	created, err := s.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
		conflict, existing = nil, nil
		for _, guard := range guards {
			v, _, err := s.getTxn(ctx, tx, guard.Key)
			if err == nil {
				if v.kind != 0 {
					return false, ErrWrongType
				}
				conflict, existing = cloneKey(guard.Key), bytes.Clone(v.payload)
				return false, nil
			}
			if !errors.Is(err, ErrNotFound) {
				return false, err
			}
		}
		all := append(append([]Entry(nil), entries...), guards...)
		return true, s.entriesTxn(ctx, tx, all, nil)
	})
	return conflict, existing, created, err
}

// CompareAndMutate checks one existing record before atomically changing keys.
func (s *TiKV) CompareAndMutate(ctx context.Context, guard Key, expected []byte, entries []Entry, keys []Key) (bool, error) {
	return s.update(ctx, func(tx *transaction.KVTxn) (bool, error) {
		v, raw, err := s.getTxn(ctx, tx, guard)
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if v.kind != 0 {
			return false, ErrWrongType
		}
		if !bytes.Equal(v.payload, expected) {
			return false, nil
		}
		// SI alone does not validate a read-only guard. Make it a write key.
		if err := tx.Set(s.recordKey(guard), raw); err != nil {
			return false, err
		}
		return true, s.entriesTxn(ctx, tx, entries, keys)
	})
}

// Close leaves the borrowed physical client open.
func (s *TiKV) Close() error { return nil }

var _ Store = (*TiKV)(nil)
