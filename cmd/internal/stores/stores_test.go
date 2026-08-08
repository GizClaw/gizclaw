package stores

import (
	"context"
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
)

func newPhysical(t *testing.T, configs map[string]physicalstorage.Config) *physicalstorage.Storage {
	t.Helper()
	registry, err := physicalstorage.New(configs)
	if err != nil {
		t.Fatalf("storage.New() error = %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	return registry
}

func TestNewWithStorageAcceptsEmptyRegistry(t *testing.T) {
	registry, err := NewWithStorage(nil, nil)
	if err != nil {
		t.Fatalf("NewWithStorage() error = %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if _, err := registry.KV("missing"); err == nil {
		t.Fatal("KV(missing) succeeded")
	}
}

func TestNewWithStorageRejectsUnknownKindAndMissingRegistry(t *testing.T) {
	if _, err := NewWithStorage(nil, map[string]Config{"bad": {Kind: "nosql"}}); err == nil {
		t.Fatal("unknown kind was accepted")
	}
	if _, err := NewWithStorage(nil, map[string]Config{
		"peers": {Kind: KindKeyValue, Storage: "main"},
	}); err == nil || !strings.Contains(err.Error(), "storage registry is nil") {
		t.Fatalf("missing storage registry error = %v", err)
	}
}

func TestNewWithStorageRejectsEmptyStoreNameAndForeignKindFields(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Memory: &physicalstorage.MemoryConfig{}},
	})
	if _, err := NewWithStorage(physical, map[string]Config{
		"": {Kind: KindKeyValue, Storage: "main"},
	}); err == nil || !strings.Contains(err.Error(), "Store name must not be empty") {
		t.Fatalf("empty name error = %v", err)
	}
	if _, err := NewWithStorage(physical, map[string]Config{
		"peers": {Kind: KindKeyValue, Storage: "main", ClickHouse: &ClickHouseConfig{Table: "ignored"}},
	}); err == nil || !strings.Contains(err.Error(), "does not support clickhouse") {
		t.Fatalf("foreign field error = %v", err)
	}
}

func TestKVStoresScopeOnePhysicalConnector(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Memory: &physicalstorage.MemoryConfig{}},
	})
	registry, err := NewWithStorage(physical, map[string]Config{
		"peers":       {Kind: KindKeyValue, Storage: "main", Prefix: "peers"},
		"credentials": {Kind: KindKeyValue, Storage: "main", Prefix: "credentials/by-name"},
	})
	if err != nil {
		t.Fatalf("NewWithStorage() error = %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })

	peers, err := registry.KV("peers")
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := registry.KV("credentials")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := peers.Set(ctx, kv.Key{"p1"}, []byte("peer")); err != nil {
		t.Fatal(err)
	}
	if err := credentials.Set(ctx, kv.Key{"provider"}, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	base, err := physical.KV("main")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := base.Get(ctx, kv.Key{"peers", "p1"}); err != nil || string(got) != "peer" {
		t.Fatalf("base peer = %q, %v", got, err)
	}
	if got, err := base.Get(ctx, kv.Key{"credentials", "by-name", "provider"}); err != nil || string(got) != "secret" {
		t.Fatalf("base credential = %q, %v", got, err)
	}
	if _, err := credentials.Get(ctx, kv.Key{"p1"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("credentials saw peer key: %v", err)
	}
}

func TestKVStoreRejectsInvalidOrWrongPhysicalStorage(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"main":    {Kind: physicalstorage.KindKeyValue, Memory: &physicalstorage.MemoryConfig{}},
		"objects": {Kind: physicalstorage.KindObjectStore, FS: &physicalstorage.FSConfig{Dir: t.TempDir()}},
	})
	tests := []Config{
		{Kind: KindKeyValue},
		{Kind: KindKeyValue, Storage: "main", Prefix: "bad:prefix"},
		{Kind: KindKeyValue, Storage: "main", Prefix: "bad//prefix"},
		{Kind: KindKeyValue, Storage: "objects"},
	}
	for index, config := range tests {
		if _, err := NewWithStorage(physical, map[string]Config{"bad": config}); err == nil {
			t.Fatalf("case %d was accepted", index)
		}
	}
}

func TestVecStoreUsesPhysicalConnector(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"vectors": {Kind: physicalstorage.KindVecStore, Memory: &physicalstorage.MemoryConfig{}},
	})
	registry, err := NewWithStorage(physical, map[string]Config{
		"memory": {Kind: KindVecStore, Storage: "vectors"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	index, err := registry.VecStore("memory")
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Insert("a", []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if index.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", index.Len())
	}
	if _, err := registry.KV("memory"); err == nil {
		t.Fatal("wrong-kind lookup succeeded")
	}
}

func TestGraphUsesLogicalKVStore(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Memory: &physicalstorage.MemoryConfig{}},
	})
	registry, err := NewWithStorage(physical, map[string]Config{
		"entities": {Kind: KindKeyValue, Storage: "main", Prefix: "entities"},
		"graph":    {Kind: KindGraph, Backend: "kv", Store: "entities"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	g, err := registry.Graph("graph")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetEntity(context.Background(), graph.Entity{Label: "alice"}); err != nil {
		t.Fatal(err)
	}
	entity, err := g.GetEntity(context.Background(), "alice")
	if err != nil || entity.Label != "alice" {
		t.Fatalf("GetEntity() = %+v, %v", entity, err)
	}
}

func TestGraphRejectsInvalidLogicalReference(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"vectors": {Kind: physicalstorage.KindVecStore, Memory: &physicalstorage.MemoryConfig{}},
	})
	for _, configs := range []map[string]Config{
		{"graph": {Kind: KindGraph, Backend: "kv"}},
		{"graph": {Kind: KindGraph, Backend: "neo4j"}},
		{"vectors": {Kind: KindVecStore, Storage: "vectors"}, "graph": {Kind: KindGraph, Backend: "kv", Store: "vectors"}},
	} {
		if _, err := NewWithStorage(physical, configs); err == nil {
			t.Fatalf("invalid graph config was accepted: %+v", configs)
		}
	}
}

func TestMetricsMemoryStore(t *testing.T) {
	registry, err := NewWithStorage(nil, map[string]Config{
		"metrics": {Kind: KindMetrics, Memory: &struct{}{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	store, err := registry.Metrics("metrics")
	if err != nil {
		t.Fatal(err)
	}
	sampleTime := time.Unix(10, 0).UTC()
	if err := store.Append(context.Background(), []metrics.Sample{{
		Name: "gizclaw_peer_battery_percent", Labels: map[string]string{"peer_id": "p1"},
		Timestamp: sampleTime, Value: 82,
	}}); err != nil {
		t.Fatal(err)
	}
	series, err := store.Latest(context.Background(), metrics.LatestQuery{
		Selector: metrics.Selector{Name: "gizclaw_peer_battery_percent"}, At: sampleTime, Lookback: time.Minute,
	})
	if err != nil || len(series) != 1 || series[0].Points[0].Value != 82 {
		t.Fatalf("Latest() = %+v, %v", series, err)
	}
}

func TestMetricsPrometheusUsesPhysicalConnector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/query" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"gizclaw_peer_battery_percent","peer_id":"p1"},"value":[10,"82"]}]}}`)
	}))
	defer server.Close()
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"prometheus": {
			Kind: physicalstorage.KindPrometheus,
			Prometheus: &metrics.PrometheusConfig{
				RemoteWriteURL: server.URL + "/api/v1/write", QueryURL: server.URL, BearerToken: "token",
			},
		},
	})
	registry, err := NewWithStorage(physical, map[string]Config{
		"metrics": {Kind: KindMetrics, Storage: "prometheus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	store, err := registry.Metrics("metrics")
	if err != nil {
		t.Fatal(err)
	}
	series, err := store.Latest(context.Background(), metrics.LatestQuery{
		Selector: metrics.Selector{Name: "gizclaw_peer_battery_percent"}, At: time.Unix(11, 0), Lookback: time.Minute,
	})
	if err != nil || len(series) != 1 || series[0].Labels["peer_id"] != "p1" {
		t.Fatalf("Latest() = %+v, %v", series, err)
	}
}

func TestMetricsRejectsAmbiguousOrWrongBackend(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Memory: &physicalstorage.MemoryConfig{}},
	})
	for _, config := range []Config{
		{Kind: KindMetrics},
		{Kind: KindMetrics, Storage: "main", Memory: &struct{}{}},
		{Kind: KindMetrics, Storage: "main"},
	} {
		if _, err := NewWithStorage(physical, map[string]Config{"metrics": config}); err == nil {
			t.Fatalf("invalid metrics config was accepted: %+v", config)
		}
	}
}

func TestSQLStoreBorrowsPhysicalPool(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"database": {Kind: physicalstorage.KindSQL, SQLite: &physicalstorage.SQLConfig{Dir: filepath.Join(t.TempDir(), "db.sqlite")}},
	})
	registry, err := NewWithStorage(physical, map[string]Config{
		"history": {Kind: KindSQL, Storage: "database"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	logical, err := registry.SQL("history")
	if err != nil {
		t.Fatal(err)
	}
	base, err := physical.SQL("database")
	if err != nil {
		t.Fatal(err)
	}
	if logical != base {
		t.Fatal("logical SQL store did not borrow the physical pool")
	}
}

func TestObjectStoreScopesAndListsPhysicalConnector(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"assets": {Kind: physicalstorage.KindObjectStore, FS: &physicalstorage.FSConfig{Dir: t.TempDir()}},
	})
	registry, err := NewWithStorage(physical, map[string]Config{
		"firmware": {Kind: KindObjectStore, Storage: "assets", Prefix: "firmware"},
		"gameplay": {Kind: KindObjectStore, Storage: "assets", Prefix: "gameplay"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	firmware, err := registry.ObjectStore("firmware")
	if err != nil {
		t.Fatal(err)
	}
	if err := firmware.Put("stable.bin", strings.NewReader("stable")); err != nil {
		t.Fatal(err)
	}
	base, err := physical.ObjectStore("assets")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := base.Get("firmware/stable.bin")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || string(data) != "stable" {
		t.Fatalf("read = %q, %v, close = %v", data, readErr, closeErr)
	}
	items, err := firmware.List("")
	if err != nil || len(items) != 1 || items[0].Name != "stable.bin" {
		t.Fatalf("List() = %+v, %v", items, err)
	}
}

func TestSharedObjectStorePrefixesMustBeNonEmptyAndDisjoint(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"assets": {Kind: physicalstorage.KindObjectStore, FS: &physicalstorage.FSConfig{Dir: t.TempDir()}},
	})
	tests := []struct {
		name   string
		stores map[string]Config
		want   string
	}{
		{"empty", map[string]Config{"a": {Kind: KindObjectStore, Storage: "assets"}, "b": {Kind: KindObjectStore, Storage: "assets", Prefix: "b"}}, "requires a non-empty prefix"},
		{"same", map[string]Config{"a": {Kind: KindObjectStore, Storage: "assets", Prefix: "icons"}, "b": {Kind: KindObjectStore, Storage: "assets", Prefix: "icons"}}, "overlapping prefixes"},
		{"parent-child", map[string]Config{"a": {Kind: KindObjectStore, Storage: "assets", Prefix: "icons"}, "b": {Kind: KindObjectStore, Storage: "assets", Prefix: "icons/workflows"}}, "overlapping prefixes"},
		{"unclean", map[string]Config{"a": {Kind: KindObjectStore, Storage: "assets", Prefix: "/icons/"}}, "is not clean"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWithStorage(physical, test.stores)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewWithStorage() error = %v, want %q", err, test.want)
			}
		})
	}
}

type fakeMutableLogStore struct{}

func (*fakeMutableLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}
func (*fakeMutableLogStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (*fakeMutableLogStore) Replace(context.Context, logstore.Record) error   { return nil }
func (*fakeMutableLogStore) Delete(context.Context, logstore.RecordKey) error { return nil }
func (*fakeMutableLogStore) Close() error                                     { return nil }

func TestMutableLogRequiresMutableDeclaration(t *testing.T) {
	store := &fakeMutableLogStore{}
	registry := &Stores{
		logs:        map[string]logstore.ImmutableStore{"immutable": store, "mutable": store},
		mutableLogs: map[string]struct{}{"mutable": {}},
	}
	if _, err := registry.MutableLog("immutable"); err == nil || !strings.Contains(err.Error(), KindLogMutable) {
		t.Fatalf("MutableLog(immutable) error = %v", err)
	}
	if got, err := registry.MutableLog("mutable"); err != nil || got != store {
		t.Fatalf("MutableLog(mutable) = %v, %v", got, err)
	}
}

func TestLogConfigRequiresPhysicalConnectorAndScope(t *testing.T) {
	physical := newPhysical(t, map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Memory: &physicalstorage.MemoryConfig{}},
	})
	for _, config := range []Config{
		{Kind: KindLogImmutable},
		{Kind: KindLogImmutable, Storage: "main"},
		{Kind: KindLogImmutable, Storage: "main", Volc: &VolcConfig{}, ClickHouse: &ClickHouseConfig{}},
		{Kind: KindLogImmutable, Storage: "main", Volc: &VolcConfig{TopicID: "logs"}},
		{Kind: KindLogMutable, Storage: "main", Volc: &VolcConfig{TopicID: "logs"}},
	} {
		if _, err := NewWithStorage(physical, map[string]Config{"logs": config}); err == nil {
			t.Fatalf("invalid log config was accepted: %+v", config)
		}
	}
}

func TestNewWithOwnedStorageClosesPhysicalRegistry(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "badger")
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Badger: &physicalstorage.BadgerConfig{Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewWithOwnedStorage(physical, map[string]Config{
		"peers": {Kind: KindKeyValue, Storage: "main", Prefix: "peers"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !registry.ownsStorage {
		t.Fatal("registry did not own physical storage")
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := physicalstorage.New(map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Badger: &physicalstorage.BadgerConfig{Dir: dir}},
	})
	if err != nil {
		t.Fatalf("physical storage was not closed: %v", err)
	}
	_ = reopened.Close()
}

func TestNewWithOwnedStorageClosesPhysicalOnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "badger")
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Badger: &physicalstorage.BadgerConfig{Dir: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewWithOwnedStorage(physical, map[string]Config{
		"bad": {Kind: KindKeyValue, Storage: "main", Prefix: "bad:prefix"},
	})
	if err == nil {
		t.Fatal("invalid logical store was accepted")
	}
	reopened, err := physicalstorage.New(map[string]physicalstorage.Config{
		"main": {Kind: physicalstorage.KindKeyValue, Badger: &physicalstorage.BadgerConfig{Dir: dir}},
	})
	if err != nil {
		t.Fatalf("physical storage was not closed after error: %v", err)
	}
	_ = reopened.Close()
}
