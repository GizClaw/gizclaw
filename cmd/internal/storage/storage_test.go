package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestNewNilConfigs(t *testing.T) {
	s, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.KV("anything"); err == nil {
		t.Fatal("KV(anything) error = nil")
	}
}

func TestNewRejectsUnknownKindAndEmptyName(t *testing.T) {
	for name, configs := range map[string]map[string]Config{
		"unknown kind": {"x": {Kind: "nosql"}},
		"empty name":   {"": {Kind: KindMemory}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(configs)
			if err == nil {
				t.Fatal("New() error = nil")
			}
			var configErr *ConfigError
			if !errors.As(err, &configErr) {
				t.Fatalf("New() error = %v, want ConfigError", err)
			}
		})
	}
}

func TestConcreteKindsRejectForeignFields(t *testing.T) {
	for name, cfg := range map[string]Config{
		"memory dir":         {Kind: KindMemory, Dir: "data"},
		"badger dsn":         {Kind: KindBadger, Dir: "data", DSN: "unused"},
		"filesystem dsn":     {Kind: KindFilesystemDir, Dir: "data", DSN: "unused"},
		"postgresql dir":     {Kind: KindPostgreSQL, DSN: "postgres://example.invalid/db", Dir: "data"},
		"clickhouse dir":     {Kind: KindClickHouse, DSN: "clickhouse://example.invalid/db", Dir: "data"},
		"prometheus dsn":     {Kind: KindPrometheus, DSN: "unused"},
		"volc-tls query url": {Kind: KindVolcTLS, QueryURL: "unused"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(map[string]Config{"storage": cfg}); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}
}

func TestMemoryKVIsSharedMemoryInstance(t *testing.T) {
	registry, err := New(map[string]Config{"memory": {Kind: KindMemory}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })

	first, err := registry.KV("memory")
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.KV("memory")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("KV(memory) returned different roots")
	}
	if _, ok := first.(*kv.Memory); !ok {
		t.Fatalf("KV(memory) = %T, want *kv.Memory", first)
	}
	if err := first.Set(t.Context(), kv.Key{"key"}, []byte("value")); err != nil {
		t.Fatal(err)
	}
	value, err := second.Get(t.Context(), kv.Key{"key"})
	if err != nil || string(value) != "value" {
		t.Fatalf("Get() = %q, %v", value, err)
	}
}

func TestBadgerKV(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "badger")
	registry, err := New(map[string]Config{"badger": {Kind: KindBadger, Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	store, err := registry.KV("badger")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(t.Context(), kv.Key{"key"}, []byte("value")); err != nil {
		t.Fatal(err)
	}
}

func TestFilesystemDirDescriptor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "objects")
	registry, err := New(map[string]Config{"objects": {Kind: KindFilesystemDir, Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	got, err := registry.Dir("objects")
	if err != nil || got != dir {
		t.Fatalf("Dir(objects) = %q, %v; want %q", got, err, dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("Stat(%q): %v", dir, err)
	}
	if _, err := registry.KV("objects"); err == nil {
		t.Fatal("KV(objects) error = nil")
	}
}

func TestSQLiteDirAndDSN(t *testing.T) {
	for name, cfg := range map[string]Config{
		"dir": {Kind: KindSQLite, Dir: filepath.Join(t.TempDir(), "dir.sqlite")},
		"dsn": {Kind: KindSQLite, DSN: filepath.Join(t.TempDir(), "dsn.sqlite")},
	} {
		t.Run(name, func(t *testing.T) {
			registry, err := New(map[string]Config{"database": cfg})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = registry.Close() })
			db, err := registry.SQL("database")
			if err != nil {
				t.Fatal(err)
			}
			if db.DriverName() != "sqlite" {
				t.Fatalf("DriverName() = %q", db.DriverName())
			}
		})
	}
}

func TestSQLiteRequiresExactlyOneLocation(t *testing.T) {
	for _, cfg := range []Config{
		{Kind: KindSQLite},
		{Kind: KindSQLite, Dir: "data.sqlite", DSN: ":memory:"},
	} {
		if _, err := New(map[string]Config{"database": cfg}); err == nil {
			t.Fatalf("New(%+v) error = nil", cfg)
		}
	}
}

func TestSQLExpandsDSNEnvironment(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "expanded.sqlite")
	t.Setenv("GIZCLAW_TEST_SQLITE_DSN", dbPath)
	registry, err := New(map[string]Config{
		"database": {Kind: KindSQLite, DSN: "${GIZCLAW_TEST_SQLITE_DSN}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("Stat(%q): %v", dbPath, err)
	}
}

func TestSQLConnectionErrorsDoNotExposeDSNSecrets(t *testing.T) {
	const secret = "leaked-password"
	_, err := New(map[string]Config{
		"database": {Kind: KindPostgreSQL, DSN: "postgres://user:" + secret + "@%"},
	})
	if err == nil {
		t.Fatal("New() error = nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("New() exposed DSN secret: %v", err)
	}
}

func TestSQLiteRejectsDriverOwnedPragmaAliases(t *testing.T) {
	for _, parameter := range []string{"_busy_timeout", "_timeout", "_foreign_keys", "_fk", "_journal_mode", "_journal"} {
		t.Run(parameter, func(t *testing.T) {
			_, err := New(map[string]Config{
				"database": {Kind: KindSQLite, DSN: "file:test.sqlite?" + parameter + "=1"},
			})
			if err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("New() error = %v", err)
			}
		})
	}
}

func TestCloseClosesSQL(t *testing.T) {
	registry, err := New(map[string]Config{"database": {Kind: KindSQLite, DSN: ":memory:"}})
	if err != nil {
		t.Fatal(err)
	}
	db, err := registry.SQL("database")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.SQL("database"); err == nil {
		t.Fatal("SQL(database) after Close() error = nil")
	}
	if _, err := registry.Kind("database"); err == nil {
		t.Fatal("Kind(database) after Close() error = nil")
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("PingContext() after Close() error = nil")
	}
}
