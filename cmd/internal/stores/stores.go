// Package stores provides a configuration-driven registry for logical stores.
// Logical stores reference physical backends from cmd/internal/storage and can
// expose scoped views such as prefixed KV stores.
package stores

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/pkgs/store/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/vecstore"
	"github.com/jmoiron/sqlx"
)

// Kind constants for logical store categories.
const (
	KindKeyValue     = storage.KindKeyValue
	KindVecStore     = storage.KindVecStore
	KindGraph        = "graph"
	KindMetrics      = "metrics"
	KindLogImmutable = "log.immutable"
	KindLogMutable   = "log.mutable"
	KindObjectStore  = storage.KindObjectStore
	KindSQL          = storage.KindSQL
)

// Config is the YAML representation of a single logical store entry.
//
//	stores:
//	  peers:
//	    kind: keyvalue
//	    storage: main-kv
//	    prefix: peers
type Config struct {
	Kind    string `yaml:"kind"`
	Storage string `yaml:"storage"` // reference to a physical storage backend
	Prefix  string `yaml:"prefix"`  // slash-separated logical key prefix for KV stores

	Backend string `yaml:"backend"` // graph implementation selector
	Store   string `yaml:"store"`   // graph backend "kv": reference to a logical keyvalue store

	ClickHouse *ClickHouseConfig `yaml:"clickhouse"`
	Memory     *struct{}         `yaml:"memory"`
	Volc       *VolcConfig       `yaml:"volc"`
}

// ConfigError identifies the logical Store entry whose construction failed.
type ConfigError struct {
	Name string
	Err  error
}

// Error preserves the underlying construction error text.
func (e *ConfigError) Error() string { return e.Err.Error() }

// Unwrap exposes the underlying construction error.
func (e *ConfigError) Unwrap() error { return e.Err }

// ClickHouseConfig selects logical database/table scope on a physical SQL
// connector.
type ClickHouseConfig struct {
	Database string `yaml:"database"`
	Table    string `yaml:"table"`
}

// VolcConfig selects one logical topic on a physical Volc TLS connector.
type VolcConfig struct {
	TopicID string `yaml:"topic_id"`
}

// Stores holds named logical store instances created eagerly by NewWithStorage.
type Stores struct {
	storage      *storage.Storage
	ownsStorage  bool
	kvs          map[string]kv.Store
	vecs         map[string]vecstore.Index
	objects      map[string]objectstore.ObjectStore
	graphs       map[string]graph.Graph
	metrics      map[string]metrics.Store
	logs         map[string]logstore.ImmutableStore
	mutableLogs  map[string]struct{}
	sqls         map[string]*sqlx.DB
	logicClosers []io.Closer
}

// NewWithOwnedStorage creates logical stores and transfers ownership of the
// provided physical storage registry to the returned Stores.
func NewWithOwnedStorage(physical *storage.Storage, configs map[string]Config) (*Stores, error) {
	s, err := NewWithStorage(physical, configs)
	if err != nil {
		if physical == nil {
			return nil, err
		}
		return nil, errors.Join(err, physical.Close())
	}
	s.ownsStorage = true
	return s, nil
}

// NewWithStorage creates logical stores on top of already-opened physical
// storage backends. The caller owns the physical storage lifecycle.
func NewWithStorage(physical *storage.Storage, configs map[string]Config) (*Stores, error) {
	if physical == nil && needsPhysicalStorage(configs) {
		return nil, fmt.Errorf("stores: storage registry is nil")
	}
	if err := validateObjectStorePrefixes(configs); err != nil {
		return nil, err
	}
	s := &Stores{
		storage:     physical,
		kvs:         make(map[string]kv.Store),
		vecs:        make(map[string]vecstore.Index),
		objects:     make(map[string]objectstore.ObjectStore),
		graphs:      make(map[string]graph.Graph),
		metrics:     make(map[string]metrics.Store),
		logs:        make(map[string]logstore.ImmutableStore),
		mutableLogs: make(map[string]struct{}),
		sqls:        make(map[string]*sqlx.DB),
	}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()

	var graphCfgs []struct {
		name string
		cfg  Config
	}
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		cfg := configs[name]
		if name == "" {
			return nil, &ConfigError{Name: name, Err: errors.New("stores: Store name must not be empty")}
		}
		if err := validateKindFields(name, cfg); err != nil {
			return nil, &ConfigError{Name: name, Err: err}
		}
		switch cfg.Kind {
		case KindKeyValue:
			st, err := s.newKV(name, cfg)
			if err != nil {
				return nil, &ConfigError{Name: name, Err: err}
			}
			s.kvs[name] = st
		case KindVecStore:
			st, err := s.newVecStore(name, cfg)
			if err != nil {
				return nil, &ConfigError{Name: name, Err: err}
			}
			s.vecs[name] = st
		case KindSQL:
			st, err := s.newSQL(name, cfg)
			if err != nil {
				return nil, &ConfigError{Name: name, Err: err}
			}
			s.sqls[name] = st
		case KindObjectStore:
			st, err := s.newObjectStore(name, cfg)
			if err != nil {
				return nil, &ConfigError{Name: name, Err: err}
			}
			s.objects[name] = st
		case KindGraph:
			graphCfgs = append(graphCfgs, struct {
				name string
				cfg  Config
			}{name, cfg})
		case KindMetrics:
			st, err := s.newMetrics(name, cfg)
			if err != nil {
				return nil, &ConfigError{Name: name, Err: err}
			}
			s.metrics[name] = st
			s.logicClosers = append(s.logicClosers, st)
		case KindLogImmutable, KindLogMutable:
			st, err := s.newLog(name, cfg)
			if err != nil {
				return nil, &ConfigError{Name: name, Err: err}
			}
			s.logs[name] = st
			if cfg.Kind == KindLogMutable {
				s.mutableLogs[name] = struct{}{}
			}
			s.logicClosers = append(s.logicClosers, st)
		default:
			return nil, &ConfigError{Name: name, Err: fmt.Errorf("stores: %q has unknown kind %q", name, cfg.Kind)}
		}
	}

	for _, g := range graphCfgs {
		st, err := s.newGraph(g.name, g.cfg)
		if err != nil {
			return nil, &ConfigError{Name: g.name, Err: err}
		}
		s.graphs[g.name] = st
		s.logicClosers = append(s.logicClosers, st)
	}

	ok = true
	return s, nil
}

func validateKindFields(name string, cfg Config) error {
	invalid := func(condition bool, field string) error {
		if !condition {
			return nil
		}
		return fmt.Errorf("stores: %s %q does not support %s", cfg.Kind, name, field)
	}
	providerFields := []struct {
		set   bool
		field string
	}{
		{cfg.ClickHouse != nil, "clickhouse"},
		{cfg.Memory != nil, "memory"},
		{cfg.Volc != nil, "volc"},
	}
	switch cfg.Kind {
	case KindKeyValue, KindObjectStore:
		if err := invalid(cfg.Backend != "", "backend"); err != nil {
			return err
		}
		if err := invalid(cfg.Store != "", "store"); err != nil {
			return err
		}
		for _, field := range providerFields {
			if err := invalid(field.set, field.field); err != nil {
				return err
			}
		}
	case KindVecStore, KindSQL:
		if err := invalid(cfg.Prefix != "", "prefix"); err != nil {
			return err
		}
		if err := invalid(cfg.Backend != "", "backend"); err != nil {
			return err
		}
		if err := invalid(cfg.Store != "", "store"); err != nil {
			return err
		}
		for _, field := range providerFields {
			if err := invalid(field.set, field.field); err != nil {
				return err
			}
		}
	case KindGraph:
		if err := invalid(cfg.Storage != "", "storage"); err != nil {
			return err
		}
		for _, field := range providerFields {
			if err := invalid(field.set, field.field); err != nil {
				return err
			}
		}
	case KindMetrics:
		if err := invalid(cfg.Prefix != "", "prefix"); err != nil {
			return err
		}
		if err := invalid(cfg.Backend != "", "backend"); err != nil {
			return err
		}
		if err := invalid(cfg.Store != "", "store"); err != nil {
			return err
		}
		if err := invalid(cfg.Volc != nil, "volc"); err != nil {
			return err
		}
	case KindLogImmutable, KindLogMutable:
		if err := invalid(cfg.Prefix != "", "prefix"); err != nil {
			return err
		}
		if err := invalid(cfg.Backend != "", "backend"); err != nil {
			return err
		}
		if err := invalid(cfg.Store != "", "store"); err != nil {
			return err
		}
		if err := invalid(cfg.Memory != nil, "memory"); err != nil {
			return err
		}
		if cfg.Kind == KindLogMutable && cfg.Volc != nil {
			return fmt.Errorf("stores: %s %q does not support volc; use a mutable ClickHouse backend", cfg.Kind, name)
		}
	}
	return nil
}

func validateObjectStorePrefixes(configs map[string]Config) error {
	type logicalStore struct {
		name   string
		prefix string
	}
	groups := map[string][]logicalStore{}
	for name, cfg := range configs {
		if cfg.Kind != KindObjectStore || cfg.Storage == "" {
			continue
		}
		prefix, err := parseObjectPrefix(cfg.Prefix)
		if err != nil {
			return &ConfigError{
				Name: name,
				Err:  fmt.Errorf("stores: objectstore %q prefix: %w", name, err),
			}
		}
		if cfg.Prefix != prefix {
			return &ConfigError{
				Name: name,
				Err:  fmt.Errorf("stores: objectstore %q prefix %q is not clean; use %q", name, cfg.Prefix, prefix),
			}
		}
		groups[cfg.Storage] = append(groups[cfg.Storage], logicalStore{name: name, prefix: prefix})
	}
	for physical, logical := range groups {
		if len(logical) < 2 {
			continue
		}
		slices.SortFunc(logical, func(a, b logicalStore) int { return strings.Compare(a.name, b.name) })
		for _, store := range logical {
			if store.prefix == "" {
				return &ConfigError{
					Name: store.name,
					Err:  fmt.Errorf("stores: physical objectstore %q is shared; logical store %q requires a non-empty prefix", physical, store.name),
				}
			}
		}
		for i := range logical {
			for j := i + 1; j < len(logical); j++ {
				a, b := logical[i], logical[j]
				if a.prefix == b.prefix || strings.HasPrefix(a.prefix, b.prefix+"/") || strings.HasPrefix(b.prefix, a.prefix+"/") {
					err := fmt.Errorf(
						"stores: physical objectstore %q has overlapping prefixes for logical stores %q (%q) and %q (%q)",
						physical, a.name, a.prefix, b.name, b.prefix,
					)
					return &ConfigError{Name: a.name, Err: &ConfigError{Name: b.name, Err: err}}
				}
			}
		}
	}
	return nil
}

// KV returns the named kv.Store.
func (r *Stores) KV(name string) (kv.Store, error) {
	s, ok := r.kvs[name]
	if !ok {
		return nil, fmt.Errorf("stores: kv %q not found", name)
	}
	return s, nil
}

// VecStore returns the named vecstore.Index.
func (r *Stores) VecStore(name string) (vecstore.Index, error) {
	s, ok := r.vecs[name]
	if !ok {
		return nil, fmt.Errorf("stores: vecstore %q not found", name)
	}
	return s, nil
}

// Graph returns the named graph.Graph.
func (r *Stores) Graph(name string) (graph.Graph, error) {
	s, ok := r.graphs[name]
	if !ok {
		return nil, fmt.Errorf("stores: graph %q not found", name)
	}
	return s, nil
}

// SQL returns the named *sqlx.DB.
func (r *Stores) SQL(name string) (*sqlx.DB, error) {
	s, ok := r.sqls[name]
	if !ok {
		return nil, fmt.Errorf("stores: sql %q not found", name)
	}
	return s, nil
}

// ObjectStore returns the named objectstore.ObjectStore.
func (r *Stores) ObjectStore(name string) (objectstore.ObjectStore, error) {
	s, ok := r.objects[name]
	if !ok {
		return nil, fmt.Errorf("stores: objectstore %q not found", name)
	}
	return s, nil
}

// Metrics returns the named metrics.Store.
func (r *Stores) Metrics(name string) (metrics.Store, error) {
	s, ok := r.metrics[name]
	if !ok {
		return nil, fmt.Errorf("stores: metrics %q not found", name)
	}
	return s, nil
}

// Log returns the named logstore.ImmutableStore.
func (r *Stores) Log(name string) (logstore.ImmutableStore, error) {
	s, ok := r.logs[name]
	if !ok {
		return nil, fmt.Errorf("stores: log %q not found", name)
	}
	return s, nil
}

// MutableLog returns the named log store when its driver supports record
// replacement and deletion.
func (r *Stores) MutableLog(name string) (logstore.MutableStore, error) {
	if _, ok := r.mutableLogs[name]; !ok {
		if _, exists := r.logs[name]; exists {
			return nil, fmt.Errorf("stores: log %q is not declared %s", name, KindLogMutable)
		}
	}
	store, err := r.Log(name)
	if err != nil {
		return nil, err
	}
	mutable, ok := store.(logstore.MutableStore)
	if !ok {
		return nil, fmt.Errorf("stores: log %q does not support mutable records", name)
	}
	return mutable, nil
}

// Close releases logical stores, then any physical storage owned by this
// registry. Stores created with NewWithStorage do not own physical storage.
func (r *Stores) Close() error {
	var errs []error
	for i := len(r.logicClosers) - 1; i >= 0; i-- {
		if err := r.logicClosers[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	r.logicClosers = nil
	if r.ownsStorage && r.storage != nil {
		if err := r.storage.Close(); err != nil {
			errs = append(errs, err)
		}
		r.storage = nil
	}
	return errors.Join(errs...)
}

// --- factory methods ---

func (r *Stores) newKV(name string, cfg Config) (kv.Store, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: keyvalue %q requires storage reference", name)
	}
	base, err := r.storage.KV(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: keyvalue %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	prefix, err := parseKeyPrefix(cfg.Prefix)
	if err != nil {
		return nil, fmt.Errorf("stores: keyvalue %q prefix: %w", name, err)
	}
	if len(prefix) == 0 {
		return base, nil
	}
	return kv.Prefixed(base, prefix), nil
}

func (r *Stores) newVecStore(name string, cfg Config) (vecstore.Index, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: vecstore %q requires storage reference", name)
	}
	st, err := r.storage.VecStore(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: vecstore %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	return st, nil
}

func (r *Stores) newGraph(name string, cfg Config) (graph.Graph, error) {
	switch cfg.Backend {
	case "kv":
		if cfg.Store == "" {
			return nil, fmt.Errorf("stores: graph %q (kv) requires store reference", name)
		}
		kvStore, err := r.kvByName(cfg.Store)
		if err != nil {
			return nil, fmt.Errorf("stores: graph %q resolve kv %q: %w", name, cfg.Store, err)
		}
		prefix, err := parseKeyPrefix(cfg.Prefix)
		if err != nil {
			return nil, fmt.Errorf("stores: graph %q prefix: %w", name, err)
		}
		if len(prefix) == 0 {
			prefix = kv.Key{name}
		}
		return graph.NewKVGraph(kvStore, prefix), nil
	default:
		return nil, fmt.Errorf("stores: graph %q unknown backend %q", name, cfg.Backend)
	}
}

func (r *Stores) newMetrics(name string, cfg Config) (metrics.Store, error) {
	memory := cfg.Memory != nil
	count := 0
	if memory {
		count++
	}
	if cfg.Storage != "" {
		count++
	}
	if count != 1 {
		return nil, fmt.Errorf("stores: metrics %q requires exactly one of memory or storage", name)
	}
	if memory {
		return metrics.NewMemoryStore(), nil
	}
	if cfg.ClickHouse == nil {
		connector, err := r.storage.Prometheus(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q resolve prometheus storage %q: %w", name, cfg.Storage, err)
		}
		return connector.Store()
	}
	if cfg.ClickHouse.Database != "" {
		return nil, fmt.Errorf("stores: metrics %q clickhouse database must be selected by the dsn", name)
	}
	db, err := r.storage.SQL(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: metrics %q resolve sql storage %q: %w", name, cfg.Storage, err)
	}
	st, err := metrics.NewClickHouseStoreWithDB(db.DB, os.ExpandEnv(cfg.ClickHouse.Table))
	if err != nil {
		return nil, fmt.Errorf("stores: metrics %q clickhouse: %w", name, err)
	}
	return st, nil
}

func (r *Stores) newLog(name string, cfg Config) (logstore.ImmutableStore, error) {
	backendCount := 0
	if cfg.Volc != nil {
		backendCount++
	}
	if cfg.ClickHouse != nil {
		backendCount++
	}
	if backendCount != 1 {
		return nil, fmt.Errorf("stores: log %q requires exactly one of volc or clickhouse", name)
	}
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: log %q requires storage reference", name)
	}
	if cfg.Prefix != "" || cfg.Backend != "" || cfg.Store != "" || cfg.Memory != nil {
		return nil, fmt.Errorf("stores: log %q contains fields owned by another store kind", name)
	}
	if cfg.ClickHouse != nil {
		clickhouse := *cfg.ClickHouse
		clickhouse.Database = os.ExpandEnv(clickhouse.Database)
		clickhouse.Table = os.ExpandEnv(clickhouse.Table)
		db, err := r.storage.SQL(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: log %q resolve sql storage %q: %w", name, cfg.Storage, err)
		}
		store, err := logstore.NewClickHouseStoreWithDB(db.DB, clickhouse.Database, clickhouse.Table)
		if err != nil {
			return nil, fmt.Errorf("stores: log %q clickhouse: %w", name, err)
		}
		return store, nil
	}
	connector, err := r.storage.VolcTLS(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: log %q resolve volc-tls storage %q: %w", name, cfg.Storage, err)
	}
	st, err := connector.Store(os.ExpandEnv(cfg.Volc.TopicID))
	if err != nil {
		return nil, fmt.Errorf("stores: log %q volc: %w", name, err)
	}
	return st, nil
}

func (r *Stores) newSQL(name string, cfg Config) (*sqlx.DB, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: sql %q requires storage reference", name)
	}
	db, err := r.storage.SQL(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: sql %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	return db, nil
}

func (r *Stores) newObjectStore(name string, cfg Config) (objectstore.ObjectStore, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: objectstore %q requires storage reference", name)
	}
	st, err := r.storage.ObjectStore(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: objectstore %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	prefix, err := parseObjectPrefix(cfg.Prefix)
	if err != nil {
		return nil, fmt.Errorf("stores: objectstore %q prefix: %w", name, err)
	}
	if prefix == "" {
		return st, nil
	}
	return prefixedObjectStore{base: st, prefix: prefix}, nil
}

func (r *Stores) kvByName(name string) (kv.Store, error) {
	s, ok := r.kvs[name]
	if !ok {
		return nil, fmt.Errorf("stores: kv %q not found", name)
	}
	return s, nil
}

func needsPhysicalStorage(configs map[string]Config) bool {
	for _, cfg := range configs {
		switch cfg.Kind {
		case KindGraph:
		case KindMetrics:
			if cfg.Memory == nil {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func parseKeyPrefix(path string) (kv.Key, error) {
	if path == "" {
		return nil, nil
	}
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, nil
	}
	parts := strings.Split(path, "/")
	key := make(kv.Key, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("empty segment in %q", path)
		}
		if strings.Contains(part, ":") {
			return nil, fmt.Errorf("segment %q contains ':'", part)
		}
		key = append(key, part)
	}
	return key, nil
}

func parseObjectPrefix(path string) (string, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return "", nil
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid segment %q in %q", part, path)
		}
	}
	return strings.Join(parts, "/"), nil
}

type prefixedObjectStore struct {
	base   objectstore.ObjectStore
	prefix string
}

func (s prefixedObjectStore) Get(name string) (io.ReadCloser, error) {
	return s.base.Get(s.name(name))
}

func (s prefixedObjectStore) Put(name string, r io.Reader) error {
	return s.base.Put(s.name(name), r)
}

func (s prefixedObjectStore) PutWithDeadline(name string, r io.Reader, deadline time.Time) error {
	return s.base.PutWithDeadline(s.name(name), r, deadline)
}

func (s prefixedObjectStore) PutWithTTL(name string, r io.Reader, ttl time.Duration) error {
	return s.base.PutWithTTL(s.name(name), r, ttl)
}

func (s prefixedObjectStore) Delete(name string) error {
	return s.base.Delete(s.name(name))
}

func (s prefixedObjectStore) DeletePrefix(prefix string) error {
	return s.base.DeletePrefix(s.name(prefix))
}

func (s prefixedObjectStore) List(prefix string) ([]objectstore.ObjectInfo, error) {
	items, err := s.base.List(s.name(prefix))
	if err != nil {
		return nil, err
	}
	basePrefix := s.prefix + "/"
	for i := range items {
		items[i].Name = strings.TrimPrefix(items[i].Name, basePrefix)
	}
	return items, nil
}

func (s prefixedObjectStore) LocalDir() (string, bool) {
	provider, ok := s.base.(objectstore.LocalDirProvider)
	if !ok {
		return "", false
	}
	dir, ok := provider.LocalDir()
	if !ok {
		return "", false
	}
	if s.prefix == "" {
		return dir, true
	}
	return filepath.Join(dir, filepath.FromSlash(s.prefix)), true
}

func (s prefixedObjectStore) name(name string) string {
	name = strings.Trim(name, "/")
	if name == "" {
		return s.prefix
	}
	return s.prefix + "/" + name
}
