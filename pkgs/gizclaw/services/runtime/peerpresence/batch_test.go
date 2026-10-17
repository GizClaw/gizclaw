package peerpresence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type batchReadStore struct {
	kv.Store
	batches [][]kv.Key
}

func (s *batchReadStore) BatchGet(ctx context.Context, keys []kv.Key) []kv.GetResult {
	s.batches = append(s.batches, keys)
	return s.Store.BatchGet(ctx, keys)
}

func TestReadBatchDeduplicatesLeasesAndIsolatesErrors(t *testing.T) {
	base := kv.NewMemory(nil)
	a, b := startService(t, base, 1), startService(t, base, 2)
	seen := time.Now().UTC()
	for _, entry := range []struct {
		peer   byte
		server *Service
		online bool
	}{{3, a, true}, {4, a, true}, {5, b, true}, {6, a, false}} {
		snapshot := Snapshot{Online: entry.online, LastSeenAt: seen, WorkspaceName: "room"}
		entry.server.Publish(giznet.PublicKey{entry.peer}, snapshot)
		if !entry.online {
			snapshot.WorkspaceName = ""
		}
		awaitSnapshot(t, entry.server, giznet.PublicKey{entry.peer}, snapshot)
	}
	if err := base.Set(t.Context(), peerKey(giznet.PublicKey{7}), []byte("invalid json")); err != nil {
		t.Fatal(err)
	}
	if err := base.Set(t.Context(), leaseKey(b.server), []byte("invalid lease")); err != nil {
		t.Fatal(err)
	}
	store := &batchReadStore{Store: base}
	reader := New(store, giznet.PublicKey{9})
	defer reader.cancel()
	peers := []giznet.PublicKey{{3}, {4}, {5}, {6}, {7}, {8}, {3}}
	results := reader.ReadBatch(t.Context(), peers)
	if len(store.batches) != 2 || len(store.batches[0]) != len(peers) || len(store.batches[1]) != 2 {
		t.Fatalf("batches = %+v; want one Peer batch and two unique Server leases", store.batches)
	}
	for i, result := range results {
		switch i {
		case 0, 1, 6:
			if result.Err != nil || !result.Snapshot.Online || result.Snapshot.WorkspaceName != "room" || !result.Snapshot.LastSeenAt.Equal(seen) {
				t.Fatalf("online result %d = %+v", i, result)
			}
		case 2, 4:
			if result.Err == nil || result.Snapshot != (Snapshot{}) {
				t.Fatalf("corrupt result %d = %+v", i, result)
			}
		case 3:
			if result.Err != nil || result.Snapshot.Online || result.Snapshot.WorkspaceName != "" || !result.Snapshot.LastSeenAt.Equal(seen) {
				t.Fatalf("offline result = %+v", result)
			}
		case 5:
			if result.Err != nil || result.Snapshot != (Snapshot{}) {
				t.Fatalf("missing result = %+v", result)
			}
		}
	}
}

type delayedBatchStore struct {
	kv.Store
	read func(context.Context, []kv.Key) []kv.GetResult
}

func (s delayedBatchStore) BatchGet(ctx context.Context, keys []kv.Key) []kv.GetResult {
	return s.read(ctx, keys)
}

func TestReadBatchSharesDeadlineAcrossBothRounds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		calls := 0
		seen := started.UTC()
		store := delayedBatchStore{read: func(ctx context.Context, keys []kv.Key) []kv.GetResult {
			calls++
			if deadline, ok := ctx.Deadline(); !ok || !deadline.Equal(started.Add(writeTimeout)) {
				t.Errorf("batch deadline = %v, %v", deadline, ok)
			}
			if calls == 1 {
				time.Sleep(1500 * time.Millisecond)
				results := make([]kv.GetResult, len(keys))
				for i := range results {
					results[i].Value, _ = json.Marshal(record{Snapshot: Snapshot{Online: i == 0, LastSeenAt: seen}, Server: giznet.PublicKey{1}.String(), Generation: "boot"})
				}
				return results
			}
			<-ctx.Done()
			return []kv.GetResult{{Err: ctx.Err()}}
		}}
		reader := New(store, giznet.PublicKey{2})
		defer reader.cancel()
		results := reader.ReadBatch(t.Context(), []giznet.PublicKey{{3}, {4}})
		if elapsed := time.Since(started); elapsed != writeTimeout || calls != 2 {
			t.Fatalf("elapsed = %s, calls = %d; lease round received a fresh timeout", elapsed, calls)
		}
		if !errors.Is(results[0].Err, context.DeadlineExceeded) || results[1].Err != nil || !results[1].Snapshot.LastSeenAt.Equal(seen) {
			t.Fatalf("partial results = %+v", results)
		}
	})
}
