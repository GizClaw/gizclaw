package kv

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestBatchGetContract(t *testing.T) {
	factories := map[string]func(*testing.T) Store{
		"memory": func(t *testing.T) Store { return NewMemory(nil) },
		"sqlite": func(t *testing.T) Store { return newSQLiteStore(t) },
		"badger": func(t *testing.T) Store {
			store, err := NewBadgerInMemory(nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			return store
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			store := WithStrongReads(Prefixed(factory(t), Key{"tenant", "nested"}))
			if err := store.BatchSet(t.Context(), []Entry{{Key: Key{"record"}, Value: []byte("value")}, {Key: Key{"empty"}, Value: []byte{}}}); err != nil {
				t.Fatal(err)
			}
			if err := store.AddMembers(t.Context(), Key{"set"}, "member"); err != nil {
				t.Fatal(err)
			}
			keys := []Key{{"record"}, {"missing"}, {"empty"}, {"record"}, {"set"}}
			results := BatchGet(t.Context(), store, keys)
			if len(results) != len(keys) || string(results[0].Value) != "value" || results[0].Err != nil || !errors.Is(results[1].Err, ErrNotFound) || results[2].Err != nil || len(results[2].Value) != 0 || string(results[3].Value) != "value" || !errors.Is(results[4].Err, ErrWrongType) {
				t.Fatalf("mixed batch = %+v", results)
			}
			results[0].Value[0] = 'X'
			if string(results[3].Value) != "value" {
				t.Fatal("duplicate result values share mutable storage")
			}
			value, err := store.Get(t.Context(), Key{"record"})
			if err != nil || string(value) != "value" {
				t.Fatalf("result mutation changed stored value: %q, %v", value, err)
			}
			if results := BatchGet(t.Context(), store, nil); len(results) != 0 {
				t.Fatalf("empty batch = %+v", results)
			}
		})
	}
}

type batchReadProbe struct {
	Store
	keys   []Key
	strong bool
}

func (s *batchReadProbe) BatchGet(ctx context.Context, keys []Key) []GetResult {
	s.keys, s.strong = keys, requiresStrongReads(ctx)
	return make([]GetResult, len(keys))
}

func TestBatchGetPreservesPrefixAndConsistency(t *testing.T) {
	probe := &batchReadProbe{Store: NewMemory(nil)}
	store := Prefixed(WithStrongReads(Prefixed(probe, Key{"outer"})), Key{"inner"})
	key := Key{"record"}
	BatchGet(t.Context(), store, []Key{key})
	if !probe.strong || len(probe.keys) != 1 || !slices.Equal(probe.keys[0], Key{"outer", "inner", "record"}) || !slices.Equal(key, Key{"record"}) {
		t.Fatalf("batch view lost scope or consistency: %+v, strong=%v, key=%v", probe.keys, probe.strong, key)
	}
}

type batchGetFunc struct {
	Store
	get func(context.Context, Key) ([]byte, error)
}

func (s batchGetFunc) Get(ctx context.Context, key Key) ([]byte, error) { return s.get(ctx, key) }

func TestBatchGetFallbackBoundsReadsAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		store := batchGetFunc{get: func(ctx context.Context, _ Key) ([]byte, error) {
			calls.Add(1)
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		started := time.Now()
		results := BatchGet(ctx, store, make([]Key, 10))
		if time.Since(started) != 50*time.Millisecond || calls.Load() != batchGetConcurrency {
			t.Fatalf("elapsed=%s provider calls=%d", time.Since(started), calls.Load())
		}
		for _, result := range results {
			if !errors.Is(result.Err, context.DeadlineExceeded) {
				t.Fatalf("canceled result = %+v", result)
			}
		}
	})
}

func TestBatchGetExpiredRecord(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := NewMemory(nil)
		if err := store.BatchSet(t.Context(), []Entry{{Key: Key{"expired"}, Value: []byte("old"), Deadline: time.Now().Add(time.Second)}}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		if result := BatchGet(t.Context(), store, []Key{{"expired"}})[0]; !errors.Is(result.Err, ErrNotFound) {
			t.Fatalf("expired result = %+v", result)
		}
	})
}
