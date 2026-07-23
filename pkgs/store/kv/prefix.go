package kv

import (
	"context"
	"fmt"
	"iter"
	"reflect"
)

// Prefixed returns a Store view that scopes all keys under prefix.
//
// The returned store does not own the underlying store. Close is intentionally
// a no-op so multiple prefixed views can share the same base store lifecycle.
func Prefixed(base Store, prefix Key) Store {
	return &prefixedStore{
		base:   base,
		prefix: cloneKey(prefix),
	}
}

// SharedAtomicStore resolves stores to their common transaction boundary and
// returns the prefix of each logical view relative to that boundary.
//
// The boolean is false when a store is nil, when the stores use different
// transaction boundaries, or when their concrete roots cannot be compared
// safely. Callers can then reject configurations that cannot provide one
// atomic BatchMutate across all of the logical views.
func SharedAtomicStore(stores ...Store) (Store, []Key, bool) {
	if len(stores) == 0 {
		return nil, nil, false
	}
	roots := make([]Store, len(stores))
	prefixes := make([]Key, len(stores))
	for i, store := range stores {
		if store == nil {
			return nil, nil, false
		}
		roots[i], prefixes[i] = atomicStoreView(store)
	}
	for _, root := range roots[1:] {
		if !sameStore(roots[0], root) {
			return nil, nil, false
		}
	}
	return roots[0], prefixes, true
}

func atomicStoreView(store Store) (Store, Key) {
	var prefix Key
	for {
		view, ok := store.(*prefixedStore)
		if !ok {
			return store, prefix
		}
		prefix = append(cloneKey(view.prefix), prefix...)
		store = view.base
	}
}

func sameStore(a, b Store) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	aValue := reflect.ValueOf(a)
	bValue := reflect.ValueOf(b)
	if aValue.Type() != bValue.Type() || !aValue.Type().Comparable() {
		return false
	}
	return aValue.Interface() == bValue.Interface()
}

type prefixedStore struct {
	base   Store
	prefix Key
}

func (s *prefixedStore) Get(ctx context.Context, key Key) ([]byte, error) {
	return s.base.Get(ctx, s.prefixedKey(key))
}

func (s *prefixedStore) Set(ctx context.Context, key Key, value []byte) error {
	return s.base.Set(ctx, s.prefixedKey(key), value)
}

func (s *prefixedStore) Delete(ctx context.Context, key Key) error {
	return s.base.Delete(ctx, s.prefixedKey(key))
}

func (s *prefixedStore) List(ctx context.Context, prefix Key) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		for entry, err := range s.base.List(ctx, s.prefixedKey(prefix)) {
			if err != nil {
				if !yield(Entry{}, err) {
					return
				}
				continue
			}
			localKey, err := s.localKey(entry.Key)
			if err != nil {
				if !yield(Entry{}, err) {
					return
				}
				continue
			}
			entry.Key = localKey
			if !yield(entry, nil) {
				return
			}
		}
	}
}

func (s *prefixedStore) ListAfter(ctx context.Context, prefix, after Key, limit int) ([]Entry, error) {
	globalAfter := Key(nil)
	if len(after) > 0 {
		globalAfter = s.prefixedKey(after)
	}
	entries, err := ListAfter(ctx, s.base, s.prefixedKey(prefix), globalAfter, limit)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		localKey, err := s.localKey(entries[i].Key)
		if err != nil {
			return nil, err
		}
		entries[i].Key = localKey
	}
	return entries, nil
}

func (s *prefixedStore) BatchSet(ctx context.Context, entries []Entry) error {
	prefixed := make([]Entry, len(entries))
	for i, entry := range entries {
		prefixed[i] = Entry{
			Key:      s.prefixedKey(entry.Key),
			Value:    entry.Value,
			Deadline: entry.Deadline,
		}
	}
	return s.base.BatchSet(ctx, prefixed)
}

func (s *prefixedStore) BatchDelete(ctx context.Context, keys []Key) error {
	prefixed := make([]Key, len(keys))
	for i, key := range keys {
		prefixed[i] = s.prefixedKey(key)
	}
	return s.base.BatchDelete(ctx, prefixed)
}

func (s *prefixedStore) BatchMutate(ctx context.Context, entries []Entry, keys []Key) error {
	prefixedEntries := make([]Entry, len(entries))
	for i, entry := range entries {
		prefixedEntries[i] = Entry{
			Key:      s.prefixedKey(entry.Key),
			Value:    entry.Value,
			Deadline: entry.Deadline,
		}
	}
	prefixedKeys := make([]Key, len(keys))
	for i, key := range keys {
		prefixedKeys[i] = s.prefixedKey(key)
	}
	return s.base.BatchMutate(ctx, prefixedEntries, prefixedKeys)
}

func (s *prefixedStore) CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) ([]byte, bool, error) {
	prefixedEntries := make([]Entry, len(entries))
	for i, entry := range entries {
		prefixedEntries[i] = Entry{Key: s.prefixedKey(entry.Key), Value: entry.Value, Deadline: entry.Deadline}
	}
	guard.Key = s.prefixedKey(guard.Key)
	return CreateIfAbsent(ctx, s.base, guard, prefixedEntries)
}

func (s *prefixedStore) Close() error {
	return nil
}

func (s *prefixedStore) prefixedKey(key Key) Key {
	out := make(Key, len(s.prefix))
	copy(out, s.prefix)
	out = append(out, key...)
	return out
}

func (s *prefixedStore) localKey(key Key) (Key, error) {
	if !hasKeyPrefix(key, s.prefix) {
		return nil, fmt.Errorf("kv: prefixed store got key %v outside prefix %v", key, s.prefix)
	}
	return cloneKey(key[len(s.prefix):]), nil
}

func cloneKey(key Key) Key {
	if len(key) == 0 {
		return nil
	}
	return append(Key(nil), key...)
}

func hasKeyPrefix(key, prefix Key) bool {
	if len(key) < len(prefix) {
		return false
	}
	for i, segment := range prefix {
		if key[i] != segment {
			return false
		}
	}
	return true
}

var _ Store = (*prefixedStore)(nil)
