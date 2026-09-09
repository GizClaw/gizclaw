package observability

import (
	"context"
	"log/slog"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type identityLogStore struct{ records []logstore.Record }

func (s *identityLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	s.records = append(s.records, records...)
	keys := make([]logstore.RecordKey, len(records))
	for i, record := range records {
		keys[i] = record.Key()
	}
	return keys, nil
}
func (*identityLogStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (*identityLogStore) Close() error                                  { return nil }
func (s *identityLogStore) Log(string) (logstore.ImmutableStore, error) { return s, nil }

func TestCompletionIdentitySurvivesProductionLogger(t *testing.T) {
	store := &identityLogStore{}
	logger, closeLogger, err := gizlog.NewLogger(gizlog.Config{Sinks: []gizlog.SinkConfig{{Kind: gizlog.SinkStore, Store: "test"}}}, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closeLogger(); err != nil {
			t.Error(err)
		}
	})
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })
	outcome := NewOutcome(TransportHTTP, SurfacePeerHTTP, "getDevice")
	outcome.SetPeer("completion-owner", "client")
	Log(gizlog.WithPeerPublicKey(context.Background(), "edge-transport-peer"), outcome)
	if len(store.records) != 1 {
		t.Fatalf("records = %d, want 1", len(store.records))
	}
	record := store.records[0]
	if record.Message != CompletionMessage || record.Attributes["peer_public_key"] != "completion-owner" {
		t.Fatalf("authenticated completion missing from LogStore: %+v", record)
	}
}
