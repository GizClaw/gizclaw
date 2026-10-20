package kv

import (
	"errors"
	"testing"
	"time"
)

type readObservationFunc func(time.Duration, int64)

func (f readObservationFunc) Begin() (time.Time, bool)           { return time.Now(), true }
func (f readObservationFunc) End(started time.Time, bytes int64) { f(time.Since(started), bytes) }

func TestObserveReadsPreservesValuesCapabilitiesAuthorityAndAtomicViews(t *testing.T) {
	memory := NewMemory(nil)
	t.Cleanup(func() { _ = memory.Close() })
	var calls, bytes int64
	observed := ObserveReads(Prefixed(memory, Key{"scope"}), readObservationFunc(func(elapsed time.Duration, n int64) {
		calls++
		bytes += n
		if elapsed < 0 {
			t.Error("negative elapsed")
		}
	}))
	if err := observed.Set(t.Context(), Key{"a"}, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	value, err := observed.Get(t.Context(), Key{"a"})
	if err != nil || string(value) != "abc" {
		t.Fatal("read behavior changed")
	}
	results := WithStrongReads(observed).BatchGet(t.Context(), []Key{{"a"}, {"missing"}, {"a"}})
	if len(results) != 3 || !errors.Is(results[1].Err, ErrNotFound) || calls != 2 || bytes != 9 {
		t.Fatalf("batch double counting or bytes: calls=%d bytes=%d results=%+v", calls, bytes, results)
	}
	if !SupportsCreateIfAbsent(observed) || !SupportsCreateIfAllAbsent(observed) || !SupportsCompareAndMutate(observed) {
		t.Fatal("optional capability lost")
	}
	_, created, err := CreateIfAbsent(t.Context(), observed, Entry{Key: Key{"guard"}, Value: []byte("yes")}, nil)
	if err != nil || !created {
		t.Fatalf("conditional create: %v", err)
	}
	matched, err := CompareAndMutate(t.Context(), observed, Key{"guard"}, []byte("yes"), []Entry{{Key: Key{"a"}, Value: []byte("new")}}, nil)
	if err != nil || !matched {
		t.Fatalf("conditional mutation: %v", err)
	}
	_, _, created, err = CreateIfAllAbsent(t.Context(), observed, []Entry{{Key: Key{"second"}, Value: []byte("new")}}, nil)
	if err != nil || !created {
		t.Fatalf("multi-guard create: %v", err)
	}
	root, prefixes, ok := SharedAtomicStore(WithStrongReads(observed), Prefixed(memory, Key{"other"}))
	if !ok || root == nil || len(prefixes) != 2 || prefixes[0].String() != "scope" {
		t.Fatal("shared transaction boundary changed")
	}
	if calls != 2 {
		t.Fatal("non-read operations were counted")
	}
}

func TestReadObserverPreservesHistoricalDelayAndStrongViews(t *testing.T) {
	backend := &TiKV{read: TiKVReadOptions{StaleReadDelay: time.Minute}}
	observe := readObservationFunc(func(time.Duration, int64) {})
	for _, view := range []Store{
		ObserveReads(Prefixed(backend, Key{"scope"}), observe),
		Prefixed(ObserveReads(backend, observe), Key{"scope"}),
	} {
		if delay := ReadDelay(view); delay != time.Minute {
			t.Fatalf("observer hid historical delay: %v", delay)
		}
	}
	for _, view := range []Store{
		ObserveReads(WithStrongReads(backend), observe),
		WithStrongReads(ObserveReads(backend, observe)),
	} {
		if delay := ReadDelay(view); delay != 0 {
			t.Fatalf("strong observer view has delay: %v", delay)
		}
	}
	base := NewMemory(nil)
	t.Cleanup(func() { _ = base.Close() })
	probe := &consistencyProbe{Store: base}
	observed := ObserveReads(probe, observe)
	if SupportsCreateIfAbsent(observed) || SupportsCreateIfAllAbsent(observed) || SupportsCompareAndMutate(observed) {
		t.Fatal("observer invented optional capabilities")
	}
	_, _ = WithStrongReads(observed).Get(t.Context(), Key{"missing"})
	if len(probe.observed) != 1 || !probe.observed[0] {
		t.Fatal("observer lost primary-read authority")
	}
}

func TestReadObserverSkipsLargeBatchInspection(t *testing.T) {
	base := NewMemory(nil)
	defer base.Close()
	key := Key{"record"}
	if err := base.Set(t.Context(), key, []byte("value")); err != nil {
		t.Fatal(err)
	}
	completed := 0
	store := ObserveReads(base, readObservationFunc(func(_ time.Duration, bytes int64) {
		completed++
		if bytes != 0 {
			t.Error("large batch values traversed for byte accounting")
		}
	}))
	keys := make([]Key, maxObservedBatchValues+1)
	for i := range keys {
		keys[i] = key
	}
	results := store.BatchGet(t.Context(), keys)
	if len(results) != len(keys) || completed != 1 {
		t.Fatal("large batch was traversed for telemetry")
	}
	for _, result := range results {
		if result.Err != nil || string(result.Value) != "value" {
			t.Fatal("business results changed")
		}
	}
}
