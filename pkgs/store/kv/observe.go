package kv

import (
	"context"
	"time"
)

const maxObservedBatchValues = 64

// ReadObserver selects best-effort samples. Begin must return immediately and
// End must not wait, perform I/O, or retain keys/values. Large batches report zero
// observed bytes rather than traversing unbounded results. A false Begin bypasses
// clocks, result traversal and completion callbacks in the borrowed Store view.
type ReadObserver interface {
	Begin() (time.Time, bool)
	End(time.Time, int64)
}

// ObserveReads borrows a Store and samples Get/BatchGet without changing their
// results, authority, optional capabilities or ownership. BatchGet is one sample.
func ObserveReads(store Store, observer ReadObserver) Store {
	if observer == nil {
		return store
	}
	return &readObserver{Store: store, observe: observer}
}

type readObserver struct {
	Store
	observe ReadObserver
}

func (s *readObserver) Get(ctx context.Context, key Key) ([]byte, error) {
	started, sampled := s.observe.Begin()
	if !sampled {
		return s.Store.Get(ctx, key)
	}
	value, err := s.Store.Get(ctx, key)
	s.observe.End(started, int64(len(value)))
	return value, err
}

func (s *readObserver) BatchGet(ctx context.Context, keys []Key) []GetResult {
	started, sampled := s.observe.Begin()
	if !sampled {
		return s.Store.BatchGet(ctx, keys)
	}
	values := s.Store.BatchGet(ctx, keys)
	if len(values) > maxObservedBatchValues {
		s.observe.End(started, 0)
		return values
	}
	var bytes int64
	for _, value := range values {
		bytes += int64(len(value.Value))
	}
	s.observe.End(started, bytes)
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
