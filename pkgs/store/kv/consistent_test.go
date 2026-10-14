package kv

import (
	"context"
	"errors"
	"testing"
)

type consistencyProbe struct {
	Store
	observed []bool
}

func (p *consistencyProbe) Get(ctx context.Context, key Key) ([]byte, error) {
	p.observed = append(p.observed, requiresStrongReads(ctx))
	return p.Store.Get(ctx, key)
}

func TestStrongReadsPreserveAtomicScopeAndCapabilities(t *testing.T) {
	base := NewMemory(nil)
	probe := &consistencyProbe{Store: base}
	// The probe deliberately exposes only Store, so it must not gain optional
	// conditional-create capabilities just by being wrapped.
	if SupportsCreateIfAbsent(WithStrongReads(probe)) {
		t.Fatal("invented conditional-create capability")
	}
	strong := WithStrongReads(Prefixed(probe, Key{"tenant"}))
	other := Prefixed(probe, Key{"other"})
	root, prefixes, ok := SharedAtomicStore(strong, other)
	if !ok || len(prefixes) != 2 || prefixes[0].String() != "tenant" || prefixes[1].String() != "other" {
		t.Fatalf("atomic scope=%v %v", prefixes, ok)
	}
	if err := strong.Set(t.Context(), Key{"key"}, []byte("value")); err != nil {
		t.Fatal(err)
	}
	if _, err := strong.Get(t.Context(), Key{"key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Get(t.Context(), Key{"tenant", "key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.Get(t.Context(), Key{"tenant", "key"}); err != nil {
		t.Fatal(err)
	}
	if len(probe.observed) != 3 || !probe.observed[0] || !probe.observed[1] || probe.observed[2] {
		t.Fatalf("read consistency=%v", probe.observed)
	}
	if err := strong.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Get(t.Context(), Key{"tenant", "key"}); err != nil {
		t.Fatal("strong view closed base")
	}
	if _, _, _, err := CreateIfAllAbsent(t.Context(), strong, []Entry{{Key: Key{"guard"}}}, nil); !errors.Is(err, ErrCreateIfAbsentUnsupported) {
		t.Fatalf("unsupported capability=%v", err)
	}
	memoryStrong := WithStrongReads(Prefixed(base, Key{"fresh"}))
	if !SupportsCreateIfAllAbsent(memoryStrong) || !SupportsCompareAndMutate(memoryStrong) {
		t.Fatal("lost conditional capabilities")
	}
}
