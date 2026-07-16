package logging

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type loggerTestStore struct {
	records []logstore.Record
	closes  int
}

func (s *loggerTestStore) Append(_ context.Context, records []logstore.Record) error {
	s.records = append(s.records, records...)
	return nil
}
func (*loggerTestStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (s *loggerTestStore) Close() error { s.closes++; return nil }

type loggerTestResolver struct{ store *loggerTestStore }

func (r loggerTestResolver) Log(string) (logstore.Store, error) { return r.store, nil }

type namedLoggerTestResolver struct {
	stores map[string]*orderedLoggerTestStore
}

func (r namedLoggerTestResolver) Log(name string) (logstore.Store, error) {
	store, ok := r.stores[name]
	if !ok {
		return nil, errors.New("missing store")
	}
	return store, nil
}

type orderedLoggerTestStore struct {
	name  string
	order *[]string
}

func (s *orderedLoggerTestStore) Append(context.Context, []logstore.Record) error {
	*s.order = append(*s.order, s.name)
	return nil
}
func (*orderedLoggerTestStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (*orderedLoggerTestStore) Close() error { return nil }

func TestConfigIsZero(t *testing.T) {
	if !((Config{}).IsZero()) {
		t.Fatal("empty config should be zero")
	}
	if !((Config{Level: "  "}).IsZero()) {
		t.Fatal("blank level should be zero")
	}
	if (Config{Level: "info"}).IsZero() {
		t.Fatal("level should make config non-zero")
	}
	if (Config{Sinks: []SinkConfig{{Kind: SinkStderr}}}).IsZero() {
		t.Fatal("sinks should make config non-zero")
	}
}

func TestNewLoggerDefaultStderrOnly(t *testing.T) {
	logger, cleanup, err := NewLogger(Config{})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	if logger == nil {
		t.Fatal("NewLogger returned nil logger")
	}
	if cleanup == nil {
		t.Fatal("NewLogger returned nil cleanup")
	}
	if !logger.Handler().Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("default logger should enable info")
	}
	if logger.Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("default logger should not enable debug")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
}

func TestNewLoggerRejectsInvalidConfig(t *testing.T) {
	if _, _, err := NewLogger(Config{Level: "verbose"}); err == nil {
		t.Fatal("NewLogger should reject invalid config")
	}
}

func TestNewLoggerStoreSinkUsesFixedSystemScope(t *testing.T) {
	store := &loggerTestStore{}
	logger, cleanup, err := NewLogger(Config{Level: "debug", Sinks: []SinkConfig{{Kind: SinkStore, Store: "logs", Level: "warn"}}}, loggerTestResolver{store: store})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.LogAttrs(context.Background(), slog.LevelInfo, "ignored")
	logger.LogAttrs(context.Background(), slog.LevelWarn, "saved", slog.String("request.id", "1"), slog.Time("at", time.Unix(1, 0)))
	if len(store.records) != 1 || store.records[0].Stream != "system" || store.records[0].Kind != "log" || store.records[0].Attributes["request.id"] != "1" {
		t.Fatalf("records = %+v", store.records)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if store.closes != 0 {
		t.Fatal("logger cleanup closed a registry-owned store")
	}
}

func TestNewLoggerFansOutToNamedStoresInConfiguredOrder(t *testing.T) {
	var order []string
	resolver := namedLoggerTestResolver{stores: map[string]*orderedLoggerTestStore{
		"first":  {name: "first", order: &order},
		"second": {name: "second", order: &order},
	}}
	logger, _, err := NewLogger(Config{Level: "info", Sinks: []SinkConfig{
		{Kind: SinkStore, Store: "first"},
		{Kind: SinkStore, Store: "second", Level: "warn"},
	}}, resolver)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.Info("info")
	logger.Warn("warn")
	want := []string{"first", "first", "second"}
	if !slices.Equal(order, want) {
		t.Fatalf("append order = %v, want %v", order, want)
	}
}

func TestInstallDefaultRestoresPreviousLogger(t *testing.T) {
	previous := slog.Default()
	cleanup, err := InstallDefault(Config{Level: "debug"})
	if err != nil {
		t.Fatalf("InstallDefault() error = %v", err)
	}
	if slog.Default() == previous {
		t.Fatal("InstallDefault did not replace default logger")
	}
	if !slog.Default().Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("installed debug logger should enable debug")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if slog.Default() != previous {
		t.Fatal("cleanup did not restore previous default logger")
	}
}
