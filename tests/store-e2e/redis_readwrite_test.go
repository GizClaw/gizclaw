//go:build store_e2e

package store_e2e_test

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	redis "github.com/redis/go-redis/v9"
)

func TestReadWriteRedisSharedPresence(t *testing.T) {
	f := newReadWriteRedisFixture(t)
	a := peerpresence.New(f.first, giznet.PublicKey{21})
	b := peerpresence.New(f.first, giznet.PublicKey{22})
	for _, s := range []*peerpresence.Service{a, b} {
		if err := s.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
	}
	peer := giznet.PublicKey{23}
	want := peerpresence.Snapshot{Online: true, LastSeenAt: time.Now().UTC(), WorkspaceName: "room"}
	assertSnapshot := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			got, err := b.Read(t.Context(), peer)
			if err == nil && got == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("shared Redis snapshot = %+v, %v; want %+v", got, err, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	a.Publish(peer, want)
	assertSnapshot()
	want.WorkspaceName = "another-room"
	a.Publish(peer, want)
	assertSnapshot()
	want.WorkspaceName = ""
	a.Publish(peer, want)
	assertSnapshot()
	want.Online = false
	want.LastSeenAt = want.LastSeenAt.Add(time.Second)
	a.Publish(peer, want)
	assertSnapshot()
	want.Online, want.WorkspaceName = true, "room"
	a.Publish(peer, want)
	assertSnapshot()
	f.waitForReplica(t)
	// Both Peer and Server-lease reads must use the replica, including while
	// the primary client is unavailable. A reader error must not fall back.
	if err := f.write.Close(); err != nil {
		t.Fatal(err)
	}
	assertSnapshot()
	if err := f.read.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Read(t.Context(), peer); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("closed reader = %v", err)
	}
}

func TestReadWriteRedisContextDeadline(t *testing.T) {
	f := newReadWriteRedisFixture(t)
	for name, client := range map[string]*redis.Client{"reader": f.read, "writer": f.write} {
		t.Run(name, func(t *testing.T) {
			// Pause only this disposable Docker Redis endpoint. A healthy TCP
			// connection whose next response is delayed must still respect the
			// presence operation's deadline, rather than the socket default.
			if err := client.Do(t.Context(), "CLIENT", "PAUSE", 1000, "ALL").Err(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 75*time.Millisecond)
			defer cancel()
			start := time.Now()
			err := client.Ping(ctx).Err()
			elapsed := time.Since(start)
			if err == nil || elapsed >= 500*time.Millisecond {
				t.Fatalf("delayed %s response: error=%v, context=%v, elapsed=%v", name, err, ctx.Err(), elapsed)
			}
		})
	}
}

type readWriteRedisFixture struct {
	physical *storage.Storage
	registry *stores.Stores
	first    kv.Store
	second   kv.Store
	read     *redis.Client
	write    *redis.Client
	control  *redis.Client
	prefix   string
}

func newReadWriteRedisFixture(t *testing.T) readWriteRedisFixture {
	t.Helper()
	writeURL := requiredEnvironment(t, "GIZCLAW_TEST_REDIS_DSN")
	readURL := requiredEnvironment(t, "GIZCLAW_TEST_REDIS_READ_DSN")
	physical, err := storage.New(map[string]storage.Config{
		"control": storage.RedisConfig{URL: writeURL},
		"split": storage.RedisReadWriteConfig{
			Read: storage.RedisConfig{URL: readURL}, Write: storage.RedisConfig{URL: writeURL},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	read, write, err := physical.RedisReadWrite("split")
	if err != nil {
		t.Fatal(err)
	}
	control, err := physical.Redis("control")
	if err != nil {
		t.Fatal(err)
	}
	prefix := "readwrite-" + rand.Text()
	registry, err := stores.New(map[string]stores.Config{
		"first":  {Kind: stores.KindKeyValue, Storage: "split", Prefix: prefix + "/first"},
		"second": {Kind: stores.KindKeyValue, Storage: "split", Prefix: prefix + "/second"},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	first, err := registry.KV("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.KV("second")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		keys := []string{prefix + ":barrier", prefix + ":readonly"}
		for _, store := range []string{"first", "second"} {
			for _, key := range []string{"single", "batch", "deleted", "record", "set", "ordered", "guard", "created", "all-a", "all-b", "joint"} {
				keys = append(keys, prefix+":"+store+":"+key)
			}
		}
		if err := control.Del(ctx, keys...).Err(); err != nil {
			t.Error("cleanup read/write Redis fixture failed")
		}
	})
	return readWriteRedisFixture{physical: physical, registry: registry, first: first, second: second, read: read, write: write, control: control, prefix: prefix}
}

func (f readWriteRedisFixture) waitForReplica(t *testing.T) {
	t.Helper()
	marker := rand.Text()
	key := f.prefix + ":barrier"
	if err := f.control.Set(t.Context(), key, marker, 0).Err(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if value, err := f.read.Get(ctx, key).Result(); err == nil && value == marker {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("replica did not apply the primary's write barrier")
		case <-ticker.C:
		}
	}
}

func TestReadWriteRedisKV(t *testing.T) {
	f := newReadWriteRedisFixture(t)
	ctx := t.Context()
	if err := f.first.Set(ctx, kv.Key{"single"}, []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := f.first.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"batch"}, Value: []byte("value")},
		{Key: kv.Key{"deleted"}, Value: []byte("value")},
		{Key: kv.Key{"empty"}, Value: []byte{}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.first.Delete(ctx, kv.Key{"deleted"}); err != nil {
		t.Fatal(err)
	}
	if err := f.first.BatchDelete(ctx, []kv.Key{{"batch"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.first.BatchMutate(ctx, []kv.Entry{{Key: kv.Key{"record"}, Value: []byte("before")}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.first.AddMembers(ctx, kv.Key{"set"}, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := f.first.RemoveMembers(ctx, kv.Key{"set"}, "a"); err != nil {
		t.Fatal(err)
	}
	if applied, err := f.first.ApplyMutation(ctx, kv.Mutation{
		Conditions:        []kv.Condition{{Key: kv.Key{"record"}, Expected: []byte("before")}},
		Entries:           []kv.Entry{{Key: kv.Key{"record"}, Value: []byte("after")}},
		AddOrderedMembers: []kv.SetMembers{{Key: kv.Key{"ordered"}, Members: []string{"c", "a", "b"}}},
	}); err != nil || !applied {
		t.Fatalf("ApplyMutation() = %v, %v", applied, err)
	}
	if applied, err := f.first.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{{Key: kv.Key{"record"}, Expected: []byte("before")}},
		Entries:    []kv.Entry{{Key: kv.Key{"record"}, Value: []byte("wrong")}},
	}); err != nil || applied {
		t.Fatalf("stale ApplyMutation() = %v, %v", applied, err)
	}
	guard := kv.Entry{Key: kv.Key{"guard"}, Value: []byte("original")}
	if _, created, err := kv.CreateIfAbsent(ctx, f.first, guard, []kv.Entry{{Key: kv.Key{"created"}, Value: []byte("value")}}); err != nil || !created {
		t.Fatalf("CreateIfAbsent() = %v, %v", created, err)
	}
	if value, created, err := kv.CreateIfAbsent(ctx, f.first, guard, nil); err != nil || created || string(value) != "original" {
		t.Fatalf("conflicting CreateIfAbsent() = %v, %v", created, err)
	}
	guards := []kv.Entry{{Key: kv.Key{"all-a"}, Value: []byte("a")}, {Key: kv.Key{"all-b"}, Value: []byte("b")}}
	if _, _, created, err := kv.CreateIfAllAbsent(ctx, f.first, guards, nil); err != nil || !created {
		t.Fatalf("CreateIfAllAbsent() = %v, %v", created, err)
	}
	if _, _, created, err := kv.CreateIfAllAbsent(ctx, f.first, guards, nil); err != nil || created {
		t.Fatalf("conflicting CreateIfAllAbsent() = %v, %v", created, err)
	}
	if matched, err := kv.CompareAndMutate(ctx, f.first, guard.Key, guard.Value, []kv.Entry{{Key: guard.Key, Value: []byte("updated")}}, nil); err != nil || !matched {
		t.Fatalf("CompareAndMutate() = %v, %v", matched, err)
	}
	root, prefixes, ok := kv.SharedAtomicStore(f.first, f.second)
	if !ok {
		t.Fatal("read/write logical Stores lost their shared atomic root")
	}
	if err := root.BatchMutate(ctx, []kv.Entry{
		{Key: append(slices.Clone(prefixes[0]), "joint"), Value: []byte("first")},
		{Key: append(slices.Clone(prefixes[1]), "joint"), Value: []byte("second")},
	}, nil); err != nil {
		t.Fatal(err)
	}
	f.waitForReplica(t)
	if err := f.read.Set(ctx, f.prefix+":readonly", "forbidden", 0).Err(); err == nil || !strings.HasPrefix(err.Error(), "READONLY") {
		t.Fatal("read endpoint is not a read-only Redis replica")
	}
	if err := f.write.Close(); err != nil {
		t.Fatal(err)
	}
	// A closed primary client proves all query methods use the replica client.
	for key, want := range map[string]string{"single": "value", "record": "after", "guard": "updated", "joint": "first"} {
		if value, err := f.first.Get(ctx, kv.Key{key}); err != nil || string(value) != want {
			t.Fatalf("replica Get(%s) failed: %v", key, err)
		}
	}
	results := kv.BatchGet(ctx, f.first, []kv.Key{{"single"}, {"missing"}, {"single"}, {"empty"}, {"set"}})
	if string(results[0].Value) != "value" || results[0].Err != nil || !errors.Is(results[1].Err, kv.ErrNotFound) || string(results[2].Value) != "value" || results[3].Err != nil || len(results[3].Value) != 0 || !errors.Is(results[4].Err, kv.ErrWrongType) {
		t.Fatalf("replica mixed BatchGet with a closed writer = %+v", results)
	}
	results[0].Value[0] = 'X'
	if string(results[2].Value) != "value" {
		t.Fatal("Redis duplicate results alias mutable bytes")
	}
	if value, err := f.second.Get(ctx, kv.Key{"joint"}); err != nil || string(value) != "second" {
		t.Fatalf("second prefix replica Get() failed: %v", err)
	}
	for _, key := range []string{"batch", "deleted"} {
		if _, err := f.first.Get(ctx, kv.Key{key}); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("replica retained deleted key %s: %v", key, err)
		}
	}
	if found, err := f.first.HasMember(ctx, kv.Key{"set"}, "b"); err != nil || !found {
		t.Fatalf("replica HasMember() = %v, %v", found, err)
	}
	if members, err := f.first.ListMembers(ctx, kv.Key{"set"}); err != nil || !slices.Equal(members, []string{"b"}) {
		t.Fatalf("replica ListMembers() failed: %v", err)
	}
	if members, err := f.first.RangeOrderedMembers(ctx, kv.Key{"ordered"}, kv.OrderedRange{After: new("a"), Limit: 1}); err != nil || !slices.Equal(members, []string{"b"}) {
		t.Fatalf("replica RangeOrderedMembers() failed: %v", err)
	}
	if err := f.first.Set(ctx, kv.Key{"single"}, []byte("wrong")); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("write with closed primary error = %v", err)
	}
}

func TestReadWriteRedisReaderFailureAndLifecycle(t *testing.T) {
	f := newReadWriteRedisFixture(t)
	if err := f.registry.Close(); err != nil {
		t.Fatal(err)
	}
	for _, client := range []*redis.Client{f.read, f.write} {
		if err := client.Ping(t.Context()).Err(); err != nil {
			t.Fatalf("logical Close() closed a physical client: %v", err)
		}
	}
	if err := f.read.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.first.Get(t.Context(), kv.Key{"single"}); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("reader failure silently fell back to the primary: %v", err)
	}
	if result := kv.BatchGet(t.Context(), f.first, []kv.Key{{"single"}})[0]; !errors.Is(result.Err, redis.ErrClosed) {
		t.Fatalf("batch reader failure silently fell back to the primary: %+v", result)
	}
	if err := f.first.Set(t.Context(), kv.Key{"single"}, []byte("written")); err != nil {
		t.Fatalf("reader failure prevented primary write: %v", err)
	}
	if value, err := f.control.Get(t.Context(), f.prefix+":first:single").Result(); err != nil || value != "written" {
		t.Fatalf("write did not reach the primary: %v", err)
	}
}

func TestReadWriteRedisPrefixValidationAndPhysicalClose(t *testing.T) {
	writeURL := requiredEnvironment(t, "GIZCLAW_TEST_REDIS_DSN")
	readURL := requiredEnvironment(t, "GIZCLAW_TEST_REDIS_READ_DSN")
	physical, err := storage.New(map[string]storage.Config{
		"split": storage.RedisReadWriteConfig{Read: storage.RedisConfig{URL: readURL}, Write: storage.RedisConfig{URL: writeURL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	for name, configs := range map[string]map[string]stores.Config{
		"empty": {"a": {Kind: stores.KindKeyValue, Storage: "split"}},
		"overlap": {
			"a": {Kind: stores.KindKeyValue, Storage: "split", Prefix: "parent"},
			"b": {Kind: stores.KindKeyValue, Storage: "split", Prefix: "parent/child"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := stores.New(configs, physical); err == nil || !strings.Contains(err.Error(), "prefix") {
				t.Fatalf("stores.New() prefix error = %v", err)
			}
		})
	}
	read, write, err := physical.RedisReadWrite("split")
	if err != nil {
		t.Fatal(err)
	}
	if err := physical.Close(); err != nil {
		t.Fatal(err)
	}
	for _, client := range []*redis.Client{read, write} {
		if err := client.Ping(t.Context()).Err(); !errors.Is(err, redis.ErrClosed) {
			t.Fatalf("physical Close() left a client open: %v", err)
		}
	}
	if _, _, err := physical.RedisReadWrite("split"); err == nil {
		t.Fatal("closed physical registry returned clients")
	}
}

func TestReadWriteRedisBorrowedClientsDoNotBlockClose(t *testing.T) {
	physical, err := storage.New(map[string]storage.Config{
		"split": storage.RedisReadWriteConfig{
			Read:  storage.RedisConfig{URL: requiredEnvironment(t, "GIZCLAW_TEST_REDIS_READ_DSN")},
			Write: storage.RedisConfig{URL: requiredEnvironment(t, "GIZCLAW_TEST_REDIS_DSN")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	read, write, err := physical.RedisReadWrite("split")
	if err != nil {
		t.Fatal(err)
	}
	// A borrower retains both clients while another goroutine closes the owner.
	for _, client := range []*redis.Client{read, write} {
		if err := client.Ping(t.Context()).Err(); err != nil {
			t.Fatalf("borrowed client was closed before its owner: %v", err)
		}
	}
	closed := make(chan error, 1)
	go func() { closed <- physical.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("closing Storage waited for borrowed Redis clients")
	}
	for _, client := range []*redis.Client{read, write} {
		if err := client.Ping(t.Context()).Err(); !errors.Is(err, redis.ErrClosed) {
			t.Fatalf("borrowed client survived owner Close(): %v", err)
		}
	}
}

func TestReadWriteRedisStrongViews(t *testing.T) {
	for _, failure := range []string{"reader", "writer"} {
		t.Run(failure, func(t *testing.T) {
			f := newReadWriteRedisFixture(t)
			ctx := t.Context()
			if err := f.first.Set(ctx, kv.Key{"record"}, []byte("current")); err != nil {
				t.Fatal(err)
			}
			if _, err := f.first.ApplyMutation(ctx, kv.Mutation{AddMembers: []kv.SetMembers{{Key: kv.Key{"set"}, Members: []string{"a"}}}, AddOrderedMembers: []kv.SetMembers{{Key: kv.Key{"ordered"}, Members: []string{"a"}}}}); err != nil {
				t.Fatal(err)
			}
			f.waitForReplica(t)
			strong := kv.WithStrongReads(f.first)
			if failure == "reader" {
				if err := f.read.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := f.first.Get(ctx, kv.Key{"record"}); !errors.Is(err, redis.ErrClosed) {
					t.Fatalf("ordinary read did not use failed reader: %v", err)
				}
				if value, err := strong.Get(ctx, kv.Key{"record"}); err != nil || string(value) != "current" {
					t.Fatalf("strong Get=%q %v", value, err)
				}
				if result := kv.BatchGet(ctx, strong, []kv.Key{{"record"}})[0]; result.Err != nil || string(result.Value) != "current" {
					t.Fatalf("strong BatchGet=%+v", result)
				}
				if member, err := strong.HasMember(ctx, kv.Key{"set"}, "a"); err != nil || !member {
					t.Fatalf("strong HasMember=%v %v", member, err)
				}
				if members, err := strong.ListMembers(ctx, kv.Key{"set"}); err != nil || len(members) != 1 {
					t.Fatalf("strong ListMembers=%v %v", members, err)
				}
				if members, err := strong.RangeOrderedMembers(ctx, kv.Key{"ordered"}, kv.OrderedRange{Limit: 1}); err != nil || len(members) != 1 {
					t.Fatalf("strong RangeOrderedMembers=%v %v", members, err)
				}
			} else {
				if err := f.write.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := f.first.Get(ctx, kv.Key{"record"}); err != nil {
					t.Fatalf("ordinary reader is unavailable: %v", err)
				}
				if _, err := strong.Get(ctx, kv.Key{"record"}); !errors.Is(err, redis.ErrClosed) {
					t.Fatalf("strong read fell back to replica: %v", err)
				}
				if result := kv.BatchGet(ctx, strong, []kv.Key{{"record"}})[0]; !errors.Is(result.Err, redis.ErrClosed) {
					t.Fatalf("strong batch read fell back to replica: %+v", result)
				}
			}
		})
	}
}
