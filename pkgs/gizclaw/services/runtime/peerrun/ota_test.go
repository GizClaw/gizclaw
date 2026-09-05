package peerrun

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestOTARuntimeSnapshotOrdering(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	s := &Server{Store: store}
	peer := testPublicKey(t)
	ctx := context.Background()
	base := time.Unix(100, 0).UTC()
	put := func(state, id string, delta int, percent *float64) {
		t.Helper()
		if err := s.PutOTAStatus(ctx, peer, apitypes.PeerOtaStatus{State: state, UpdateId: id, ObservedAt: base.Add(time.Duration(delta) * time.Second), DownloadPercent: percent}); err != nil {
			t.Fatal(err)
		}
	}
	put("started", "one", 0, nil)
	put("downloading", "one", 1, new(50.0))
	put("downloading", "one", 0, new(10.0))
	put("downloading", "one", 2, new(20.0))
	got, err := s.GetStatus(ctx, peer)
	if err != nil || got.Ota == nil || got.Ota.DownloadPercent == nil || *got.Ota.DownloadPercent != 50 {
		t.Fatalf("progress: %+v, %v", got, err)
	}
	put("succeeded", "one", 3, nil)
	put("downloading", "one", 4, new(90.0))
	put("failed", "one", 5, nil)
	got, err = s.GetStatus(ctx, peer)
	if err != nil || got.Ota.State != "succeeded" || got.ReportedAt == nil || !got.ReportedAt.Equal(base.Add(3*time.Second)) {
		t.Fatalf("terminal: %+v, %v", got, err)
	}
	if _, err = s.PutStatus(ctx, peer, apitypes.PeerStatus{Volume: new(22), Ota: &apitypes.PeerOtaStatus{State: "started"}}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetStatus(ctx, peer)
	if err != nil || got.Ota.State != "succeeded" || got.Volume == nil || *got.Volume != 22 {
		t.Fatalf("control write overwrote OTA: %+v, %v", got, err)
	}
	put("started", "two", 6, nil)
	put("downloading", "one", 2, new(100.0))
	got, err = s.GetStatus(ctx, peer)
	if err != nil || got.Ota.UpdateId != "two" || got.Ota.DownloadPercent != nil {
		t.Fatalf("new attempt: %+v, %v", got, err)
	}
}

type delayedOTAStore struct {
	kv.Store
	entered, resume chan struct{}
}

func (s *delayedOTAStore) CompareAndMutate(ctx context.Context, guard kv.Key, expected []byte, entries []kv.Entry, keys []kv.Key) (bool, error) {
	close(s.entered)
	select {
	case <-s.resume:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return kv.CompareAndMutate(ctx, s.Store, guard, expected, entries, keys)
}

func TestConcurrentOTAProgressCannotOverwriteSuccess(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	peer := testPublicKey(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	base := time.Unix(100, 0).UTC()
	s := &Server{Store: store}
	if err := s.PutOTAStatus(ctx, peer, apitypes.PeerOtaStatus{State: "started", UpdateId: "one", ObservedAt: base}); err != nil {
		t.Fatal(err)
	}
	delayed := &delayedOTAStore{Store: store, entered: make(chan struct{}), resume: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		result <- (&Server{Store: delayed}).PutOTAStatus(ctx, peer, apitypes.PeerOtaStatus{State: "downloading", UpdateId: "one", ObservedAt: base.Add(time.Second), DownloadPercent: new(50.0)})
	}()
	select {
	case <-delayed.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	err := s.PutOTAStatus(ctx, peer, apitypes.PeerOtaStatus{State: "succeeded", UpdateId: "one", ObservedAt: base.Add(2 * time.Second)})
	close(delayed.resume)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	got, err := s.GetStatus(ctx, peer)
	if err != nil || got.Ota == nil || got.Ota.State != "succeeded" {
		t.Fatalf("concurrent result: %+v, %v", got, err)
	}
}
