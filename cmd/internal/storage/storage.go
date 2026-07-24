// Package storage provides a configuration-driven registry for physical
// storage backends. Logical stores can build scoped views on top of these
// backend instances.
package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/vecstore"
	"github.com/jmoiron/sqlx"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Kind constants for physical storage categories.
const (
	KindKeyValue    = "keyvalue"
	KindVecStore    = "vecstore"
	KindObjectStore = "objectstore"
	KindSQL         = "sql"
)

// Config is the YAML representation of a physical storage backend.
//
//	storage:
//	  main-kv:
//	    kind: keyvalue
//	    badger:
//	      dir: data/kv
type Config struct {
	Kind     string        `yaml:"kind"`
	Memory   *MemoryConfig `yaml:"memory"`
	Badger   *BadgerConfig `yaml:"badger"`
	FS       *FSConfig     `yaml:"fs"`
	SQLite   *SQLConfig    `yaml:"sqlite"`
	Postgres *SQLConfig    `yaml:"postgres"`
	Backend  string        `yaml:"backend"` // legacy driver field
	Dir      string        `yaml:"dir"`     // legacy driver dir field
	Dim      int           `yaml:"dim"`     // legacy vecstore dimension field
	DSN      string        `yaml:"dsn"`     // legacy sql connection string field
}

// ConfigError identifies the physical storage entry whose construction failed.
type ConfigError struct {
	Name string
	Err  error
}

// Error preserves the underlying construction error text.
func (e *ConfigError) Error() string { return e.Err.Error() }

// Unwrap exposes the underlying construction error.
func (e *ConfigError) Unwrap() error { return e.Err }

type MemoryConfig struct{}

type BadgerConfig struct {
	Dir string `yaml:"dir"`
}

type FSConfig struct {
	Dir string `yaml:"dir"`
}

type SQLConfig struct {
	DSN string `yaml:"dsn"`
	Dir string `yaml:"dir"`
}

// Storage holds physical backend instances created eagerly by New.
type Storage struct {
	kvs     map[string]kv.Store
	vecs    map[string]vecstore.Index
	objects map[string]objectstore.ObjectStore
	sqls    map[string]*sqlx.DB
	closers []io.Closer
}

// New creates a Storage registry and eagerly instantiates every configured
// physical backend. Dir fields are used as provided by the caller.
func New(configs map[string]Config) (*Storage, error) {
	s := &Storage{
		kvs:     make(map[string]kv.Store),
		vecs:    make(map[string]vecstore.Index),
		objects: make(map[string]objectstore.ObjectStore),
		sqls:    make(map[string]*sqlx.DB),
	}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()

	states := make(map[string]buildState, len(configs))
	for name := range configs {
		if err := s.build(name, configs, states); err != nil {
			return nil, err
		}
	}

	ok = true
	return s, nil
}

// KV returns the named physical key-value backend.
func (s *Storage) KV(name string) (kv.Store, error) {
	st, ok := s.kvs[name]
	if !ok {
		return nil, fmt.Errorf("storage: kv %q not found", name)
	}
	return st, nil
}

// VecStore returns the named physical vector store backend.
func (s *Storage) VecStore(name string) (vecstore.Index, error) {
	st, ok := s.vecs[name]
	if !ok {
		return nil, fmt.Errorf("storage: vecstore %q not found", name)
	}
	return st, nil
}

// SQL returns the named physical SQL backend.
func (s *Storage) SQL(name string) (*sqlx.DB, error) {
	st, ok := s.sqls[name]
	if !ok {
		return nil, fmt.Errorf("storage: sql %q not found", name)
	}
	return st, nil
}

// ObjectStore returns the named physical object store backend.
func (s *Storage) ObjectStore(name string) (objectstore.ObjectStore, error) {
	st, ok := s.objects[name]
	if !ok {
		return nil, fmt.Errorf("storage: objectstore %q not found", name)
	}
	return st, nil
}

// Close releases all opened physical backends in reverse creation order.
func (s *Storage) Close() error {
	var errs []error
	for i := len(s.closers) - 1; i >= 0; i-- {
		if err := s.closers[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	s.closers = nil
	return errors.Join(errs...)
}

type buildState uint8

const (
	building buildState = 1
	built    buildState = 2
)

func (s *Storage) build(name string, configs map[string]Config, states map[string]buildState) error {
	switch states[name] {
	case built:
		return nil
	case building:
		return fmt.Errorf("storage: dependency cycle at %q", name)
	}
	cfg, ok := configs[name]
	if !ok {
		return fmt.Errorf("storage: %q not configured", name)
	}
	states[name] = building
	var err error
	switch cfg.Kind {
	case KindKeyValue:
		var st kv.Store
		st, err = newKV(name, cfg)
		if err == nil {
			s.kvs[name] = st
			s.closers = append(s.closers, st)
		}
	case KindVecStore:
		var st vecstore.Index
		st, err = newVecStore(name, cfg)
		if err == nil {
			s.vecs[name] = st
			s.closers = append(s.closers, st)
		}
	case KindObjectStore:
		var st objectstore.ObjectStore
		st, err = newObjectStore(name, cfg)
		if err == nil {
			s.objects[name] = st
		}
	case KindSQL:
		var st *sqlx.DB
		st, err = newSQL(name, cfg)
		if err == nil {
			s.sqls[name] = st
			s.closers = append(s.closers, st)
		}
	default:
		err = fmt.Errorf("storage: %q has unknown kind %q", name, cfg.Kind)
	}
	if err != nil {
		return &ConfigError{Name: name, Err: err}
	}
	states[name] = built
	return nil
}

func newKV(name string, cfg Config) (kv.Store, error) {
	if blocks := driverBlocks(cfg); len(blocks) > 0 {
		if err := validateDriverBlocks(name, KindKeyValue, blocks, "memory", "badger"); err != nil {
			return nil, err
		}
		switch {
		case cfg.Memory != nil:
			return kv.NewBadgerInMemory(nil)
		case cfg.Badger != nil:
			return newBadgerKV(name, cfg.Badger.Dir)
		}
	}
	switch cfg.Backend {
	case "memory":
		return kv.NewBadgerInMemory(nil)
	case "badger":
		return newBadgerKV(name, cfg.Dir)
	case "":
		return nil, fmt.Errorf("storage: keyvalue %q requires driver", name)
	default:
		return nil, fmt.Errorf("storage: keyvalue %q unknown backend %q", name, cfg.Backend)
	}
}

func newBadgerKV(name, dir string) (kv.Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("storage: keyvalue %q (badger) requires dir", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: keyvalue %q mkdir: %w", name, err)
	}
	return kv.NewBadger(dir, nil)
}

func newVecStore(name string, cfg Config) (vecstore.Index, error) {
	if blocks := driverBlocks(cfg); len(blocks) > 0 {
		if err := validateDriverBlocks(name, KindVecStore, blocks, "memory"); err != nil {
			return nil, err
		}
		return vecstore.NewMemory(), nil
	}
	switch cfg.Backend {
	case "memory":
		return vecstore.NewMemory(), nil
	case "":
		return nil, fmt.Errorf("storage: vecstore %q requires driver", name)
	default:
		return nil, fmt.Errorf("storage: vecstore %q unknown backend %q", name, cfg.Backend)
	}
}

func newObjectStore(name string, cfg Config) (objectstore.ObjectStore, error) {
	if blocks := driverBlocks(cfg); len(blocks) > 0 {
		if err := validateDriverBlocks(name, KindObjectStore, blocks, "fs"); err != nil {
			return nil, err
		}
	}
	if cfg.FS == nil {
		return nil, fmt.Errorf("storage: objectstore %q requires fs driver", name)
	}
	if cfg.FS.Dir == "" {
		return nil, fmt.Errorf("storage: objectstore %q (fs) requires dir", name)
	}
	if err := os.MkdirAll(cfg.FS.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: objectstore %q mkdir: %w", name, err)
	}
	return objectstore.Dir(cfg.FS.Dir), nil
}

func newSQL(name string, cfg Config) (*sqlx.DB, error) {
	if blocks := driverBlocks(cfg); len(blocks) > 0 {
		if err := validateDriverBlocks(name, KindSQL, blocks, "sqlite", "postgres"); err != nil {
			return nil, err
		}
	}
	backend, dsn := sqlDriverConfig(cfg)
	if backend == "" {
		return nil, fmt.Errorf("storage: sql %q requires backend (driver name)", name)
	}
	if backend == "sqlite" {
		sqlx.BindDriver(backend, sqlx.QUESTION)
	}
	if sqlx.BindType(backend) == sqlx.UNKNOWN {
		return nil, fmt.Errorf("storage: sql %q unsupported dialect %q", name, backend)
	}
	if dsn == "" {
		return nil, fmt.Errorf("storage: sql %q requires dsn", name)
	}
	if err := prepareSQLDir(name, cfg); err != nil {
		return nil, err
	}
	db, err := sqlx.Open(backend, dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: sql %q open: %w", name, err)
	}
	if backend == "sqlite" {
		configureSQLitePool(db)
		if err := configureSQLiteConnection(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("storage: sql %q configure sqlite: %w", name, err)
		}
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: sql %q ping: %w", name, err)
	}
	return db, nil
}

func configureSQLitePool(db *sqlx.DB) {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
}

func configureSQLiteConnection(db *sqlx.DB) error {
	for _, stmt := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func prepareSQLDir(name string, cfg Config) error {
	var dir string
	if cfg.SQLite != nil && cfg.SQLite.DSN == "" {
		dir = cfg.SQLite.Dir
	} else if cfg.Backend == "sqlite" && cfg.DSN == "" {
		dir = cfg.Dir
	}
	if dir == "" {
		return nil
	}
	parent := filepath.Dir(dir)
	if parent == "." || parent == "" {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("storage: sql %q mkdir: %w", name, err)
	}
	return nil
}

func driverBlocks(cfg Config) []string {
	var blocks []string
	if cfg.Memory != nil {
		blocks = append(blocks, "memory")
	}
	if cfg.Badger != nil {
		blocks = append(blocks, "badger")
	}
	if cfg.FS != nil {
		blocks = append(blocks, "fs")
	}
	if cfg.SQLite != nil {
		blocks = append(blocks, "sqlite")
	}
	if cfg.Postgres != nil {
		blocks = append(blocks, "postgres")
	}
	return blocks
}

func validateDriverBlocks(name, kind string, blocks []string, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, driver := range allowed {
		allowedSet[driver] = struct{}{}
	}
	for _, driver := range blocks {
		if _, ok := allowedSet[driver]; !ok {
			return fmt.Errorf("storage: %s %q does not support %s driver", kind, name, driver)
		}
	}
	if len(blocks) != 1 {
		return fmt.Errorf("storage: %s %q requires exactly one driver, got %s", kind, name, strings.Join(blocks, ", "))
	}
	return nil
}

func sqlDriverConfig(cfg Config) (string, string) {
	if cfg.SQLite != nil {
		if cfg.SQLite.DSN != "" {
			return "sqlite", cfg.SQLite.DSN
		}
		return "sqlite", cfg.SQLite.Dir
	}
	if cfg.Postgres != nil {
		return "postgres", cfg.Postgres.DSN
	}
	dsn := cfg.DSN
	if cfg.Backend == "sqlite" && dsn == "" && cfg.Dir != "" {
		dsn = cfg.Dir
	}
	return cfg.Backend, dsn
}
