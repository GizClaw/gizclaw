package stores

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	physicalstorage "github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/pkgs/store/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/goccy/go-yaml"
	"github.com/jmoiron/sqlx"
)

type fakeDriver struct{}

func (fakeDriver) Open(_ string) (driver.Conn, error) { return fakeConn{}, nil }

type fakeConn struct{}

func (fakeConn) Prepare(_ string) (driver.Stmt, error) { return nil, nil }
func (fakeConn) Close() error                          { return nil }
func (fakeConn) Begin() (driver.Tx, error)             { return nil, nil }
func (fakeConn) Ping(_ context.Context) error          { return nil }

type fakePingFailDriver struct{}

func (fakePingFailDriver) Open(_ string) (driver.Conn, error) { return fakePingFailConn{}, nil }

type fakePingFailConn struct{}

func (fakePingFailConn) Prepare(_ string) (driver.Stmt, error) { return nil, nil }
func (fakePingFailConn) Close() error                          { return nil }
func (fakePingFailConn) Begin() (driver.Tx, error)             { return nil, nil }
func (fakePingFailConn) Ping(_ context.Context) error          { return errors.New("ping refused") }

func init() {
	sql.Register("fake", fakeDriver{})
	sql.Register("fake_ping_fail", fakePingFailDriver{})
	sqlx.BindDriver("fake", sqlx.QUESTION)
	sqlx.BindDriver("fake_ping_fail", sqlx.QUESTION)
}

type fakeLogStore struct{ closes int }

func (*fakeLogStore) Append(context.Context, []logstore.Record) error { return nil }
func (*fakeLogStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (s *fakeLogStore) Close() error { s.closes++; return nil }

func TestLogStoreRegistry(t *testing.T) {
	want := &fakeLogStore{}
	old := openVolcLogStore
	openVolcLogStore = func(logstore.VolcConfig) (logstore.Store, error) { return want, nil }
	t.Cleanup(func() { openVolcLogStore = old })
	registry, err := NewWithStorage(nil, map[string]Config{"logs": {Kind: KindLog, Volc: &logstore.VolcConfig{Endpoint: "endpoint"}}})
	if err != nil {
		t.Fatalf("NewWithStorage() error = %v", err)
	}
	got, err := registry.Log("logs")
	if err != nil || got != want {
		t.Fatalf("Log() = %v, %v", got, err)
	}
	if _, err := registry.Metrics("logs"); err == nil {
		t.Fatal("wrong-kind lookup succeeded")
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	_ = registry.Close()
	if want.closes != 1 {
		t.Fatalf("close count = %d", want.closes)
	}
}

func TestLogStoreRegistryValidatesBackendsAndClosesPartialConstruction(t *testing.T) {
	if _, err := NewWithStorage(nil, map[string]Config{"logs": {Kind: KindLog}}); err == nil {
		t.Fatal("missing Volc backend was accepted")
	}
	if _, err := NewWithStorage(nil, map[string]Config{"logs": {Kind: KindLog, Volc: &logstore.VolcConfig{}, Memory: &struct{}{}}}); err == nil {
		t.Fatal("mixed backend fields were accepted")
	}

	first := &fakeLogStore{}
	calls := 0
	old := openVolcLogStore
	openVolcLogStore = func(logstore.VolcConfig) (logstore.Store, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return nil, errors.New("open failed")
	}
	t.Cleanup(func() { openVolcLogStore = old })
	_, err := NewWithStorage(nil, map[string]Config{
		"first":  {Kind: KindLog, Volc: &logstore.VolcConfig{Endpoint: "first"}},
		"second": {Kind: KindLog, Volc: &logstore.VolcConfig{Endpoint: "second"}},
	})
	if err == nil || first.closes != 1 {
		t.Fatalf("construction error = %v, first closes = %d", err, first.closes)
	}
}

func TestLogStoreRegistryExpandsVolcEnvironment(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_VOLC_TOPIC", "expanded-topic")
	var got logstore.VolcConfig
	old := openVolcLogStore
	openVolcLogStore = func(config logstore.VolcConfig) (logstore.Store, error) {
		got = config
		return &fakeLogStore{}, nil
	}
	t.Cleanup(func() { openVolcLogStore = old })
	registry, err := NewWithStorage(nil, map[string]Config{"logs": {
		Kind: KindLog, Volc: &logstore.VolcConfig{TopicID: "$GIZCLAW_TEST_VOLC_TOPIC"},
	}})
	if err != nil {
		t.Fatalf("NewWithStorage() error = %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if got.TopicID != "expanded-topic" {
		t.Fatalf("topic = %q", got.TopicID)
	}
	if _, exists := legacyStorageConfigs(map[string]Config{"logs": {Kind: KindLog}})["logs"]; exists {
		t.Fatal("logical LogStore was projected as physical storage")
	}
}

func mustStores(t *testing.T, dataDir string, yml []byte) *Stores {
	t.Helper()
	var wrapper struct {
		Stores map[string]Config `yaml:"stores"`
	}
	if err := yaml.Unmarshal(yml, &wrapper); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	s, err := New(resolveTestConfigs(dataDir, wrapper.Stores))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func resolveTestConfigs(baseDir string, configs map[string]Config) map[string]Config {
	if len(configs) == 0 {
		return nil
	}
	resolved := make(map[string]Config, len(configs))
	for name, cfg := range configs {
		if cfg.Dir != "" && baseDir != "" && !filepath.IsAbs(cfg.Dir) {
			cfg.Dir = filepath.Join(baseDir, cfg.Dir)
		}
		resolved[name] = cfg
	}
	return resolved
}

// --- New ---

func TestNewNilConfigs(t *testing.T) {
	s, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	if _, err := s.KV("anything"); err == nil {
		t.Fatal("expected error for empty stores")
	}
}

func TestNewUnknownKind(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: "nosql", Backend: "magic"},
	}); err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestNewWithStorageRequiresRegistry(t *testing.T) {
	if _, err := NewWithStorage(nil, map[string]Config{
		"kv": {Kind: KindKeyValue, Storage: "main"},
	}); err == nil {
		t.Fatal("expected error for nil storage registry")
	}
}

func TestNewRelativeDir(t *testing.T) {
	dir := t.TempDir()
	s, err := New(resolveTestConfigs(dir, map[string]Config{
		"bg": {Kind: KindKeyValue, Backend: "badger", Dir: "bg-data"},
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	if _, err := s.KV("bg"); err != nil {
		t.Fatalf("KV(bg): %v", err)
	}
}

// --- KV ---

func TestKVMemory(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  mem:
    kind: keyvalue
    backend: memory
`))
	defer reg.Close()

	s, err := reg.KV("mem")
	if err != nil {
		t.Fatalf("KV(mem): %v", err)
	}

	ctx := context.Background()
	if err := s.Set(ctx, kv.Key{"a"}, []byte("1")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, kv.Key{"a"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "1" {
		t.Fatalf("Get = %q, want %q", got, "1")
	}

	s2, err := reg.KV("mem")
	if err != nil {
		t.Fatalf("KV(mem) second call: %v", err)
	}
	if s != s2 {
		t.Fatal("expected same instance on second call")
	}
}

func TestKVWithStoragePrefix(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer physical.Close()

	reg, err := NewWithStorage(physical, map[string]Config{
		"peers":       {Kind: KindKeyValue, Storage: "main", Prefix: "peers"},
		"credentials": {Kind: KindKeyValue, Storage: "main", Prefix: "credentials/by-name"},
	})
	if err != nil {
		t.Fatalf("NewWithStorage: %v", err)
	}
	defer reg.Close()

	peers, err := reg.KV("peers")
	if err != nil {
		t.Fatalf("KV(peers): %v", err)
	}
	credentials, err := reg.KV("credentials")
	if err != nil {
		t.Fatalf("KV(credentials): %v", err)
	}
	ctx := context.Background()
	if err := peers.Set(ctx, kv.Key{"abc"}, []byte("peer")); err != nil {
		t.Fatalf("Set peer: %v", err)
	}
	if err := credentials.Set(ctx, kv.Key{"mini-max"}, []byte("secret")); err != nil {
		t.Fatalf("Set credential: %v", err)
	}

	base, err := physical.KV("main")
	if err != nil {
		t.Fatalf("storage KV(main): %v", err)
	}
	if got, err := base.Get(ctx, kv.Key{"peers", "abc"}); err != nil || string(got) != "peer" {
		t.Fatalf("base peer = %q, %v", got, err)
	}
	if got, err := base.Get(ctx, kv.Key{"credentials", "by-name", "mini-max"}); err != nil || string(got) != "secret" {
		t.Fatalf("base credential = %q, %v", got, err)
	}
	if _, err := credentials.Get(ctx, kv.Key{"abc"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("credentials should not see peer key, got %v", err)
	}
}

func TestKVWithStorageInvalidPrefix(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer physical.Close()

	if _, err := NewWithStorage(physical, map[string]Config{
		"bad": {Kind: KindKeyValue, Storage: "main", Prefix: "bad:prefix"},
	}); err == nil {
		t.Fatal("expected error for invalid prefix")
	}
	if _, err := NewWithStorage(physical, map[string]Config{
		"bad": {Kind: KindKeyValue, Storage: "main", Prefix: "bad//prefix"},
	}); err == nil {
		t.Fatal("expected error for empty prefix segment")
	}
}

func TestKVBadger(t *testing.T) {
	dir := t.TempDir()
	reg := mustStores(t, dir, []byte(`
stores:
  bg:
    kind: keyvalue
    backend: badger
    dir: bg-data
`))
	defer reg.Close()

	s, err := reg.KV("bg")
	if err != nil {
		t.Fatalf("KV(bg): %v", err)
	}
	ctx := context.Background()
	if err := s.Set(ctx, kv.Key{"k"}, []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, kv.Key{"k"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("Get = %q", got)
	}
}

func TestKVBadgerAbsoluteDir(t *testing.T) {
	absDir := filepath.Join(t.TempDir(), "abs-badger")
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  bg-abs:
    kind: keyvalue
    backend: badger
    dir: `+absDir+`
`))
	defer reg.Close()

	if _, err := reg.KV("bg-abs"); err != nil {
		t.Fatalf("KV(bg-abs): %v", err)
	}
}

func TestKVNotFound(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  mem:
    kind: keyvalue
    backend: memory
  vec:
    kind: vecstore
    backend: memory
`))
	defer reg.Close()

	if _, err := reg.KV("missing"); err == nil {
		t.Fatal("expected error for missing store")
	}
	if _, err := reg.KV("vec"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

func TestKVWithStorageWrongKindReference(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"objects": {Kind: physicalstorage.KindObjectStore, FS: &physicalstorage.FSConfig{Dir: t.TempDir()}},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer physical.Close()

	if _, err := NewWithStorage(physical, map[string]Config{
		"kv": {Kind: KindKeyValue, Storage: "objects"},
	}); err == nil {
		t.Fatal("expected error for keyvalue store referencing object storage")
	}
}

func TestNewKVUnknownBackend(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindKeyValue, Backend: "redis"},
	}); err == nil {
		t.Fatal("expected error for unknown kv backend")
	}
}

func TestNewKVBadgerNoDir(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindKeyValue, Backend: "badger"},
	}); err == nil {
		t.Fatal("expected error for badger without dir")
	}
}

// --- VecStore ---

func TestVecStoreMemory(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  vec:
    kind: vecstore
    backend: memory
`))
	defer reg.Close()

	idx, err := reg.VecStore("vec")
	if err != nil {
		t.Fatalf("VecStore(vec): %v", err)
	}
	if err := idx.Insert("a", []float32{1, 0, 0}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if idx.Len() != 1 {
		t.Fatalf("Len = %d", idx.Len())
	}

	idx2, err := reg.VecStore("vec")
	if err != nil {
		t.Fatalf("VecStore(vec) second: %v", err)
	}
	if idx != idx2 {
		t.Fatal("expected same instance")
	}
}

func TestVecStoreNotFound(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  kv:
    kind: keyvalue
    backend: memory
`))
	defer reg.Close()

	if _, err := reg.VecStore("missing"); err == nil {
		t.Fatal("expected error for missing")
	}
	if _, err := reg.VecStore("kv"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

func TestNewVecStoreUnknownBackend(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindVecStore, Backend: "qdrant"},
	}); err == nil {
		t.Fatal("expected error for unknown vecstore backend")
	}
}

// --- Graph ---

func TestGraphKV(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  mem:
    kind: keyvalue
    backend: memory
  g:
    kind: graph
    backend: kv
    store: mem
`))
	defer reg.Close()

	g, err := reg.Graph("g")
	if err != nil {
		t.Fatalf("Graph(g): %v", err)
	}
	ctx := context.Background()
	if err := g.SetEntity(ctx, graph.Entity{Label: "alice"}); err != nil {
		t.Fatalf("SetEntity: %v", err)
	}
	e, err := g.GetEntity(ctx, "alice")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if e.Label != "alice" {
		t.Fatalf("Label = %q", e.Label)
	}

	g2, err := reg.Graph("g")
	if err != nil {
		t.Fatalf("Graph(g) second: %v", err)
	}
	if g != g2 {
		t.Fatal("expected same instance")
	}
}

func TestGraphNotFound(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  kv:
    kind: keyvalue
    backend: memory
`))
	defer reg.Close()

	if _, err := reg.Graph("missing"); err == nil {
		t.Fatal("expected error for missing")
	}
	if _, err := reg.Graph("kv"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

func TestNewGraphNoStoreRef(t *testing.T) {
	if _, err := New(map[string]Config{
		"g": {Kind: KindGraph, Backend: "kv"},
	}); err == nil {
		t.Fatal("expected error for missing store reference")
	}
}

func TestNewGraphBadStoreRef(t *testing.T) {
	if _, err := New(map[string]Config{
		"g": {Kind: KindGraph, Backend: "kv", Store: "nonexistent"},
	}); err == nil {
		t.Fatal("expected error for undefined kv reference")
	}
}

func TestNewGraphWrongKindRef(t *testing.T) {
	if _, err := New(resolveTestConfigs(t.TempDir(), map[string]Config{
		"vec": {Kind: KindVecStore, Backend: "memory"},
		"g":   {Kind: KindGraph, Backend: "kv", Store: "vec"},
	})); err == nil {
		t.Fatal("expected error for kv ref pointing at non-kv store")
	}
}

func TestNewGraphUnknownBackend(t *testing.T) {
	if _, err := New(map[string]Config{
		"g": {Kind: KindGraph, Backend: "neo4j"},
	}); err == nil {
		t.Fatal("expected error for unknown graph backend")
	}
}

// --- Metrics ---

func TestMetricsPrometheus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"gizclaw_peer_battery_percent","peer_id":"p1"},"value":[10,"82"]}]}}`)
	}))
	defer server.Close()

	var wrapper struct {
		Stores map[string]Config `yaml:"stores"`
	}
	if err := yaml.Unmarshal([]byte(`
stores:
  metrics:
    kind: metrics
    prometheus:
      remote_write_url: `+server.URL+`/api/v1/write
      query_url: `+server.URL+`
      bearer_token: token
`), &wrapper); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	reg, err := New(wrapper.Stores)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	st, err := reg.Metrics("metrics")
	if err != nil {
		t.Fatalf("Metrics(metrics): %v", err)
	}
	series, err := st.Latest(context.Background(), metrics.LatestQuery{Selector: metrics.Selector{Name: "gizclaw_peer_battery_percent"}, At: time.Unix(11, 0), Lookback: time.Minute})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(series) != 1 || series[0].Name != "gizclaw_peer_battery_percent" || series[0].Labels["peer_id"] != "p1" {
		t.Fatalf("series = %+v", series)
	}
}

func TestMetricsNotFound(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  mem:
    kind: keyvalue
    backend: memory
`))
	defer reg.Close()

	if _, err := reg.Metrics("missing"); err == nil {
		t.Fatal("expected error for missing metrics store")
	}
	if _, err := reg.Metrics("mem"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

func TestNewMetricsRequiresPrometheusConfig(t *testing.T) {
	if _, err := New(map[string]Config{
		"metrics": {Kind: KindMetrics},
	}); err == nil || !strings.Contains(err.Error(), "requires exactly one") {
		t.Fatalf("New metrics missing prometheus err = %v", err)
	}
	if _, err := New(map[string]Config{
		"metrics": {Kind: KindMetrics, Prometheus: &metrics.PrometheusConfig{QueryURL: "http://example.test"}},
	}); err == nil || !strings.Contains(err.Error(), "remote_write_url is required") {
		t.Fatalf("New metrics missing remote_write_url err = %v", err)
	}
}

func TestNewMetricsRejectsMultipleBackends(t *testing.T) {
	_, err := New(map[string]Config{"metrics": {Kind: KindMetrics, Memory: &struct{}{}, Prometheus: &metrics.PrometheusConfig{}}})
	if err == nil || !strings.Contains(err.Error(), "requires exactly one") {
		t.Fatalf("New metrics multiple backends err = %v", err)
	}
}

func TestMetricsMemory(t *testing.T) {
	var wrapper struct {
		Stores map[string]Config `yaml:"stores"`
	}
	if err := yaml.Unmarshal([]byte(`
stores:
  metrics:
    kind: metrics
    memory: {}
`), &wrapper); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	reg, err := New(wrapper.Stores)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer reg.Close()

	st, err := reg.Metrics("metrics")
	if err != nil {
		t.Fatalf("Metrics(metrics): %v", err)
	}
	if err := st.Append(context.Background(), []metrics.Sample{{
		Name:      "gizclaw_peer_battery_percent",
		Labels:    map[string]string{"peer_id": "p1"},
		Timestamp: time.Unix(10, 0).UTC(),
		Value:     82,
	}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	series, err := st.Latest(context.Background(), metrics.LatestQuery{
		Selector: metrics.Selector{Name: "gizclaw_peer_battery_percent", Matchers: []metrics.LabelMatcher{{Name: "peer_id", Op: metrics.MatchEqual, Value: "p1"}}},
		At:       time.Unix(10, 0).UTC(), Lookback: time.Minute,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 82 {
		t.Fatalf("series = %+v", series)
	}
}

// --- SQL ---

func TestSQL(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  db:
    kind: sql
    backend: fake
    dsn: test
`))
	defer reg.Close()

	db, err := reg.SQL("db")
	if err != nil {
		t.Fatalf("SQL(db): %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil *sqlx.DB")
	}

	db2, err := reg.SQL("db")
	if err != nil {
		t.Fatalf("SQL(db) second: %v", err)
	}
	if db != db2 {
		t.Fatal("expected same instance")
	}
}

func TestSQLWithDSN(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  db:
    kind: sql
    backend: fake
    dsn: mydb
`))
	defer reg.Close()

	db, err := reg.SQL("db")
	if err != nil {
		t.Fatalf("SQL(db) with dsn: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil *sqlx.DB")
	}
}

func TestSQLNotFound(t *testing.T) {
	reg := mustStores(t, t.TempDir(), []byte(`
stores:
  kv:
    kind: keyvalue
    backend: memory
`))
	defer reg.Close()

	if _, err := reg.SQL("missing"); err == nil {
		t.Fatal("expected error for missing")
	}
	if _, err := reg.SQL("kv"); err == nil {
		t.Fatal("expected error for wrong kind lookup")
	}
}

// --- ObjectStore ---

func TestObjectStoreWithStoragePrefix(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"assets": {Kind: physicalstorage.KindObjectStore, FS: &physicalstorage.FSConfig{Dir: t.TempDir()}},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer physical.Close()

	reg, err := NewWithStorage(physical, map[string]Config{
		"firmware-assets": {Kind: KindObjectStore, Storage: "assets", Prefix: "firmware"},
	})
	if err != nil {
		t.Fatalf("NewWithStorage: %v", err)
	}
	defer reg.Close()

	objects, err := reg.ObjectStore("firmware-assets")
	if err != nil {
		t.Fatalf("ObjectStore(firmware-assets): %v", err)
	}
	if err := objects.Put("stable.bin", strings.NewReader("stable")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	base, err := physical.ObjectStore("assets")
	if err != nil {
		t.Fatalf("storage ObjectStore(assets): %v", err)
	}
	r, err := base.Get("firmware/stable.bin")
	if err != nil {
		t.Fatalf("base Get: %v", err)
	}
	got, err := io.ReadAll(r)
	if closeErr := r.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "stable" {
		t.Fatalf("base object = %q, want stable", got)
	}

	items, err := objects.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "stable.bin" {
		t.Fatalf("List = %#v, want stable.bin", items)
	}
}

func TestObjectStoreNotFound(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"assets": {Kind: physicalstorage.KindObjectStore, FS: &physicalstorage.FSConfig{Dir: t.TempDir()}},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer physical.Close()

	reg, err := NewWithStorage(physical, map[string]Config{
		"assets": {Kind: KindObjectStore, Storage: "assets"},
	})
	if err != nil {
		t.Fatalf("NewWithStorage: %v", err)
	}
	defer reg.Close()

	if _, err := reg.ObjectStore("missing"); err == nil {
		t.Fatal("expected error for missing store")
	}
	if _, err := reg.ObjectStore("assets"); err != nil {
		t.Fatalf("ObjectStore(assets): %v", err)
	}
}

func TestNewObjectStoreBadStorageReference(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"kv":     {Kind: physicalstorage.KindKeyValue, Memory: &physicalstorage.MemoryConfig{}},
		"assets": {Kind: physicalstorage.KindObjectStore, FS: &physicalstorage.FSConfig{Dir: t.TempDir()}},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer physical.Close()

	if _, err := NewWithStorage(physical, map[string]Config{
		"objects": {Kind: KindObjectStore, Storage: "kv"},
	}); err == nil {
		t.Fatal("expected error for objectstore referencing keyvalue storage")
	}
	if _, err := NewWithStorage(physical, map[string]Config{
		"objects": {Kind: KindObjectStore, Storage: "missing"},
	}); err == nil {
		t.Fatal("expected error for missing objectstore storage")
	}
	if _, err := NewWithStorage(physical, map[string]Config{
		"objects": {Kind: KindObjectStore, Storage: "assets", Prefix: "../bad"},
	}); err == nil {
		t.Fatal("expected error for invalid objectstore prefix")
	}
}

func TestNewSQLNoBackend(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindSQL, DSN: "x"},
	}); err == nil {
		t.Fatal("expected error for empty backend")
	}
}

func TestNewSQLNoDSN(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindSQL, Backend: "fake"},
	}); err == nil {
		t.Fatal("expected error for missing dsn")
	}
}

func TestNewSQLBadDriver(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindSQL, Backend: "nonexistent_driver", DSN: "x"},
	}); err == nil {
		t.Fatal("expected error for unregistered driver")
	}
}

func TestNewSQLPingFail(t *testing.T) {
	if _, err := New(map[string]Config{
		"x": {Kind: KindSQL, Backend: "fake_ping_fail", DSN: "x"},
	}); err == nil {
		t.Fatal("expected error for ping failure")
	}
}

// --- Close ---

func TestCloseOrder(t *testing.T) {
	dir := t.TempDir()
	reg := mustStores(t, dir, []byte(`
stores:
  bg:
    kind: keyvalue
    backend: badger
    dir: close-test
  g:
    kind: graph
    backend: kv
    store: bg
`))

	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewWithOwnedStorageClosesPhysicalRegistry(t *testing.T) {
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"memory": {Kind: physicalstorage.KindKeyValue, Memory: &physicalstorage.MemoryConfig{}},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	reg, err := NewWithOwnedStorage(physical, map[string]Config{
		"kv": {Kind: KindKeyValue, Storage: "memory"},
	})
	if err != nil {
		t.Fatalf("NewWithOwnedStorage: %v", err)
	}
	if !reg.ownsStorage {
		t.Fatal("expected registry to own physical storage")
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if reg.storage != nil {
		t.Fatal("expected Close to release physical storage reference")
	}
}

func TestNewWithOwnedStorageClosesPhysicalOnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kv")
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"badger": {Kind: physicalstorage.KindKeyValue, Badger: &physicalstorage.BadgerConfig{Dir: dir}},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	_, err = NewWithOwnedStorage(physical, map[string]Config{
		"kv": {Kind: KindKeyValue, Storage: "badger", Prefix: "bad:prefix"},
	})
	if err == nil {
		t.Fatal("expected NewWithOwnedStorage error")
	}

	reopened, err := physicalstorage.New(map[string]physicalstorage.Config{
		"badger": {Kind: physicalstorage.KindKeyValue, Badger: &physicalstorage.BadgerConfig{Dir: dir}},
	})
	if err != nil {
		t.Fatalf("storage should be closed after constructor error, reopen: %v", err)
	}
	defer reopened.Close()
}

func TestCloseEmpty(t *testing.T) {
	s, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close empty: %v", err)
	}
}
