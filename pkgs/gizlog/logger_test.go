package gizlog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type loggerTestStore struct {
	mu      sync.Mutex
	records []logstore.Record
	closes  int
}

func (s *loggerTestStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, records...)
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}
func (*loggerTestStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (s *loggerTestStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closes++
	return nil
}

type loggerTestResolver struct{ store *loggerTestStore }

func (r loggerTestResolver) Log(string) (logstore.Store, error) { return r.store, nil }

type namedLoggerTestResolver struct {
	stores map[string]*loggerTestStore
}

func (r namedLoggerTestResolver) Log(name string) (logstore.Store, error) {
	store, ok := r.stores[name]
	if !ok {
		return nil, errors.New("missing store")
	}
	return store, nil
}

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
	if (Config{NodeID: "node-a"}).IsZero() {
		t.Fatal("node ID should make config non-zero")
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
	logger, cleanup, err := NewLogger(Config{Level: "debug", NodeID: "server-a", Sinks: []SinkConfig{{Kind: SinkStore, Store: "logs", Level: "warn"}}}, loggerTestResolver{store: store})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.LogAttrs(context.Background(), slog.LevelInfo, "ignored")
	logger.LogAttrs(context.Background(), slog.LevelWarn, "saved", slog.String("request.id", "1"), slog.Time("at", time.Unix(1, 0)))
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if len(store.records) != 1 || store.records[0].Stream != "system" || store.records[0].Kind != "log" || store.records[0].Attributes["request.id"] != "1" || store.records[0].Attributes["node_id"] != "server-a" || store.records[0].Attributes["source_file"] == "" || store.records[0].Attributes["source_line"] == "" {
		t.Fatalf("records = %+v", store.records)
	}
	contextual, ok := logger.Handler().(*contextHandler)
	if !ok {
		t.Fatalf("logger handler = %#v", logger.Handler())
	}
	fanout, ok := contextual.root.(*FanoutHandler)
	if !ok || len(fanout.handlers) != 1 {
		t.Fatalf("logger handler = %#v", logger.Handler())
	}
	if _, ok := fanout.handlers[0].(*storeFailureReportingHandler); !ok {
		t.Fatalf("store handler = %T", fanout.handlers[0])
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if store.closes != 0 {
		t.Fatal("logger cleanup closed a registry-owned store")
	}
}

func TestNewLoggerKeepsBinaryIdentityOnGroupedRecords(t *testing.T) {
	store := &loggerTestStore{}
	logger, cleanup, err := NewLogger(Config{
		NodeID:         "server-a",
		RuntimeVersion: "0.35.9",
		BuildCommit:    "source-commit",
		Sinks:          []SinkConfig{{Kind: SinkStore, Store: "logs"}},
	}, loggerTestResolver{store: store})
	if err != nil {
		t.Fatal(err)
	}
	logger = logger.With("runtime_version", "caller-version").WithGroup("request")
	logger.WarnContext(WithPeerPublicKey(context.Background(), "peer-a"), "saved",
		"build_commit", "caller-commit", "method", "GET")
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if len(store.records) != 1 {
		t.Fatalf("records = %d, want 1", len(store.records))
	}
	attrs := store.records[0].Attributes
	if attrs["runtime_version"] != "0.35.9" || attrs["build_commit"] != "source-commit" ||
		attrs["node_id"] != "server-a" || attrs["peer_public_key"] != "peer-a" || attrs["request.method"] != "GET" {
		t.Fatalf("record attributes = %+v", attrs)
	}
	if _, exists := attrs["request.build_commit"]; exists {
		t.Fatal("caller build identity was retained")
	}
}

func TestNewLoggerKeepsBinaryIdentityOnInlineAttributes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		attr          slog.Attr
		wantPreserved bool
	}{
		{name: "inline group", attr: slog.Group("", "runtime_version", "caller-version", "build_commit", "caller-commit", "kept", "value"), wantPreserved: true},
		{name: "dynamic inline group", attr: slog.Any("", loggerTestIdentityGroup{}), wantPreserved: true},
		{name: "dotted version", attr: slog.String("runtime_version.child", "caller-version")},
		{name: "dotted commit", attr: slog.String("build_commit.child", "caller-commit")},
		{name: "normalized version", attr: slog.String(" runtime_version ", "caller-version")},
	} {
		for _, child := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/child=%t", tc.name, child), func(t *testing.T) {
				store := &loggerTestStore{}
				logger, cleanup, err := NewLogger(Config{
					RuntimeVersion: "binary-version",
					BuildCommit:    "binary-commit",
					Sinks:          []SinkConfig{{Kind: SinkStore, Store: "logs"}},
				}, loggerTestResolver{store: store})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := cleanup(); err != nil {
						t.Error(err)
					}
				})
				attrs := []slog.Attr{slog.String("method", "GET")}
				if child {
					logger = logger.With(tc.attr)
				} else {
					attrs = append(attrs, tc.attr)
				}
				logger.LogAttrs(t.Context(), slog.LevelWarn, "saved", attrs...)
				if err := cleanup(); err != nil {
					t.Fatal(err)
				}
				if len(store.records) != 1 {
					t.Fatalf("records = %d, want 1", len(store.records))
				}
				got := store.records[0].Attributes
				if got["runtime_version"] != "binary-version" || got["build_commit"] != "binary-commit" || got["method"] != "GET" {
					t.Fatalf("record attributes = %+v", got)
				}
				if tc.wantPreserved && got["kept"] != "value" {
					t.Fatalf("ordinary inline group attributes were dropped: %+v", got)
				}
			})
		}
	}
}

type loggerTestIdentityGroup struct{}

func (loggerTestIdentityGroup) LogValue() slog.Value {
	return slog.GroupValue(slog.String("runtime_version", "caller-version"), slog.String("build_commit", "caller-commit"), slog.String("kept", "value"))
}

func TestStoreFailureReportingHandlerReportsWithoutProviderText(t *testing.T) {
	storeErr := errors.New("provider body contains a secret")
	fallbackState := &recordingState{}
	queue := newStoreQueue(&recordingHandler{min: slog.LevelInfo, state: fallbackState})
	handler := newStoreFailureReportingHandler(
		&recordingHandler{min: slog.LevelInfo, err: storeErr},
		queue,
		"logs",
	)
	err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "original", 0))
	if !errors.Is(err, storeErr) {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(fallbackState.records) != 0 {
		t.Fatal("failure reporting ran on the producer")
	}
	go queue.run()
	if err := queue.close(); err != nil {
		t.Fatal(err)
	}
	if len(fallbackState.records) != 1 || fallbackState.records[0].Message != "system log store records rejected" {
		t.Fatalf("fallback records = %#v", fallbackState.records)
	}
	if fallbackState.attrs["store"] != "logs" || strings.Contains(fallbackState.records[0].Message, "provider body") {
		t.Fatalf("fallback attrs = %#v record = %#v", fallbackState.attrs, fallbackState.records[0])
	}
	frame, _ := runtime.CallersFrames([]uintptr{fallbackState.records[0].PC}).Next()
	if !strings.HasSuffix(frame.File, "store_queue.go") || frame.Line <= 0 {
		t.Fatalf("fallback source = %s:%d", frame.File, frame.Line)
	}
}

func TestNewLoggerFansOutToNamedStoresWithSinkLevels(t *testing.T) {
	resolver := namedLoggerTestResolver{stores: map[string]*loggerTestStore{
		"first":  {},
		"second": {},
	}}
	logger, cleanup, err := NewLogger(Config{Level: "info", Sinks: []SinkConfig{
		{Kind: SinkStore, Store: "first"},
		{Kind: SinkStore, Store: "second", Level: "warn"},
	}}, resolver)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.Info("info")
	logger.Warn("warn")
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]string{"first": {"info", "warn"}, "second": {"warn"}} {
		var messages []string
		for _, record := range resolver.stores[name].records {
			messages = append(messages, record.Message)
		}
		slices.Sort(messages)
		if !slices.Equal(messages, want) {
			t.Fatalf("store %s messages = %v, want %v", name, messages, want)
		}
	}
}

func TestInstallDefaultRestoresPreviousLogger(t *testing.T) {
	previous := slog.Default()
	cleanup, err := InstallDefault(Config{Level: "debug"})
	if err != nil {
		t.Fatalf("InstallDefault() error = %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
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

func TestInstallDefaultRejectsOverlappingLease(t *testing.T) {
	previous := slog.Default()
	firstCleanup, err := InstallDefault(Config{Level: "debug"})
	if err != nil {
		t.Fatalf("first InstallDefault() error = %v", err)
	}
	t.Cleanup(func() { _ = firstCleanup() })
	first := slog.Default()
	secondCleanup, err := InstallDefault(Config{Level: "warn"})
	if !errors.Is(err, ErrDefaultLoggerInstalled) {
		t.Fatalf("second InstallDefault() error = %v, want %v", err, ErrDefaultLoggerInstalled)
	}
	if secondCleanup != nil {
		t.Fatal("second InstallDefault returned cleanup without owning the logger lease")
	}
	if slog.Default() != first {
		t.Fatal("rejected InstallDefault changed the active logger")
	}
	if err := firstCleanup(); err != nil {
		t.Fatalf("first cleanup() error = %v", err)
	}
	if slog.Default() != previous {
		t.Fatal("first cleanup did not restore previous default logger")
	}

	thirdCleanup, err := InstallDefault(Config{Level: "info"})
	if err != nil {
		t.Fatalf("InstallDefault() after release error = %v", err)
	}
	t.Cleanup(func() { _ = thirdCleanup() })
	if err := thirdCleanup(); err != nil {
		t.Fatalf("third cleanup() error = %v", err)
	}
}
