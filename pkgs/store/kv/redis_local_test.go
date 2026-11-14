package kv

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
)

const redisLocalTestID = "1234567890abcdef1234567890abcdef12345678"

func redisLocalTestInfo(offset int64) string {
	return fmt.Sprintf("role:slave\r\nmaster_replid:%s\r\nmaster_sync_in_progress:0\r\nslave_repl_offset:%d\r\nslave_read_repl_offset:99999\r\n", redisLocalTestID, offset)
}

type redisLocalTestHook struct {
	pipeline func(context.Context, []redis.Cmder) error
	process  func(context.Context, redis.Cmder) error
}

func (*redisLocalTestHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *redisLocalTestHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	if h.process != nil {
		return h.process
	}
	return next
}
func (h *redisLocalTestHook) ProcessPipelineHook(redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return h.pipeline
}

func redisLocalTestStore(t *testing.T, hook *redisLocalTestHook) *Redis {
	t.Helper()
	reader := redis.NewClient(&redis.Options{Addr: "reader.invalid:6379"})
	writer := redis.NewClient(&redis.Options{Addr: "writer.invalid:6379"})
	if hook.process == nil {
		hook.process = func(_ context.Context, cmd redis.Cmder) error {
			if cmd.Name() != "info" {
				return errors.New("unexpected standalone reader command")
			}
			cmd.(*redis.StringCmd).SetVal("")
			return nil
		}
	}
	reader.AddHook(hook)
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	store, err := NewRedisReadWriteWithClients(reader, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestRedisReplicationProgressValidation(t *testing.T) {
	good := redisLocalTestInfo(40)
	for _, tc := range []struct {
		name, info    string
		writer, valid bool
		offset        int64
	}{
		{name: "applied offset", info: good, valid: true, offset: 40},
		{name: "negative", info: redisLocalTestInfo(-1)},
		{name: "sync in progress", info: strings.ReplaceAll(good, "master_sync_in_progress:0", "master_sync_in_progress:1")},
		{name: "missing applied offset", info: strings.ReplaceAll(good, "slave_repl_offset:40", "omitted:40")},
		{name: "wrong role", info: strings.ReplaceAll(good, "role:slave", "role:sentinel")},
		{name: "writer replica", info: good, writer: true},
		{name: "invalid identity", info: strings.ReplaceAll(good, redisLocalTestID, strings.Repeat("z", 40))},
		{name: "missing identity", info: strings.ReplaceAll(good, redisLocalTestID, strings.Repeat("0", 40))},
		{name: "primary", info: "role:master\nmaster_replid:" + redisLocalTestID + "\nmaster_repl_offset:99\n", writer: true, valid: true, offset: 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRedisProgress(tc.info, tc.writer)
			if (got.id != "") != tc.valid || tc.valid && got.offset != tc.offset {
				t.Fatalf("progress=%+v", got)
			}
		})
	}
}

func TestRedisLocalRetirementAndRegression(t *testing.T) {
	info := redisLocalTestInfo(9)
	value := "ABA"
	var infoErr error
	hook := &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
		var first error
		for _, cmd := range cmds {
			switch cmd := cmd.(type) {
			case *redis.StringCmd:
				if cmd.Name() == "info" {
					cmd.SetVal(info)
					cmd.SetErr(infoErr)
					if infoErr != nil {
						first = infoErr
					}
				} else {
					cmd.SetVal(value)
				}
			case *redis.SliceCmd:
				cmd.SetVal([]any{value, value})
			}
		}
		return first
	}}
	store := redisLocalTestStore(t, hook)
	key := string(store.opts.encode(Key{"key"}))
	store.local.images[key] = redisPostimage{kind: "string", value: []byte("local"), progress: redisProgress{id: redisLocalTestID, offset: 10}}
	// Matching bytes are not progress: the replica can have an older ABA value.
	value = "local"
	if got, err := store.Get(t.Context(), Key{"key"}); err != nil || string(got) != "local" {
		t.Fatalf("pending=%q %v", got, err)
	}
	if len(store.local.images) != 1 {
		t.Fatal("retired by byte equality or received offset")
	}
	info = redisLocalTestInfo(10)
	value = "remote-newer"
	if got, err := store.Get(t.Context(), Key{"key"}); err != nil || string(got) != "remote-newer" {
		t.Fatalf("caught up=%q %v", got, err)
	}
	if len(store.local.images) != 0 || store.local.floor.offset != 10 {
		t.Fatalf("not physically retired: %+v", store.local)
	}
	for _, invalid := range []string{redisLocalTestInfo(9), strings.ReplaceAll(redisLocalTestInfo(20), redisLocalTestID, strings.Repeat("f", 40)), ""} {
		info = invalid
		value = "pre-write"
		if got, err := store.Get(t.Context(), Key{"key"}); err == nil {
			t.Fatalf("regression returned %q", got)
		}
	}
	info = redisLocalTestInfo(20)
	infoErr = errors.New("INFO denied")
	if _, err := store.Get(t.Context(), Key{"key"}); err == nil {
		t.Fatal("missing proof after retirement succeeded")
	}
	// A pending key can still satisfy a read despite a root floor proof failure.
	store.local.images[key] = redisPostimage{kind: "string", value: []byte("new-local")}
	if got, err := store.Get(t.Context(), Key{"key"}); err != nil || string(got) != "new-local" {
		t.Fatalf("pending with floor=%q %v", got, err)
	}
	results := store.BatchGet(t.Context(), []Key{{"key"}, {"key"}})
	if results[0].Err != nil || results[1].Err != nil {
		t.Fatalf("cached batch=%+v", results)
	}
	results[0].Value[0] = 'X'
	if string(results[1].Value) != "new-local" {
		t.Fatal("cached duplicate values alias")
	}
	if string(store.local.images[key].value) != "new-local" {
		t.Fatal("caller modified postimage")
	}
}

func TestRedisLocalCanceledReadsAndExpiredTombstones(t *testing.T) {
	store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if result, ok := cmd.(*redis.StringCmd); ok {
				result.SetVal("stale")
			}
		}
		return nil
	}})
	key := string(store.opts.encode(Key{"key"}))
	store.local.images[key] = redisPostimage{kind: "string", value: []byte("old"), deadline: time.Now().Add(-time.Second).UnixMilli()}
	if _, err := store.Get(t.Context(), Key{"key"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired local=%v", err)
	}
	if len(store.local.images) != 1 {
		t.Fatal("expiration discarded tombstone before replication proof")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Get(ctx, Key{"key"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	for _, result := range store.BatchGet(ctx, []Key{{"key"}, {"key"}}) {
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("batch cancel=%v", result.Err)
		}
	}
}

func TestRedisLocalRepeatedProofRetainsUncoveredImages(t *testing.T) {
	info := redisLocalTestInfo(10)
	proofs := 0
	store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if result, ok := cmd.(*redis.StringCmd); ok {
				if cmd.Name() == "info" {
					proofs++
					result.SetVal(info)
				} else {
					result.SetVal("reader")
				}
			}
		}
		return nil
	}})
	pending := redisPostimage{kind: "string", value: []byte("local"), progress: redisProgress{id: redisLocalTestID, offset: 20}}
	store.local.images["pending"] = pending
	expired := pending
	expired.deadline = time.Now().Add(-time.Second).UnixMilli()
	store.local.images["expired"] = expired
	store.local.images["unknown"] = redisPostimage{kind: "string", value: []byte("unproven")}
	for range 100 {
		if got, err := store.Get(t.Context(), Key{"pending"}); err != nil || string(got) != "local" {
			t.Fatalf("unchanged proof read=%q %v", got, err)
		}
	}
	if proofs != 100 || len(store.local.images) != 3 || store.local.floor.id != "" {
		t.Fatalf("unchanged proof attempts=%d pending=%d floor=%+v", proofs, len(store.local.images), store.local.floor)
	}
	if _, err := store.Get(t.Context(), Key{"expired"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repeated proof lost the expiration tombstone: %v", err)
	}
	info = ""
	if got, err := store.Get(t.Context(), Key{"pending"}); err != nil || string(got) != "local" {
		t.Fatalf("missing INFO read=%q %v", got, err)
	}
	info = redisLocalTestInfo(20)
	if got, err := store.Get(t.Context(), Key{"pending"}); err != nil || string(got) != "reader" {
		t.Fatalf("advanced proof read=%q %v", got, err)
	}
	if len(store.local.images) != 1 || store.local.images["unknown"].kind != "string" || store.local.floor.offset != 20 {
		t.Fatalf("covered retirement pending=%v floor=%+v", store.local.images, store.local.floor)
	}
	info = ""
	if _, err := store.Get(t.Context(), Key{"uncovered"}); err == nil {
		t.Fatal("missing current proof reused an earlier successful proof")
	}
}

func TestRedisLocalLatePublicationAtUnchangedReaderProgress(t *testing.T) {
	captured, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if result, ok := cmd.(*redis.StringCmd); ok {
				if cmd.Name() == "info" {
					result.SetVal(redisLocalTestInfo(40))
				} else {
					result.SetVal("reader")
				}
			}
		}
		return nil
	}})
	infos := 0
	store.client.AddHook(&redisLocalTestHook{
		process: func(ctx context.Context, cmd redis.Cmder) error {
			infos++
			cmd.(*redis.StringCmd).SetVal("role:master\nmaster_replid:" + redisLocalTestID + "\nmaster_repl_offset:40\n")
			if infos == 2 {
				close(captured)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
		pipeline: func(_ context.Context, cmds []redis.Cmder) error {
			for _, cmd := range cmds {
				if result, ok := cmd.(*redis.Cmd); ok {
					result.SetVal([]any{int64(1), []any{[]any{int64(1), "string", int64(0), "committed"}}})
				}
			}
			return nil
		},
	})
	store.local.images["seed"] = redisPostimage{kind: "string", progress: redisProgress{id: redisLocalTestID, offset: 10}}
	done := make(chan error, 1)
	go func() { done <- store.Set(t.Context(), Key{"late"}, []byte("committed")) }()
	select {
	case <-captured:
	case <-time.After(3 * time.Second):
		t.Fatal("writer did not capture its postimage")
	}
	// The independent read sweeps position 40 before the writer publishes its
	// already captured image at that same position.
	if _, err := store.Get(t.Context(), Key{"probe"}); err != nil {
		t.Fatal(err)
	}
	if len(store.local.images) != 0 || store.local.floor.offset != 10 {
		t.Fatalf("initial sweep pending=%v floor=%+v", store.local.images, store.local.floor)
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, ok := store.local.images["late"]; !ok {
		t.Fatal("writer did not publish its captured image")
	}
	if _, err := store.Get(t.Context(), Key{"probe"}); err != nil {
		t.Fatal(err)
	}
	if len(store.local.images) != 0 || store.local.floor.offset != 40 {
		t.Fatalf("unchanged proof stranded late publication: pending=%v floor=%+v", store.local.images, store.local.floor)
	}
}

func TestRedisLocalRetirementRechecksFullLineageProof(t *testing.T) {
	nextID := strings.Repeat("a", 40)
	for _, scenario := range []string{"previous_identity", "previous_offset"} {
		t.Run(scenario, func(t *testing.T) {
			local := &redisLocal{
				images: map[string]redisPostimage{"inherited": {progress: redisProgress{id: redisLocalTestID, offset: 20}}},
				floor:  redisProgress{id: nextID, offset: 5},
			}
			progress := redisProgress{id: nextID, offset: 30, previousID: redisLocalTestID, previousOffset: 21}
			if scenario == "previous_identity" {
				progress.previousID = strings.Repeat("b", 40)
			} else {
				progress.previousOffset = 20
			}
			if !local.retire(progress) || len(local.images) != 1 {
				t.Fatal("incomplete lineage proof retired an uncovered image")
			}
			progress.previousID, progress.previousOffset = redisLocalTestID, 21
			if !local.retire(progress) || len(local.images) != 0 || local.floor.id != nextID || local.floor.offset != 20 {
				t.Fatalf("changed lineage at the same offset was ignored: pending=%v floor=%+v", local.images, local.floor)
			}
			if local.retire(redisProgress{}) || local.floor.id != nextID || local.floor.offset != 20 {
				t.Fatal("missing INFO bypassed or changed the required floor")
			}
		})
	}
}

func BenchmarkRedisLocalRetirementUnchangedProgress(b *testing.B) {
	for _, known := range []bool{false, true} {
		b.Run(fmt.Sprintf("known=%v", known), func(b *testing.B) {
			local := &redisLocal{images: make(map[string]redisPostimage, 100000)}
			for i := range 100000 {
				local.images[fmt.Sprint(i)] = redisPostimage{progress: redisProgress{id: redisLocalTestID, offset: 20}}
			}
			var progress redisProgress
			if known {
				progress = redisProgress{id: redisLocalTestID, offset: 10}
			}
			local.retire(progress)
			b.ResetTimer()
			for b.Loop() {
				if !local.retire(progress) {
					b.Fatal("unexpected floor rejection")
				}
			}
		})
	}
}

func TestRedisLocalIndependentKeysAndSameKeyPublication(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})
	sameStarted := make(chan struct{})
	sameNetwork := make(chan struct{}, 1)
	var once sync.Once
	store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if result, ok := cmd.(*redis.StringCmd); ok {
				result.SetVal("")
			}
		}
		return nil
	}})
	store.client.AddHook(&redisLocalTestHook{process: func(_ context.Context, cmd redis.Cmder) error { cmd.(*redis.StringCmd).SetVal(""); return nil }, pipeline: func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			switch result := cmd.(type) {
			case *redis.StringCmd:
				result.SetVal("") // unavailable proof must not fail writes
			case *redis.Cmd:
				args := result.Args()
				key := args[3].(string)
				value := args[5].([]byte)
				if key == "held" && string(value) == "first" {
					once.Do(func() { close(blocked) })
					select {
					case <-unblock:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				if key == "held" && string(value) == "second" {
					sameNetwork <- struct{}{}
				}
				result.SetVal([]any{int64(1), []any{[]any{int64(1), "string", int64(0), string(value)}}})
			}
		}
		return nil
	}})
	first := make(chan error, 1)
	go func() { first <- store.Set(t.Context(), Key{"held"}, []byte("first")) }()
	select {
	case <-blocked:
	case <-time.After(3 * time.Second):
		t.Fatal("first did not block")
	}
	second := make(chan error, 1)
	go func() { close(sameStarted); second <- store.Set(t.Context(), Key{"held"}, []byte("second")) }()
	<-sameStarted
	independent := make(chan error, 1)
	go func() { independent <- store.Set(t.Context(), Key{"free"}, []byte("independent")) }()
	select {
	case err := <-independent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		close(unblock)
		t.Fatal("independent key stalled")
	}
	// Acquiring the held key with an already canceled context gives a deterministic
	// same-resource control without relying on scheduler timing.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.Set(ctx, Key{"held"}, []byte("canceled")); !errors.Is(err, context.Canceled) {
		t.Fatalf("same-key cancel=%v", err)
	}
	select {
	case <-sameNetwork:
		close(unblock)
		t.Fatal("same key entered IO concurrently")
	default:
	}
	close(unblock)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if got := store.local.images["held"].value; string(got) != "second" {
		t.Fatalf("publication=%q", got)
	}
	if store.local.locks.ActiveKeys() != 0 {
		t.Fatal("idle keyed coordination leaked")
	}
}

func TestRedisLocalCollectionsTypesAndPrimaryBypass(t *testing.T) {
	store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			switch result := cmd.(type) {
			case *redis.StringCmd:
				result.SetVal("")
			case *redis.StringSliceCmd:
				result.SetVal(nil)
			case *redis.BoolCmd:
				result.SetVal(false)
			}
		}
		return nil
	}})
	store.local.images["set"] = redisPostimage{kind: "set", members: []string{"a", "b"}}
	store.local.images["ordered"] = redisPostimage{kind: "zset", members: []string{"a", "b", "c"}}
	if _, err := store.Get(t.Context(), Key{"set"}); !errors.Is(err, ErrWrongType) {
		t.Fatalf("Get set=%v", err)
	}
	if got, err := store.HasMember(t.Context(), Key{"set"}, "b"); err != nil || !got {
		t.Fatalf("Has=%v %v", got, err)
	}
	if _, err := store.ListMembers(t.Context(), Key{"ordered"}); !errors.Is(err, ErrWrongType) {
		t.Fatalf("List ordered=%v", err)
	}
	members, err := store.RangeOrderedMembers(t.Context(), Key{"ordered"}, OrderedRange{After: new("a"), Before: new("c"), Limit: 1})
	if err != nil || !slices.Equal(members, []string{"b"}) {
		t.Fatalf("range=%v %v", members, err)
	}
	if _, err := store.RangeOrderedMembers(t.Context(), Key{"set"}, OrderedRange{Limit: 1}); !errors.Is(err, ErrWrongType) {
		t.Fatalf("range set=%v", err)
	}
	store.client.AddHook(&redisLocalTestHook{process: func(_ context.Context, cmd redis.Cmder) error { cmd.(*redis.StringCmd).SetVal("primary"); return nil }})
	if got, err := store.Get(WithPrimaryReads(t.Context()), Key{"set"}); err != nil || string(got) != "primary" {
		t.Fatalf("primary bypass=%q %v", got, err)
	}
}

// This opt-in test requires real Redis replication. Fake command hooks above
// validate coordination and proof handling, not replication behavior.
func TestRedisLocalRealReplication(t *testing.T) {
	writeURL, readURL := os.Getenv("GIZCLAW_TEST_REDIS_DSN"), os.Getenv("GIZCLAW_TEST_REDIS_READ_DSN")
	if writeURL == "" || readURL == "" {
		t.Skip("real Redis primary/replica DSNs are not configured")
	}
	connect := func(dsn string) *redis.Client {
		opts, err := redis.ParseURL(dsn)
		if err != nil {
			t.Fatal(err)
		}
		opts.ContextTimeoutEnabled = true
		client := redis.NewClient(opts)
		t.Cleanup(func() { _ = client.Close() })
		return client
	}
	writer, reader := connect(writeURL), connect(readURL)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	store, err := NewRedisReadWriteWithClients(reader, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	observer, err := NewRedisReadWriteWithClients(reader, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "redis-local-" + rand.Text()
	key := func(name string) Key { return Key{prefix, name} }
	encoded := func(name string) string { return string(store.opts.encode(key(name))) }
	names := []string{"record", "set", "ordered", "deleted", "expired", "guard", "created", "all-a", "all-b", "other", "barrier"}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = reader.Do(ctx, "CLIENT", "UNPAUSE").Err()
		keys := []string{}
		for _, name := range names {
			keys = append(keys, encoded(name))
		}
		_ = writer.Del(ctx, keys...).Err()
	})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	wait := func() {
		t.Helper()
		marker := rand.Text()
		must(writer.Set(ctx, encoded("barrier"), marker, 0).Err())
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			v, err := reader.Get(ctx, encoded("barrier")).Result()
			if err == nil && v == marker {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal("replica did not catch up")
			case <-ticker.C:
			}
		}
	}
	must(writer.Set(ctx, encoded("record"), "old", 0).Err())
	must(writer.Set(ctx, encoded("deleted"), "old", 0).Err())
	must(writer.SAdd(ctx, encoded("set"), "preexisting", "remove").Err())
	must(writer.ZAdd(ctx, encoded("ordered"), redis.Z{Member: "a"}, redis.Z{Member: "c"}).Err())
	wait()
	must(reader.Do(ctx, "CLIENT", "PAUSE", 15000, "WRITE").Err())
	must(store.Set(ctx, key("record"), []byte{0, 255, 'x'}))
	if got, err := store.Get(ctx, key("record")); err != nil || !slices.Equal(got, []byte{0, 255, 'x'}) {
		t.Fatalf("local Set=%q %v", got, err)
	}
	if got, err := observer.Get(ctx, key("record")); err != nil || string(got) != "old" {
		t.Fatalf("cross-instance stale=%q %v", got, err)
	}
	must(store.Delete(ctx, key("deleted")))
	if _, err := store.Get(ctx, key("deleted")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete=%v", err)
	}
	expiry := time.Now().Add(100 * time.Millisecond)
	must(store.BatchSet(ctx, []Entry{{Key: key("expired"), Value: []byte("expires"), Deadline: expiry}, {Key: key("other"), Value: []byte("batch")}}))
	if got := store.local.images[encoded("expired")].deadline; got != expiry.UnixMilli() {
		t.Fatalf("absolute batch deadline=%d want %d", got, expiry.UnixMilli())
	}
	must(store.BatchDelete(ctx, []Key{key("other")}))
	if _, err := store.Get(ctx, key("other")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("batch delete=%v", err)
	}
	must(store.AddMembers(ctx, key("set"), "new"))
	must(store.RemoveMembers(ctx, key("set"), "remove"))
	members, err := store.ListMembers(ctx, key("set"))
	slices.Sort(members)
	if err != nil || !slices.Equal(members, []string{"new", "preexisting"}) {
		t.Fatalf("complete postimage=%v %v", members, err)
	}
	matched, err := store.ApplyMutation(ctx, Mutation{Conditions: []Condition{{Key: key("record"), Expected: []byte{0, 255, 'x'}}}, Entries: []Entry{{Key: key("record"), Value: []byte("applied")}}, AddOrderedMembers: []SetMembers{{Key: key("ordered"), Members: []string{"b"}}}, RemoveOrderedMembers: []SetMembers{{Key: key("ordered"), Members: []string{"c"}}}})
	if err != nil || !matched {
		t.Fatalf("Apply=%v %v", matched, err)
	}
	members, err = store.RangeOrderedMembers(ctx, key("ordered"), OrderedRange{After: new("a"), Limit: 1})
	if err != nil || !slices.Equal(members, []string{"b"}) {
		t.Fatalf("ordered=%v %v", members, err)
	}
	guardExpiry := time.Now().Add(5 * time.Second)
	guard := Entry{Key: key("guard"), Value: []byte("claim"), Deadline: guardExpiry}
	_, created, err := store.CreateIfAbsent(ctx, guard, []Entry{{Key: key("created"), Value: []byte("created")}})
	if err != nil || !created {
		t.Fatalf("create=%v %v", created, err)
	}
	_, _, created, err = store.CreateIfAllAbsent(ctx, []Entry{{Key: key("all-a"), Value: []byte("a")}, {Key: key("all-b"), Value: []byte("b")}}, nil)
	if err != nil || !created {
		t.Fatalf("all-create=%v %v", created, err)
	}
	matched, err = store.CompareAndMutate(ctx, guard.Key, guard.Value, []Entry{{Key: key("created"), Value: []byte("cas-related")}}, nil)
	if err != nil || !matched {
		t.Fatalf("unchanged guard CAS=%v %v", matched, err)
	}
	if got := store.local.images[encoded("guard")].deadline; got != guardExpiry.UnixMilli() {
		t.Fatalf("unchanged guard deadline=%d want %d", got, guardExpiry.UnixMilli())
	}
	matched, err = store.CompareAndMutate(ctx, guard.Key, guard.Value, []Entry{{Key: guard.Key, Value: []byte("cas")}}, nil)
	if err != nil || !matched {
		t.Fatalf("CAS=%v %v", matched, err)
	}
	_, created, err = store.CreateIfAbsent(ctx, Entry{Key: guard.Key, Value: []byte("bad")}, []Entry{{Key: key("record"), Value: []byte("bad")}})
	if err != nil || created {
		t.Fatalf("failed create=%v %v", created, err)
	}
	matched, err = store.CompareAndMutate(ctx, guard.Key, []byte("claim"), []Entry{{Key: key("record"), Value: []byte("bad")}}, nil)
	if err != nil || matched {
		t.Fatalf("failed CAS=%v %v", matched, err)
	}
	matched, err = store.ApplyMutation(ctx, Mutation{Conditions: []Condition{{Key: key("record"), Expected: []byte("old")}}, Entries: []Entry{{Key: key("record"), Value: []byte("bad")}}})
	if err != nil || matched {
		t.Fatalf("failed Apply=%v %v", matched, err)
	}
	if got, err := store.Get(ctx, key("record")); err != nil || string(got) != "applied" {
		t.Fatalf("failed mutation published=%q %v", got, err)
	}
	if _, err := store.Get(ctx, key("set")); !errors.Is(err, ErrWrongType) {
		t.Fatalf("Get set=%v", err)
	}
	matched, err = store.ApplyMutation(ctx, Mutation{Entries: []Entry{{Key: key("record"), Value: []byte("bad")}}, AddMembers: []SetMembers{{Key: key("ordered"), Members: []string{"wrong-type"}}}})
	if !errors.Is(err, ErrWrongType) || matched {
		t.Fatalf("wrongtype Apply=%v %v", matched, err)
	}
	matched, err = store.ApplyMutation(ctx, Mutation{Entries: []Entry{{Key: key("record"), Value: []byte("bad")}}, AddMembers: []SetMembers{{Key: key("set"), Members: []string{"over-limit"}, MaxMembers: 1}}})
	if !errors.Is(err, ErrMemberLimit) || matched {
		t.Fatalf("limit Apply=%v %v", matched, err)
	}
	results := store.BatchGet(ctx, []Key{key("record"), key("record"), key("set"), key("ordered"), key("deleted")})
	if results[0].Err != nil || results[1].Err != nil {
		t.Fatalf("batch=%+v", results)
	}
	results[0].Value[0] = 'X'
	if string(results[1].Value) != "applied" {
		t.Fatal("batch ownership")
	}
	for _, result := range results[2:] {
		if !errors.Is(result.Err, ErrNotFound) {
			t.Fatalf("batch type/tombstone=%v", result.Err)
		}
	}
	// A different writer is visible only to explicit primary reads while pending.
	must(writer.Set(ctx, encoded("record"), "external", 0).Err())
	if got, err := store.Get(WithPrimaryReads(ctx), key("record")); err != nil || string(got) != "external" {
		t.Fatalf("primary bypass=%q %v", got, err)
	}
	if got, err := store.Get(ctx, key("record")); err != nil || string(got) != "applied" {
		t.Fatalf("ordinary postimage=%q %v", got, err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := store.Get(ctx, key("expired"))
		if errors.Is(err, ErrNotFound) {
			break
		}
		must(err)
		select {
		case <-ctx.Done():
			t.Fatal("local deadline did not expire")
		case <-ticker.C:
		}
	}
	if _, ok := store.local.images[encoded("expired")]; !ok {
		t.Fatal("lost expiry tombstone while replica lagged")
	}
	if len(store.local.images) == 0 {
		t.Fatal("lagging replica retired pending writes")
	}
	must(reader.Do(ctx, "CLIENT", "UNPAUSE").Err())
	wait()
	if got, err := store.Get(ctx, key("record")); err != nil || string(got) != "external" {
		t.Fatalf("caught-up external=%q %v", got, err)
	}
	if _, ok := store.local.images[encoded("record")]; ok {
		t.Fatal("caught-up postimage not retired")
	}
	if store.local.floor.id == "" {
		t.Fatal("retirement did not establish replication floor")
	}
	for _, name := range names {
		_, _ = store.Get(ctx, key(name))
	}
	if len(store.local.images) != 0 {
		t.Fatalf("caught-up images retained=%d", len(store.local.images))
	}
}

func TestRedisLocalKnownReplicationLineage(t *testing.T) {
	nextID := strings.Repeat("a", 40)
	info := strings.ReplaceAll(redisLocalTestInfo(30), redisLocalTestID, nextID) + "master_replid2:" + redisLocalTestID + "\nsecond_repl_offset:21\n"
	store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if result, ok := cmd.(*redis.StringCmd); ok {
				if cmd.Name() == "info" {
					result.SetVal(info)
				} else {
					result.SetVal("inherited")
				}
			}
		}
		return nil
	}})
	store.local.floor = redisProgress{id: redisLocalTestID, offset: 10}
	store.local.images["key"] = redisPostimage{kind: "string", value: []byte("local"), progress: redisProgress{id: redisLocalTestID, offset: 20}}
	if got, err := store.Get(t.Context(), Key{"key"}); err != nil || string(got) != "inherited" {
		t.Fatalf("known lineage=%q %v", got, err)
	}
	if len(store.local.images) != 0 || store.local.floor.id != nextID || store.local.floor.offset != 20 {
		t.Fatalf("lineage floor=%+v pending=%v", store.local.floor, store.local.images)
	}
	// The old ID is not valid at the exclusive boundary itself.
	progress := parseRedisProgress(info, false)
	if progress.covers(redisProgress{id: redisLocalTestID, offset: 21}) {
		t.Fatal("accepted history beyond promotion boundary")
	}
}

func TestRedisLocalOldInflightReadAfterRetirement(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached=%v", cached), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(ctx context.Context, cmds []redis.Cmder) error {
				old := false
				for _, cmd := range cmds {
					if cmd.Name() == "get" && cmd.Args()[1] == "old" {
						old = true
					}
				}
				if old {
					close(started)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				for _, cmd := range cmds {
					if result, ok := cmd.(*redis.StringCmd); ok {
						if cmd.Name() == "info" {
							offset := int64(10)
							if old {
								offset = 9
							}
							result.SetVal(redisLocalTestInfo(offset))
						} else {
							result.SetVal("stale")
						}
					}
				}
				return nil
			}})
			image := redisPostimage{kind: "string", value: []byte("local"), progress: redisProgress{id: redisLocalTestID, offset: 10}}
			store.local.images["new"] = image
			if cached {
				store.local.images["old"] = image
			}
			type result struct {
				value []byte
				err   error
			}
			done := make(chan result, 1)
			go func() { value, err := store.Get(t.Context(), Key{"old"}); done <- result{value, err} }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("old reader did not start")
			}
			if _, err := store.Get(t.Context(), Key{"new"}); err != nil {
				close(release)
				t.Fatal(err)
			}
			// The other-key sweep retires even the blocked reader's requested image.
			if len(store.local.images) != 0 {
				close(release)
				t.Fatal("global sweep retained covered images")
			}
			close(release)
			got := <-done
			if cached {
				if got.err != nil || string(got.value) != "local" {
					t.Fatalf("pinned local write unavailable after sweep: %q %v", got.value, got.err)
				}
			} else if got.err == nil {
				t.Fatal("uncovered in-flight read returned data behind retired floor")
			}
		})
	}
}

func TestRedisLocalMutationPostimages(t *testing.T) {
	for _, matched := range []bool{false, true} {
		t.Run(fmt.Sprintf("matched=%v", matched), func(t *testing.T) {
			store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
				for _, cmd := range cmds {
					switch result := cmd.(type) {
					case *redis.StringCmd:
						result.SetVal("")
					case *redis.StringSliceCmd:
						result.SetVal(nil)
					}
				}
				return nil
			}})
			infos := 0
			store.client.AddHook(&redisLocalTestHook{
				process: func(_ context.Context, cmd redis.Cmder) error {
					infos++
					cmd.(*redis.StringCmd).SetVal("")
					return nil
				},
				pipeline: func(_ context.Context, cmds []redis.Cmder) error {
					for _, cmd := range cmds {
						if cmd.Name() == "eval" {
							status, images := int64(0), []any{}
							if matched {
								status = 1
								images = []any{[]any{int64(2), "zset", int64(0), []any{"c", "a", "b"}}}
							}
							cmd.(*redis.Cmd).SetVal([]any{status, images})
						}
					}
					return nil
				},
			})
			got, err := store.ApplyMutation(t.Context(), Mutation{Conditions: []Condition{{Key: Key{"guard"}}}, AddOrderedMembers: []SetMembers{{Key: Key{"ordered"}, Members: []string{"b"}}}})
			if err != nil || got != matched {
				t.Fatalf("mutation=%v %v", got, err)
			}
			wantInfos := 1
			if matched {
				wantInfos = 2
			}
			if infos != wantInfos || len(store.local.images) != wantInfos-1 {
				t.Fatalf("INFO requests=%d postimages=%d", infos, len(store.local.images))
			}
			if matched {
				for _, query := range []OrderedRange{{After: new("a"), Before: new("c"), Limit: 1}, {After: new("aa"), Before: new("c"), Limit: 10}} {
					members, err := store.RangeOrderedMembers(t.Context(), Key{"ordered"}, query)
					if err != nil || !slices.Equal(members, []string{"b"}) {
						t.Fatalf("range=%v %v", members, err)
					}
					members[0] = "changed by caller"
				}
			}
		})
	}
}

func TestRedisLocalOptionalWriterInfoFailureDoesNotReplay(t *testing.T) {
	store := redisLocalTestStore(t, &redisLocalTestHook{pipeline: func(_ context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if result, ok := cmd.(*redis.StringCmd); ok {
				result.SetVal("")
			}
		}
		return nil
	}})
	infos, evals := 0, 0
	store.client.AddHook(&redisLocalTestHook{
		process: func(_ context.Context, cmd redis.Cmder) error {
			if cmd.Name() != "info" {
				t.Fatalf("unexpected writer request %s", cmd.Name())
			}
			infos++
			if infos == 1 {
				cmd.(*redis.StringCmd).SetVal("role:master\nmaster_replid:" + redisLocalTestID + "\nmaster_repl_offset:10\n")
				return nil
			}
			cmd.SetErr(io.EOF)
			return io.EOF
		},
		pipeline: func(_ context.Context, cmds []redis.Cmder) error {
			for _, cmd := range cmds {
				if cmd.Name() == "multi" || cmd.Name() == "exec" {
					continue
				}
				if cmd.Name() != "eval" {
					t.Fatalf("optional metadata joined mutation transaction: %s", cmd.Name())
				}
				evals++
				cmd.(*redis.Cmd).SetVal([]any{int64(1), []any{[]any{int64(1), "string", int64(0), "committed"}}})
			}
			return nil
		},
	})
	if err := store.Set(t.Context(), Key{"key"}, []byte("committed")); err != nil {
		t.Fatalf("optional INFO turned commit into failure: %v", err)
	}
	if evals != 1 || infos != 2 {
		t.Fatalf("requests: EVAL=%d INFO=%d", evals, infos)
	}
	if store.local.images["key"].progress.id != "" {
		t.Fatal("failed INFO published progress proof")
	}
	if got, err := store.Get(t.Context(), Key{"key"}); err != nil || string(got) != "committed" {
		t.Fatalf("retained successful write=%q %v", got, err)
	}
}

func TestRedisLocalRealMetadataDeniedAndDelayedExpiry(t *testing.T) {
	writeURL, readURL := os.Getenv("GIZCLAW_TEST_REDIS_DSN"), os.Getenv("GIZCLAW_TEST_REDIS_READ_DSN")
	if writeURL == "" || readURL == "" {
		t.Skip("real Redis primary/replica DSNs are not configured")
	}
	opts, err := redis.ParseURL(writeURL)
	if err != nil {
		t.Fatal(err)
	}
	readerOpts, err := redis.ParseURL(readURL)
	if err != nil {
		t.Fatal(err)
	}
	primary, reader := redis.NewClient(opts), redis.NewClient(readerOpts)
	t.Cleanup(func() { _ = primary.Close(); _ = reader.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	key := "redis-local-delay-" + rand.Text()
	user, password := "redis-local-"+rand.Text(), rand.Text()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = primary.Del(ctx, key).Err()
		_ = primary.Do(ctx, "ACL", "DELUSER", user).Err()
	})
	// INFO is optional observability; denying it cannot reject committed writes.
	if err := primary.Do(ctx, "ACL", "SETUSER", user, "on", ">"+password, "+@all", "-info", "~*").Err(); err != nil {
		t.Fatal(err)
	}
	deniedOpts := *opts
	deniedOpts.Username = user
	deniedOpts.Password = password
	denied := redis.NewClient(&deniedOpts)
	t.Cleanup(func() { _ = denied.Close() })
	store, err := NewRedisReadWriteWithClients(reader, denied, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, Key{key}, []byte("acknowledged")); err != nil {
		t.Fatalf("INFO-denied successful write=%v", err)
	}
	if store.local.images[key].progress.id != "" {
		t.Fatal("denied INFO produced proof")
	}
	if got, err := store.Get(ctx, Key{key}); err != nil || string(got) != "acknowledged" {
		t.Fatalf("INFO-denied local read=%q %v", got, err)
	}
	if _, ok := store.local.images[key]; !ok {
		t.Fatal("unproven write was retired")
	}
	// Deliberately run a long Lua section between the business PXAT write and
	// postimage capture. PEXPIRETIME must retain the exact absolute deadline.
	source := redisMutationScript.postimage
	source = strings.Replace(source, "local images = {}", `local total = 0
 for i=1,10000000 do total = total + i end
 local images = {}`, 1)
	deadline := time.Now().Add(5 * time.Second).UnixMilli()
	started := time.Now()
	raw, err := primary.Eval(ctx, source, []string{key}, 1, "exact", deadline).Slice()
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 20*time.Millisecond {
		t.Fatal("script delay did not execute")
	}
	images := raw[1].([]any)
	row := images[0].([]any)
	if got := row[2].(int64); got != deadline {
		t.Fatalf("delayed absolute expiry=%d want %d", got, deadline)
	}
	if got, err := primary.Do(ctx, "PEXPIRETIME", key).Int64(); err != nil || got != deadline {
		t.Fatalf("physical absolute expiry=%d %v", got, err)
	}
}
