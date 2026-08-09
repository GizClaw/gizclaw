package store

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	physicalstorage "github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestKeyValueUsesCompatiblePhysicalStorage(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": physicalstorage.MemoryConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	registry, err := New(map[string]Config{
		"first":  {Kind: KindKeyValue, Storage: "memory", Prefix: "first"},
		"second": {Kind: KindKeyValue, Storage: "memory", Prefix: "second"},
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
	if _, _, ok := kv.SharedAtomicStore(first, second); ok {
		t.Fatal("memory-backed logical Stores unexpectedly share an instance")
	}
}

func TestKeyValueBadgerStoresSharePhysicalAtomicRoot(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"badger": physicalstorage.BadgerConfig{Dir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	registry, err := New(map[string]Config{
		"first":  {Kind: KindKeyValue, Storage: "badger", Prefix: "first"},
		"second": {Kind: KindKeyValue, Storage: "badger", Prefix: "second"},
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
	root, prefixes, ok := kv.SharedAtomicStore(first, second)
	if !ok || root == nil || len(prefixes) != 2 {
		t.Fatalf("SharedAtomicStore() = %T, %v, %v", root, prefixes, ok)
	}
}

func TestKeyValueRejectsIncompatibleStorage(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"objects": physicalstorage.FilesystemDirConfig{Dir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	_, err = New(map[string]Config{
		"kv": {Kind: KindKeyValue, Storage: "objects"},
	}, physical)
	if err == nil || !strings.Contains(err.Error(), `storage "objects" kind "filesystem.dir"`) {
		t.Fatalf("New() error = %v", err)
	}
}

func TestStoreKindsRejectIncompatibleStorageKinds(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory":   physicalstorage.MemoryConfig{},
		"files":    physicalstorage.FilesystemDirConfig{Dir: t.TempDir()},
		"database": physicalstorage.SQLiteConfig{DSN: ":memory:"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	for _, test := range []struct {
		name        string
		config      Config
		storageKind string
	}{
		{"keyvalue", Config{Kind: KindKeyValue, Storage: "files"}, physicalstorage.KindFilesystemDir},
		{"objectstore", Config{Kind: KindObjectStore, Storage: "memory"}, physicalstorage.KindMemory},
		{"sql", Config{Kind: KindSQL, Storage: "files"}, physicalstorage.KindFilesystemDir},
		{"metrics", Config{Kind: KindMetrics, Storage: "files"}, physicalstorage.KindFilesystemDir},
		{"log.immutable", Config{Kind: KindLogImmutable, Storage: "memory"}, physicalstorage.KindMemory},
		{"log.mutable", Config{Kind: KindLogMutable, Storage: "memory"}, physicalstorage.KindMemory},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(map[string]Config{"store": test.config}, physical)
			if err == nil || !strings.Contains(err.Error(), `kind "`+test.storageKind+`"`) {
				t.Fatalf("New() error = %v", err)
			}
		})
	}
}

func TestObjectStoreConstructedFromFilesystemDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "objects")
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"files": physicalstorage.FilesystemDirConfig{Dir: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	registry, err := New(map[string]Config{
		"assets": {Kind: KindObjectStore, Storage: "files", Prefix: "assets"},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	assets, err := registry.ObjectStore("assets")
	if err != nil {
		t.Fatal(err)
	}
	if err := assets.Put("file.txt", strings.NewReader("value")); err != nil {
		t.Fatal(err)
	}
	if err := assets.Put("/absolute.txt", strings.NewReader("value")); err == nil {
		t.Fatal("prefixed ObjectStore accepted an absolute name")
	}
	reader, err := assets.Get("file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	value, err := io.ReadAll(reader)
	if err != nil || string(value) != "value" {
		t.Fatalf("ReadAll() = %q, %v", value, err)
	}
}

func TestSQLUsesCompatibleDatabaseStorage(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"database": physicalstorage.SQLiteConfig{DSN: ":memory:"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	registry, err := New(map[string]Config{
		"database": {Kind: KindSQL, Storage: "database"},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	db, err := registry.SQL("database")
	if err != nil || db.DriverName() != "sqlite" {
		t.Fatalf("SQL(database) = %v, %v", db, err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Stores.Close closed borrowed SQL pool: %v", err)
	}
}

func TestSQLiteSupportsTableScopedKVMetricAndLogStores(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"database": physicalstorage.SQLiteConfig{DSN: ":memory:"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	registry, err := New(map[string]Config{
		"kv":      {Kind: KindKeyValue, Storage: "database", Table: "kv_items", Prefix: "scope"},
		"metrics": {Kind: KindMetrics, Storage: "database", Table: "metric_samples"},
		"logs":    {Kind: KindLogImmutable, Storage: "database", Table: "immutable_logs"},
		"history": {Kind: KindLogMutable, Storage: "database", Table: "mutable_logs"},
		"raw":     {Kind: KindSQL, Storage: "database"},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	ctx := context.Background()
	kvStore, err := registry.KV("kv")
	if err != nil {
		t.Fatal(err)
	}
	if err := kvStore.Set(ctx, kv.Key{"one"}, []byte("value")); err != nil {
		t.Fatal(err)
	}
	metricStore, err := registry.Metrics("metrics")
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1000).UTC()
	if err := metricStore.Append(ctx, []metrics.Sample{{Name: "test", Timestamp: now, Value: 1}}); err != nil {
		t.Fatal(err)
	}
	logStore, err := registry.Log("logs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logStore.Append(ctx, []logstore.Record{{ID: "one", Stream: "events", Kind: "created", Time: now, Payload: json.RawMessage(`{"ok":true}`)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.MutableLog("logs"); err == nil {
		t.Fatal("immutable declaration exposed mutable capability")
	}
	if _, err := registry.MutableLog("history"); err != nil {
		t.Fatal(err)
	}
	raw, err := registry.SQL("raw")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if err := raw.Ping(); err != nil {
		t.Fatalf("Stores.Close closed shared pool: %v", err)
	}
}

func TestSQLiteTableClaimsFailBeforeDDL(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"database": physicalstorage.SQLiteConfig{DSN: ":memory:"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	_, err = New(map[string]Config{
		"first":  {Kind: KindKeyValue, Storage: "database", Table: "SharedTable"},
		"second": {Kind: KindMetrics, Storage: "database", Table: "sharedtable"},
	}, physical)
	if err == nil || !strings.Contains(err.Error(), "claimed by both") {
		t.Fatalf("New() error = %v", err)
	}
	db, err := physical.SQL("database")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name LIKE 'gizclaw_%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("preflight failure created %d migration tables", count)
	}
}

func TestMetricsMemoryStoresAreIndependent(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": physicalstorage.MemoryConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	registry, err := New(map[string]Config{
		"first":  {Kind: KindMetrics, Storage: "memory"},
		"second": {Kind: KindMetrics, Storage: "memory"},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	first, err := registry.Metrics("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Metrics("second")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("metrics stores over one memory Storage share an instance")
	}
	if _, ok := first.(*metrics.MemoryStore); !ok {
		t.Fatalf("Metrics(first) = %T, want *metrics.MemoryStore", first)
	}
}

func TestRemovedCommandStoreKindsAreRejected(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": physicalstorage.MemoryConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer physical.Close()
	for _, kind := range []string{"vecstore", "graph"} {
		t.Run(kind, func(t *testing.T) {
			_, err := New(map[string]Config{
				"removed": {Kind: kind, Storage: "memory"},
			}, physical)
			if err == nil || !strings.Contains(err.Error(), "unknown kind") {
				t.Fatalf("New() error = %v", err)
			}
		})
	}
}

func TestLogicalScopeFieldsAreKindSpecific(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": physicalstorage.MemoryConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer physical.Close()
	for name, cfg := range map[string]Config{
		"keyvalue table": {Kind: KindKeyValue, Storage: "memory", Table: "table"},
		"metrics prefix": {Kind: KindMetrics, Storage: "memory", Prefix: "prefix"},
		"sql topic":      {Kind: KindSQL, Storage: "memory", TopicID: "topic"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(map[string]Config{"store": cfg}, physical); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}
}

func TestNilPhysicalStorage(t *testing.T) {
	if _, err := New(map[string]Config{
		"store": {Kind: KindKeyValue, Storage: "memory"},
	}, nil); err == nil {
		t.Fatal("New(nil) error = nil")
	}
}

func TestConfigHasNoSerializationTags(t *testing.T) {
	typeOf := reflect.TypeFor[Config]()
	for field := range typeOf.Fields() {
		if field.Tag.Get("yaml") != "" || field.Tag.Get("json") != "" || field.Tag.Get("mapstructure") != "" {
			t.Fatalf("Config.%s has serialization tag %q", field.Name, field.Tag)
		}
	}
}
