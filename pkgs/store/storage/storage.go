// Package storage provides a configuration-driven registry for physical
// storage backends. Logical stores can build scoped views on top of these
// backend instances.
package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sync"

	"github.com/dgraph-io/badger/v4"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/api"
	"github.com/volcengine/volc-sdk-golang/service/tls"
)

var (
	errNilConfig         = errors.New("storage: config must not be nil")
	errUnsupportedConfig = errors.New("storage: unsupported config type")
)

// ConfigError identifies the physical storage entry whose construction failed.
type ConfigError struct {
	Name string
	Err  error
}

// Error preserves the underlying construction error text.
func (e *ConfigError) Error() string { return e.Err.Error() }

// Unwrap exposes the underlying construction error.
func (e *ConfigError) Unwrap() error { return e.Err }

// Storage owns physical backend instances and configured memory markers.
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

// New creates a Storage registry, validates every configured physical backend,
// and opens stateful resources. Process-local memory roots remain lazy because
// their concrete type is selected by the consuming Store contract. Dir fields
// are used as provided by the caller.
func New(configs map[string]Config) (*Storage, error) {
	s := &Storage{
		configs:    make(map[string]Config, len(configs)),
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
	return cfg.storageKind(), nil
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
	cfg, normalizeErr := normalizeConfig(cfg)
	if normalizeErr != nil {
		return &ConfigError{Name: name, Err: fmt.Errorf("storage: %q: %w", name, normalizeErr)}
	}
	s.configs[name] = cfg
	states[name] = building
	var err error
	switch cfg := cfg.(type) {
	case BadgerConfig:
		var st *badger.DB
		st, err = newBadger(name, cfg.Dir)
		if err == nil {
			s.badgers[name] = st
			s.closers = append(s.closers, st)
		}
	case MemoryConfig:
	case FilesystemDirConfig:
		var root *os.Root
		root, err = newFilesystemDir(name, cfg)
		if err == nil {
			s.dirs[name] = root
			s.closers = append(s.closers, root)
		}
	case SQLiteConfig:
		var st *sqlx.DB
		st, err = newSQLite(name, cfg)
		if err == nil {
			s.sqls[name] = st
			s.closers = append(s.closers, st)
		}
	case PostgreSQLConfig:
		var st *sqlx.DB
		st, err = newPostgreSQL(name, cfg)
		if err == nil {
			s.sqls[name] = st
			s.closers = append(s.closers, st)
		}
	case ClickHouseConfig:
		var st *sqlx.DB
		st, err = newClickHouse(name, cfg)
		if err == nil {
			s.sqls[name] = st
			s.closers = append(s.closers, st)
		}
	case PrometheusConfig:
		var resource prometheusResource
		resource, err = newPrometheus(name, cfg)
		if err == nil {
			s.prometheus[name] = resource
			if closer, ok := resource.client.(api.CloseIdler); ok {
				s.closers = append(s.closers, closeIdleAdapter{closer})
			}
		}
	case VolcTLSConfig:
		var client tls.Client
		client, err = newVolcTLS(name, cfg)
		if err == nil {
			s.volcs[name] = client
			if httpClient := client.GetHttpClient(); httpClient != nil {
				s.closers = append(s.closers, closeIdleAdapter{httpClient})
			}
		}
	default:
		err = fmt.Errorf("storage: %q: %w", name, errUnsupportedConfig)
	}
	if err != nil {
		return &ConfigError{Name: name, Err: err}
	}
	states[name] = built
	return nil
}

type externalOperationError struct {
	operation string
	err       error
}

func (e *externalOperationError) Error() string { return e.operation + " failed" }
func (e *externalOperationError) Unwrap() error { return e.err }

type closeIdler interface {
	CloseIdleConnections()
}

type closeIdleAdapter struct{ closeIdler }

func (c closeIdleAdapter) Close() error {
	c.CloseIdleConnections()
	return nil
}
