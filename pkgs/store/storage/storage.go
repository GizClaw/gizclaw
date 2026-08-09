// Package storage provides a configuration-driven registry for physical
// storage backends. Logical stores can build scoped views on top of these
// backend instances.
package storage

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/api"
	"github.com/volcengine/volc-sdk-golang/service/tls"
)

var (
	errNilConfig         = errors.New("storage: config must not be nil")
	errUnsupportedConfig = errors.New("storage: unsupported config type")
)

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
	if cfg, ok := s.configs[name]; !ok || cfg.storageKind() != KindMemory {
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
	return cfg.storageKind(), nil
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
		if cfg.Dir == "" {
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

func newPrometheus(name string, cfg PrometheusConfig) (prometheusResource, error) {
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

func newVolcTLS(name string, cfg VolcTLSConfig) (tls.Client, error) {
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
