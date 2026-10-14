package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func redisMigrationTestConfig() ConfigFile {
	return ConfigFile{
		Storage: map[string]storageFileConfig{
			"old":       {Kind: storage.KindRedisReadWrite, ReadURL: "redis://stale.invalid:6379/2", WriteURL: "redis://primary.invalid:6379/2"},
			"new":       {Kind: storage.KindTiKV, PDAddresses: []string{"pd.invalid:2379"}},
			"unrelated": {Kind: "not-a-real-backend"},
		},
		Stores: map[string]storeFileConfig{
			"peers":    {Kind: stores.KindKeyValue, Storage: "old", Prefix: "peers"},
			"friends":  {Kind: stores.KindKeyValue, Storage: "new", Prefix: "social/friends"},
			"business": {Kind: stores.KindSQL, Storage: "unrelated"},
		},
	}
}

func TestRedisMigrationConfigPrimaryAndCutoverBindings(t *testing.T) {
	configs, prefixes, err := redisMigrationConfig(redisMigrationTestConfig(), "old", "new", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 2 || configs["old"].(storage.RedisConfig).URL != "redis://primary.invalid:6379/2" {
		t.Fatalf("opened reader or unrelated connector: %#v", configs)
	}
	if !reflect.DeepEqual(prefixes, []kv.Key{{"social", "friends"}, {"peers"}}) {
		t.Fatalf("prefix mapping: %+v", prefixes)
	}
	_, prefixes, err = redisMigrationConfig(redisMigrationTestConfig(), "old", "new", []string{"peers"})
	if err != nil || !reflect.DeepEqual(prefixes, []kv.Key{{"peers"}}) {
		t.Fatalf("selected stores: %+v %v", prefixes, err)
	}
}

func TestRedisMigrationConfigRejectsUnsafeScope(t *testing.T) {
	for _, prefix := range []string{"", "/peers", "peers/../routes", "peers//routes", "peers:routes", "peers\\routes"} {
		t.Run(prefix, func(t *testing.T) {
			cfg := redisMigrationTestConfig()
			cfg.Stores["peers"] = storeFileConfig{Kind: stores.KindKeyValue, Storage: "old", Prefix: prefix}
			if _, _, err := redisMigrationConfig(cfg, "old", "new", nil); err == nil {
				t.Fatal("accepted unsafe prefix")
			}
		})
	}
	cfg := redisMigrationTestConfig()
	cfg.Stores["nested"] = storeFileConfig{Kind: stores.KindKeyValue, Storage: "old", Prefix: "peers/nested"}
	if _, _, err := redisMigrationConfig(cfg, "old", "new", nil); err == nil {
		t.Fatal("accepted overlapping namespaces")
	}
	for _, names := range [][]string{{"missing"}, {"business"}} {
		if _, _, err := redisMigrationConfig(redisMigrationTestConfig(), "old", "new", names); err == nil {
			t.Fatal("accepted missing or non-KV store")
		}
	}
	if _, _, err := redisMigrationConfig(redisMigrationTestConfig(), "new", "old", nil); err == nil {
		t.Fatal("accepted reversed backends")
	}
}

func TestRedisMigrationCanceledDoesNotOpenConnectors(t *testing.T) {
	dir := t.TempDir()
	data := []byte("storage:\n  old:\n    kind: redis\n    url: redis://primary.invalid:6379\n  new:\n    kind: tikv\n    pd_addresses: [pd.invalid:2379]\nstores:\n  peers:\n    kind: keyvalue\n    storage: old\n    prefix: peers\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := MigrateRedisKV(ctx, dir, "old", "new", nil, kv.RedisMigrationOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("opened a connector before cancellation: %v", err)
	}
}
