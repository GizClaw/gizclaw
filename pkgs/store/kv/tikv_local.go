package kv

import (
	"bytes"
	"container/heap"
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	tikverr "github.com/tikv/client-go/v2/error"
	"github.com/tikv/client-go/v2/oracle"
	"github.com/tikv/client-go/v2/txnkv/transaction"
	"github.com/tikv/client-go/v2/txnkv/txnsnapshot"
)

// Local writes are immutable physical transaction postimages. Nil means a
// committed deletion. They never participate in transaction conditions.
// A reader pins its relevant postimages BEFORE obtaining its timestamp, so a
// concurrent reader cannot retire data still needed by an older in-flight read.
type tiKVLocalWrite struct {
	value []byte
	ts    uint64
}

type tiKVLocalKey struct {
	key   string
	clear bool // reset of an entire collection, including historical members
}

// tiKVLocalMemberPrefix recognizes only the existing self-describing member
// encoding. The member suffix may contain arbitrary bytes or be empty.
func tiKVLocalMemberPrefix(key tiKVLocalKey) string {
	header := len(tiKVNamespace) + 1 + 8
	if key.clear || len(key.key) < header || !strings.HasPrefix(key.key, string(tiKVNamespace)) || key.key[len(tiKVNamespace)] != 2 {
		return ""
	}
	size := binary.BigEndian.Uint64([]byte(key.key[header-8 : header]))
	if size > uint64(len(key.key)-header) {
		return ""
	}
	return key.key[:header+int(size)]
}

type tiKVLocalRange struct {
	start []byte
	end   []byte
}

type tiKVLocalExpiry struct {
	key tiKVLocalKey
	ts  uint64
}

type tiKVLocalHeap []tiKVLocalExpiry

func (h tiKVLocalHeap) Len() int           { return len(h) }
func (h tiKVLocalHeap) Less(i, j int) bool { return h[i].ts < h[j].ts }
func (h tiKVLocalHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *tiKVLocalHeap) Push(value any)    { *h = append(*h, value.(tiKVLocalExpiry)) }
func (h *tiKVLocalHeap) Pop() any {
	last := len(*h) - 1
	value := (*h)[last]
	(*h)[last] = tiKVLocalExpiry{}
	*h = (*h)[:last]
	return value
}

type tiKVLocalWrites struct {
	mu         sync.Mutex
	writes     map[tiKVLocalKey]tiKVLocalWrite
	memberKeys map[string]map[string]struct{}
	expiry     tiKVLocalHeap
	observed   uint64
	readTS     uint64
	readLag    time.Duration
}

func (c *tiKVLocalWrites) coveredTimestamp() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.observed
}

func (c *tiKVLocalWrites) readDelay() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readLag
}

// observeRead records lag at read completion without changing the MVCC frontier.
// Older concurrent snapshots cannot replace the latest snapshot's measurement.
func (c *tiKVLocalWrites) observeRead(ts uint64, at time.Time) {
	if ts == 0 {
		return
	}
	lag := max(0, at.Sub(oracle.GetTimeFromTS(ts)))
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts > c.readTS {
		c.readTS, c.readLag = ts, lag
	} else if ts == c.readTS {
		c.readLag = max(c.readLag, lag)
	}
}

func (c *tiKVLocalWrites) publish(ts uint64, writes map[tiKVLocalKey][]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts <= c.observed {
		return
	}
	if c.writes == nil {
		c.writes = make(map[tiKVLocalKey]tiKVLocalWrite)
	}
	for key, value := range writes {
		if previous, ok := c.writes[key]; ok && previous.ts >= ts {
			continue
		}
		c.writes[key] = tiKVLocalWrite{value: value, ts: ts}
		if prefix := tiKVLocalMemberPrefix(key); prefix != "" {
			if c.memberKeys == nil {
				c.memberKeys = make(map[string]map[string]struct{})
			}
			bucket := c.memberKeys[prefix]
			if bucket == nil {
				bucket = make(map[string]struct{})
				// A short collection prefix must not retain a huge member suffix.
				c.memberKeys[strings.Clone(prefix)] = bucket
			}
			bucket[key.key] = struct{}{}
		}
		heap.Push(&c.expiry, tiKVLocalExpiry{key: key, ts: ts})
	}
	// Overwrites leave obsolete scheduling entries in the heap. Bound them by
	// the current postimages even when no reader has advanced the frontier yet.
	if len(c.expiry) > 2*len(c.writes)+64 {
		c.expiry = make(tiKVLocalHeap, 0, len(c.writes))
		for key, write := range c.writes {
			c.expiry = append(c.expiry, tiKVLocalExpiry{key: key, ts: write.ts})
		}
		heap.Init(&c.expiry)
	}
}

// retire uses the monotonic historical timestamp frontier, never wall-clock age
// or byte equality. A late publisher cannot resurrect an already covered commit.
func (c *tiKVLocalWrites) retire(ts uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observed = max(c.observed, ts)
	for len(c.expiry) > 0 && c.expiry[0].ts <= c.observed {
		entry := heap.Pop(&c.expiry).(tiKVLocalExpiry)
		if current, ok := c.writes[entry.key]; ok && current.ts == entry.ts {
			delete(c.writes, entry.key)
			if prefix := tiKVLocalMemberPrefix(entry.key); prefix != "" {
				bucket := c.memberKeys[prefix]
				delete(bucket, entry.key.key)
				if len(bucket) == 0 {
					delete(c.memberKeys, prefix)
				}
			}
		}
	}
}

func (c *tiKVLocalWrites) capture(keys [][]byte, prefix []byte, scan bool, ranges ...tiKVLocalRange) map[tiKVLocalKey]tiKVLocalWrite {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result map[tiKVLocalKey]tiKVLocalWrite
	add := func(key tiKVLocalKey, value tiKVLocalWrite) {
		if result == nil {
			result = make(map[tiKVLocalKey]tiKVLocalWrite)
		}
		result[key] = value
	}
	for _, key := range keys {
		lookup := tiKVLocalKey{key: string(key)}
		if value, ok := c.writes[lookup]; ok {
			add(lookup, value)
		}
	}
	if prefix != nil {
		lookup := tiKVLocalKey{key: string(prefix), clear: true}
		if value, ok := c.writes[lookup]; ok {
			add(lookup, value)
		}
		if scan {
			var bounds tiKVLocalRange
			if len(ranges) != 0 {
				bounds = ranges[0]
			}
			for key := range c.memberKeys[string(prefix)] {
				if key < string(bounds.start) || bounds.end != nil && key >= string(bounds.end) {
					continue
				}
				lookup := tiKVLocalKey{key: key}
				if value, ok := c.writes[lookup]; ok {
					add(lookup, value)
				}
			}
		}
	}
	return result
}

func (s *TiKV) bufferedLocalWrites(ctx context.Context, tx *transaction.KVTxn) (map[tiKVLocalKey][]byte, error) {
	it, err := tx.GetMemBuffer().Iter(nil, nil)
	if err != nil {
		return nil, err
	}
	defer it.Close()
	writes := make(map[tiKVLocalKey][]byte)
	for it.Valid() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key, value := it.Key(), it.Value()
		// Internal expiry discovery retains its existing bounded recovery path.
		if bytes.HasPrefix(key, tiKVNamespace) && len(key) > len(tiKVNamespace) {
			switch key[len(tiKVNamespace)] {
			case 1, 2:
				writes[tiKVLocalKey{key: string(key)}] = bytes.Clone(value)
			}
			if key[len(tiKVNamespace)] == 1 {
				reset := len(value) == 0 || (len(value) >= 2 && value[1] == 0)
				if !reset {
					// A newly created collection must also hide members from an
					// older incarnation that only the historical reader still sees.
					previous, err := tx.GetSnapshot().Get(ctx, key)
					if tikverr.IsErrNotFound(err) {
						reset = true
					} else if err != nil {
						return nil, err
					} else {
						_, err = decodeTiKVValue(previous.Value, time.Now())
						if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrMigrationInProgress) {
							return nil, err
						}
						// Import claims protect members written by earlier chunks;
						// completing a claim must not reset that collection again.
						reset = errors.Is(err, ErrNotFound)
					}
				}
				if reset {
					logical := s.opts.decode(key[len(tiKVNamespace)+1:])
					writes[tiKVLocalKey{key: string(s.memberPrefix(logical)), clear: true}] = nil
				}
			}
		}
		if err := it.Next(); err != nil {
			return nil, err
		}
	}
	return writes, nil
}

type tiKVReadView struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	snapshot *txnsnapshot.KVSnapshot
	ts       uint64
	cache    *tiKVLocalWrites
	writes   map[tiKVLocalKey]tiKVLocalWrite
	prefix   string
}

func (s *TiKV) readView(ctx context.Context, keys [][]byte, prefix []byte, scan bool, ranges ...tiKVLocalRange) (*tiKVReadView, error) {
	view := &tiKVReadView{ctx: ctx, prefix: string(prefix)}
	if s.local != nil && !requiresStrongReads(ctx) {
		view.cache = s.local
		view.writes = s.local.capture(keys, prefix, scan, ranges...)
		readCtx, cancel := context.WithCancelCause(ctx)
		view.ctx = context.WithValue(context.WithValue(readCtx, tiKVReadCancelKey{}, cancel), tiKVReadStoreKey{}, s)
		view.cancel = cancel
	}
	var err error
	view.snapshot, view.ts, err = s.snapshotWithTimestamp(view.ctx)
	if err != nil {
		view.close()
	}
	return view, err
}

func (v *tiKVReadView) close() {
	if v.cancel != nil {
		v.cancel(nil)
	}
}

func (v *tiKVReadView) readError(err error) error {
	if err != nil && v.ctx != nil && errors.Is(context.Cause(v.ctx), errTiKVRegionalReadTarget) {
		return errTiKVRegionalReadTarget
	}
	return err
}

func (v *tiKVReadView) observed() {
	if v.cache != nil {
		at := time.Now()
		v.cache.retire(v.ts)
		v.cache.observeRead(v.ts, at)
	}
}

func (v *tiKVReadView) cleared() uint64 {
	clear := v.writes[tiKVLocalKey{key: v.prefix, clear: true}]
	if clear.ts > v.ts {
		return clear.ts
	}
	return 0
}

func (v *tiKVReadView) resolve(key []byte, value []byte, err error) ([]byte, error) {
	local, ok := v.writes[tiKVLocalKey{key: string(key)}]
	clearTS := uint64(0)
	if v.prefix != "" && bytes.HasPrefix(key, []byte(v.prefix)) {
		clearTS = v.cleared()
	}
	if ok && local.ts > v.ts && local.ts >= clearTS {
		if len(local.value) == 0 {
			return nil, ErrNotFound
		}
		return local.value, nil
	}
	if clearTS != 0 {
		return nil, ErrNotFound
	}
	if tikverr.IsErrNotFound(err) {
		return nil, ErrNotFound
	}
	return value, err
}

func (v *tiKVReadView) get(ctx context.Context, key []byte) ([]byte, error) {
	raw, err := v.snapshot.Get(v.ctx, key)
	err = v.readError(err)
	if err == nil || tikverr.IsErrNotFound(err) {
		v.observed()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return v.resolve(key, raw.Value, err)
}

// Only the methods needed for a merge are named here; SDK iterators and the
// deterministic merge tests both satisfy this interface.
type tiKVKeyIterator interface {
	Valid() bool
	Key() []byte
	Next() error
}

func (v *tiKVReadView) rangeMembers(ctx context.Context, start, end []byte, limit int) ([]string, error) {
	var it tiKVKeyIterator
	if v.cleared() == 0 {
		remote, err := v.snapshot.Iter(start, end)
		if err != nil {
			return nil, v.readError(err)
		}
		defer remote.Close()
		it = remote
	}
	members, err := v.mergeMembers(ctx, it, start, end, limit)
	if err == nil && it != nil {
		v.observed()
	}
	return members, v.readError(err)
}

func (v *tiKVReadView) mergeMembers(ctx context.Context, it tiKVKeyIterator, start, end []byte, limit int) ([]string, error) {
	var local []string
	clearTS := v.cleared()
	if clearTS != 0 {
		it = nil
	}
	for key, write := range v.writes {
		if !key.clear && write.ts > v.ts && write.ts >= clearTS && key.key >= string(start) && (end == nil || key.key < string(end)) {
			local = append(local, key.key)
		}
	}
	slices.Sort(local)
	var result []string
	for i := 0; i < len(local) || (it != nil && it.Valid()); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		useLocal := i < len(local) && (it == nil || !it.Valid() || local[i] <= string(it.Key()))
		advanceRemote := !useLocal || (it != nil && it.Valid() && local[i] == string(it.Key()))
		if useLocal {
			key := local[i]
			if len(v.writes[tiKVLocalKey{key: key}].value) != 0 {
				result = append(result, strings.Clone(key[len(v.prefix):]))
			}
			i++
		} else {
			result = append(result, string(it.Key()[len(v.prefix):]))
		}
		if limit > 0 && len(result) == limit {
			break
		}
		if advanceRemote {
			if err := it.Next(); err != nil {
				return nil, err
			}
		}
	}
	return result, ctx.Err()
}
