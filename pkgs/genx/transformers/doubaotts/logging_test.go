package doubaotts

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

func TestSeedV2FirstAudioLogIncludesPeerPublicKey(t *testing.T) {
	t.Setenv("GIZCLAW_TTS_DEBUG", "1")
	wantAudio := []byte("pcm audio")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"reqid":"req-audio","code":0,"message":"","data":"%s"}`+"\n", base64.StdEncoding.EncodeToString(wantAudio))
		_, _ = fmt.Fprintln(w, `{"reqid":"req-audio","code":20000000,"message":"ok","data":null}`)
	}))
	defer server.Close()

	logs := installPeerLogStore(t)
	transformer := newSeedV2ForTest(t, server.URL, "pcm")
	ctx := gizlog.WithPeerPublicKey(t.Context(), "peer-test")
	if err := transformer.synthesize(ctx, "readable text", streamkit.TTSMeta{}, transformer.mimeType(), func([]byte) error {
		return nil
	}); err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	record := logs.waitFor(t, "doubao tts: first audio")
	if got := record.Attributes["peer_public_key"]; got != "peer-test" {
		t.Fatalf("peer_public_key = %q, want peer-test", got)
	}
}

func TestPeerLogStoreCleanupFlushesBacklogAfterWait(t *testing.T) {
	logs := installPeerLogStore(t)
	slog.Info("target")
	// Keep logging after the waited-for record, like a provider retry loop, so
	// cleanup must flush a backlog that nobody reads.
	for range 256 {
		slog.Info("backlog")
	}
	logs.waitFor(t, "target")
}

type peerLogStore struct {
	mu       sync.Mutex
	records  []logstore.Record
	cursor   int // waitFor consumes records in order
	appended chan struct{}
}

// Append never blocks: logger cleanup flushes the whole backlog after the test
// stopped waiting, so a bounded hand-off would wedge the flush forever.
func (s *peerLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	s.mu.Lock()
	s.records = append(s.records, records...)
	s.mu.Unlock()
	select {
	case s.appended <- struct{}{}:
	default:
	}
	return keys, nil
}

func (*peerLogStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}

func (*peerLogStore) Close() error { return nil }

func (s *peerLogStore) next(message string) (logstore.Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.cursor < len(s.records) {
		record := s.records[s.cursor]
		s.cursor++
		if record.Message == message {
			return record, true
		}
	}
	return logstore.Record{}, false
}

func (s *peerLogStore) waitFor(t *testing.T, message string) logstore.Record {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		if record, ok := s.next(message); ok {
			return record
		}
		select {
		case <-s.appended:
		case <-timer.C:
			t.Fatalf("log message %q was not persisted", message)
		}
	}
}

type peerLogResolver struct{ store *peerLogStore }

func (r peerLogResolver) Log(string) (logstore.ImmutableStore, error) { return r.store, nil }

func installPeerLogStore(t *testing.T) *peerLogStore {
	t.Helper()
	store := &peerLogStore{appended: make(chan struct{}, 1)}
	logger, cleanup, err := gizlog.NewLogger(
		gizlog.Config{Sinks: []gizlog.SinkConfig{{Kind: gizlog.SinkStore, Store: "logs"}}},
		peerLogResolver{store: store},
	)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
		_ = cleanup()
	})
	return store
}
