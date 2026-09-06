package pendingdeletion

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	redis "github.com/redis/go-redis/v9"
)

type indexedTaskReadStore struct {
	kv.Store
	gets   atomic.Int64
	rows   atomic.Int64
	ranges atomic.Int64
}

func (s *indexedTaskReadStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.gets.Add(1)
	return s.Store.Get(ctx, key)
}

func (s *indexedTaskReadStore) RangeOrderedMembers(ctx context.Context, key kv.Key, query kv.OrderedRange) ([]string, error) {
	s.ranges.Add(1)
	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	members, err := s.Store.RangeOrderedMembers(ctx, key, query)
	s.rows.Add(int64(len(members)))
	return members, err
}

func TestKVTaskIndexesExcludeFutureAndFailedTasks(t *testing.T) {
	factories := map[string]func(*testing.T) kv.Store{"memory": func(t *testing.T) kv.Store { return kv.NewMemory(nil) }}
	if dsn := os.Getenv("GIZCLAW_TEST_REDIS_DSN"); dsn != "" {
		factories["redis"] = func(t *testing.T) kv.Store {
			options, err := redis.ParseURL(dsn)
			if err != nil {
				t.Fatal("invalid Redis test URL")
			}
			client := redis.NewClient(options)
			t.Cleanup(func() { _ = client.Close() })
			store, err := kv.NewRedisWithClient(client, nil)
			if err != nil {
				t.Fatal(err)
			}
			return kv.Prefixed(store, kv.Key{"task-index-test", rand.Text()})
		}
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) { testKVTaskIndexes(t, factory(t)) })
	}
}

func testKVTaskIndexes(t *testing.T, store kv.Store) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	source := KVSource{Store: store, SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	var cleanup []kv.Key
	for shard := range kvTaskIndexShards {
		cleanup = append(cleanup, kvDueIndexKey(KindPeer, shard))
		for _, status := range []Status{StatusQueued, StatusRetryWait, StatusRunning, StatusFailed} {
			cleanup = append(cleanup, kvCreatedIndexKey(KindPeer, status, shard))
		}
	}
	t.Cleanup(func() { _ = store.BatchDelete(context.Background(), cleanup) })
	const dueCount, failedCount, total = 7, 10, 2000
	for i := range total {
		at := now.Add(24 * time.Hour)
		if i < dueCount+failedCount {
			at = now.Add(-time.Hour)
		}
		record, err := New(KindPeer, fmt.Sprintf("peer-index-%04d", i), nil, ReasonPeerDelete, struct{}{}, at)
		if err != nil {
			t.Fatal(err)
		}
		cleanup = append(cleanup, byIDKey(record.DeletionID), byLocatorKey(record.Kind, record.ResourceID), kvTaskKey(record.DeletionID))
		if _, created, err := CreateOrGet(t.Context(), store, record); err != nil || !created {
			t.Fatalf("create %d: %v, %v", i, created, err)
		}
		if i >= dueCount && i < dueCount+failedCount {
			fingerprint, err := Fingerprint(record)
			if err != nil {
				t.Fatal(err)
			}
			claim, ok, err := source.Claim(t.Context(), Reference{Source: source.Name(), DeletionID: record.DeletionID, MarkerFingerprint: fingerprint}, now, time.Minute)
			if err != nil || !ok {
				t.Fatalf("claim failed fixture: %v, %v", ok, err)
			}
			if err := source.Fail(t.Context(), claim, "terminal", "test", true, now, now, 3); err != nil {
				t.Fatal(err)
			}
		}
	}
	reader := &indexedTaskReadStore{Store: store}
	source.Store = reader
	seen := make(map[string]bool)
	cursor := ""
	var first Reference
	for page := 0; ; page++ {
		if page > dueCount {
			t.Fatal("due pagination did not terminate")
		}
		reader.gets.Store(0)
		reader.rows.Store(0)
		reader.ranges.Store(0)
		refs, next, err := source.ScanDue(t.Context(), now, 3, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) > 3 || reader.gets.Load() != 0 || reader.rows.Load() > dueCount || reader.ranges.Load() != kvTaskIndexShards {
			t.Fatalf("due page: refs=%d record reads=%d index rows=%d ranges=%d", len(refs), reader.gets.Load(), reader.rows.Load(), reader.ranges.Load())
		}
		for _, ref := range refs {
			if seen[ref.DeletionID] {
				t.Fatal("duplicate due task")
			}
			seen[ref.DeletionID] = true
			first = ref
		}
		if next == "" {
			break
		}
		if next == cursor {
			t.Fatal("due cursor did not advance")
		}
		cursor = next
	}
	if len(seen) != dueCount {
		t.Fatalf("due tasks = %d", len(seen))
	}
	failed := SourceListOptions{Statuses: map[Status]bool{StatusFailed: true}, Limit: 3}
	seen = make(map[string]bool)
	for page := 0; ; page++ {
		if page > failedCount {
			t.Fatal("failed pagination did not terminate")
		}
		reader.gets.Store(0)
		tasks, err := source.ListTasks(t.Context(), failed)
		if err != nil {
			t.Fatal(err)
		}
		if reader.gets.Load() > 6 {
			t.Fatalf("read records outside the page: %d", reader.gets.Load())
		}
		if len(tasks) == 0 {
			break
		}
		for _, task := range tasks {
			if seen[task.Record.DeletionID] {
				t.Fatal("duplicate failed task")
			}
			seen[task.Record.DeletionID] = true
		}
		last := tasks[len(tasks)-1]
		failed.AfterCreatedAt, failed.AfterSource, failed.AfterDeletionID = &last.Record.DeletedAt, last.Source, last.Record.DeletionID
	}
	if len(seen) != failedCount {
		t.Fatalf("failed tasks = %d", len(seen))
	}
	claim, ok, err := source.Claim(t.Context(), first, now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v, %v", ok, err)
	}
	if err := source.Renew(t.Context(), claim, now, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	refs, _, err := source.ScanDue(t.Context(), now.Add(61*time.Second), 100, "")
	if err != nil || len(refs) != dueCount-1 {
		t.Fatalf("renewed task was redispatched: %d, %v", len(refs), err)
	}
	refs, _, err = source.ScanDue(t.Context(), now.Add(121*time.Second), 100, "")
	if err != nil || len(refs) != dueCount {
		t.Fatalf("expired lease was not redispatched: %d, %v", len(refs), err)
	}
	claim, err = source.Checkpoint(t.Context(), claim, PhaseFinalize, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	running, err := source.ListTasks(t.Context(), SourceListOptions{Statuses: map[Status]bool{StatusRunning: true}, Limit: 10})
	if err != nil || len(running) != 1 {
		t.Fatalf("checkpoint lost unchanged creation index: %d, %v", len(running), err)
	}
	if err := source.Finalize(t.Context(), claim, now.Add(2*time.Second), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := source.GetTask(t.Context(), claim.Record.DeletionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("finalized task = %v", err)
	}
	refs, _, err = source.ScanDue(t.Context(), now.Add(121*time.Second), 100, "")
	if err != nil || len(refs) != dueCount-1 {
		t.Fatalf("finalization left due index: %d, %v", len(refs), err)
	}
}
