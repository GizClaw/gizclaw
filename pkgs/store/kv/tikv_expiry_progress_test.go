package kv

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tikv/client-go/v2/oracle"
)

// Freeze only the cache accessor. Mutations and explicit primary reads still
// obtain real timestamps, and the SDK's future-TS and GC validators stay active.
type tiKVExpiryReadOracle struct {
	oracle.Oracle
	frozenTS    atomic.Uint64
	lastCached  atomic.Uint64
	lastStaleTS atomic.Uint64
	freshCalls  atomic.Int64
}

func (o *tiKVExpiryReadOracle) GetLowResolutionTimestamp(ctx context.Context, opt *oracle.Option) (uint64, error) {
	ts := o.frozenTS.Load()
	if ts == 0 {
		var err error
		ts, err = o.Oracle.GetLowResolutionTimestamp(ctx, opt)
		if err != nil {
			return 0, err
		}
	}
	o.lastCached.Store(ts)
	return ts, nil
}

func (o *tiKVExpiryReadOracle) GetTimestamp(ctx context.Context, opt *oracle.Option) (uint64, error) {
	o.freshCalls.Add(1)
	return o.Oracle.GetTimestamp(ctx, opt)
}

func (o *tiKVExpiryReadOracle) ValidateReadTS(ctx context.Context, readTS uint64, stale bool, opt *oracle.Option) error {
	if stale {
		o.lastStaleTS.Store(readTS)
	}
	return o.Oracle.ValidateReadTS(ctx, readTS, stale, opt)
}

func testTiKVStaleExpiryProgress(t *testing.T, primary *TiKV) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	// The expiry index belongs to the physical root, including earlier contract
	// subtests. Drain it before asserting exact batch counts for these rows.
	for batch := 0; ; batch++ {
		count, err := primary.PurgeExpired(ctx, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if count < 1024 {
			break
		}
		if batch == 31 {
			t.Fatal("expiry setup did not drain within 32 batches")
		}
	}
	const delay = 1501 * time.Millisecond
	stale, err := NewTiKVWithClient(primary.client, TiKVReadOptions{StaleReadDelay: delay, Labels: map[string]string{"region": "local"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "stale-expiry-" + rand.Text()
	marker := Key{prefix, "ready"}
	keys := []Key{marker}
	deadline := time.Now().Add(3 * time.Second)
	entries := make([]Entry, 256)
	for i := range entries {
		key := Key{prefix, fmt.Sprintf("%03d", i)}
		keys = append(keys, key)
		entries[i] = Entry{Key: key, Value: []byte("expired"), Deadline: deadline}
	}
	t.Cleanup(func() { _ = primary.BatchDelete(context.Background(), keys) })
	if err := primary.BatchSet(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if err := primary.Set(ctx, marker, []byte("ready")); err != nil {
		t.Fatal(err)
	}
	original := primary.client.GetOracle()
	clock := &tiKVExpiryReadOracle{Oracle: original}
	primary.client.SetOracle(clock)
	t.Cleanup(func() { primary.client.SetOracle(original) })
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Wait for the actual ordinary snapshot to include the persistent
		// marker: the configured delay alone omits the oracle cache's age.
		value, err := stale.Get(ctx, marker)
		if err == nil && bytes.Equal(value, []byte("ready")) && !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("ordinary snapshot did not include the committed marker: value=%q err=%v: %v", value, err, ctx.Err())
		case <-ticker.C:
		}
	}
	clock.frozenTS.Store(clock.lastCached.Load())
	freshBefore := clock.freshCalls.Load()
	if value, err := stale.Get(ctx, marker); err != nil || string(value) != "ready" {
		t.Fatalf("frozen ordinary read=%q %v", value, err)
	}
	wantTS := oracle.GoTimeToTS(oracle.GetTimeFromTS(clock.frozenTS.Load()).Add(-delay))
	if got := clock.lastStaleTS.Load(); got != wantTS {
		t.Fatalf("stale read timestamp=%d, want cached timestamp minus all 1501ms=%d", got, wantTS)
	}
	first, err := stale.expiredCandidates(ctx, 128)
	if err != nil || len(first) != 128 {
		t.Fatalf("historical discovery count=%d err=%v", len(first), err)
	}
	if got := clock.freshCalls.Load(); got != freshBefore {
		t.Fatalf("ordinary Get/expiry scan fetched %d fresh timestamps", got-freshBefore)
	}
	if count, err := stale.PurgeExpired(ctx, 128); err != nil || count != 128 {
		t.Fatalf("first stale expiry batch=%d %v", count, err)
	}
	// Keep every historical snapshot fixed before the deletions. The next call
	// must encounter the same 128 removed candidates and still reclaim the rest.
	again, err := stale.expiredCandidates(ctx, 128)
	if err != nil || len(again) != len(first) {
		t.Fatalf("frozen discovery count=%d err=%v", len(again), err)
	}
	for i := range first {
		if !bytes.Equal(first[i], again[i]) {
			t.Fatal("historical discovery advanced past the deleted first batch")
		}
	}
	if count, err := stale.PurgeExpired(ctx, 128); err != nil || count != 128 {
		t.Fatalf("immediate second stale expiry batch=%d %v", count, err)
	}
	if count, err := stale.PurgeExpired(ctx, 128); err != nil || count != 0 {
		t.Fatalf("completed sweep reported phantom progress=%d %v", count, err)
	}
	ts, err := primary.timestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	physicalKeys := make([][]byte, 0, 2*len(entries))
	for _, entry := range entries {
		physicalKeys = append(physicalKeys, primary.recordKey(entry.Key), primary.expiryKey(entry.Key, deadline.UnixNano()))
	}
	remaining, err := primary.client.GetSnapshot(ts).BatchGet(ctx, physicalKeys)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("physical expired records/indexes remaining=%d err=%v", len(remaining), err)
	}
	if value, err := primary.Get(ctx, marker); err != nil || string(value) != "ready" {
		t.Fatalf("persistent marker=%q %v", value, err)
	}
}
