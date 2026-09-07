//go:build store_e2e

package store_e2e_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestRedisKV(t *testing.T) {
	dsn := requiredEnvironment(t, "GIZCLAW_TEST_REDIS_DSN")
	firstPhysical, err := storage.New(map[string]storage.Config{"redis": storage.RedisConfig{URL: dsn}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = firstPhysical.Close() })
	secondPhysical, err := storage.New(map[string]storage.Config{"redis": storage.RedisConfig{URL: dsn}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondPhysical.Close() })
	firstClient, err := firstPhysical.Redis("redis")
	if err != nil {
		t.Fatal(err)
	}
	secondClient, err := secondPhysical.Redis("redis")
	if err != nil {
		t.Fatal(err)
	}
	firstRoot, err := kv.NewRedisWithClient(firstClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, err := kv.NewRedisWithClient(secondClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := kv.Key{"gizclaw-store-e2e", fmt.Sprintf("%d", time.Now().UnixNano())}
	first := kv.Prefixed(firstRoot, prefix)
	second := kv.Prefixed(secondRoot, prefix)
	t.Cleanup(func() {
		_ = first.BatchDelete(context.Background(), []kv.Key{
			{"empty"}, {"ordered"}, {"ordered-other"},
			{"mutation", "set"}, {"mutation", "delete-wins"},
			{"deadline"}, {"expired"}, {"invalid-deadline"}, {"cancelled"},
			{"race", "create"}, {"race", "all-a"}, {"race", "all-b"}, {"race", "compare"},
		})
	})

	ctx := t.Context()
	if err := first.Set(ctx, kv.Key{"empty"}, nil); err != nil {
		t.Fatal(err)
	}
	if value, err := second.Get(ctx, kv.Key{"empty"}); err != nil || value == nil || len(value) != 0 {
		t.Fatalf("cross-client zero value = %#v, %v", value, err)
	}
	if err := first.Set(ctx, kv.Key{"ordered-other"}, []byte("other")); err != nil {
		t.Fatal(err)
	}
	if ok, err := first.ApplyMutation(ctx, kv.Mutation{AddOrderedMembers: []kv.SetMembers{{Key: kv.Key{"ordered"}, Members: []string{"c", "a", "b"}}}}); err != nil || !ok {
		t.Fatalf("publish ordered collection: %v, %v", ok, err)
	}
	page, err := second.RangeOrderedMembers(ctx, kv.Key{"ordered"}, kv.OrderedRange{After: new("a"), Limit: 1})
	if err != nil || !slices.Equal(page, []string{"b"}) {
		t.Fatalf("cross-client ordered page = %v, %v", page, err)
	}

	if err := first.BatchMutate(ctx,
		[]kv.Entry{{Key: kv.Key{"mutation", "set"}, Value: []byte("value")}, {Key: kv.Key{"mutation", "delete-wins"}, Value: []byte("value")}},
		[]kv.Key{{"mutation", "delete-wins"}},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Get(ctx, kv.Key{"mutation", "delete-wins"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("delete-wins error = %v", err)
	}

	deadlineKey := kv.Key{"deadline"}
	if err := first.BatchSet(ctx, []kv.Entry{{Key: deadlineKey, Value: []byte("value"), Deadline: time.Now().Add(150 * time.Millisecond)}}); err != nil {
		t.Fatal(err)
	}
	if err := second.Set(ctx, deadlineKey, []byte("persistent")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if value, err := first.Get(ctx, deadlineKey); err != nil || string(value) != "persistent" {
		t.Fatalf("expiration removal Get() = %q, %v", value, err)
	}
	if err := first.BatchSet(ctx, []kv.Entry{{Key: kv.Key{"expired"}, Value: []byte("gone"), Deadline: time.Now().Add(100 * time.Millisecond)}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := second.Get(ctx, kv.Key{"expired"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expired Get() error = %v", err)
	}
	if err := first.BatchSet(ctx, []kv.Entry{{Key: kv.Key{"invalid-deadline"}, Deadline: time.Now().Add(-time.Second)}}); !errors.Is(err, kv.ErrInvalidDeadline) {
		t.Fatalf("past deadline error = %v", err)
	}

	assertRedisCreateWinner(t, first, second)
	assertRedisCreateAllWinner(t, first, second)
	assertRedisCompareWinner(t, first, second)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := first.Set(cancelled, kv.Key{"cancelled"}, []byte("value")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Set() error = %v", err)
	}
	if _, err := first.Get(ctx, kv.Key{"cancelled"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("cancelled Set() wrote value: %v", err)
	}
}

func assertRedisCreateWinner(t *testing.T, stores ...kv.Store) {
	t.Helper()
	results := runRedisRace(t, len(stores), func(index int) (bool, error) {
		_, created, err := kv.CreateIfAbsent(t.Context(), stores[index], kv.Entry{Key: kv.Key{"race", "create"}, Value: fmt.Appendf(nil, "value-%d", index)}, nil)
		return created, err
	})
	assertOneRedisWinner(t, results)
}

func assertRedisCreateAllWinner(t *testing.T, stores ...kv.Store) {
	t.Helper()
	results := runRedisRace(t, len(stores), func(index int) (bool, error) {
		_, _, created, err := kv.CreateIfAllAbsent(t.Context(), stores[index], []kv.Entry{
			{Key: kv.Key{"race", "all-a"}, Value: fmt.Appendf(nil, "a-%d", index)},
			{Key: kv.Key{"race", "all-b"}, Value: fmt.Appendf(nil, "b-%d", index)},
		}, nil)
		return created, err
	})
	assertOneRedisWinner(t, results)
}

func assertRedisCompareWinner(t *testing.T, stores ...kv.Store) {
	t.Helper()
	guard := kv.Key{"race", "compare"}
	if err := stores[0].Set(t.Context(), guard, []byte("before")); err != nil {
		t.Fatal(err)
	}
	results := runRedisRace(t, len(stores), func(index int) (bool, error) {
		return kv.CompareAndMutate(t.Context(), stores[index], guard, []byte("before"), []kv.Entry{{Key: guard, Value: fmt.Appendf(nil, "after-%d", index)}}, nil)
	})
	assertOneRedisWinner(t, results)
}

type redisRaceResult struct {
	won bool
	err error
}

func runRedisRace(t *testing.T, count int, operation func(int) (bool, error)) []redisRaceResult {
	t.Helper()
	start := make(chan struct{})
	results := make([]redisRaceResult, count)
	var workers sync.WaitGroup
	for index := range count {
		workers.Go(func() {
			<-start
			results[index].won, results[index].err = operation(index)
		})
	}
	close(start)
	workers.Wait()
	return results
}

func assertOneRedisWinner(t *testing.T, results []redisRaceResult) {
	t.Helper()
	winners := 0
	for _, result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.won {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("race winners = %d, want 1", winners)
	}
}

func TestRedisStoreRegistryPrefixesAndLifecycle(t *testing.T) {
	dsn := requiredEnvironment(t, "GIZCLAW_TEST_REDIS_DSN")
	physical, err := storage.New(map[string]storage.Config{"redis": storage.RedisConfig{URL: dsn}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	prefix := fmt.Sprintf("registry-%d", time.Now().UnixNano())
	registry, err := stores.New(map[string]stores.Config{
		"friends": {Kind: stores.KindKeyValue, Storage: "redis", Prefix: prefix + "/friends"},
		"groups":  {Kind: stores.KindKeyValue, Storage: "redis", Prefix: prefix + "/groups"},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	friendStore, _ := registry.KV("friends")
	groupStore, _ := registry.KV("groups")
	if _, _, ok := kv.SharedAtomicStore(friendStore, groupStore); !ok {
		t.Fatal("Redis logical Stores do not share one atomic root")
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	client, err := physical.Redis("redis")
	if err != nil || client.Ping(t.Context()).Err() != nil {
		t.Fatalf("logical Close() closed physical Redis client: %v", err)
	}
	for name, configs := range map[string]map[string]stores.Config{
		"empty": {
			"a": {Kind: stores.KindKeyValue, Storage: "redis"},
		},
		"duplicate": {
			"a": {Kind: stores.KindKeyValue, Storage: "redis", Prefix: prefix + "/same"},
			"b": {Kind: stores.KindKeyValue, Storage: "redis", Prefix: prefix + "/same"},
		},
		"overlap": {
			"a": {Kind: stores.KindKeyValue, Storage: "redis", Prefix: prefix + "/parent"},
			"b": {Kind: stores.KindKeyValue, Storage: "redis", Prefix: prefix + "/parent/child"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := stores.New(configs, physical); err == nil || !strings.Contains(err.Error(), "prefix") {
				t.Fatalf("stores.New() error = %v, want prefix validation", err)
			}
		})
	}
}
