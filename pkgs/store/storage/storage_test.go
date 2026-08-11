package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewNilConfigs(t *testing.T) {
	s, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Memory("anything"); err == nil {
		t.Fatal("Memory(anything) error = nil")
	}
}

func TestNewRejectsNilConfigAndEmptyName(t *testing.T) {
	for name, configs := range map[string]map[string]Config{
		"nil config": {"x": (*BadgerConfig)(nil)},
		"empty name": {"": MemoryConfig{}},
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

func TestMemoryIsMarker(t *testing.T) {
	registry, err := New(map[string]Config{"memory": MemoryConfig{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })

	if _, err := registry.Memory("memory"); err != nil {
		t.Fatal(err)
	}
}

func TestBadgerKV(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "badger")
	registry, err := New(map[string]Config{"badger": BadgerConfig{Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	db, err := registry.Badger("badger")
	if err != nil {
		t.Fatal(err)
	}
	if db == nil {
		t.Fatal("Badger(badger) = nil")
	}
}

func TestFilesystemDirDescriptor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "objects")
	registry, err := New(map[string]Config{"objects": FilesystemDirConfig{Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	got, err := registry.FilesystemDir("objects")
	if err != nil || got.Name() != dir {
		t.Fatalf("FilesystemDir(objects) = %v, %v; want %q", got, err, dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("Stat(%q): %v", dir, err)
	}
	if _, err := registry.Badger("objects"); err == nil {
		t.Fatal("Badger(objects) error = nil")
	}
}

func TestSQLiteDirAndDSN(t *testing.T) {
	for name, cfg := range map[string]Config{
		"dir": SQLiteConfig{Dir: filepath.Join(t.TempDir(), "dir.sqlite")},
		"dsn": SQLiteConfig{DSN: filepath.Join(t.TempDir(), "dsn.sqlite")},
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
		SQLiteConfig{},
		SQLiteConfig{Dir: "data.sqlite", DSN: ":memory:"},
	} {
		if _, err := New(map[string]Config{"database": cfg}); err == nil {
			t.Fatalf("New(%+v) error = nil", cfg)
		}
	}
}

func TestSQLiteConnectionConfiguration(t *testing.T) {
	registry, err := New(map[string]Config{
		"database": SQLiteConfig{Dir: filepath.Join(t.TempDir(), "configured.sqlite")},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	db, err := registry.SQL("database")
	if err != nil {
		t.Fatal(err)
	}

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}
	for _, check := range []struct {
		query string
		want  any
	}{
		{query: `PRAGMA busy_timeout`, want: int64(5000)},
		{query: `PRAGMA foreign_keys`, want: int64(1)},
		{query: `PRAGMA journal_mode`, want: "wal"},
	} {
		var got any
		if err := db.Get(&got, check.query); err != nil {
			t.Fatalf("%s: %v", check.query, err)
		}
		if got != check.want {
			t.Fatalf("%s = %#v, want %#v", check.query, got, check.want)
		}
	}
}

func TestNetworkSQLRequiresDSN(t *testing.T) {
	for name, cfg := range map[string]Config{
		"postgresql": PostgreSQLConfig{},
		"clickhouse": ClickHouseConfig{},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(map[string]Config{"database": cfg}); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}
}

func TestSQLPreservesLiteralDSNEnvironmentReferences(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIZCLAW_TEST_SQLITE_DSN", filepath.Join(dir, "expanded.sqlite"))
	const literalPath = "$GIZCLAW_TEST_SQLITE_DSN"
	registry, err := New(map[string]Config{
		"database": SQLiteConfig{DSN: literalPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if _, err := os.Stat(filepath.Join(dir, literalPath)); err != nil {
		t.Fatalf("Stat(%q): %v", literalPath, err)
	}
	if _, err := os.Stat(os.Getenv("GIZCLAW_TEST_SQLITE_DSN")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expanded DSN path unexpectedly created: %v", err)
	}
}

func TestSQLConnectionErrorsDoNotExposeDSNSecrets(t *testing.T) {
	const secret = "leaked-password"
	_, err := New(map[string]Config{
		"database": PostgreSQLConfig{DSN: "postgres://user:" + secret + "@%"},
	})
	if err == nil {
		t.Fatal("New() error = nil")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "invalid URL escape") {
		t.Fatalf("New() exposed driver details: %v", err)
	}
	if err.Error() != `storage: sql "database" ping failed` {
		t.Fatalf("New() omitted operation context: %v", err)
	}
}

func TestSQLiteRejectsDriverOwnedPragmaAliases(t *testing.T) {
	for _, parameter := range []string{"_busy_timeout", "_timeout", "_foreign_keys", "_fk", "_journal_mode", "_journal"} {
		t.Run(parameter, func(t *testing.T) {
			_, err := New(map[string]Config{
				"database": SQLiteConfig{DSN: "file:test.sqlite?" + parameter + "=1"},
			})
			if err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("New() error = %v", err)
			}
		})
	}
}

func TestCloseClosesSQL(t *testing.T) {
	registry, err := New(map[string]Config{"database": SQLiteConfig{DSN: ":memory:"}})
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
