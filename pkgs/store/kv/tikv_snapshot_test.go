package kv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tikv/client-go/v2/oracle"
	"github.com/tikv/client-go/v2/tikv"
)

// A zero KVStore can create snapshots without network I/O. Unimplemented oracle
// methods panic, so accidentally using an extrapolated timestamp also fails.
type tiKVSnapshotOracle struct {
	oracle.Oracle
	cachedTS    uint64
	freshTS     uint64
	cachedErr   error
	cachedCalls int
	freshCalls  int
	afterRead   func()
}

func (o *tiKVSnapshotOracle) GetLowResolutionTimestamp(ctx context.Context, opt *oracle.Option) (uint64, error) {
	o.cachedCalls++
	if opt.TxnScope != oracle.GlobalTxnScope {
		return 0, errors.New("unexpected cached timestamp scope")
	}
	if o.afterRead != nil {
		o.afterRead()
	}
	return o.cachedTS, o.cachedErr
}

func (o *tiKVSnapshotOracle) GetTimestamp(ctx context.Context, opt *oracle.Option) (uint64, error) {
	o.freshCalls++
	if opt.TxnScope != oracle.GlobalTxnScope {
		return 0, errors.New("unexpected fresh timestamp scope")
	}
	if o.afterRead != nil {
		o.afterRead()
	}
	return o.freshTS, nil
}

func newTiKVSnapshotTestStore(t *testing.T, clock *tiKVSnapshotOracle, stale bool) *TiKV {
	t.Helper()
	client := new(tikv.KVStore)
	client.SetOracle(clock)
	var read TiKVReadOptions
	if stale {
		read = TiKVReadOptions{StaleReadDelay: 1501 * time.Millisecond, Labels: map[string]string{"region": "local"}}
	}
	store, err := NewTiKVWithClient(client, read, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestTiKVSnapshotOracle(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stale   bool
		primary bool
	}{
		{name: "cached-stale", stale: true},
		{name: "explicit-primary", stale: true, primary: true},
		{name: "strong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := &tiKVSnapshotOracle{
				cachedTS: oracle.GoTimeToTS(time.Now().Add(-time.Second)),
				freshTS:  oracle.GoTimeToTS(time.Now()),
			}
			store := newTiKVSnapshotTestStore(t, clock, tc.stale)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if tc.primary {
				ctx = WithPrimaryReads(ctx)
			}
			for range 2 {
				snapshot, err := store.snapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if timeout := snapshot.GetKVReadTimeout(); timeout <= 0 || timeout > time.Second {
					t.Fatalf("snapshot timeout=%v, want caller's remaining budget", timeout)
				}
			}
			if tc.stale && !tc.primary {
				if clock.cachedCalls != 2 || clock.freshCalls != 0 {
					t.Fatalf("ordinary snapshots: cached=%d fresh=%d", clock.cachedCalls, clock.freshCalls)
				}
			} else if clock.cachedCalls != 0 || clock.freshCalls != 2 {
				t.Fatalf("authority snapshots: cached=%d fresh=%d", clock.cachedCalls, clock.freshCalls)
			}
		})
	}
}

func TestTiKVSnapshotCacheError(t *testing.T) {
	cacheErr := errors.New("timestamp cache unavailable")
	clock := &tiKVSnapshotOracle{cachedErr: cacheErr}
	store := newTiKVSnapshotTestStore(t, clock, true)
	if _, err := store.snapshot(t.Context()); !errors.Is(err, cacheErr) {
		t.Fatalf("cache error=%v, want %v", err, cacheErr)
	}
	if clock.cachedCalls != 1 || clock.freshCalls != 0 {
		t.Fatalf("cache failure triggered authority lookup: cached=%d fresh=%d", clock.cachedCalls, clock.freshCalls)
	}
}

func TestTiKVSnapshotCancellation(t *testing.T) {
	for _, primary := range []bool{false, true} {
		for _, duringRead := range []bool{false, true} {
			name := "stale"
			if primary {
				name = "primary"
			}
			if duringRead {
				name += "-during-timestamp"
			} else {
				name += "-before-timestamp"
			}
			t.Run(name, func(t *testing.T) {
				clock := &tiKVSnapshotOracle{cachedTS: oracle.GoTimeToTS(time.Now()), freshTS: oracle.GoTimeToTS(time.Now())}
				store := newTiKVSnapshotTestStore(t, clock, true)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if primary {
					ctx = WithPrimaryReads(ctx)
				}
				if duringRead {
					// The pinned cache-only method ignores cancellation. The caller
					// must still reject a snapshot canceled while fetching it.
					clock.afterRead = cancel
				} else {
					cancel()
				}
				if snapshot, err := store.snapshot(ctx); !errors.Is(err, context.Canceled) || snapshot != nil {
					t.Fatalf("canceled snapshot=%v %v", snapshot, err)
				}
				wantCalls := 0
				if duringRead {
					wantCalls = 1
				}
				if clock.cachedCalls+clock.freshCalls != wantCalls {
					t.Fatalf("timestamp calls=%d, want %d", clock.cachedCalls+clock.freshCalls, wantCalls)
				}
			})
		}
	}
}
