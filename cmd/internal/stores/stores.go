// Package stores provides a configuration-driven registry for logical stores.
// Logical stores reference physical backends from cmd/internal/storage and can
// expose scoped views such as prefixed KV stores.
package stores

import (
	"context"
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
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/vecstore"
	"github.com/jmoiron/sqlx"
)

// Kind constants for logical store categories.
const (
	KindKeyValue    = storage.KindKeyValue
	KindVecStore    = storage.KindVecStore
	KindGraph       = "graph"
	KindMetrics     = "metrics"
	KindLog         = "log"
	KindMemoryStore = "memory"
	KindObjectStore = storage.KindObjectStore
	KindSQL         = storage.KindSQL
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

	Backend string `yaml:"backend"` // legacy backend field; graph backend is still logical
	Dir     string `yaml:"dir"`     // legacy physical dir field
	Store   string `yaml:"store"`   // graph backend "kv": reference to a logical keyvalue store
	Dim     int    `yaml:"dim"`     // legacy vecstore dimension field
	DSN     string `yaml:"dsn"`     // legacy sql connection string field

	Prometheus *metrics.PrometheusConfig    `yaml:"prometheus"`
	ClickHouse *ClickHouseConfig            `yaml:"clickhouse"`
	Memory     *struct{}                    `yaml:"memory"`
	Volc       *logstore.VolcConfig         `yaml:"volc"`
	Flowcraft  *memorystore.FlowcraftConfig `yaml:"flowcraft"`
	Mem0       *memorystore.Mem0Config      `yaml:"mem0"`
	VolcMemory *memorystore.VolcConfig      `yaml:"volc_memory"`
}

// Options supplies runtime dependencies that cannot be represented in YAML.
type Options struct {
	FlowcraftModelLoader memorystore.FlowcraftModelLoader
	VolcResolver         memorystore.VolcCredentialResolver
	HTTPClient           memorystore.HTTPClient
}

// ClickHouseConfig is the command-layer connection configuration projected
// into either a metrics or log store driver according to Config.Kind.
type ClickHouseConfig struct {
	DSN      string `yaml:"dsn"`
	Database string `yaml:"database"`
	Table    string `yaml:"table"`
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
	memories     map[string]memorystore.Store
	sqls         map[string]*sqlx.DB
	logicClosers []io.Closer
}

// New creates a Stores instance from the legacy one-layer config. New callers
// should use NewWithStorage with a separate physical storage registry.
func New(configs map[string]Config) (*Stores, error) {
	return NewWithOptions(context.Background(), configs, Options{})
}

// NewWithOptions creates a Stores instance from the legacy one-layer config
// with explicit remote and model dependencies.
func NewWithOptions(ctx context.Context, configs map[string]Config, options Options) (*Stores, error) {
	physical, err := storage.New(legacyStorageConfigs(configs))
	if err != nil {
		return nil, err
	}
	s, err := NewWithStorageOptions(ctx, physical, legacyStoreConfigs(configs), options)
	if err != nil {
		_ = physical.Close()
		return nil, err
	}
	s.ownsStorage = true
	return s, nil
}

// NewWithOwnedStorage creates logical stores and transfers ownership of the
// provided physical storage registry to the returned Stores.
func NewWithOwnedStorage(physical *storage.Storage, configs map[string]Config) (*Stores, error) {
	return NewWithOwnedStorageOptions(context.Background(), physical, configs, Options{})
}

// NewWithOwnedStorageOptions creates logical stores with explicit remote and
// model dependencies and transfers physical registry ownership.
func NewWithOwnedStorageOptions(ctx context.Context, physical *storage.Storage, configs map[string]Config, options Options) (*Stores, error) {
	s, err := NewWithStorageOptions(ctx, physical, configs, options)
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
	return NewWithStorageOptions(context.Background(), physical, configs, Options{})
}

// NewWithStorageOptions creates logical stores with explicit remote and model
// dependencies. The caller owns the physical storage lifecycle.
func NewWithStorageOptions(ctx context.Context, physical *storage.Storage, configs map[string]Config, options Options) (*Stores, error) {
	if err := validateMemoryProviderOwnership(configs); err != nil {
		return nil, err
	}
	if physical == nil && needsPhysicalStorage(configs) {
		return nil, fmt.Errorf("stores: storage registry is nil")
	}
	if err := validateObjectStorePrefixes(configs); err != nil {
		return nil, err
	}
	s := &Stores{
		storage:  physical,
		kvs:      make(map[string]kv.Store),
		vecs:     make(map[string]vecstore.Index),
		objects:  make(map[string]objectstore.ObjectStore),
		graphs:   make(map[string]graph.Graph),
		metrics:  make(map[string]metrics.Store),
		logs:     make(map[string]logstore.ImmutableStore),
		memories: make(map[string]memorystore.Store),
		sqls:     make(map[string]*sqlx.DB),
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
	for name, cfg := range configs {
		switch cfg.Kind {
		case KindKeyValue:
			st, err := s.newKV(name, cfg)
			if err != nil {
				return nil, err
			}
			s.kvs[name] = st
		case KindVecStore:
			st, err := s.newVecStore(name, cfg)
			if err != nil {
				return nil, err
			}
			s.vecs[name] = st
		case KindSQL:
			st, err := s.newSQL(name, cfg)
			if err != nil {
				return nil, err
			}
			s.sqls[name] = st
		case KindObjectStore:
			st, err := s.newObjectStore(name, cfg)
			if err != nil {
				return nil, err
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
				return nil, err
			}
			s.metrics[name] = st
			s.logicClosers = append(s.logicClosers, st)
		case KindLog:
			st, err := s.newLog(name, cfg)
			if err != nil {
				return nil, err
			}
			s.logs[name] = st
			s.logicClosers = append(s.logicClosers, st)
		case KindMemoryStore:
			st, closer, err := s.newMemory(ctx, name, cfg, options)
			if err != nil {
				return nil, err
			}
			s.memories[name] = st
			if closer != nil {
				s.logicClosers = append(s.logicClosers, closer)
			}
		default:
			return nil, fmt.Errorf("stores: %q has unknown kind %q", name, cfg.Kind)
		}
	}

	for _, g := range graphCfgs {
		st, err := s.newGraph(g.name, g.cfg)
		if err != nil {
			return nil, err
		}
		s.graphs[g.name] = st
		s.logicClosers = append(s.logicClosers, st)
	}

	ok = true
	return s, nil
}

func hasMemoryProviderConfig(cfg Config) bool {
	return cfg.Flowcraft != nil || cfg.Mem0 != nil || cfg.VolcMemory != nil
}

func validateMemoryProviderOwnership(configs map[string]Config) error {
	for name, cfg := range configs {
		if cfg.Kind != KindMemoryStore && hasMemoryProviderConfig(cfg) {
			return fmt.Errorf("stores: %q kind %q contains memory provider fields", name, cfg.Kind)
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
			return fmt.Errorf("stores: objectstore %q prefix: %w", name, err)
		}
		if cfg.Prefix != prefix {
			return fmt.Errorf("stores: objectstore %q prefix %q is not clean; use %q", name, cfg.Prefix, prefix)
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
				return fmt.Errorf("stores: physical objectstore %q is shared; logical store %q requires a non-empty prefix", physical, store.name)
			}
		}
		for i := range logical {
			for j := i + 1; j < len(logical); j++ {
				a, b := logical[i], logical[j]
				if a.prefix == b.prefix || strings.HasPrefix(a.prefix, b.prefix+"/") || strings.HasPrefix(b.prefix, a.prefix+"/") {
					return fmt.Errorf("stores: physical objectstore %q has overlapping prefixes for logical stores %q (%q) and %q (%q)", physical, a.name, a.prefix, b.name, b.prefix)
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

// Memory returns the named provider-neutral memory store.
func (r *Stores) Memory(name string) (memorystore.Store, error) {
	s, ok := r.memories[name]
	if !ok {
		return nil, fmt.Errorf("stores: memory %q not found", name)
	}
	return s, nil
}

// MutableLog returns the named log store when its driver supports record
// replacement and deletion.
func (r *Stores) MutableLog(name string) (logstore.MutableStore, error) {
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
	memory := cfg.Memory != nil || cfg.Backend == "memory"
	count := 0
	if memory {
		count++
	}
	if cfg.Prometheus != nil {
		count++
	}
	if cfg.ClickHouse != nil {
		count++
	}
	if count != 1 {
		return nil, fmt.Errorf("stores: metrics %q requires exactly one of memory, prometheus, or clickhouse", name)
	}
	if memory {
		return metrics.NewMemoryStore(), nil
	}
	if cfg.Prometheus != nil {
		st, err := metrics.NewPrometheusStore(*cfg.Prometheus)
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q prometheus: %w", name, err)
		}
		return st, nil
	}
	if cfg.ClickHouse.Database != "" {
		return nil, fmt.Errorf("stores: metrics %q clickhouse database must be selected by the dsn", name)
	}
	st, err := metrics.NewClickHouseStore(metrics.ClickHouseConfig{
		DSN:   os.ExpandEnv(cfg.ClickHouse.DSN),
		Table: os.ExpandEnv(cfg.ClickHouse.Table),
	})
	if err != nil {
		return nil, fmt.Errorf("stores: metrics %q clickhouse: %w", name, err)
	}
	return st, nil
}

var openVolcLogStore = func(config logstore.VolcConfig) (logstore.ImmutableStore, error) {
	return logstore.NewVolcStore(config)
}

var openClickHouseLogStore = func(config logstore.ClickHouseConfig) (logstore.MutableStore, error) {
	return logstore.NewClickHouseStore(config)
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
	if cfg.Storage != "" || cfg.Prefix != "" || cfg.Backend != "" || cfg.Dir != "" || cfg.Store != "" || cfg.Dim != 0 || cfg.DSN != "" || cfg.Prometheus != nil || cfg.Memory != nil {
		return nil, fmt.Errorf("stores: log %q contains fields owned by another store kind", name)
	}
	if cfg.ClickHouse != nil {
		clickhouse := *cfg.ClickHouse
		clickhouse.DSN = os.ExpandEnv(clickhouse.DSN)
		clickhouse.Database = os.ExpandEnv(clickhouse.Database)
		clickhouse.Table = os.ExpandEnv(clickhouse.Table)
		store, err := openClickHouseLogStore(logstore.ClickHouseConfig{
			DSN:      clickhouse.DSN,
			Database: clickhouse.Database,
			Table:    clickhouse.Table,
		})
		if err != nil {
			return nil, fmt.Errorf("stores: log %q clickhouse: %w", name, err)
		}
		return store, nil
	}
	volc := *cfg.Volc
	volc.Endpoint = os.ExpandEnv(volc.Endpoint)
	volc.Region = os.ExpandEnv(volc.Region)
	volc.TopicID = os.ExpandEnv(volc.TopicID)
	volc.AccessKeyID = os.ExpandEnv(volc.AccessKeyID)
	volc.AccessKeySecret = os.ExpandEnv(volc.AccessKeySecret)
	st, err := openVolcLogStore(volc)
	if err != nil {
		return nil, fmt.Errorf("stores: log %q volc: %w", name, err)
	}
	return st, nil
}

func (r *Stores) newMemory(ctx context.Context, name string, cfg Config, options Options) (memorystore.Store, io.Closer, error) {
	backendCount := 0
	for _, configured := range []bool{cfg.Flowcraft != nil, cfg.Mem0 != nil, cfg.VolcMemory != nil} {
		if configured {
			backendCount++
		}
	}
	if backendCount != 1 {
		return nil, nil, fmt.Errorf("stores: memory %q requires exactly one of flowcraft, mem0, or volc_memory", name)
	}
	if cfg.Storage != "" || cfg.Prefix != "" || cfg.Backend != "" || cfg.Dir != "" || cfg.Store != "" || cfg.Dim != 0 || cfg.DSN != "" || cfg.Prometheus != nil || cfg.ClickHouse != nil || cfg.Memory != nil || cfg.Volc != nil {
		return nil, nil, fmt.Errorf("stores: memory %q contains fields owned by another store kind", name)
	}
	if cfg.Flowcraft != nil {
		config := *cfg.Flowcraft
		var err error
		config.Dir, err = expandFlowcraftDir(config.Dir)
		if err != nil {
			return nil, nil, fmt.Errorf("stores: memory %q flowcraft: %w", name, err)
		}
		expandFlowcraftConfig(&config)
		store, err := memorystore.OpenFlowcraftStore(ctx, config, options.FlowcraftModelLoader)
		if err != nil {
			return nil, nil, fmt.Errorf("stores: memory %q flowcraft: %w", name, err)
		}
		return store, store, nil
	}
	if cfg.Mem0 != nil {
		config := *cfg.Mem0
		expandMem0Config(&config)
		store, err := memorystore.NewMem0Store(config, options.HTTPClient)
		if err != nil {
			return nil, nil, fmt.Errorf("stores: memory %q mem0: %w", name, err)
		}
		return store, nil, nil
	}
	config := *cfg.VolcMemory
	expandVolcMemoryConfig(&config)
	store, err := memorystore.OpenVolcStore(ctx, config, options.VolcResolver, options.HTTPClient)
	if err != nil {
		return nil, nil, fmt.Errorf("stores: memory %q volc_memory: %w", name, err)
	}
	return store, nil, nil
}

func expandFlowcraftConfig(config *memorystore.FlowcraftConfig) {
	config.RuntimeID = os.ExpandEnv(config.RuntimeID)
	config.AgentID = os.ExpandEnv(config.AgentID)
	config.UserID = os.ExpandEnv(config.UserID)
	config.ExtractionModel = os.ExpandEnv(config.ExtractionModel)
	config.EmbeddingModel = os.ExpandEnv(config.EmbeddingModel)
	config.RerankModel = os.ExpandEnv(config.RerankModel)
	config.ExtractionMode = os.ExpandEnv(config.ExtractionMode)
	config.SystemPrompt = os.ExpandEnv(config.SystemPrompt)
	config.SchemaName = os.ExpandEnv(config.SchemaName)
	config.Async.WorkerID = os.ExpandEnv(config.Async.WorkerID)
}

func expandFlowcraftDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return dir, nil
	}
	missing := make(map[string]struct{})
	expanded := os.Expand(dir, func(name string) string {
		value, ok := os.LookupEnv(name)
		if !ok {
			missing[name] = struct{}{}
		}
		return value
	})
	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name := range missing {
			names = append(names, name)
		}
		slices.Sort(names)
		return "", fmt.Errorf("%w: flowcraft dir references unset environment variables: %s", memorystore.ErrInvalidInput, strings.Join(names, ", "))
	}
	if strings.TrimSpace(expanded) == "" {
		return "", fmt.Errorf("%w: configured flowcraft dir expands to an empty path", memorystore.ErrInvalidInput)
	}
	return expanded, nil
}

func expandMem0Config(config *memorystore.Mem0Config) {
	config.Endpoint = os.ExpandEnv(config.Endpoint)
	config.APIKey = os.ExpandEnv(config.APIKey)
	config.Flavor = memorystore.Mem0Flavor(os.ExpandEnv(string(config.Flavor)))
	config.AppID = os.ExpandEnv(config.AppID)
	config.UserID = os.ExpandEnv(config.UserID)
	config.AgentID = os.ExpandEnv(config.AgentID)
	config.RunID = os.ExpandEnv(config.RunID)
}

func expandVolcMemoryConfig(config *memorystore.VolcConfig) {
	expandMem0Config(&config.Mem0)
	config.APIKeyID = os.ExpandEnv(config.APIKeyID)
	config.MemoryProjectID = os.ExpandEnv(config.MemoryProjectID)
	config.ControlEndpoint = os.ExpandEnv(config.ControlEndpoint)
	config.Region = os.ExpandEnv(config.Region)
	config.AccessKeyID = os.ExpandEnv(config.AccessKeyID)
	config.AccessKeySecret = os.ExpandEnv(config.AccessKeySecret)
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

func legacyStorageConfigs(configs map[string]Config) map[string]storage.Config {
	if len(configs) == 0 {
		return nil
	}
	out := make(map[string]storage.Config, len(configs))
	for name, cfg := range configs {
		if cfg.Kind == KindGraph || cfg.Kind == KindMetrics || cfg.Kind == KindLog || cfg.Kind == KindMemoryStore {
			continue
		}
		out[name] = storage.Config{
			Kind:    cfg.Kind,
			Backend: cfg.Backend,
			Dir:     cfg.Dir,
			Dim:     cfg.Dim,
			DSN:     cfg.DSN,
		}
	}
	return out
}

func legacyStoreConfigs(configs map[string]Config) map[string]Config {
	if len(configs) == 0 {
		return nil
	}
	out := make(map[string]Config, len(configs))
	for name, cfg := range configs {
		if cfg.Kind != KindGraph && cfg.Kind != KindMetrics && cfg.Kind != KindLog && cfg.Kind != KindMemoryStore && cfg.Storage == "" {
			cfg.Storage = name
		}
		out[name] = cfg
	}
	return out
}

func needsPhysicalStorage(configs map[string]Config) bool {
	for _, cfg := range configs {
		switch cfg.Kind {
		case KindGraph, KindMetrics, KindLog, KindMemoryStore:
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
