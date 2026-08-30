package doubaorealtime

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

func TestTransformerProviderRetryLogIncludesPeerPublicKey(t *testing.T) {
	ctx, cancel := context.WithCancel(gizlog.WithPeerPublicKey(t.Context(), "peer-test"))
	defer cancel()
	logs := installPeerLogStore(t)
	transformer := newTransformer(nil, withDoubaoRealtimeOpener(&fakeTransformerOpener{
		results: []fakeTransformerOpenResult{{err: errors.New("provider unavailable")}},
	}))
	input := newBufferStream(1)
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer input.Close()
	defer output.Close()

	record := logs.waitFor(t, "doubao: realtime provider connection failed; retrying")
	if got := record.Attributes["peer_public_key"]; got != "peer-test" {
		t.Fatalf("peer_public_key = %q, want peer-test", got)
	}
}

type peerLogStore struct {
	records chan logstore.Record
}

func (s *peerLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		s.records <- record
		keys[index] = record.Key()
	}
	return keys, nil
}

func (*peerLogStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}

func (*peerLogStore) Close() error { return nil }

func (s *peerLogStore) waitFor(t *testing.T, message string) logstore.Record {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case record := <-s.records:
			if record.Message == message {
				return record
			}
		case <-timer.C:
			t.Fatalf("log message %q was not persisted", message)
		}
	}
}

type peerLogResolver struct{ store *peerLogStore }

func (r peerLogResolver) Log(string) (logstore.ImmutableStore, error) { return r.store, nil }

func installPeerLogStore(t *testing.T) *peerLogStore {
	t.Helper()
	store := &peerLogStore{records: make(chan logstore.Record, 64)}
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
