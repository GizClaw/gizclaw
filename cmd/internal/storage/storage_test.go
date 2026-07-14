package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/goccy/go-yaml"
)

type fakeDriver struct{}

func (fakeDriver) Open(_ string) (driver.Conn, error) { return fakeConn{}, nil }

type fakeConn struct{}

func (fakeConn) Prepare(_ string) (driver.Stmt, error) { return nil, nil }
func (fakeConn) Close() error                          { return nil }
func (fakeConn) Begin() (driver.Tx, error)             { return nil, nil }
func (fakeConn) Ping(_ context.Context) error          { return nil }

type fakePingFailDriver struct{}

func (fakePingFailDriver) Open(_ string) (driver.Conn, error) {
	return fakePingFailConn{}, nil
}

type fakePingFailConn struct{}

func (fakePingFailConn) Prepare(_ string) (driver.Stmt, error) { return nil, nil }
func (fakePingFailConn) Close() error                          { return nil }
func (fakePingFailConn) Begin() (driver.Tx, error)             { return nil, nil }
func (fakePingFailConn) Ping(_ context.Context) error          { return errors.New("ping refused") }

func init() {
	sql.Register("storage_fake", fakeDriver{})
	sql.Register("storage_fake_ping_fail", fakePingFailDriver{})
}

func TestNewNilConfigs(t *testing.T) {
	s, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	if _, err := s.KV("anything"); err == nil {
		t.Fatal("expected error for empty storage registry")
	}
}

func TestNewUnknownKind(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: "nosql", Backend: "magic"},
	}); err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestKVMemory(t *testing.T) {
	reg, err := New(map[string]Config{
		"mem": {Kind: KindKeyValue, Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	s, err := reg.KV("mem")
	if err != nil {
		t.Fatalf("KV(mem): %v", err)
	}

	ctx := context.Background()
	if err := s.Set(ctx, kv.Key{"a"}, []byte("1")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, kv.Key{"a"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "1" {
		t.Fatalf("Get = %q, want %q", got, "1")
	}

	s2, err := reg.KV("mem")
	if err != nil {
		t.Fatalf("KV(mem) second call: %v", err)
	}
	if s != s2 {
		t.Fatal("expected same instance on second call")
	}
}

func TestKVMemoryDriverBlock(t *testing.T) {
	reg, err := New(map[string]Config{
		"mem": {Kind: KindKeyValue, Memory: &MemoryConfig{}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()
	if _, err := reg.KV("mem"); err != nil {
		t.Fatalf("KV(mem): %v", err)
	}
}

func TestKVBadger(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "badger")
	reg, err := New(map[string]Config{
		"bg": {Kind: KindKeyValue, Backend: "badger", Dir: dir},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	s, err := reg.KV("bg")
	if err != nil {
		t.Fatalf("KV(bg): %v", err)
	}
	ctx := context.Background()
	if err := s.Set(ctx, kv.Key{"k"}, []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, kv.Key{"k"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("Get = %q", got)
	}
}

func TestKVBadgerDriverBlock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "badger")
	reg, err := New(map[string]Config{
		"bg": {Kind: KindKeyValue, Badger: &BadgerConfig{Dir: dir}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()
	if _, err := reg.KV("bg"); err != nil {
		t.Fatalf("KV(bg): %v", err)
	}
}

func TestNewRejectsWrongDriverBlockForKind(t *testing.T) {
	if _, err := New(map[string]Config{
		"bad": {Kind: KindKeyValue, FS: &FSConfig{Dir: t.TempDir()}},
	}); err == nil {
		t.Fatal("expected error for wrong driver block")
	}
	if _, err := New(map[string]Config{
		"bad": {Kind: KindKeyValue, Memory: &MemoryConfig{}, Badger: &BadgerConfig{Dir: t.TempDir()}},
	}); err == nil {
		t.Fatal("expected error for multiple driver blocks")
	}
	if _, err := New(map[string]Config{
		"bad": {Kind: KindObjectStore, Badger: &BadgerConfig{Dir: t.TempDir()}},
	}); err == nil {
		t.Fatal("expected error for objectstore with badger driver")
	}
	if _, err := New(map[string]Config{
		"bad": {Kind: KindSQL, FS: &FSConfig{Dir: t.TempDir()}},
	}); err == nil {
		t.Fatal("expected error for sql with fs driver")
	}
}

func TestNewRejectsMissingDriverBlock(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "keyvalue", cfg: Config{Kind: KindKeyValue}},
		{name: "vecstore", cfg: Config{Kind: KindVecStore}},
		{name: "objectstore", cfg: Config{Kind: KindObjectStore}},
		{name: "sql", cfg: Config{Kind: KindSQL}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(map[string]Config{"bad": tc.cfg}); err == nil {
				t.Fatal("expected error for missing driver")
			}
		})
	}
}

func TestKVNotFound(t *testing.T) {
	reg, err := New(map[string]Config{
		"vec": {Kind: KindVecStore, Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	if _, err := reg.KV("missing"); err == nil {
		t.Fatal("expected error for missing backend")
	}
	if _, err := reg.KV("vec"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

func TestNewKVUnknownBackend(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindKeyValue, Backend: "redis"},
	}); err == nil {
		t.Fatal("expected error for unknown kv backend")
	}
}

func TestNewKVBadgerNoDir(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindKeyValue, Backend: "badger"},
	}); err == nil {
		t.Fatal("expected error for badger without dir")
	}
}

func TestVecStoreMemory(t *testing.T) {
	reg, err := New(map[string]Config{
		"vec": {Kind: KindVecStore, Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	idx, err := reg.VecStore("vec")
	if err != nil {
		t.Fatalf("VecStore(vec): %v", err)
	}
	if err := idx.Insert("a", []float32{1, 0, 0}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if idx.Len() != 1 {
		t.Fatalf("Len = %d", idx.Len())
	}
}

func TestVecStoreNotFound(t *testing.T) {
	reg, err := New(map[string]Config{
		"kv": {Kind: KindKeyValue, Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	if _, err := reg.VecStore("missing"); err == nil {
		t.Fatal("expected error for missing backend")
	}
	if _, err := reg.VecStore("kv"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

func TestNewVecStoreUnknownBackend(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindVecStore, Backend: "qdrant"},
	}); err == nil {
		t.Fatal("expected error for unknown vecstore backend")
	}
}

func TestObjectStoreFilesystemDriverBlock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "objects")
	reg, err := New(map[string]Config{
		"objects": {Kind: KindObjectStore, FS: &FSConfig{Dir: dir}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	objects, err := reg.ObjectStore("objects")
	if err != nil {
		t.Fatalf("ObjectStore(objects): %v", err)
	}
	if err := objects.Put("firmware/stable.bin", strings.NewReader("stable")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	r, err := objects.Get("firmware/stable.bin")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(r)
	if closeErr := r.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "stable" {
		t.Fatalf("Get = %q, want stable", got)
	}
}

func TestObjectStoreRejectsLegacyFilesystemDriverName(t *testing.T) {
	var cfg struct {
		Storage map[string]Config `yaml:"storage"`
	}
	data := []byte(`
storage:
  objects:
    kind: objectstore
    filesystem:
      dir: objects
`)
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, err := New(cfg.Storage); err == nil {
		t.Fatal("expected error for objectstore filesystem driver alias")
	}
}

func TestObjectStoreNotFound(t *testing.T) {
	reg, err := New(map[string]Config{
		"kv": {Kind: KindKeyValue, Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	if _, err := reg.ObjectStore("missing"); err == nil {
		t.Fatal("expected error for missing backend")
	}
	if _, err := reg.ObjectStore("kv"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

func TestSQL(t *testing.T) {
	reg, err := New(map[string]Config{
		"db": {Kind: KindSQL, Backend: "storage_fake", DSN: "test"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	db, err := reg.SQL("db")
	if err != nil {
		t.Fatalf("SQL(db): %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil *sql.DB")
	}
}

func TestSQLSQLiteUsesDirAsDSN(t *testing.T) {
	reg, err := New(map[string]Config{
		"db": {Kind: KindSQL, Backend: "sqlite", Dir: filepath.Join(t.TempDir(), "db.sqlite")},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	if _, err := reg.SQL("db"); err != nil {
		t.Fatalf("SQL(db): %v", err)
	}
}

func TestSQLSQLiteCreatesParentDir(t *testing.T) {
	reg, err := New(map[string]Config{
		"db": {
			Kind:   KindSQL,
			SQLite: &SQLConfig{Dir: filepath.Join(t.TempDir(), "data", "db.sqlite")},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	if _, err := reg.SQL("db"); err != nil {
		t.Fatalf("SQL(db): %v", err)
	}
}

func TestSQLSQLiteConfiguresConnection(t *testing.T) {
	reg, err := New(map[string]Config{
		"db": {
			Kind:   KindSQL,
			SQLite: &SQLConfig{Dir: filepath.Join(t.TempDir(), "data", "db.sqlite")},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	db, err := reg.SQL("db")
	if err != nil {
		t.Fatalf("SQL(db): %v", err)
	}
	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}

	var busyTimeout int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}

	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestSQLSQLiteConcurrentWrites(t *testing.T) {
	reg, err := New(map[string]Config{
		"db": {
			Kind:   KindSQL,
			SQLite: &SQLConfig{Dir: filepath.Join(t.TempDir(), "data", "db.sqlite")},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	db, err := reg.SQL("db")
	if err != nil {
		t.Fatalf("SQL(db): %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE writes (id INTEGER PRIMARY KEY, worker INTEGER NOT NULL, seq INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	const workers = 12
	const writesPerWorker = 25
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := range writesPerWorker {
				if _, err := db.Exec(`INSERT INTO writes (worker, seq) VALUES (?, ?)`, worker, seq); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent write: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM writes`).Scan(&count); err != nil {
		t.Fatalf("count writes: %v", err)
	}
	if want := workers * writesPerWorker; count != want {
		t.Fatalf("write count = %d, want %d", count, want)
	}
}

func TestSQLNotFound(t *testing.T) {
	reg, err := New(map[string]Config{
		"kv": {Kind: KindKeyValue, Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	if _, err := reg.SQL("missing"); err == nil {
		t.Fatal("expected error for missing backend")
	}
	if _, err := reg.SQL("kv"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

func TestNewSQLNoBackend(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindSQL, DSN: "x"},
	}); err == nil {
		t.Fatal("expected error for empty backend")
	}
}

func TestNewSQLNoDSN(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindSQL, Backend: "storage_fake"},
	}); err == nil {
		t.Fatal("expected error for missing dsn")
	}
}

func TestNewSQLBadDriver(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindSQL, Backend: "nonexistent_driver", DSN: "x"},
	}); err == nil {
		t.Fatal("expected error for unregistered driver")
	}
}

func TestNewSQLPingFail(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindSQL, Backend: "storage_fake_ping_fail", DSN: "x"},
	}); err == nil {
		t.Fatal("expected error for ping failure")
	}
}

func TestCloseEmpty(t *testing.T) {
	s, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close empty: %v", err)
	}
}
