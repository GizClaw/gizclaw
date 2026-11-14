package kv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tikv/client-go/v2/oracle"
)

func TestReadDelayFollowsStoreViews(t *testing.T) {
	tikv := &TiKV{local: new(tiKVLocalWrites)}
	at := time.Now().Truncate(time.Millisecond)
	tikv.local.observeRead(oracle.GoTimeToTS(at.Add(-time.Minute)), at)
	if delay := ReadDelay(Prefixed(tikv, Key{"peers", "presence"})); delay != time.Minute {
		t.Fatalf("prefixed delay = %v", delay)
	}
	if ReadDelay(Prefixed(WithStrongReads(tikv), Key{"peers"})) != 0 || ReadDelay(NewMemory(nil)) != 0 || ReadDelay(&TiKV{}) != 0 || ReadDelay(&TiKV{local: new(tiKVLocalWrites)}) != 0 {
		t.Fatal("strong, nonregional or unobserved store has a read delay")
	}
}

func TestReadDelayDoesNotAccumulateWithoutReads(t *testing.T) {
	tikv := &TiKV{local: new(tiKVLocalWrites)}
	// Model a successful read a day ago, with two seconds of lag at completion.
	// Neither idle time nor subsequent writes are another lag observation.
	at := time.Now().Add(-24 * time.Hour).Truncate(time.Millisecond)
	ts := oracle.GoTimeToTS(at.Add(-2 * time.Second))
	tikv.local.retire(ts)
	tikv.local.observeRead(ts, at)
	for i := range 3 {
		commitTS := oracle.GoTimeToTS(time.Now()) + uint64(i)
		tikv.local.publish(commitTS, map[tiKVLocalKey][]byte{{key: "lease"}: []byte("renewed")})
		if delay := ReadDelay(tikv); delay != 2*time.Second {
			t.Fatalf("writer-only delay = %v", delay)
		}
		if covered := tikv.local.coveredTimestamp(); covered != ts {
			t.Fatalf("lag measurement changed coverage: %d, want %d", covered, ts)
		}
	}
}

func TestReadDelayOrdersConcurrentSnapshotObservations(t *testing.T) {
	tikv := &TiKV{local: new(tiKVLocalWrites)}
	at := time.Now().Truncate(time.Millisecond)
	ts := oracle.GoTimeToTS(at.Add(-2 * time.Second))
	tikv.local.observeRead(ts, at)
	// An older snapshot can complete later without replacing the latest sample.
	tikv.local.observeRead(oracle.GoTimeToTS(at.Add(-time.Minute)), at.Add(time.Minute))
	tikv.local.observeRead(0, at.Add(time.Hour))
	if delay := ReadDelay(tikv); delay != 2*time.Second {
		t.Fatalf("older or zero snapshot changed delay: %v", delay)
	}
	// Repeated reads at one timestamp use the later completion measurement,
	// even when the calls acquire the observation mutex out of order.
	tikv.local.observeRead(ts, at.Add(time.Second))
	tikv.local.observeRead(ts, at)
	if delay := ReadDelay(tikv); delay != 3*time.Second {
		t.Fatalf("same-timestamp delay = %v", delay)
	}
	tikv.local.observeRead(oracle.GoTimeToTS(at), at.Add(time.Millisecond))
	if delay := ReadDelay(tikv); delay != time.Millisecond {
		t.Fatalf("new snapshot did not refresh delay: %v", delay)
	}
	tikv.local.observeRead(oracle.GoTimeToTS(at.Add(time.Hour)), at)
	if delay := ReadDelay(tikv); delay != 0 {
		t.Fatalf("future snapshot delay = %v", delay)
	}
	if covered := tikv.local.coveredTimestamp(); covered != 0 {
		t.Fatalf("lag observation advanced MVCC coverage: %d", covered)
	}
}

func TestReadDelayRecordsSuccessfulReadView(t *testing.T) {
	tikv := &TiKV{local: new(tiKVLocalWrites)}
	ts := oracle.GoTimeToTS(time.Now().Add(-time.Minute))
	view := &tiKVReadView{cache: tikv.local, ts: ts}
	view.observed()
	if delay := ReadDelay(tikv); delay < time.Minute || delay > time.Minute+time.Second {
		t.Fatalf("read-view delay = %v", delay)
	}
	if covered := tikv.local.coveredTimestamp(); covered != ts {
		t.Fatalf("read-view coverage = %d, want %d", covered, ts)
	}
}

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
