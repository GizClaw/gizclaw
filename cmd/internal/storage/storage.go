// Package storage provides a configuration-driven registry for physical
// storage backends. Logical stores can build scoped views on top of these
// backend instances.
package storage

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/vecstore"
	"github.com/jmoiron/sqlx"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Kind constants for physical storage categories.
const (
	KindKeyValue    = "keyvalue"
	KindVecStore    = "vecstore"
	KindObjectStore = "objectstore"
	KindSQL         = "sql"
	KindPrometheus  = "prometheus"
	KindVolcTLS     = "volc-tls"
)

// Config is the YAML representation of a physical storage backend.
//
//	storage:
//	  main-kv:
//	    kind: keyvalue
//	    badger:
//	      dir: data/kv
type Config struct {
	Kind       string                        `yaml:"kind"`
	Memory     *MemoryConfig                 `yaml:"memory"`
	Badger     *BadgerConfig                 `yaml:"badger"`
	FS         *FSConfig                     `yaml:"fs"`
	SQLite     *SQLConfig                    `yaml:"sqlite"`
	Postgres   *SQLConfig                    `yaml:"postgres"`
	ClickHouse *SQLConfig                    `yaml:"clickhouse"`
	Prometheus *metrics.PrometheusConfig     `yaml:"prometheus"`
	Volc       *logstore.VolcConnectorConfig `yaml:"volc"`
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
	kvs        map[string]kv.Store
	vecs       map[string]vecstore.Index
	objects    map[string]objectstore.ObjectStore
	sqls       map[string]*sqlx.DB
	prometheus map[string]*metrics.PrometheusConnector
	volcs      map[string]*logstore.VolcConnector
	closers    []io.Closer
}

// New creates a Storage registry and eagerly instantiates every configured
// physical backend. Dir fields are used as provided by the caller.
func New(configs map[string]Config) (*Storage, error) {
	s := &Storage{
		kvs:        make(map[string]kv.Store),
		vecs:       make(map[string]vecstore.Index),
		objects:    make(map[string]objectstore.ObjectStore),
		sqls:       make(map[string]*sqlx.DB),
		prometheus: make(map[string]*metrics.PrometheusConnector),
		volcs:      make(map[string]*logstore.VolcConnector),
	}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()

	states := make(map[string]buildState, len(configs))
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if name == "" {
			return nil, &ConfigError{Name: name, Err: errors.New("storage: connector name must not be empty")}
		}
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

// Prometheus returns the named physical Prometheus connector.
func (s *Storage) Prometheus(name string) (*metrics.PrometheusConnector, error) {
	connector, ok := s.prometheus[name]
	if !ok {
		return nil, fmt.Errorf("storage: prometheus %q not found", name)
	}
	return connector, nil
}

// VolcTLS returns the named physical Volc TLS connector.
func (s *Storage) VolcTLS(name string) (*logstore.VolcConnector, error) {
	connector, ok := s.volcs[name]
	if !ok {
		return nil, fmt.Errorf("storage: volc-tls %q not found", name)
	}
	return connector, nil
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
	case KindPrometheus:
		var connector *metrics.PrometheusConnector
		connector, err = newPrometheus(name, cfg)
		if err == nil {
			s.prometheus[name] = connector
			s.closers = append(s.closers, connector)
		}
	case KindVolcTLS:
		var connector *logstore.VolcConnector
		connector, err = newVolcTLS(name, cfg)
		if err == nil {
			s.volcs[name] = connector
			s.closers = append(s.closers, connector)
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
	if err := validateDriverBlocks(name, KindKeyValue, driverBlocks(cfg), "memory", "badger"); err != nil {
		return nil, err
	}
	switch {
	case cfg.Memory != nil:
		return kv.NewBadgerInMemory(nil)
	case cfg.Badger != nil:
		return newBadgerKV(name, cfg.Badger.Dir)
	default:
		return nil, fmt.Errorf("storage: keyvalue %q requires driver", name)
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
	if err := validateDriverBlocks(name, KindVecStore, driverBlocks(cfg), "memory"); err != nil {
		return nil, err
	}
	return vecstore.NewMemory(), nil
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
	if err := validateDriverBlocks(name, KindSQL, driverBlocks(cfg), "sqlite", "postgres", "clickhouse"); err != nil {
		return nil, err
	}
	if err := validateSQLDriverFields(name, cfg); err != nil {
		return nil, err
	}
	backend, dsn := sqlDriverConfig(cfg)
	dsn = os.ExpandEnv(dsn)
	if backend == "sqlite" || backend == "clickhouse" {
		sqlx.BindDriver(backend, sqlx.QUESTION)
	}
	if sqlx.BindType(backend) == sqlx.UNKNOWN {
		return nil, fmt.Errorf("storage: sql %q unsupported dialect %q", name, backend)
	}
	if dsn == "" {
		return nil, fmt.Errorf("storage: sql %q requires dsn", name)
	}
	if backend == "sqlite" {
		if err := validateSQLiteDSN(dsn); err != nil {
			return nil, fmt.Errorf("storage: sql %q sqlite dsn: %w", name, err)
		}
	}
	if err := prepareSQLDir(name, cfg); err != nil {
		return nil, err
	}
	db, err := sqlx.Open(backend, dsn)
	if err != nil {
		return nil, &externalOperationError{operation: fmt.Sprintf("storage: sql %q open", name), err: err}
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
		return nil, &externalOperationError{operation: fmt.Sprintf("storage: sql %q ping", name), err: err}
	}
	return db, nil
}

func validateSQLDriverFields(name string, cfg Config) error {
	switch {
	case cfg.SQLite != nil:
		if cfg.SQLite.DSN != "" && cfg.SQLite.Dir != "" {
			return fmt.Errorf("storage: sql %q sqlite requires exactly one of dsn or dir", name)
		}
	case cfg.Postgres != nil:
		if cfg.Postgres.Dir != "" {
			return fmt.Errorf("storage: sql %q postgres does not support dir", name)
		}
	case cfg.ClickHouse != nil:
		if cfg.ClickHouse.Dir != "" {
			return fmt.Errorf("storage: sql %q clickhouse does not support dir", name)
		}
	}
	return nil
}

type externalOperationError struct {
	operation string
	err       error
}

func (e *externalOperationError) Error() string { return e.operation + " failed" }
func (e *externalOperationError) Unwrap() error { return e.err }

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

func validateSQLiteDSN(dsn string) error {
	queryStart := strings.IndexRune(dsn, '?')
	if queryStart < 1 {
		return nil
	}
	query, err := url.ParseQuery(dsn[queryStart+1:])
	if err != nil {
		return fmt.Errorf("parse query: %w", err)
	}
	for _, key := range []string{
		"_busy_timeout",
		"_timeout",
		"_foreign_keys",
		"_fk",
		"_journal_mode",
		"_journal",
		"_synchronous",
		"_sync",
		"_auto_vacuum",
		"_vacuum",
		"_query_only",
	} {
		if _, ok := query[key]; ok {
			return fmt.Errorf("query parameter %q is unsupported; GizClaw owns SQLite PRAGMA configuration", key)
		}
	}
	return nil
}

func prepareSQLDir(name string, cfg Config) error {
	var dir string
	if cfg.SQLite != nil && cfg.SQLite.DSN == "" {
		dir = cfg.SQLite.Dir
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
	if cfg.ClickHouse != nil {
		blocks = append(blocks, "clickhouse")
	}
	if cfg.Prometheus != nil {
		blocks = append(blocks, "prometheus")
	}
	if cfg.Volc != nil {
		blocks = append(blocks, "volc")
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
	if cfg.ClickHouse != nil {
		return "clickhouse", cfg.ClickHouse.DSN
	}
	return "", ""
}

func newPrometheus(name string, cfg Config) (*metrics.PrometheusConnector, error) {
	if err := validateDriverBlocks(name, KindPrometheus, driverBlocks(cfg), "prometheus"); err != nil {
		return nil, err
	}
	config := *cfg.Prometheus
	config.RemoteWriteURL = os.ExpandEnv(config.RemoteWriteURL)
	config.QueryURL = os.ExpandEnv(config.QueryURL)
	config.BearerToken = os.ExpandEnv(config.BearerToken)
	connector, err := metrics.NewPrometheusConnector(config)
	if err != nil {
		return nil, fmt.Errorf("storage: prometheus %q: %w", name, err)
	}
	return connector, nil
}

func newVolcTLS(name string, cfg Config) (*logstore.VolcConnector, error) {
	if err := validateDriverBlocks(name, KindVolcTLS, driverBlocks(cfg), "volc"); err != nil {
		return nil, err
	}
	config := *cfg.Volc
	config.Endpoint = os.ExpandEnv(config.Endpoint)
	config.Region = os.ExpandEnv(config.Region)
	config.AccessKeyID = os.ExpandEnv(config.AccessKeyID)
	config.AccessKeySecret = os.ExpandEnv(config.AccessKeySecret)
	connector, err := logstore.NewVolcConnector(config)
	if err != nil {
		return nil, fmt.Errorf("storage: volc-tls %q: %w", name, err)
	}
	return connector, nil
}
