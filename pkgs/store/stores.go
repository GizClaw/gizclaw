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
	"github.com/GizClaw/gizclaw-go/pkgs/store/sqlmeta"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

// Kind constants for logical store categories.
const (
	KindKeyValue    = "keyvalue"
	KindMetrics     = "metrics"
	KindLog         = "log"
	KindObjectStore = "objectstore"
	KindSQL         = "sql"
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
	Prefix   string // logical key prefix; SQL KV uses one segment as its table name
	Database string
	Table    string
	TopicID  string
	TTL      time.Duration
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
	logs         map[string]logstore.Store
	sqls         map[string]*sqlx.DB
	logicClosers []io.Closer
	ttls         map[string]time.Duration
}

// ObserveKVReads installs a bounded in-memory read observer before Stores are
// borrowed by services. It does not change Store ownership. The caller must not
// mutate the registry concurrently or install multiple observers for one Store.
func (s *Stores) ObserveKVReads(name string, observe kv.ReadObserver) error {
	store, err := s.KV(name)
	if err != nil {
		return err
	}
	s.kvs[name] = kv.ObserveReads(store, observe)
	return nil
}

// New creates logical stores on top of already-opened physical storage
// backends. TiKV roots start a bounded expiration worker owned by Stores;
// Close cancels and joins it. The caller owns the physical storage lifecycle
// and must close Stores before Storage.
func New(configs map[string]Config, physical *storage.Storage) (*Stores, error) {
	if physical == nil && needsPhysicalStorage(configs) {
		return nil, fmt.Errorf("stores: storage registry is nil")
	}
	if err := validateConfigs(configs, physical); err != nil {
		return nil, err
	}
	if err := validateObjectStorePrefixes(configs); err != nil {
		return nil, err
	}
	if err := validateRemoteKVPrefixes(configs, physical); err != nil {
		return nil, err
	}
	s := &Stores{
		storage: physical,
		kvs:     make(map[string]kv.Store),
		kvRoots: make(map[string]kv.Store),
		objects: make(map[string]objectstore.ObjectStore),
		metrics: make(map[string]metrics.Store),
		logs:    make(map[string]logstore.Store),
		sqls:    make(map[string]*sqlx.DB),
		ttls:    make(map[string]time.Duration),
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
		s.ttls[name] = cfg.TTL
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
		case KindLog:
			st, err := s.newLog(name, cfg)
			if err != nil {
				return nil, &ConfigError{Name: name, Err: err}
			}
			s.logs[name] = st
			s.logicClosers = append(s.logicClosers, st)
		default:
			return nil, &ConfigError{Name: name, Err: fmt.Errorf("stores: %q has unknown kind %q", name, cfg.Kind)}
		}
	}

	ok = true
	return s, nil
}

func validateRemoteKVPrefixes(configs map[string]Config, physical *storage.Storage) error {
	type logicalStore struct {
		name   string
		prefix string
	}
	groups := make(map[string][]logicalStore)
	for name, cfg := range configs {
		if cfg.Kind != KindKeyValue || cfg.Storage == "" {
			continue
		}
		kind, err := physical.Kind(cfg.Storage)
		if err != nil {
			return &ConfigError{Name: name, Err: fmt.Errorf("stores: keyvalue %q resolve storage %q: %w", name, cfg.Storage, err)}
		}
		if kind != storage.KindRedis && kind != storage.KindRedisReadWrite && kind != storage.KindTiKV {
			continue
		}
		prefix, err := parseKeyPrefix(cfg.Prefix)
		if err != nil {
			return &ConfigError{Name: name, Err: fmt.Errorf("stores: keyvalue %q prefix: %w", name, err)}
		}
		canonical := strings.Join(prefix, "/")
		if canonical == "" {
			return &ConfigError{Name: name, Err: fmt.Errorf("stores: remote keyvalue %q requires a non-empty prefix", name)}
		}
		if cfg.Prefix != canonical {
			return &ConfigError{Name: name, Err: fmt.Errorf("stores: remote keyvalue %q prefix %q is not clean; use %q", name, cfg.Prefix, canonical)}
		}
		groups[cfg.Storage] = append(groups[cfg.Storage], logicalStore{name: name, prefix: canonical})
	}
	for physicalName, logical := range groups {
		slices.SortFunc(logical, func(a, b logicalStore) int { return strings.Compare(a.name, b.name) })
		for i := range logical {
			for j := i + 1; j < len(logical); j++ {
				a, b := logical[i], logical[j]
				if a.prefix == b.prefix || strings.HasPrefix(a.prefix, b.prefix+"/") || strings.HasPrefix(b.prefix, a.prefix+"/") {
					err := fmt.Errorf(
						"stores: physical remote KV %q has overlapping prefixes for logical stores %q (%q) and %q (%q)",
						physicalName, a.name, a.prefix, b.name, b.prefix,
					)
					return &ConfigError{Name: a.name, Err: &ConfigError{Name: b.name, Err: err}}
				}
			}
		}
	}
	return nil
}

func validateConfigs(configs map[string]Config, physical *storage.Storage) error {
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		cfg := configs[name]
		if name == "" {
			return &ConfigError{Name: name, Err: errors.New("stores: Store name must not be empty")}
		}
		if cfg.Storage == "" {
			return &ConfigError{Name: name, Err: fmt.Errorf("stores: %s %q requires storage reference", cfg.Kind, name)}
		}
		kind, err := physical.Kind(cfg.Storage)
		if err != nil {
			return &ConfigError{Name: name, Err: fmt.Errorf("stores: %s %q resolve storage %q: %w", cfg.Kind, name, cfg.Storage, err)}
		}
		if err := validateConfigFields(name, cfg, kind); err != nil {
			return &ConfigError{Name: name, Err: err}
		}
	}
	return nil
}

func validateConfigFields(name string, cfg Config, storageKind string) error {
	invalid := func(condition bool, field string) error {
		if condition {
			return fmt.Errorf("stores: %s %q does not support %s", cfg.Kind, name, field)
		}
		return nil
	}
	isSQL := storageKind == storage.KindSQLite || storageKind == storage.KindPostgreSQL
	requireSQLTable := func() error {
		if cfg.Table == "" {
			return fmt.Errorf("stores: %s %q requires table for %s storage", cfg.Kind, name, storageKind)
		}
		if err := sqlmeta.ValidateSQLIdentifier(cfg.Table); err != nil {
			return fmt.Errorf("stores: %s %q table %q: %w", cfg.Kind, name, cfg.Table, err)
		}
		return nil
	}
	switch cfg.Kind {
	case KindKeyValue:
		if cfg.TTL != 0 {
			return fmt.Errorf("stores: keyvalue %q does not support ttl", name)
		}
		if storageKind != storage.KindBadger && storageKind != storage.KindMemory && storageKind != storage.KindRedis && storageKind != storage.KindRedisReadWrite && storageKind != storage.KindTiKV && !isSQL {
			return fmt.Errorf("stores: keyvalue %q does not support storage %q kind %q", name, cfg.Storage, storageKind)
		}
		if err := invalid(cfg.Database != "", "database"); err != nil {
			return err
		}
		if err := invalid(cfg.TopicID != "", "topic_id"); err != nil {
			return err
		}
		if _, err := parseKeyPrefix(cfg.Prefix); err != nil {
			return fmt.Errorf("stores: keyvalue %q prefix: %w", name, err)
		}
		if isSQL {
			if err := invalid(cfg.Table != "", "table"); err != nil {
				return err
			}
			if cfg.Prefix == "" {
				return fmt.Errorf("stores: keyvalue %q requires prefix for %s storage", name, storageKind)
			}
			if err := sqlmeta.ValidateSQLTableName(cfg.Prefix); err != nil {
				return fmt.Errorf("stores: keyvalue %q prefix %q: %w", name, cfg.Prefix, err)
			}
			return nil
		}
		return invalid(cfg.Table != "", "table")
	case KindSQL:
		if cfg.TTL != 0 {
			return fmt.Errorf("stores: sql %q does not support ttl", name)
		}
		if storageKind != storage.KindSQLite && storageKind != storage.KindPostgreSQL && storageKind != storage.KindClickHouse {
			return fmt.Errorf("stores: sql %q does not support storage %q kind %q", name, cfg.Storage, storageKind)
		}
		if cfg.Prefix != "" || cfg.Database != "" || cfg.Table != "" || cfg.TopicID != "" {
			return fmt.Errorf("stores: sql %q does not support logical scope", name)
		}
	case KindObjectStore:
		if cfg.TTL < 0 {
			return fmt.Errorf("stores: objectstore %q ttl must be positive", name)
		}
		if !isObjectStorageKind(storageKind) {
			return fmt.Errorf("stores: objectstore %q does not support storage %q kind %q", name, cfg.Storage, storageKind)
		}
		if cfg.Database != "" || cfg.Table != "" || cfg.TopicID != "" {
			return fmt.Errorf("stores: objectstore %q does not support database, table, or topic_id", name)
		}
	case KindMetrics:
		if cfg.TTL != 0 {
			return fmt.Errorf("stores: metrics %q does not support ttl", name)
		}
		if storageKind != storage.KindMemory && storageKind != storage.KindPrometheus && storageKind != storage.KindClickHouse && !isSQL {
			return fmt.Errorf("stores: metrics %q does not support storage %q kind %q", name, cfg.Storage, storageKind)
		}
		if err := invalid(cfg.Prefix != "", "prefix"); err != nil {
			return err
		}
		if err := invalid(cfg.TopicID != "", "topic_id"); err != nil {
			return err
		}
		if cfg.Database != "" {
			return fmt.Errorf("stores: metrics %q %s storage does not support database", name, storageKind)
		}
		if isSQL || storageKind == storage.KindClickHouse {
			if cfg.Table == "" {
				return fmt.Errorf("stores: metrics %q requires table for %s storage", name, storageKind)
			}
			if isSQL {
				return requireSQLTable()
			}
			return nil
		}
		return invalid(cfg.Table != "", "table")
	case KindLog:
		if storageKind != storage.KindVolcTLS {
			return fmt.Errorf("stores: log %q requires a logging service; storage %q kind %q is unsupported", name, cfg.Storage, storageKind)
		}
		if cfg.TTL != 0 || cfg.Prefix != "" || cfg.Database != "" || cfg.Table != "" {
			return fmt.Errorf("stores: log %q supports only storage and topic_id; retention belongs to the logging service", name)
		}
		if strings.TrimSpace(cfg.TopicID) == "" {
			return fmt.Errorf("stores: log %q volc-tls storage requires topic_id", name)
		}
	default:
		return fmt.Errorf("stores: %q has unknown kind %q", name, cfg.Kind)
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

// TTL returns the initialization-time retention of one logical Store.
func (r *Stores) TTL(name string) (time.Duration, error) {
	ttl, ok := r.ttls[name]
	if !ok {
		return 0, fmt.Errorf("stores: %q not found", name)
	}
	return ttl, nil
}

// Metrics returns the named metrics.Store.
func (r *Stores) Metrics(name string) (metrics.Store, error) {
	s, ok := r.metrics[name]
	if !ok {
		return nil, fmt.Errorf("stores: metrics %q not found", name)
	}
	return s, nil
}

// Log returns the named logstore.Store.
func (r *Stores) Log(name string) (logstore.Store, error) {
	s, ok := r.logs[name]
	if !ok {
		return nil, fmt.Errorf("stores: log %q not found", name)
	}
	return s, nil
}

// Close releases logical stores. Physical storage is always caller-owned.
func (r *Stores) Close() error {
	var errs []error
	for i := range slices.Backward(r.logicClosers) {
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
	if kind != storage.KindBadger && kind != storage.KindMemory && kind != storage.KindRedis && kind != storage.KindRedisReadWrite && kind != storage.KindTiKV && kind != storage.KindSQLite && kind != storage.KindPostgreSQL {
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
	case storage.KindTiKV:
		if existing := r.kvRoots[cfg.Storage]; existing != nil {
			base = existing
			break
		}
		client, readConfig, err := r.storage.TiKV(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q resolve tikv storage %q: %w", name, cfg.Storage, err)
		}
		root, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{StaleReadDelay: readConfig.StaleReadDelay, Labels: readConfig.ReadLabels}, nil)
		base = root
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q use tikv storage %q: %w", name, cfg.Storage, err)
		}
		r.logicClosers = append(r.logicClosers, startTiKVExpiryWorker(root))
		r.kvRoots[cfg.Storage] = base
	case storage.KindRedis:
		if existing := r.kvRoots[cfg.Storage]; existing != nil {
			base = existing
			break
		}
		client, err := r.storage.Redis(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q resolve redis storage %q: %w", name, cfg.Storage, err)
		}
		base, err = kv.NewRedisWithClient(client, nil)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q use redis storage %q: %w", name, cfg.Storage, err)
		}
		r.kvRoots[cfg.Storage] = base
	case storage.KindRedisReadWrite:
		if existing := r.kvRoots[cfg.Storage]; existing != nil {
			base = existing
			break
		}
		read, write, err := r.storage.RedisReadWrite(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q resolve redis.readwrite storage %q: %w", name, cfg.Storage, err)
		}
		base, err = kv.NewRedisReadWriteWithClients(read, write, nil)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q use redis.readwrite storage %q: %w", name, cfg.Storage, err)
		}
		r.kvRoots[cfg.Storage] = base
	case storage.KindSQLite, storage.KindPostgreSQL:
		db, err := r.storage.SQL(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q resolve SQL storage %q: %w", name, cfg.Storage, err)
		}
		root, err := kv.NewSQLWithDB(db, cfg.Prefix, nil)
		if err != nil {
			return nil, fmt.Errorf("stores: keyvalue %q %s prefix %q: %w", name, kind, cfg.Prefix, err)
		}
		r.logicClosers = append(r.logicClosers, root)
		return root, nil
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
	case storage.KindSQLite, storage.KindPostgreSQL:
		db, err := r.storage.SQL(cfg.Storage)
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q resolve %s storage %q: %w", name, kind, cfg.Storage, err)
		}
		st, err := metrics.NewSQLStoreWithDB(db, cfg.Table)
		if err != nil {
			return nil, fmt.Errorf("stores: metrics %q %s table %q: %w", name, kind, cfg.Table, err)
		}
		r.logicClosers = append(r.logicClosers, st)
		return st, nil
	default:
		return nil, fmt.Errorf("stores: metrics %q does not support storage %q kind %q", name, cfg.Storage, kind)
	}
}

func (r *Stores) newLog(name string, cfg Config) (logstore.Store, error) {
	if cfg.Storage == "" {
		return nil, fmt.Errorf("stores: log %q requires storage reference", name)
	}
	kind, err := r.storage.Kind(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("stores: log %q resolve storage %q: %w", name, cfg.Storage, err)
	}
	if kind != storage.KindVolcTLS {
		return nil, fmt.Errorf("stores: log %q requires a logging service; storage %q kind %q is unsupported", name, cfg.Storage, kind)
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
	if !isObjectStorageKind(kind) {
		return nil, fmt.Errorf("stores: objectstore %q does not support storage %q kind %q", name, cfg.Storage, kind)
	}
	var st objectstore.ObjectStore
	switch kind {
	case storage.KindFilesystemDir:
		root, lookupErr := r.storage.FilesystemDir(cfg.Storage)
		if lookupErr != nil {
			err = lookupErr
		} else {
			st, err = objectstore.NewRoot(root)
		}
	case storage.KindVolcTOS:
		client, bucket, lookupErr := r.storage.VolcTOS(cfg.Storage)
		if lookupErr != nil {
			err = lookupErr
		} else {
			st, err = objectstore.NewVolcTOS(client, bucket)
		}
	case storage.KindAliyunOSS:
		bucket, lookupErr := r.storage.AliyunOSS(cfg.Storage)
		if lookupErr != nil {
			err = lookupErr
		} else {
			st, err = objectstore.NewAliyunOSS(bucket)
		}
	case storage.KindGCS:
		client, bucket, lookupErr := r.storage.GCS(cfg.Storage)
		if lookupErr != nil {
			err = lookupErr
		} else {
			st, err = objectstore.NewGCS(client, bucket)
		}
	case storage.KindAzureBlob:
		client, container, lookupErr := r.storage.AzureBlob(cfg.Storage)
		if lookupErr != nil {
			err = lookupErr
		} else {
			st, err = objectstore.NewAzureBlob(client, container)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("stores: objectstore %q %s: %w", name, kind, err)
	}
	prefix, err := parseObjectPrefix(cfg.Prefix)
	if err != nil {
		return nil, fmt.Errorf("stores: objectstore %q prefix: %w", name, err)
	}
	if prefix == "" {
		if cfg.TTL > 0 {
			return retainedObjectStore{ObjectStore: st, ttl: cfg.TTL}, nil
		}
		return st, nil
	}
	st = prefixedObjectStore{base: st, prefix: prefix}
	if cfg.TTL > 0 {
		st = retainedObjectStore{ObjectStore: st, ttl: cfg.TTL}
	}
	return st, nil
}

func isObjectStorageKind(kind string) bool {
	switch kind {
	case storage.KindFilesystemDir, storage.KindVolcTOS, storage.KindAliyunOSS, storage.KindGCS, storage.KindAzureBlob:
		return true
	default:
		return false
	}
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

type retainedObjectStore struct {
	objectstore.ObjectStore
	ttl time.Duration
}

func (s retainedObjectStore) Put(name string, r io.Reader) error {
	return s.ObjectStore.PutWithTTL(name, r, s.ttl)
}

func (s retainedObjectStore) LocalDir() (string, bool) {
	provider, ok := s.ObjectStore.(objectstore.LocalDirProvider)
	if !ok {
		return "", false
	}
	return provider.LocalDir()
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
