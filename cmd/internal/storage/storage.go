// Package storage provides a configuration-driven registry for physical
// storage backends. Logical stores can build scoped views on top of these
// backend instances.
package storage

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/jmoiron/sqlx"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Kind constants identify concrete physical storage backends.
const (
	KindBadger        = "badger"
	KindMemory        = "memory"
	KindFilesystemDir = "filesystem.dir"
	KindSQLite        = "sqlite"
	KindPostgreSQL    = "postgresql"
	KindClickHouse    = "clickhouse"
	KindPrometheus    = "prometheus"
	KindVolcTLS       = "volc-tls"
)

// Config is the YAML representation of a physical storage backend.
//
//	storage:
//	  main-kv:
//	    kind: badger
//	    dir: data/kv
type Config struct {
	Kind            string `yaml:"kind"`
	Dir             string `yaml:"dir"`
	DSN             string `yaml:"dsn"`
	RemoteWriteURL  string `yaml:"remote_write_url"`
	QueryURL        string `yaml:"query_url"`
	BearerToken     string `yaml:"bearer_token"`
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
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

// Storage owns physical backend instances and lazy in-process allocations.
type Storage struct {
	mu         sync.Mutex
	closed     bool
	configs    map[string]Config
	kvs        map[string]kv.Store
	sqls       map[string]*sqlx.DB
	prometheus map[string]*metrics.PrometheusConnector
	volcs      map[string]*logstore.VolcConnector
	closers    []io.Closer
}

// New creates a Storage registry, validates every configured physical backend,
// and opens stateful resources. Process-local memory roots remain lazy because
// their concrete type is selected by the consuming Store contract. Dir fields
// are used as provided by the caller.
func New(configs map[string]Config) (*Storage, error) {
	s := &Storage{
		configs:    maps.Clone(configs),
		kvs:        make(map[string]kv.Store),
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	st, ok := s.kvs[name]
	if ok {
		return st, nil
	}
	cfg, ok := s.configs[name]
	if !ok || cfg.Kind != KindMemory {
		return nil, fmt.Errorf("storage: kv %q not found", name)
	}
	st = kv.NewMemory(nil)
	s.kvs[name] = st
	s.closers = append(s.closers, st)
	return st, nil
}

// SQL returns the named physical SQL backend.
func (s *Storage) SQL(name string) (*sqlx.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	st, ok := s.sqls[name]
	if !ok {
		return nil, fmt.Errorf("storage: sql %q not found", name)
	}
	return st, nil
}

// Kind returns the concrete kind of the named physical Storage.
func (s *Storage) Kind(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", errors.New("storage: registry is closed")
	}
	cfg, ok := s.configs[name]
	if !ok {
		return "", fmt.Errorf("storage: %q not found", name)
	}
	return cfg.Kind, nil
}

// Dir returns the filesystem directory owned by the named Storage.
func (s *Storage) Dir(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", errors.New("storage: registry is closed")
	}
	cfg, ok := s.configs[name]
	if !ok || cfg.Kind != KindFilesystemDir {
		return "", fmt.Errorf("storage: filesystem.dir %q not found", name)
	}
	return cfg.Dir, nil
}

// Prometheus returns the named physical Prometheus connector.
func (s *Storage) Prometheus(name string) (*metrics.PrometheusConnector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	connector, ok := s.prometheus[name]
	if !ok {
		return nil, fmt.Errorf("storage: prometheus %q not found", name)
	}
	return connector, nil
}

// VolcTLS returns the named physical Volc TLS connector.
func (s *Storage) VolcTLS(name string) (*logstore.VolcConnector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	connector, ok := s.volcs[name]
	if !ok {
		return nil, fmt.Errorf("storage: volc-tls %q not found", name)
	}
	return connector, nil
}

// Close releases all opened physical backends in reverse creation order.
func (s *Storage) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	closers := s.closers
	s.closers = nil
	s.mu.Unlock()
	var errs []error
	for i := len(closers) - 1; i >= 0; i-- {
		if err := closers[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
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
	case KindBadger:
		var st kv.Store
		if err = validateFields(name, cfg, "dir"); err == nil {
			st, err = newBadgerKV(name, cfg.Dir)
		}
		if err == nil {
			s.kvs[name] = st
			s.closers = append(s.closers, st)
		}
	case KindMemory:
		err = validateFields(name, cfg)
	case KindFilesystemDir:
		err = validateFields(name, cfg, "dir")
		if err == nil && cfg.Dir == "" {
			err = fmt.Errorf("storage: filesystem.dir %q requires dir", name)
		}
		if err == nil {
			err = os.MkdirAll(cfg.Dir, 0o755)
			if err != nil {
				err = fmt.Errorf("storage: filesystem.dir %q mkdir: %w", name, err)
			}
		}
	case KindSQLite, KindPostgreSQL, KindClickHouse:
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

func newBadgerKV(name, dir string) (kv.Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("storage: badger %q requires dir", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: badger %q mkdir: %w", name, err)
	}
	return kv.NewBadger(dir, nil)
}

func newSQL(name string, cfg Config) (*sqlx.DB, error) {
	allowed := []string{"dsn"}
	if cfg.Kind == KindSQLite {
		allowed = append(allowed, "dir")
	}
	if err := validateFields(name, cfg, allowed...); err != nil {
		return nil, err
	}
	if cfg.Kind == KindSQLite && (cfg.DSN == "") == (cfg.Dir == "") {
		return nil, fmt.Errorf("storage: sqlite %q requires exactly one of dsn or dir", name)
	}
	if cfg.Kind != KindSQLite && cfg.DSN == "" {
		return nil, fmt.Errorf("storage: %s %q requires dsn", cfg.Kind, name)
	}
	backend := cfg.Kind
	if backend == KindPostgreSQL {
		backend = "postgres"
	}
	dsn := cfg.DSN
	if cfg.Kind == KindSQLite && dsn == "" {
		dsn = cfg.Dir
	}
	dsn = os.ExpandEnv(dsn)
	if backend == KindSQLite || backend == KindClickHouse {
		sqlx.BindDriver(backend, sqlx.QUESTION)
	}
	if sqlx.BindType(backend) == sqlx.UNKNOWN {
		return nil, fmt.Errorf("storage: sql %q unsupported dialect %q", name, backend)
	}
	if dsn == "" {
		return nil, fmt.Errorf("storage: sql %q requires dsn", name)
	}
	if backend == KindSQLite {
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
	if backend == KindSQLite {
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
	dir := cfg.Dir
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

func validateFields(name string, cfg Config, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	fields := []struct {
		name string
		set  bool
	}{
		{"dir", cfg.Dir != ""},
		{"dsn", cfg.DSN != ""},
		{"remote_write_url", cfg.RemoteWriteURL != ""},
		{"query_url", cfg.QueryURL != ""},
		{"bearer_token", cfg.BearerToken != ""},
		{"endpoint", cfg.Endpoint != ""},
		{"region", cfg.Region != ""},
		{"access_key_id", cfg.AccessKeyID != ""},
		{"access_key_secret", cfg.AccessKeySecret != ""},
	}
	for _, field := range fields {
		if !field.set {
			continue
		}
		if _, ok := allowedSet[field.name]; !ok {
			return fmt.Errorf("storage: %s %q does not support %s", cfg.Kind, name, field.name)
		}
	}
	return nil
}

func newPrometheus(name string, cfg Config) (*metrics.PrometheusConnector, error) {
	if err := validateFields(name, cfg, "remote_write_url", "query_url", "bearer_token"); err != nil {
		return nil, err
	}
	config := metrics.PrometheusConfig{
		RemoteWriteURL: cfg.RemoteWriteURL,
		QueryURL:       cfg.QueryURL,
		BearerToken:    cfg.BearerToken,
	}
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
	if err := validateFields(name, cfg, "endpoint", "region", "access_key_id", "access_key_secret"); err != nil {
		return nil, err
	}
	config := logstore.VolcConnectorConfig{
		Endpoint:        cfg.Endpoint,
		Region:          cfg.Region,
		AccessKeyID:     cfg.AccessKeyID,
		AccessKeySecret: cfg.AccessKeySecret,
	}
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
