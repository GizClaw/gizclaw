package peersync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func testServer() *Server {
	return &Server{Store: kv.NewMemory(nil), now: func() time.Time { return time.Now().Truncate(time.Hour) }}
}

func TestSyncFinalStateAndDeletion(t *testing.T) {
	s := testServer()
	owner := giznet.PublicKey{1}
	initial := Snapshot{"runtime": json.RawMessage(`{"online":false}`), "contact/alice": json.RawMessage(`{"name":"alice"}`)}
	first, err := s.Sync(t.Context(), owner, 0, initial)
	if err != nil || !first.Reset || len(first.Upserts) != 2 {
		t.Fatalf("initial sync = %#v, %v", first, err)
	}
	// A new Server instance over the same store can resume the old checkpoint.
	restarted := &Server{Store: s.Store, now: s.now}
	unchanged, err := restarted.Sync(t.Context(), owner, first.Timestamp, initial)
	if err != nil || unchanged.Reset || len(unchanged.Upserts) != 0 || len(unchanged.Deletes) != 0 || unchanged.Timestamp <= first.Timestamp {
		t.Fatalf("unchanged sync = %#v, %v", unchanged, err)
	}
	current := Snapshot{"runtime": json.RawMessage(`{"online":true}`)}
	next, err := s.Sync(t.Context(), owner, first.Timestamp, current)
	if err != nil || next.Reset || len(next.Upserts) != 1 || !slices.Equal(next.Deletes, []string{"contact/alice"}) {
		t.Fatalf("incremental sync = %#v, %v", next, err)
	}
	if !bytes.Equal(next.Upserts["runtime"], current["runtime"]) {
		t.Fatalf("final runtime = %s", next.Upserts["runtime"])
	}
	// An interrupted consumer retries with its last *completed* timestamp.
	retry, err := s.Sync(t.Context(), owner, first.Timestamp, current)
	if err != nil || len(retry.Upserts) != 1 || !slices.Equal(retry.Deletes, next.Deletes) {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	data, err := s.Store.Get(t.Context(), checkpointKey(owner, next.Timestamp))
	if err != nil || bytes.Contains(data, []byte("online")) || bytes.Contains(data, []byte("alice\"}")) {
		t.Fatalf("checkpoint contains payload or is absent: %s, %v", data, err)
	}
}

func TestSyncOwnerIsolationAndUnknownTimestamp(t *testing.T) {
	s := testServer()
	a, b := giznet.PublicKey{1}, giznet.PublicKey{2}
	first, err := s.Sync(t.Context(), a, 0, Snapshot{"contact/a": json.RawMessage(`{"name":"private-a"}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, timestamp := range []int64{first.Timestamp, first.Timestamp + 1000} {
		other, err := s.Sync(t.Context(), b, timestamp, Snapshot{"contact/b": json.RawMessage(`{"name":"private-b"}`)})
		if err != nil || !other.Reset || len(other.Upserts) != 1 || len(other.Deletes) != 0 || other.Upserts["contact/a"] != nil {
			t.Fatalf("foreign/unknown timestamp sync = %#v, %v", other, err)
		}
	}
}

func TestSyncRetentionBound(t *testing.T) {
	s := testServer()
	owner := giznet.PublicKey{3}
	first, err := s.Sync(t.Context(), owner, 0, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	for range maxCheckpoints {
		if _, err := s.Sync(t.Context(), owner, first.Timestamp, Snapshot{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Store.Get(t.Context(), checkpointKey(owner, first.Timestamp)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("old checkpoint retained: %v", err)
	}
	old, err := s.Sync(t.Context(), owner, first.Timestamp, Snapshot{})
	if err != nil || !old.Reset {
		t.Fatalf("evicted checkpoint = %#v, %v", old, err)
	}
	data, err := s.Store.Get(t.Context(), stateKey(owner, "head"))
	if err != nil {
		t.Fatal(err)
	}
	var h head
	if err := json.Unmarshal(data, &h); err != nil || len(h.Timestamps) != maxCheckpoints {
		t.Fatalf("retained head = %#v, %v", h, err)
	}
}

func TestSyncExpiredCheckpointResets(t *testing.T) {
	s := testServer()
	owner := giznet.PublicKey{6}
	initial, err := s.Sync(t.Context(), owner, 0, Snapshot{"item": json.RawMessage(`{"name":"old"}`)})
	if err != nil {
		t.Fatal(err)
	}
	now := s.clock().Add(retention + time.Second)
	s.now = func() time.Time { return now }
	result, err := s.Sync(t.Context(), owner, initial.Timestamp, Snapshot{"new": json.RawMessage(`{}`)})
	if err != nil || !result.Reset || len(result.Deletes) != 0 || len(result.Upserts) != 1 {
		t.Fatalf("expired checkpoint = %#v, %v", result, err)
	}
}

func TestSyncSQLiteCheckpointSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync.db")
	open := func() (*Server, *sqlx.DB) {
		db, err := sqlx.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		store, err := kv.NewSQLWithDB(db, "peer_state", nil)
		if err != nil {
			t.Fatal(err)
		}
		return &Server{Store: store}, db
	}
	s, db := open()
	owner := giznet.PublicKey{7}
	state := Snapshot{"runtime": json.RawMessage(`{"online":false}`)}
	first, err := s.Sync(t.Context(), owner, 0, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, _ := open()
	resumed, err := restarted.Sync(t.Context(), owner, first.Timestamp, state)
	if err != nil || resumed.Reset || len(resumed.Upserts) != 0 || len(resumed.Deletes) != 0 {
		t.Fatalf("SQLite resume = %#v, %v", resumed, err)
	}
}

func TestSyncConcurrentCheckpoints(t *testing.T) {
	s := testServer()
	owner := giznet.PublicKey{4}
	const count = 16
	results := make(chan Result, count)
	errorsCh := make(chan error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			result, err := s.Sync(t.Context(), owner, 0, Snapshot{"runtime": json.RawMessage(fmt.Sprintf(`{"value":%d}`, i))})
			results <- result
			errorsCh <- err
		})
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[int64]bool)
	for result := range results {
		if seen[result.Timestamp] {
			t.Fatalf("duplicate checkpoint timestamp %d", result.Timestamp)
		}
		seen[result.Timestamp] = true
		resumed, err := s.Sync(t.Context(), owner, result.Timestamp, result.Upserts)
		if err != nil || resumed.Reset || len(resumed.Upserts) != 0 || len(resumed.Deletes) != 0 {
			t.Fatalf("concurrent checkpoint lost: %#v, %v", resumed, err)
		}
	}
}

type failingStore struct {
	kv.Store
}

func (s failingStore) ApplyMutation(context.Context, kv.Mutation) (bool, error) {
	return false, errors.New("backend failure with private details")
}

func TestSyncRejectsInvalidInputCancellationAndStoreFailures(t *testing.T) {
	s := testServer()
	owner := giznet.PublicKey{5}
	for _, timestamp := range []int64{-1, MaxTimestamp + 1} {
		if _, err := s.Sync(t.Context(), owner, timestamp, Snapshot{}); !errors.Is(err, ErrInvalidTimestamp) {
			t.Fatalf("invalid timestamp: %v", err)
		}
	}
	if _, err := s.Sync(t.Context(), owner, 0, Snapshot{"invalid": json.RawMessage(`{`)}); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Sync(ctx, owner, 0, Snapshot{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled sync: %v", err)
	}
	s.Store = failingStore{Store: s.Store}
	if _, err := s.Sync(t.Context(), owner, 0, Snapshot{}); err == nil {
		t.Fatal("store failure accepted")
	}
	if _, err := s.Store.Get(t.Context(), stateKey(owner, "head")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("failed sync advanced checkpoint: %v", err)
	}
}
