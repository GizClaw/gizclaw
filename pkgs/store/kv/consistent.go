package kv

import (
	"context"
	"time"
)

type strongReadsKey struct{}

// ReadDelay returns the snapshot lag measured when the latest ordinary regional
// TiKV snapshot was successfully read. The measurement does not grow between
// reads and is not a configured delay or maximum staleness bound. Strong views,
// nonregional stores and stores without a successful observation return zero.
// Lease owners can account for measured reader lag in their validity window.
func ReadDelay(store Store) time.Duration {
	switch s := store.(type) {
	case *strongReadStore:
		return 0
	case *prefixedStore:
		return ReadDelay(s.base)
	case *TiKV:
		if s.local != nil {
			return s.local.readDelay()
		}
		return 0
	default:
		return 0
	}
}

// WithPrimaryReads selects the write authority for fallback reads after a
// rejected condition or an unavailable replica. Ordinary reads should use the
// caller's context unchanged. Redis uses the writer; TiKV bypasses stale reads.
// This does not provide a multi-operation snapshot.
func WithPrimaryReads(ctx context.Context) context.Context {
	return strongReadContext(ctx)
}

// WithStrongReads returns a borrowed view whose record and collection reads
// observe committed state at the start of each operation. Redis read/write
// stores use the writer; TiKV bypasses configured stale Learner reads. It does
// not turn multiple operations into a snapshot. Errors never fall back to a
// stale replica. Close does not close the underlying store.
func WithStrongReads(store Store) Store {
	if store == nil {
		return nil
	}
	if _, ok := store.(*strongReadStore); ok {
		return store
	}
	return &strongReadStore{Store: store}
}

type strongReadStore struct{ Store }

func strongReadContext(ctx context.Context) context.Context {
	if requiresStrongReads(ctx) {
		return ctx
	}
	return context.WithValue(ctx, strongReadsKey{}, true)
}

func requiresStrongReads(ctx context.Context) bool {
	strong, _ := ctx.Value(strongReadsKey{}).(bool)
	return strong
}

func (s *strongReadStore) Get(ctx context.Context, key Key) ([]byte, error) {
	return s.Store.Get(strongReadContext(ctx), key)
}

func (s *strongReadStore) BatchGet(ctx context.Context, keys []Key) []GetResult {
	return s.Store.BatchGet(strongReadContext(ctx), keys)
}

func (s *strongReadStore) HasMember(ctx context.Context, key Key, member string) (bool, error) {
	return s.Store.HasMember(strongReadContext(ctx), key, member)
}

func (s *strongReadStore) ListMembers(ctx context.Context, key Key) ([]string, error) {
	return s.Store.ListMembers(strongReadContext(ctx), key)
}

func (s *strongReadStore) RangeOrderedMembers(ctx context.Context, key Key, query OrderedRange) ([]string, error) {
	return s.Store.RangeOrderedMembers(strongReadContext(ctx), key, query)
}

func (s *strongReadStore) CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) ([]byte, bool, error) {
	return CreateIfAbsent(ctx, s.Store, guard, entries)
}

func (s *strongReadStore) CreateIfAllAbsent(ctx context.Context, guards []Entry, entries []Entry) (Key, []byte, bool, error) {
	return CreateIfAllAbsent(ctx, s.Store, guards, entries)
}

func (s *strongReadStore) CompareAndMutate(ctx context.Context, guard Key, expected []byte, entries []Entry, keys []Key) (bool, error) {
	return CompareAndMutate(ctx, s.Store, guard, expected, entries, keys)
}

func (*strongReadStore) Close() error { return nil }
