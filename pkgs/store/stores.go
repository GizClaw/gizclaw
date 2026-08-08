// Package store provides a configuration-driven registry for logical stores.
// Logical stores reference physical backends from store/storage and can
// expose scoped views such as prefixed KV stores.
package store

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

// Kind constants for logical store categories.
const (
	KindKeyValue     = "keyvalue"
	KindMetrics      = "metrics"
	KindLogImmutable = "log.immutable"
	KindLogMutable   = "log.mutable"
	KindObjectStore  = "objectstore"
	KindSQL          = "sql"
)

// Config describes one logical Store entry. Serialization belongs to callers.
//
//	stores:
//	  peers:
//	    kind: keyvalue
//	    storage: main-kv
//	    prefix: peers
type Config struct {
	Kind     string
	Storage  string // reference to a physical storage backend
	Prefix   string // slash-separated logical key prefix for KV stores
	Database string
	Table    string
	TopicID  string
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

// Stores holds named logical store instances created eagerly by New.
type Stores struct {
	storage      *storage.Storage
	kvs          map[string]kv.Store
	kvRoots      map[string]kv.Store
	objects      map[string]objectstore.ObjectStore
	metrics      map[string]metrics.Store
	logs         map[string]logstore.ImmutableStore
	mutableLogs  map[string]struct{}
	sqls         map[string]*sqlx.DB
	logicClosers []io.Closer
}

// New creates logical stores on top of already-opened physical storage
// backends. The caller always owns the physical storage lifecycle.
func New(configs map[string]Config, physical *storage.Storage) (*Stores, error) {
	if physical == nil && needsPhysicalStorage(configs) {
		return nil, fmt.Errorf("stores: storage registry is nil")
	}
	if err := validateObjectStorePrefixes(configs); err != nil {
		return nil, err
	}
	s := &Stores{
		storage:     physical,
		kvs:         make(map[string]kv.Store),
		kvRoots:     make(map[string]kv.Store),
		objects:     make(map[string]objectstore.ObjectStore),
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
		case KindMetrics:
			st, err := s.newMetrics(name, cfg)
			if err != nil {
				return nil, &ConfigError{Name: name, Err: err}
			}
			s.metrics[name] = st
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
	switch cfg.Kind {
	case KindKeyValue, KindObjectStore:
		for _, field := range []struct {
			set  bool
			name string
		}{
			{cfg.Database != "", "database"}, {cfg.Table != "", "table"}, {cfg.TopicID != "", "topic_id"},
		} {
			if err := invalid(field.set, field.name); err != nil {
				return err
			}
		}
	case KindSQL:
		if err := invalid(cfg.Prefix != "", "prefix"); err != nil {
			return err
		}
		if cfg.Database != "" || cfg.Table != "" || cfg.TopicID != "" {
			return fmt.Errorf("stores: sql %q does not support logical scope", name)
		}
	case KindMetrics:
		if err := invalid(cfg.Prefix != "", "prefix"); err != nil {
			return err
		}
		if err := invalid(cfg.TopicID != "", "topic_id"); err != nil {
			return err
		}
	case KindLogImmutable, KindLogMutable:
		if err := invalid(cfg.Prefix != "", "prefix"); err != nil {
			return err
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

// Close releases logical stores. Physical storage is always caller-owned.
func (r *Stores) Close() error {
	var errs []error
	for i := len(r.logicClosers) - 1; i >= 0; i-- {
		if err := r.logicClosers[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	r.logicClosers = nil
	return errors.Join(errs...)
}

// --- factory methods ---

func (r *Stores) newKV(name string, cfg Config) (kv.Store, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: keyvalue %q requires storage reference", name)
	}
	kind, err := r.storage.Kind(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: keyvalue %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	if kind != storage.KindBadger && kind != storage.KindMemory {
		return nil, fmt.Errorf("stores: keyvalue %q does not support storage %q kind %q", name, cfg.Storage, kind)
	}
	var base kv.Store
	switch kind {
	case storage.KindBadger:
		if existing := r.kvRoots[cfg.Storage]; existing != nil {
			base = existing
			break
		}
		db, err := r.storage.Badger(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q resolve storage %q: %w", name, cfg.Storage, err)
		}
		base, err = kv.NewBadgerWithDB(db, nil)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q use badger storage %q: %w", name, cfg.Storage, err)
		}
		r.kvRoots[cfg.Storage] = base
	case storage.KindMemory:
		if _, err := r.storage.Memory(cfg.Storage); err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q resolve storage %q: %w", name, cfg.Storage, err)
		}
		base = kv.NewMemory(nil)
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

func (r *Stores) newMetrics(name string, cfg Config) (metrics.Store, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: metrics %q requires storage reference", name)
	}
	kind, err := r.storage.Kind(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: metrics %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	switch kind {
	case storage.KindMemory:
		if _, err := r.storage.Memory(cfg.Storage); err != nil {
			return nil, fmt.Errorf("stores: metrics %q resolve memory storage %q: %w", name, cfg.Storage, err)
		}
		if cfg.Table != "" || cfg.Database != "" {
			return nil, fmt.Errorf("stores: metrics %q memory storage does not support table or database", name)
		}
		st := metrics.NewMemoryStore()
		r.logicClosers = append(r.logicClosers, st)
		return st, nil
	case storage.KindPrometheus:
		if cfg.Table != "" || cfg.Database != "" {
			return nil, fmt.Errorf("stores: metrics %q prometheus storage does not support table or database", name)
		}
		client, remoteWriteURL, err := r.storage.Prometheus(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q resolve prometheus storage %q: %w", name, cfg.Storage, err)
		}
		connector, err := metrics.NewPrometheusConnectorWithClient(client, remoteWriteURL)
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q prometheus: %w", name, err)
		}
		st, err := connector.Store()
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q prometheus: %w", name, err)
		}
		r.logicClosers = append(r.logicClosers, st)
		return st, nil
	case storage.KindClickHouse:
		if cfg.Database != "" {
			return nil, fmt.Errorf("stores: metrics %q clickhouse database must be selected by the dsn", name)
		}
		db, err := r.storage.SQL(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q resolve clickhouse storage %q: %w", name, cfg.Storage, err)
		}
		st, err := metrics.NewClickHouseStoreWithDB(db.DB, cfg.Table)
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q clickhouse: %w", name, err)
		}
		r.logicClosers = append(r.logicClosers, st)
		return st, nil
	default:
		return nil, fmt.Errorf("stores: metrics %q does not support storage %q kind %q", name, cfg.Storage, kind)
	}
}

func (r *Stores) newLog(name string, cfg Config) (logstore.ImmutableStore, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: log %q requires storage reference", name)
	}
	kind, err := r.storage.Kind(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: log %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	switch kind {
	case storage.KindClickHouse:
		db, err := r.storage.SQL(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: log %q resolve clickhouse storage %q: %w", name, cfg.Storage, err)
		}
		store, err := logstore.NewClickHouseStoreWithDB(db.DB, cfg.Database, cfg.Table)
		if err != nil {
			return nil, fmt.Errorf("stores: log %q clickhouse: %w", name, err)
		}
		return store, nil
	case storage.KindVolcTLS:
		if cfg.Kind == KindLogMutable {
			return nil, fmt.Errorf("stores: %s %q does not support volc-tls", cfg.Kind, name)
		}
		if cfg.Database != "" || cfg.Table != "" {
			return nil, fmt.Errorf("stores: log %q volc-tls storage does not support database or table", name)
		}
		client, err := r.storage.VolcTLS(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: log %q resolve volc-tls storage %q: %w", name, cfg.Storage, err)
		}
		connector, err := logstore.NewVolcConnectorWithClient(client)
		if err != nil {
			return nil, fmt.Errorf("stores: log %q volc-tls: %w", name, err)
		}
		st, err := connector.Store(cfg.TopicID)
		if err != nil {
			return nil, fmt.Errorf("stores: log %q volc-tls: %w", name, err)
		}
		return st, nil
	default:
		return nil, fmt.Errorf("stores: %s %q does not support storage %q kind %q", cfg.Kind, name, cfg.Storage, kind)
	}
}

func (r *Stores) newSQL(name string, cfg Config) (*sqlx.DB, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: sql %q requires storage reference", name)
	}
	kind, err := r.storage.Kind(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: sql %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	if kind != storage.KindSQLite && kind != storage.KindPostgreSQL && kind != storage.KindClickHouse {
		return nil, fmt.Errorf("stores: sql %q does not support storage %q kind %q", name, cfg.Storage, kind)
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
	kind, err := r.storage.Kind(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: objectstore %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	if kind != storage.KindFilesystemDir {
		return nil, fmt.Errorf("stores: objectstore %q does not support storage %q kind %q", name, cfg.Storage, kind)
	}
	root, err := r.storage.FilesystemDir(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: objectstore %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	st, err := objectstore.NewRoot(root)
	if err != nil {
		return nil, fmt.Errorf("stores: objectstore %q filesystem.dir: %w", name, err)
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

func needsPhysicalStorage(configs map[string]Config) bool {
	return len(configs) != 0
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
	name, err := s.name(name)
	if err != nil {
		return nil, err
	}
	return s.base.Get(name)
}

func (s prefixedObjectStore) Put(name string, r io.Reader) error {
	name, err := s.name(name)
	if err != nil {
		return err
	}
	return s.base.Put(name, r)
}

func (s prefixedObjectStore) PutWithDeadline(name string, r io.Reader, deadline time.Time) error {
	name, err := s.name(name)
	if err != nil {
		return err
	}
	return s.base.PutWithDeadline(name, r, deadline)
}

func (s prefixedObjectStore) PutWithTTL(name string, r io.Reader, ttl time.Duration) error {
	name, err := s.name(name)
	if err != nil {
		return err
	}
	return s.base.PutWithTTL(name, r, ttl)
}

func (s prefixedObjectStore) Delete(name string) error {
	name, err := s.name(name)
	if err != nil {
		return err
	}
	return s.base.Delete(name)
}

func (s prefixedObjectStore) DeletePrefix(prefix string) error {
	prefix, err := s.name(prefix)
	if err != nil {
		return err
	}
	return s.base.DeletePrefix(prefix)
}

func (s prefixedObjectStore) List(prefix string) ([]objectstore.ObjectInfo, error) {
	prefix, err := s.name(prefix)
	if err != nil {
		return nil, err
	}
	items, err := s.base.List(prefix)
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

func (s prefixedObjectStore) name(name string) (string, error) {
	if strings.HasPrefix(name, "/") || filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("stores: invalid absolute object name %q", name)
	}
	name = strings.Trim(name, "/")
	if name == "" {
		return s.prefix, nil
	}
	return s.prefix + "/" + name, nil
}
