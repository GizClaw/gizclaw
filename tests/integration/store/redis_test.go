//go:build store_e2e

package store_e2e_test

import (
	"fmt"
	"strings"
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
	assertSharedKV(t, first, second)
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
