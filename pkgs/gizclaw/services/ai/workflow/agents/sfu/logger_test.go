package sfu

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger/pionlogger"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

type sdkLogStore struct {
	mu      sync.Mutex
	records []logstore.Record
}

func (s *sdkLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, records...)
	keys := make([]logstore.RecordKey, len(records))
	for i, record := range records {
		keys[i] = record.Key()
	}
	return keys, nil
}

func (*sdkLogStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}

func (*sdkLogStore) Close() error { return nil }

func (s *sdkLogStore) Log(string) (logstore.Store, error) { return s, nil }

func sdkTestLogger(t *testing.T, level string) (*slog.Logger, *sdkLogStore, func() error) {
	t.Helper()
	store := &sdkLogStore{}
	logger, cleanup, err := gizlog.NewLogger(gizlog.Config{Level: level, NodeID: "test-server", Sinks: []gizlog.SinkConfig{{Kind: gizlog.SinkStore, Store: "logs"}}}, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	return logger, store, cleanup
}

func TestLiveKitLoggerPreservesLevelsThroughConfiguredStore(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  []string
	}{
		{level: "debug", want: []string{"DEBUG", "ERROR", "INFO", "WARN"}},
		{level: "info", want: []string{"ERROR", "INFO", "WARN"}},
		{level: "warn", want: []string{"ERROR", "WARN"}},
		{level: "error", want: []string{"ERROR"}},
	} {
		t.Run(tc.level, func(t *testing.T) {
			logger, store, cleanup := sdkTestLogger(t, tc.level)
			ctx, cancel := context.WithCancel(gizlog.WithPeerPublicKey(t.Context(), "trusted-peer"))
			cancel()
			sdk := newLiveKitLogger(ctx, logger)
			sdk.Debugw("SDK debug")
			sdk.Infow("SDK info")
			sdk.Warnw("SDK warn", errors.New("private provider body"))
			sdk.Errorw("SDK error", errors.New("private provider body"))
			if err := cleanup(); err != nil {
				t.Fatal(err)
			}
			var levels []string
			for _, record := range store.records {
				levels = append(levels, record.Severity)
				if record.Attributes["node_id"] != "test-server" || record.Attributes["peer_public_key"] != "trusted-peer" || record.Attributes["sdk"] != "livekit" {
					t.Fatalf("SDK context metadata = %v", record.Attributes)
				}
				if !strings.HasSuffix(record.Attributes["source_file"], "logger_test.go") {
					t.Fatalf("SDK source = %s", record.Attributes["source_file"])
				}
				if strings.Contains(record.Message, "private") || record.Attributes["error"] != "" {
					t.Fatal("SDK copied provider error text")
				}
			}
			slices.Sort(levels)
			if !slices.Equal(levels, tc.want) {
				t.Fatalf("SDK levels=%v want=%v", levels, tc.want)
			}
		})
	}
}

func TestLiveKitDerivedAndPionLoggersPreserveWarnAndError(t *testing.T) {
	logger, store, cleanup := sdkTestLogger(t, "warn")
	sdk := newLiveKitLogger(t.Context(), logger).WithName("room").WithValues("transport", livekit.SignalTarget_PUBLISHER, "token", "private-token")
	sdk.WithUnlikelyValues("state", "connected", "offer", "private SDP").Warnw("derived warning", errors.New("private error"))
	sampled, resolver := sdk.WithItemSampler().WithoutSampler().WithDeferredValues()
	resolver.Resolve("httpResponse", "private HTTP response")
	resolver.Reset()
	sampled.Errorw("derived error", nil)
	pion := pionlogger.NewLoggerFactory(sdk).NewLogger("ice")
	pion.Debug("pion debug")
	pion.Info("pion info")
	pion.Warnf("pion %s", "warning")
	pion.Errorf("pion %s", "error")
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	levels := map[string]string{}
	for _, record := range store.records {
		levels[record.Message] = record.Severity
		if record.Attributes["transport"] != "PUBLISHER" {
			t.Fatalf("transport metadata = %v", record.Attributes)
		}
		if strings.HasPrefix(record.Message, "pion ") && record.Attributes["component"] != "room.pion.ice" {
			t.Fatalf("Pion component = %v", record.Attributes)
		}
		for _, key := range []string{"token", "offer", "httpResponse", "error"} {
			if record.Attributes[key] != "" {
				t.Fatalf("SDK retained sensitive field %s", key)
			}
		}
	}
	if len(levels) != 4 || levels["derived warning"] != "WARN" || levels["derived error"] != "ERROR" || levels["pion warning"] != "WARN" || levels["pion error"] != "ERROR" {
		t.Fatalf("derived levels=%v", levels)
	}
}

func TestLiveKitConnectorErrorsReachWarnStore(t *testing.T) {
	logger, store, cleanup := sdkTestLogger(t, "warn")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private validation body", http.StatusForbidden)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(gizlog.WithPeerPublicKey(t.Context(), "validated-peer"), 2*time.Second)
	defer cancel()
	connector := livekitConnector{apiKey: "local-test", apiSecret: "local-test-secret-with-at-least-32-characters", logger: logger}
	client, err := connector.connect(ctx, connectParams{URL: server.URL, Room: "logger-test", Identity: "validated-peer"}, &session{disconnects: make(chan disconnectReason, 8)})
	if client != nil {
		client.Disconnect()
	}
	if err == nil {
		t.Fatal("unavailable signaling unexpectedly connected")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if len(store.records) == 0 {
		t.Fatal("real SDK validation error was filtered by WARN")
	}
	for _, record := range store.records {
		if record.Severity != "ERROR" || record.Attributes["peer_public_key"] != "validated-peer" || record.Attributes["error_present"] != "true" {
			t.Fatalf("real SDK error metadata = severity %s attrs %v", record.Severity, record.Attributes)
		}
		if strings.Contains(record.Message, "private") || record.Attributes["httpResponse"] != "" || record.Attributes["error"] != "" {
			t.Fatal("real SDK copied raw validation details")
		}
	}
}

func TestLiveKitProcessLoggerFollowsDefaultLogger(t *testing.T) {
	installLiveKitLogger()
	previous := slog.Default()
	defer slog.SetDefault(previous)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private validation body", http.StatusForbidden)
	}))
	defer server.Close()
	for range 2 {
		logger, store, cleanup := sdkTestLogger(t, "warn")
		slog.SetDefault(logger)
		room := lksdk.NewRoom(nil)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		err := room.JoinWithContextAndToken(ctx, server.URL, "local-unused-token", lksdk.WithConnectTimeout(100*time.Millisecond))
		cancel()
		if err == nil {
			t.Fatal("unavailable SDK signaling unexpectedly connected")
		}
		room.Disconnect()
		if err := cleanup(); err != nil {
			t.Fatal(err)
		}
		if len(store.records) == 0 {
			t.Fatal("process SDK error was filtered by WARN")
		}
		for _, record := range store.records {
			if record.Severity != "ERROR" || record.Attributes["peer_public_key"] != "" {
				t.Fatalf("process SDK record = severity %s attrs %v", record.Severity, record.Attributes)
			}
		}
	}
}
