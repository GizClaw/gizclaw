package stores

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	physicalstorage "github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func TestKeyValueUsesCompatiblePhysicalStorage(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": {Kind: physicalstorage.KindMemory},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewWithOwnedStorage(physical, map[string]Config{
		"first":  {Kind: KindKeyValue, Storage: "memory", Prefix: "first"},
		"second": {Kind: KindKeyValue, Storage: "memory", Prefix: "second"},
	})
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
	if _, ok := root.(*kv.Memory); !ok {
		t.Fatalf("root = %T, want *kv.Memory", root)
	}
}

func TestKeyValueRejectsIncompatibleStorage(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"objects": {Kind: physicalstorage.KindFilesystemDir, Dir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	_, err = NewWithStorage(physical, map[string]Config{
		"kv": {Kind: KindKeyValue, Storage: "objects"},
	})
	if err == nil || !strings.Contains(err.Error(), `storage "objects" kind "filesystem.dir"`) {
		t.Fatalf("NewWithStorage() error = %v", err)
	}
}

func TestStoreKindsRejectIncompatibleStorageKinds(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory":   {Kind: physicalstorage.KindMemory},
		"files":    {Kind: physicalstorage.KindFilesystemDir, Dir: t.TempDir()},
		"database": {Kind: physicalstorage.KindSQLite, DSN: ":memory:"},
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
		{"keyvalue", Config{Kind: KindKeyValue, Storage: "database"}, physicalstorage.KindSQLite},
		{"objectstore", Config{Kind: KindObjectStore, Storage: "memory"}, physicalstorage.KindMemory},
		{"sql", Config{Kind: KindSQL, Storage: "files"}, physicalstorage.KindFilesystemDir},
		{"metrics", Config{Kind: KindMetrics, Storage: "database"}, physicalstorage.KindSQLite},
		{"log.immutable", Config{Kind: KindLogImmutable, Storage: "database"}, physicalstorage.KindSQLite},
		{"log.mutable", Config{Kind: KindLogMutable, Storage: "memory"}, physicalstorage.KindMemory},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWithStorage(physical, map[string]Config{"store": test.config})
			if err == nil || !strings.Contains(err.Error(), `kind "`+test.storageKind+`"`) {
				t.Fatalf("NewWithStorage() error = %v", err)
			}
		})
	}
}

func TestObjectStoreConstructedFromFilesystemDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "objects")
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"files": {Kind: physicalstorage.KindFilesystemDir, Dir: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewWithOwnedStorage(physical, map[string]Config{
		"assets": {Kind: KindObjectStore, Storage: "files", Prefix: "assets"},
	})
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
		"database": {Kind: physicalstorage.KindSQLite, DSN: ":memory:"},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewWithOwnedStorage(physical, map[string]Config{
		"database": {Kind: KindSQL, Storage: "database"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	db, err := registry.SQL("database")
	if err != nil || db.DriverName() != "sqlite" {
		t.Fatalf("SQL(database) = %v, %v", db, err)
	}
}

func TestMetricsMemoryRootIsShared(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": {Kind: physicalstorage.KindMemory},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewWithOwnedStorage(physical, map[string]Config{
		"first":  {Kind: KindMetrics, Storage: "memory"},
		"second": {Kind: KindMetrics, Storage: "memory"},
	})
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
	if first != second {
		t.Fatal("metrics stores over one memory Storage do not share a root")
	}
	if _, ok := first.(*metrics.MemoryStore); !ok {
		t.Fatalf("Metrics(first) = %T, want *metrics.MemoryStore", first)
	}
}

func TestRemovedCommandStoreKindsAreRejected(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": {Kind: physicalstorage.KindMemory},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer physical.Close()
	for _, kind := range []string{"vecstore", "graph"} {
		t.Run(kind, func(t *testing.T) {
			_, err := NewWithStorage(physical, map[string]Config{
				"removed": {Kind: kind, Storage: "memory"},
			})
			if err == nil || !strings.Contains(err.Error(), "unknown kind") {
				t.Fatalf("NewWithStorage() error = %v", err)
			}
		})
	}
}

func TestLogicalScopeFieldsAreKindSpecific(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": {Kind: physicalstorage.KindMemory},
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
			if _, err := NewWithStorage(physical, map[string]Config{"store": cfg}); err == nil {
				t.Fatal("NewWithStorage() error = nil")
			}
		})
	}
}

func TestNilPhysicalStorage(t *testing.T) {
	if _, err := NewWithStorage(nil, map[string]Config{
		"store": {Kind: KindKeyValue, Storage: "memory"},
	}); err == nil {
		t.Fatal("NewWithStorage(nil) error = nil")
	}
}
