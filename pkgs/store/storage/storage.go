// Package storage provides a configuration-driven registry for physical
// storage backends. Logical stores can build scoped views on top of these
// backend instances.
package storage

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/api"
	"github.com/volcengine/volc-sdk-golang/service/tls"

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

// Config describes one physical storage backend. Serialization belongs to
// callers such as cmd/internal/server.
//
//	storage:
//	  main-kv:
//	    kind: badger
//	    dir: data/kv
type Config struct {
	Kind            string
	Dir             string
	DSN             string
	RemoteWriteURL  string
	QueryURL        string
	BearerToken     string
	Endpoint        string
	Region          string
	AccessKeyID     string
	AccessKeySecret string
}

// Memory marks a process-local physical slot. Logical Store constructors use
// the marker to create independent in-memory backends.
type Memory struct{}

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
	badgers    map[string]*badger.DB
	dirs       map[string]*os.Root
	sqls       map[string]*sqlx.DB
	prometheus map[string]prometheusResource
	volcs      map[string]tls.Client
	closers    []io.Closer
}

type prometheusResource struct {
	client         api.Client
	remoteWriteURL string
}

// New creates a Storage registry, validates every configured physical backend,
// and opens stateful resources. Process-local memory roots remain lazy because
// their concrete type is selected by the consuming Store contract. Dir fields
// are used as provided by the caller.
func New(configs map[string]Config) (*Storage, error) {
	s := &Storage{
		configs:    maps.Clone(configs),
		badgers:    make(map[string]*badger.DB),
		dirs:       make(map[string]*os.Root),
		sqls:       make(map[string]*sqlx.DB),
		prometheus: make(map[string]prometheusResource),
		volcs:      make(map[string]tls.Client),
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

// Badger returns the named physical Badger DB.
func (s *Storage) Badger(name string) (*badger.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	st, ok := s.badgers[name]
	if !ok {
		return nil, fmt.Errorf("storage: badger %q not found", name)
	}
	return st, nil
}

// Memory returns the named process-local marker.
func (s *Storage) Memory(name string) (Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Memory{}, errors.New("storage: registry is closed")
	}
	if cfg, ok := s.configs[name]; !ok || cfg.Kind != KindMemory {
		return Memory{}, fmt.Errorf("storage: memory %q not found", name)
	}
	return Memory{}, nil
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

// FilesystemDir returns the named rooted filesystem handle.
func (s *Storage) FilesystemDir(name string) (*os.Root, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	root, ok := s.dirs[name]
	if !ok {
		return nil, fmt.Errorf("storage: filesystem.dir %q not found", name)
	}
	return root, nil
}

// Prometheus returns the named API client and validated remote-write URL.
func (s *Storage) Prometheus(name string) (api.Client, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", errors.New("storage: registry is closed")
	}
	resource, ok := s.prometheus[name]
	if !ok {
		return nil, "", fmt.Errorf("storage: prometheus %q not found", name)
	}
	return resource.client, resource.remoteWriteURL, nil
}

// VolcTLS returns the named physical Volc TLS SDK client.
func (s *Storage) VolcTLS(name string) (tls.Client, error) {
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
		var st *badger.DB
		if err = validateFields(name, cfg, "dir"); err == nil {
			st, err = newBadger(name, cfg.Dir)
		}
		if err == nil {
			s.badgers[name] = st
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
		if err == nil {
			var root *os.Root
			root, err = os.OpenRoot(cfg.Dir)
			if err == nil {
				s.dirs[name] = root
				s.closers = append(s.closers, root)
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
		var resource prometheusResource
		resource, err = newPrometheus(name, cfg)
		if err == nil {
			s.prometheus[name] = resource
			if closer, ok := resource.client.(api.CloseIdler); ok {
				s.closers = append(s.closers, closeIdleAdapter{closer})
			}
		}
	case KindVolcTLS:
		var client tls.Client
		client, err = newVolcTLS(name, cfg)
		if err == nil {
			s.volcs[name] = client
			if httpClient := client.GetHttpClient(); httpClient != nil {
				s.closers = append(s.closers, closeIdleAdapter{httpClient})
			}
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

func newBadger(name, dir string) (*badger.DB, error) {
	if dir == "" {
		return nil, fmt.Errorf("storage: badger %q requires dir", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: badger %q mkdir: %w", name, err)
	}
	db, err := badger.Open(badger.DefaultOptions(dir).WithLogger(nil))
	if err != nil {
		return nil, &externalOperationError{operation: fmt.Sprintf("storage: badger %q open", name), err: err}
	}
	return db, nil
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

func newPrometheus(name string, cfg Config) (prometheusResource, error) {
	if err := validateFields(name, cfg, "remote_write_url", "query_url", "bearer_token"); err != nil {
		return prometheusResource{}, err
	}
	remoteWriteURL := strings.TrimSpace(cfg.RemoteWriteURL)
	queryURL := strings.TrimSpace(cfg.QueryURL)
	bearerToken := cfg.BearerToken
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "remote_write_url", value: remoteWriteURL},
		{name: "query_url", value: queryURL},
	} {
		u, err := url.ParseRequestURI(field.value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return prometheusResource{}, fmt.Errorf("storage: prometheus %q invalid %s", name, field.name)
		}
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	client, err := api.NewClient(api.Config{
		Address: queryURL,
		Client: &http.Client{
			Transport: bearerRoundTripper{token: bearerToken, next: transport},
			Timeout:   30 * time.Second,
		},
	})
	if err != nil {
		return prometheusResource{}, fmt.Errorf("storage: prometheus %q: %w", name, err)
	}
	return prometheusResource{client: client, remoteWriteURL: remoteWriteURL}, nil
}

func newVolcTLS(name string, cfg Config) (tls.Client, error) {
	if err := validateFields(name, cfg, "endpoint", "region", "access_key_id", "access_key_secret"); err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	region := strings.TrimSpace(cfg.Region)
	accessKeyID := strings.TrimSpace(cfg.AccessKeyID)
	accessKeySecret := strings.TrimSpace(cfg.AccessKeySecret)
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "endpoint", value: endpoint},
		{name: "region", value: region},
		{name: "access_key_id", value: accessKeyID},
		{name: "access_key_secret", value: accessKeySecret},
	} {
		if field.value == "" {
			return nil, fmt.Errorf("storage: volc-tls %q requires %s", name, field.name)
		}
	}
	client := tls.NewClient(endpoint, accessKeyID, accessKeySecret, "", region)
	client.SetTimeout(30 * time.Second)
	retryPolicy := client.GetRetryPolicy()
	retryPolicy.TotalTimeout = 30 * time.Second
	client.SetRetryPolicy(retryPolicy)
	return client, nil
}

type bearerRoundTripper struct {
	token string
	next  http.RoundTripper
}

func (r bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	if r.token != "" {
		clone.Header.Set("Authorization", "Bearer "+r.token)
	}
	return r.next.RoundTrip(clone)
}

type closeIdler interface {
	CloseIdleConnections()
}

type closeIdleAdapter struct{ closeIdler }

func (c closeIdleAdapter) Close() error {
	c.CloseIdleConnections()
	return nil
}
