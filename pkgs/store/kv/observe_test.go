package kv

import (
	"errors"
	"testing"
	"time"
)

func TestObserveReadsPreservesValuesCapabilitiesAuthorityAndAtomicViews(t *testing.T) {
	memory := NewMemory(nil)
	t.Cleanup(func() { _ = memory.Close() })
	var calls, bytes int64
	observed := ObserveReads(Prefixed(memory, Key{"scope"}), func(elapsed time.Duration, n int64) {
		calls++
		bytes += n
		if elapsed < 0 {
			t.Error("negative elapsed")
		}
	})
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
