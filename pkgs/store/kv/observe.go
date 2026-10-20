package kv

import (
	"context"
	"time"
)

// ObserveReads borrows a Store and observes completed Get and BatchGet calls.
// The callback receives elapsed time and returned value bytes (not keys or
// network overhead). BatchGet is one call. Missing/failed reads count as attempts.
// The callback must be bounded and nonblocking. No values or keys are retained.
// All other operations preserve the underlying Store's behavior and ownership.
func ObserveReads(store Store, observe func(time.Duration, int64)) Store {
	if observe == nil {
		return store
	}
	return &readObserver{Store: store, observe: observe}
}

type readObserver struct {
	Store
	observe func(time.Duration, int64)
}

func (s *readObserver) Get(ctx context.Context, key Key) ([]byte, error) {
	started := time.Now()
	value, err := s.Store.Get(ctx, key)
	s.observe(time.Since(started), int64(len(value)))
	return value, err
}

func (s *readObserver) BatchGet(ctx context.Context, keys []Key) []GetResult {
	started := time.Now()
	values := s.Store.BatchGet(ctx, keys)
	var bytes int64
	for _, value := range values {
		bytes += int64(len(value.Value))
	}
	s.observe(time.Since(started), bytes)
	return values
}

func (s *readObserver) CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) ([]byte, bool, error) {
	return CreateIfAbsent(ctx, s.Store, guard, entries)
}
func (s *readObserver) CreateIfAllAbsent(ctx context.Context, guards []Entry, entries []Entry) (Key, []byte, bool, error) {
	return CreateIfAllAbsent(ctx, s.Store, guards, entries)
}
func (s *readObserver) CompareAndMutate(ctx context.Context, guard Key, expected []byte, entries []Entry, keys []Key) (bool, error) {
	return CompareAndMutate(ctx, s.Store, guard, expected, entries, keys)
}
